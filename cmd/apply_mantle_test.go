package cmd

import (
	"strings"
	"testing"
)

// TestApply_NoMantleConflict verifies --no-mantle cannot be combined with
// --mantle (exercises the error branch of resolveMantle).
func TestApply_NoMantleConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--mantle", "--no-mantle", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error when combining --mantle and --no-mantle")
	}
	if !strings.Contains(err.Error(), "--no-mantle cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestApply_NoMantleConflictWithURL verifies the same guard for --mantle-url.
func TestApply_NoMantleConflictWithURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--mantle-url=https://example.test", "--no-mantle", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error when combining --mantle-url and --no-mantle")
	}
	if !strings.Contains(err.Error(), "--no-mantle cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}
