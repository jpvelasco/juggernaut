package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_RejectsBareFlagShapedArgument(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{"apply", "cli=codex", "--dry-run"})
	if err == nil {
		t.Fatal("expected bare cli=codex argument to be rejected")
	}
	if !strings.Contains(err.Error(), "did you mean --cli=codex?") {
		t.Fatalf("expected actionable flag hint, got: %v", err)
	}
	if _, statErr := readFileForTestErr(filepath.Join(home, ".claude", "settings.json")); statErr == nil {
		t.Fatal("invalid apply arguments must not write Claude configuration")
	}
}

// TestApply_NoOpusplanConflict verifies --no-opusplan cannot be combined with
// --opusplan (mirrors the --no-mantle conflict guard).
func TestApply_NoOpusplanConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--opusplan", "--no-opusplan", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error when combining --opusplan and --no-opusplan")
	}
	if !strings.Contains(err.Error(), "--no-opusplan cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestApply_NoOpusplanAloneIsNoop verifies --no-opusplan by itself is accepted
// (opusplan defaults off, so disabling it is a harmless no-op).
func TestApply_NoOpusplanAloneIsNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--no-opusplan", "--skip-preflight",
	}); err != nil {
		t.Fatalf("--no-opusplan alone should succeed, got: %v", err)
	}
}

// TestApply_SkipPreflightAccepted verifies --skip-preflight remains accepted as
// a no-op so existing scripts/CI passing it do not break.
func TestApply_SkipPreflightAccepted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("--skip-preflight should be accepted as a no-op, got: %v", err)
	}
}
