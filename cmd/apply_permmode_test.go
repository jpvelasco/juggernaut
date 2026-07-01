package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// setNativeDefaultMode rewrites settings.json setting permissions.defaultMode,
// simulating an external editor (e.g. Claude Code's Shift+Tab) that touches the
// native key without updating Juggernaut's meta block.
func setNativeDefaultMode(t *testing.T, home, mode string) {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		perms = map[string]any{}
		settings["permissions"] = perms
	}
	perms["defaultMode"] = mode
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshaling settings.json: %v", err)
	}
	if err := safepath.WriteFile(home, filepath.Join(home, ".claude", "settings.json"), b); err != nil {
		t.Fatalf("writing settings.json: %v", err)
	}
}

// readJuggernautPermissionMode returns the persisted meta.permissionMode from
// the user-scope settings.json juggernaut block ("" if absent).
func readJuggernautPermissionMode(t *testing.T, home string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	block, ok := settings["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("missing juggernaut block")
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		t.Fatal("missing meta in juggernaut block")
	}
	mode, _ := meta["permissionMode"].(string)
	return mode
}

// readNativeDefaultMode returns the native permissions.defaultMode from
// settings.json ("" if absent).
func readNativeDefaultMode(t *testing.T, home string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		return ""
	}
	mode, _ := perms["defaultMode"].(string)
	return mode
}

// readNativeEnvValue returns settings.json env[key] ("" if absent).
func readNativeEnvValue(t *testing.T, home, key string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := env[key].(string)
	return v
}

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
