package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestWrite_ExistingFileCreatesBackup(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// First write — no backup created (file doesn't exist yet).
	_ = m.Write(map[string]any{"v": 1})
	backups1, _ := filepath.Glob(path + ".backup.*")
	if len(backups1) > 0 {
		t.Errorf("first write should not create backup, got %d", len(backups1))
	}

	// Second write — backup should be created.
	_ = m.Write(map[string]any{"v": 2})
	backups2, _ := filepath.Glob(path + ".backup.*")
	if len(backups2) != 1 {
		t.Errorf("second write should create 1 backup, got %d", len(backups2))
	}
}

func TestWrite_LockContention(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	lockPath := path + ".lock"
	fl := flock.New(lockPath)

	// Acquire the lock externally to simulate contention.
	err = fl.Lock()
	if err != nil {
		t.Fatalf("failed to acquire test lock: %v", err)
	}
	defer func() { _ = fl.Unlock() }()

	m := NewManager(path)
	err = m.Write(map[string]any{"k": "v"})
	if err == nil {
		t.Error("expected lock contention error")
	}
}

func TestWrite_RenameFailure(t *testing.T) {
	// On Windows admin, read-only dirs may still be writable. Skip gracefully.
	dir := t.TempDir()
	readonlyDir, err := safepath.JoinUnder(dir, "readonly")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	_ = os.MkdirAll(readonlyDir, 0o555)
	defer func() { _ = os.Chmod(readonlyDir, 0o755) }()

	path, err := safepath.JoinUnder(readonlyDir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)
	err = m.Write(map[string]any{"k": "v"})
	if err == nil {
		// If write succeeded, the directory was writable (common on Windows as admin).
		// The important thing is no crash — just skip the assertion.
		t.Skip("read-only directory test skipped (directory was writable)")
	}
}

func TestWrite_BOMStripping(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}

	// Write a file with BOM prefix manually.
	bomData := []byte{0xEF, 0xBB, 0xBF, '{', '}', '\n'}
	_ = safepath.WriteFile(dir, path, bomData)

	// Read should strip BOM.
	m := NewManager(path)
	data, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty map after BOM stripping, got %v", data)
	}

	// Re-write should not re-introduce BOM.
	_ = m.Write(map[string]any{"k": "v"})
	raw, _ := os.ReadFile(path)
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		t.Error("rewritten file should not contain BOM")
	}
}

func TestStripUTF8BOM(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		out  []byte
	}{
		{"no BOM", []byte(`{"a":1}`), []byte(`{"a":1}`)},
		{"with BOM", []byte{0xEF, 0xBB, 0xBF, '"', 'a'}, []byte{'"', 'a'}},
		{"short data", []byte{0xEF, 0xBB}, []byte{0xEF, 0xBB}},
		{"empty", []byte{}, []byte{}},
		{"partial BOM", []byte{0xEF, 0xBB, 0x00}, []byte{0xEF, 0xBB, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripUTF8BOM(tt.in)
			if len(got) != len(tt.out) {
				t.Errorf("len(%v) = %d, want %d", got, len(got), len(tt.out))
				return
			}
			for i := range tt.out {
				if got[i] != tt.out[i] {
					t.Errorf("byte[%d] = 0x%02x, want 0x%02x", i, got[i], tt.out[i])
				}
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src, err := safepath.JoinUnder(dir, "src.txt")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	dst, err := safepath.JoinUnder(dir, "dst.txt")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}

	_ = safepath.WriteFile(dir, src, []byte("hello"))
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "hello" {
		t.Errorf("copied content = %q, want hello", data)
	}
}

func TestCopyFile_MissingSource(t *testing.T) {
	dir := t.TempDir()
	src, _ := safepath.JoinUnder(dir, "missing.txt")
	dst, _ := safepath.JoinUnder(dir, "dst.txt")
	err := copyFile(src, dst)
	if err == nil {
		t.Error("expected error for missing source")
	}
}
