package cmd

import (
	"encoding/json"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

// readJuggernautAuthMode returns the persisted auth.mode from the user-scope
// settings.json, failing the test if the structure is missing.
func readJuggernautAuthMode(t *testing.T, home string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	block, ok := settings["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("missing juggernaut block")
	}
	auth, ok := block["auth"].(map[string]any)
	if !ok {
		t.Fatal("missing auth in juggernaut block")
	}
	mode, _ := auth["mode"].(string)
	return mode
}

// TestApply_BedrockKey_FlagStoresViaFallback runs a real (non-dry-run) apply
// with --bedrock-key. This needs no keychain backend: resolveCredential returns
// the flag value directly, and commitApply's SetWithFallback writes the file
// fallback when the OS keychain is unavailable (as on headless CI). It covers
// the resolveCredential flag path and commitApply's API-key storage branch.
func TestApply_BedrockKey_FlagStoresViaFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-apply-flag-test")
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=flag-provided-key",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --bedrock-key error: %v", err)
	}

	// The key must be retrievable via the fallback layer regardless of backend.
	store := keychain.Default()
	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "flag-provided-key" {
		t.Errorf("stored key = %q, want %q", got, "flag-provided-key")
	}
	t.Cleanup(func() { _ = store.DeleteWithFallback(home) })

	// And settings.json should carry the API-key auth mode.
	if mode := readJuggernautAuthMode(t, home); mode != authmode.BedrockAPIKey {
		t.Errorf("auth mode = %q, want %q", mode, authmode.BedrockAPIKey)
	}
}

// TestApply_ReapplyWithoutAuth_PreservesExistingMode verifies the documented
// design pattern: re-applying without --auth reads the auth mode back from the
// existing block (exercises the has-block branch of resolveApplyInputs).
func TestApply_ReapplyWithoutAuth_PreservesExistingMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// First apply pins IAM.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	if got := readJuggernautAuthMode(t, home); got != authmode.IAM {
		t.Fatalf("after first apply, auth mode = %q, want %q", got, authmode.IAM)
	}

	// Re-apply WITHOUT --auth: the existing IAM mode must be preserved.
	if err := ExecuteArgs([]string{
		"apply", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply error: %v", err)
	}
	if got := readJuggernautAuthMode(t, home); got != authmode.IAM {
		t.Errorf("re-apply without --auth changed auth mode to %q, want %q (preserved)", got, authmode.IAM)
	}
}
