package cmd

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
)

// TestApply_BedrockKey_FlagStoresViaFallback runs a real (non-dry-run) apply
// with --bedrock-key. This needs no keychain backend: resolveCredential returns
// the flag value directly, and commitApply's SetWithFallback writes the file
// fallback when the OS keychain is unavailable (as on headless CI). It covers
// the resolveCredential flag path and commitApply's API-key storage branch.
func TestApply_BedrockKey_FlagStoresViaFallback(t *testing.T) {
	home := setupApplyTest(t)
	// commitApply calls SetWithFallback, which reaches the real OS keychain on
	// macOS (security(1) can hang). Use the probed/isolated store so the test
	// skips gracefully when the backend is unavailable rather than timing out.
	store := setupIsolatedKeychain(t)
	t.Cleanup(func() { _ = store.DeleteWithFallback(home) })

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=flag-provided-key",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --bedrock-key error: %v", err)
	}

	// The key must be retrievable via the fallback layer.
	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "flag-provided-key" {
		t.Errorf("stored key = %q, want %q", got, "flag-provided-key")
	}

	// And settings.json should carry the API-key auth mode.
	if mode := readJuggernautAuthMode(t, home); mode != authmode.BedrockAPIKey {
		t.Errorf("auth mode = %q, want %q", mode, authmode.BedrockAPIKey)
	}
}

// TestApply_ReapplyWithoutAuth_PreservesExistingMode verifies the documented
// design pattern: re-applying without --auth reads the auth mode back from the
// existing block (exercises the has-block branch of resolveApplyInputs).
func TestApply_ReapplyWithoutAuth_PreservesExistingMode(t *testing.T) {
	home := setupApplyTest(t)

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
