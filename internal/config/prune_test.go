package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPruneBackups_NoBackups(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "settings.json")
	// No backups exist — should be a no-op.
	err := pruneBackups(base, 5)
	if err != nil {
		t.Errorf("pruneBackups with no backups: %v", err)
	}
}

func TestPruneBackups_ExactlyKeep(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "settings.json")
	// Create exactly 5 backups — none should be removed.
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(filepath.Join(dir, "settings.json.backup.20240101_00000"+runeFor(i)), []byte("data"), 0o600)
	}
	err := pruneBackups(base, 5)
	if err != nil {
		t.Fatalf("pruneBackups with exactly keep: %v", err)
	}
	matches, _ := filepath.Glob(base + ".backup.*")
	if len(matches) != 5 {
		t.Errorf("expected 5 backups, got %d", len(matches))
	}
}

func TestPruneBackups_MoreThanKeep(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "settings.json")
	// Create 8 backups — 3 should be removed.
	for i := 0; i < 8; i++ {
		_ = os.WriteFile(filepath.Join(dir, "settings.json.backup.2024010"+runeFor(i)+"_000000"), []byte("data"), 0o600)
	}
	err := pruneBackups(base, 5)
	if err != nil {
		t.Fatalf("pruneBackups with more than keep: %v", err)
	}
	matches, _ := filepath.Glob(base + ".backup.*")
	if len(matches) != 5 {
		t.Errorf("expected 5 backups after prune, got %d", len(matches))
	}
}

func TestPruneBackups_MissingDirectory(t *testing.T) {
	// Base path points to a non-existent directory — Glob should still work
	// (returns no matches), so pruneBackups should succeed with no-op.
	base := filepath.Join(t.TempDir(), "nonexistent", "settings.json")
	err := pruneBackups(base, 5)
	if err != nil {
		t.Errorf("pruneBackups with missing directory: %v", err)
	}
}

func TestPruneBackups_KeepsNewest(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "settings.json")
	// Create backups with sortable timestamps — oldest should be removed.
	names := []string{
		"settings.json.backup.20240101_000001",
		"settings.json.backup.20240101_000002",
		"settings.json.backup.20240101_000003",
		"settings.json.backup.20240101_000004",
		"settings.json.backup.20240101_000005",
		"settings.json.backup.20240101_000006",
	}
	for _, n := range names {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("data"), 0o600)
	}
	err := pruneBackups(base, 3)
	if err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}
	matches, _ := filepath.Glob(base + ".backup.*")
	if len(matches) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(matches))
	}
	// The 3 newest should remain (sorted lexicographically, last 3).
	for _, m := range matches {
		baseName := filepath.Base(m)
		if baseName <= "settings.json.backup.20240101_000003" {
			t.Errorf("oldest backup %s should have been pruned", baseName)
		}
	}
}

func runeFor(i int) string {
	return fmt.Sprintf("%d", i)
}
