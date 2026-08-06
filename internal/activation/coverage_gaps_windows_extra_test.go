//go:build windows

package activation

import (
	"testing"
)

// TestPathsEqualCI_EqualPaths returns true.
func TestPathsEqualCI_EqualPaths(t *testing.T) {
	if !pathsEqualCI("/usr/bin:/usr/local/bin", "/usr/bin:/usr/local/bin") {
		t.Error("expected equal paths to be equal")
	}
}

// TestPathsEqualCI_DifferentPaths returns false.
func TestPathsEqualCI_DifferentPaths(t *testing.T) {
	if pathsEqualCI("/usr/bin:/usr/local/bin", "/opt/bin:/usr/local/bin") {
		t.Error("expected different paths to not be equal")
	}
}

// TestDefaultResolveDocumentsFolder_Exists returns the existing folder.
func TestDefaultResolveDocumentsFolder_Exists(t *testing.T) {
	got, err := defaultResolveDocumentsFolder()
	if err != nil {
		t.Fatalf("defaultResolveDocumentsFolder error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty documents folder")
	}
}

// TestResolveHomeDir_WithHomeEnv uses the HOME env var.
func TestResolveHomeDir_WithHomeEnv(t *testing.T) {
	t.Setenv("HOME", "/tmp/test-home")
	t.Setenv("USERPROFILE", "")
	got := resolveHomeDir()
	if got != "/tmp/test-home" {
		t.Errorf("expected /tmp/test-home, got %q", got)
	}
}
