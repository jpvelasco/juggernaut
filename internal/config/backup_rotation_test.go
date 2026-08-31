package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestRotateBackup_RapidWrites_PreservesDistinctStates is the deterministic
// regression for #384: rapid writes to the same config within one second must
// not collapse into a single backup. Same-second timestamps must disambiguate
// so each backup preserves a distinct recovery point.
func TestRotateBackup_RapidWrites_PreservesDistinctStates(t *testing.T) {
	const n = 7

	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// First write: no prior config, so no backup.
	if err := m.Write(map[string]any{"v": float64(0)}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	// Each subsequent write backs up the current content first. Keep the
	// loop fast (no deliberate sleeps) so all writes land within a second —
	// that is exactly the window where same-second timestamps collide.
	for i := 1; i < n; i++ {
		if err := m.Write(map[string]any{"v": float64(i)}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	// n-1 writes happened after the initial write. Rotation keeps the
	// latest backupRetain states, so expect min(n-1, backupRetain) backups.
	expected := n - 1
	if expected > backupRetain {
		expected = backupRetain
	}

	matches, err := filepath.Glob(path + ".backup.*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != expected {
		t.Fatalf("expected %d distinct backups, got %d: %v", expected, len(matches), matches)
	}

	// Every backup must contain the pre-write state of some write, i.e.
	// {"v": k} for 0 <= k <= n-2 — each recovery point distinct.
	seen := map[int]bool{}
	for _, mp := range matches {
		data, err := os.ReadFile(mp)
		if err != nil {
			t.Fatalf("read %s: %v", mp, err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("parse %s: %v", mp, err)
		}
		fv, _ := got["v"].(float64)
		v := int(fv)
		if v < 0 || v >= n-1 || seen[v] {
			t.Errorf("backup %s: value %v duplicates or out of range; already seen %v", filepath.Base(mp), v, seen)
		}
		seen[v] = true
	}
	if len(seen) != expected {
		t.Errorf("expected %d distinct pre-write states, got %d: %v", expected, len(seen), seen)
	}
}

// TestUniqueBackupPath_SuffixZeroPadded pins the -%03d suffix: unpadded -N
// names ("...-10" < "...-9" lexically) break pruneBackups' lexical sort,
// so a burst of >=11 same-second rotations would prune a newer backup while
// keeping an older one.
func TestUniqueBackupPath_SuffixZeroPadded(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Occupy 10 same-second backup names, forcing the 11th candidate onto
	// the zero-padded "-010" form.
	for i := 0; i < 10; i++ {
		p, err := m.uniqueBackupPath()
		if err != nil {
			t.Fatalf("uniqueBackupPath %d: %v", i, err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	p11, err := m.uniqueBackupPath()
	if err != nil {
		t.Fatalf("uniqueBackupPath 11: %v", err)
	}
	if !strings.HasSuffix(filepath.Base(p11), "-010") {
		t.Errorf("expected zero-padded -010 suffix on the 11th candidate, got %q", filepath.Base(p11))
	}

	// Padded names must sort chronologically: -009 < -010.
	older := m.path + ".backup.19700101_000000-009"
	newer := m.path + ".backup.19700101_000000-010"
	if strings.Compare(older, newer) != -1 {
		t.Errorf("padded suffix must sort chronologically: %q !< %q", older, newer)
	}
}

// TestUniqueBackupPath_RotationErrorSurfaces covers the uniqueBackupPath
// rotation branch: if a same-second backup path resolves to an error (not
// NotExist), rotation must fail loudly instead of guessing a free name. The
// stat is injected through a pathExists override so the branch is exercised
// on every platform (chmod-based injection is a no-op for admin processes on
// Windows).
func TestUniqueBackupPath_RotationErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	orig := pathExists
	pathExists = func(string) (bool, error) {
		return false, fmt.Errorf("injected stat error")
	}
	defer func() { pathExists = orig }()

	if _, err := m.uniqueBackupPath(); err == nil {
		t.Fatal("expected rotation error from stat failure, got nil")
	}
}
