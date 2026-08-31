package config

import (
	"os"
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
