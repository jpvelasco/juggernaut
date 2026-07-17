package cmd

import (
	"testing"
)

// TestApply_ReapplyWithoutMode_PreservesAutoMode is the regression for #231:
// applying with --mode=auto then re-applying WITHOUT --mode must preserve the
// auto permission mode and its CLAUDE_CODE_ENABLE_AUTO_MODE env var.
func TestApply_ReapplyWithoutMode_PreservesAutoMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// First apply pins auto mode.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=auto", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	if got := readJuggernautPermissionMode(t, home); got != "auto" {
		t.Fatalf("after first apply, permissionMode = %q, want %q", got, "auto")
	}
	if got := readNativeDefaultMode(t, home); got != "auto" {
		t.Fatalf("after first apply, defaultMode = %q, want %q", got, "auto")
	}
	if got := readNativeEnvValue(t, home, "CLAUDE_CODE_ENABLE_AUTO_MODE"); got != "1" {
		t.Fatalf("after first apply, CLAUDE_CODE_ENABLE_AUTO_MODE = %q, want %q", got, "1")
	}

	// Re-apply WITHOUT --mode (e.g. a routine effort/model tweak).
	if err := ExecuteArgs([]string{
		"apply", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply error: %v", err)
	}

	if got := readJuggernautPermissionMode(t, home); got != "auto" {
		t.Errorf("re-apply without --mode changed permissionMode to %q, want %q (preserved)", got, "auto")
	}
	if got := readNativeDefaultMode(t, home); got != "auto" {
		t.Errorf("re-apply without --mode changed defaultMode to %q, want %q (preserved)", got, "auto")
	}
	if got := readNativeEnvValue(t, home, "CLAUDE_CODE_ENABLE_AUTO_MODE"); got != "1" {
		t.Errorf("re-apply without --mode dropped CLAUDE_CODE_ENABLE_AUTO_MODE (got %q, want %q)", got, "1")
	}
}

// TestApply_ReapplyPreservesExternallySetMode is the true regression for #231:
// a mode set OUTSIDE Juggernaut (e.g. Claude Code's Shift+Tab writes the native
// permissions.defaultMode but not Juggernaut's meta block) must survive a
// routine re-apply that omits --mode. Previously mergePermissions deleted the
// native defaultMode whenever Juggernaut had no --mode opinion.
func TestApply_ReapplyPreservesExternallySetMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// First apply with no permission mode (Juggernaut asserts nothing).
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}

	// Simulate the user enabling auto via Claude Code Shift+Tab: it edits the
	// native permissions.defaultMode directly and does not touch Juggernaut's
	// meta block.
	setNativeDefaultMode(t, home, "auto")

	// Routine re-apply with no --mode (e.g. a region/effort tweak).
	if err := ExecuteArgs([]string{
		"apply", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply error: %v", err)
	}

	if got := readNativeDefaultMode(t, home); got != "auto" {
		t.Errorf("re-apply wiped externally-set defaultMode: got %q, want %q", got, "auto")
	}
	if got := readNativeEnvValue(t, home, "CLAUDE_CODE_ENABLE_AUTO_MODE"); got != "1" {
		t.Errorf("re-apply did not restore CLAUDE_CODE_ENABLE_AUTO_MODE for adopted auto mode (got %q)", got)
	}
}

// TestApply_ExplicitModeOverridesExternalMode confirms preservation only kicks
// in when --mode is omitted: an explicit --mode=default must still override an
// externally-set auto mode (and clear its env var).
func TestApply_ExplicitModeOverridesExternalMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	setNativeDefaultMode(t, home, "auto")

	// Explicit --mode=default must win over the externally-set auto.
	if err := ExecuteArgs([]string{
		"apply", "--region=us-west-2", "--mode=default", "--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply --mode=default error: %v", err)
	}

	if got := readNativeDefaultMode(t, home); got != "default" {
		t.Errorf("explicit --mode=default did not override external auto: got %q, want %q", got, "default")
	}
	if got := readNativeEnvValue(t, home, "CLAUDE_CODE_ENABLE_AUTO_MODE"); got != "" {
		t.Errorf("explicit --mode=default should clear CLAUDE_CODE_ENABLE_AUTO_MODE, got %q", got)
	}
}
