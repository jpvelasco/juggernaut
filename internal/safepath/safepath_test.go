package safepath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestJoinUnder_RejectsTraversal(t *testing.T) {
	base := t.TempDir()
	_, err := safepath.JoinUnder(base, "..", "etc", "passwd")
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

func TestJoinUnder_AllowsChildPath(t *testing.T) {
	base := t.TempDir()
	got, err := safepath.JoinUnder(base, ".claude", "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	want := filepath.Join(base, ".claude", "settings.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadWriteFileWithinBase(t *testing.T) {
	base := t.TempDir()
	path, err := safepath.JoinUnder(base, "nested", "file.txt")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	if err := safepath.WriteFile(base, path, []byte("ok")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := safepath.ReadFile(base, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("got %q, want %q", string(data), "ok")
	}
}

// TestJoinUnder_RejectsImmediateParent covers the exact rel == ".." branch of
// withinBase (going up exactly one level to the immediate parent), distinct from
// the "../"-prefixed multi-level escape.
func TestJoinUnder_RejectsImmediateParent(t *testing.T) {
	base := filepath.Join(t.TempDir(), "child")
	if _, err := safepath.JoinUnder(base, ".."); err == nil {
		t.Fatal("expected immediate-parent escape to be rejected")
	}
}

// TestReadFile_RejectsAbsoluteOutsideBase covers withinBase rejecting an
// absolute target that lives entirely outside base.
func TestWriteFile_RejectsOutsideBase(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.txt")
	if err := safepath.WriteFile(base, outside, []byte("x")); err == nil {
		t.Fatal("expected write outside base to fail")
	}
}

func TestReadFile_RejectsOutsideBase(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := safepath.ReadFile(base, outside); err == nil {
		t.Fatal("expected read outside base to fail")
	}
}
