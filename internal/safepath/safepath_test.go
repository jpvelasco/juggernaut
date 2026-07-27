package safepath_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestHomeDir_PrefersHOME(t *testing.T) {
	t.Setenv("HOME", "/tmp/test-home")
	t.Setenv("USERPROFILE", "/tmp/test-userprofile")
	got, err := safepath.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}
	if got != "/tmp/test-home" {
		t.Errorf("HomeDir = %q, want HOME value", got)
	}
}

func TestHomeDir_FallsBackToUserProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "/tmp/test-userprofile")
	got, err := safepath.HomeDir()
	if err != nil {
		t.Fatalf("HomeDir: %v", err)
	}
	if got != "/tmp/test-userprofile" {
		t.Errorf("HomeDir = %q, want USERPROFILE value", got)
	}
}

func TestHomeDir_BothEmpty(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	got, err := safepath.HomeDir()
	if err != nil {
		// os.UserHomeDir may fail or return the real home; either is fine
		if got == "" && !strings.Contains(err.Error(), "could not determine home directory") {
			t.Errorf("unexpected error: %v", err)
		}
		return
	}
	if got == "" {
		t.Error("expected non-empty path from UserHomeDir fallback")
	}
}

func TestHomeDirOrEmpty(t *testing.T) {
	t.Setenv("HOME", "/tmp/test-home")
	got := safepath.HomeDirOrEmpty()
	if got != "/tmp/test-home" {
		t.Errorf("HomeDirOrEmpty = %q, want /tmp/test-home", got)
	}
}

func TestHomeDirOrEmpty_NoErrorOnMissing(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	// Should return empty string without returning an error (the function swallows errors)
	got := safepath.HomeDirOrEmpty()
	// If UserHomeDir works, it returns the real home; if not, empty string — both acceptable
	_ = got
}

func TestIsUnderBase_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		base   string
		target string
		want   bool
	}{
		{"same path", "/a/b", "/a/b", true},
		{"child", "/a/b", "/a/b/c", true},
		{"deep child", "/a/b", "/a/b/c/d/e", true},
		{"escape one level", "/a/b", "/a/c", false},
		{"escape parent", "/a/b", "/a", false},
		{"escape root", "/a/b", "/", false},
		{"dot cleans to base", "/a/b", "/a/b/.", true},
		{"dotdot escape", "/a/b", "/a/b/../c", false},
		{"unclean child", "/a/b", "/a/b/./c", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safepath.IsUnderBase(tt.base, tt.target)
			if got != tt.want {
				t.Errorf("IsUnderBase(%q, %q) = %v, want %v", tt.base, tt.target, got, tt.want)
			}
		})
	}
}

func TestIsUnderBase_WithTempDir(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "child")
	outside := t.TempDir()

	if !safepath.IsUnderBase(base, base) {
		t.Error("base should be under itself")
	}
	if !safepath.IsUnderBase(base, child) {
		t.Error("child should be under base")
	}
	if safepath.IsUnderBase(base, outside) {
		t.Error("outside should not be under base")
	}
}
