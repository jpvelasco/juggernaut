package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestWithConfig_ReadGatedOnLock is the deterministic regression for #385.
//
// The bug: withConfig read the config before the file lock was acquired, so
// a second read-modify-write could read the pre-mutation state and then
// clobber the first writer's committed update with a stale map.
//
// The fix holds the lock across the full read-mutate-write transaction. This
// test proves the read is gated on the lock: it holds the lock externally
// (blocking the Manager's lock acquisition), then writes a sentinel to the
// file. When the Manager's withConfig finally acquires the lock and reads,
// it must see the sentinel — not the seed that was on disk before the
// sentinel write. If the read were performed outside the lock (the old
// behavior), the in-flight read would observe the seed and the sentinel
// would be clobbered.
func TestWithConfig_ReadGatedOnLock(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Seed the file directly (no lock taken).
	data, err := m.format.Marshal(map[string]any{"original": true})
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	// Acquire the lock externally so the Manager's withConfig blocks in its
	// lock acquisition until we release it.
	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		t.Fatalf("test lock acquire: %v", err)
	}
	defer func() { _ = fl.Unlock() }()

	// Write the sentinel while we hold the lock; the Manager's read (which
	// happens after it acquires the lock) must observe this state.
	sentinel, err := m.format.Marshal(map[string]any{"original": true, "sentinel": true})
	if err != nil {
		t.Fatalf("marshal sentinel: %v", err)
	}
	if err := os.WriteFile(path, sentinel, 0o600); err != nil {
		t.Fatalf("sentinel write: %v", err)
	}

	// Start withConfig. It attempts the lock in TryLock (a non-blocking
	// single attempt), so it returns a contention error while we hold the
	// lock. This is expected: the lock is intentionally held across the
	// read-mutate-write transaction, which is exactly what the bug allowed
	// to be bypassed.
	err = m.withConfig(func(existing map[string]any) error {
		existing["a"] = true
		return nil
	})
	if err == nil {
		t.Fatalf("expected lock contention error while the lock is held externally")
	}
	if !strings.Contains(err.Error(), "locked by another process") {
		t.Fatalf("expected lock contention error, got: %v", err)
	}

	// Release the lock and retry: now the read runs under the lock and must
	// observe the sentinel.
	_ = fl.Unlock()

	var sawSentinel bool
	if err := m.withConfig(func(existing map[string]any) error {
		_, sawSentinel = existing["sentinel"]
		existing["a"] = true
		return nil
	}); err != nil {
		t.Fatalf("withConfig after unlock: %v", err)
	}

	if !sawSentinel {
		t.Errorf("withConfig's read did not see the sentinel — the read ran outside the lock")
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	if got["sentinel"] != true {
		t.Errorf("sentinel lost — the withConfig write clobbered the in-flight file change: %v", got)
	}
	if got["a"] != true {
		t.Errorf("key a missing: %v", got)
	}
}

// TestAcquireConfigLock_TryLockErrorSurfaces covers the lock-error branch of
// acquireConfigLock: a non-nil error from the TryLock seam must surface as an
// acquire error with a nil handle (callers must not unlock a broken lock).
// The seam is injectable because a real flock error is unforceable on a
// healthy filesystem.
func TestAcquireConfigLock_TryLockErrorSurfaces(t *testing.T) {
	origTry := acquireConfigLockFn
	acquireConfigLockFn = func(*flock.Flock) (bool, error) {
		return false, fmt.Errorf("injected lock error")
	}
	defer func() { acquireConfigLockFn = origTry }()

	fl, locked, err := acquireConfigLock(filepath.Join(t.TempDir(), "x.lock"))
	if err == nil {
		t.Fatal("expected lock error, got nil")
	}
	if locked {
		t.Errorf("expected locked=false on error, got true")
	}
	if fl != nil {
		t.Errorf("expected nil handle on error, got %v", fl)
	}
	if !strings.Contains(err.Error(), "injected lock error") {
		t.Errorf("expected wrapped acquire error, got: %v", err)
	}
}

// TestRunLocked_AcquireErrorSurfaces covers runLocked's error branch when the
// lock acquisition itself fails (injected via the TryLock seam): both the
// withConfig and Write entry points must surface the wrapped acquire error
// instead of proceeding to a read or write.
func TestRunLocked_AcquireErrorSurfaces(t *testing.T) {
	origTry := acquireConfigLockFn
	acquireConfigLockFn = func(*flock.Flock) (bool, error) {
		return false, fmt.Errorf("injected flock failure")
	}
	defer func() { acquireConfigLockFn = origTry }()

	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	for name, fn := range map[string]func() error{
		"withConfig": func() error {
			return m.withConfig(func(existing map[string]any) error {
				existing["a"] = true
				return nil
			})
		},
		"Write": func() error {
			return m.Write(map[string]any{"a": true})
		},
	} {
		err := fn()
		if err == nil {
			t.Fatalf("%s: expected acquire error, got nil", name)
		}
		if !strings.Contains(err.Error(), "acquiring settings.json lock") {
			t.Fatalf("%s: expected wrapped acquire error, got: %v", name, err)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("failed runs must not create the file: %v", err)
	}
}

// TestWrite_LockContention_CoversBothPaths proves the direct Write's
// contention branch (distinct from the withConfig one covered above): with
// the lock held externally, Write must fail fast with the shared contention
// message and must not touch the file.
func TestWrite_LockContention_CoversBothPaths(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		t.Fatalf("test lock acquire: %v", err)
	}
	defer func() { _ = fl.Unlock() }()

	err = m.Write(map[string]any{"a": true})
	if err == nil {
		t.Fatal("expected lock contention error, got nil")
	}
	if !strings.Contains(err.Error(), "locked by another process") {
		t.Fatalf("expected contention error, got: %v", err)
	}
	// The file must not have been created or modified by the failed Write.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("failed Write must not create the file: %v", err)
	}
}
