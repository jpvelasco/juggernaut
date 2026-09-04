package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
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

func TestApply_UserScopePersistsNonSecretRuntimeFallback(t *testing.T) {
	home := setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	state, found, err := activation.LoadRuntimeState(home, "claude")
	if err != nil || !found {
		t.Fatalf("LoadRuntimeState = found %v, err %v", found, err)
	}
	if state.AuthMode != authmode.IAM {
		t.Errorf("runtime auth mode = %q, want iam", state.AuthMode)
	}
	if state.Env["CLAUDE_CODE_USE_BEDROCK"] != "1" || state.Env["AWS_REGION"] != "us-west-2" {
		t.Errorf("runtime env missing Bedrock routing: %v", state.Env)
	}
	if _, ok := state.Env[authmode.BedrockAuthEnvName]; ok {
		t.Error("runtime fallback must never persist a bearer token")
	}
}

func TestApply_ProjectScopeDoesNotCreateGlobalRuntimeFallback(t *testing.T) {
	home := setupApplyTest(t)
	configBytes, err := os.ReadFile(findBedrockConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	originalEmbeddedConfig := embeddedConfigBytes
	SetEmbeddedConfig(configBytes)
	t.Cleanup(func() { SetEmbeddedConfig(originalEmbeddedConfig) })
	t.Chdir(t.TempDir())

	if err := ExecuteArgs([]string{
		"apply", "--scope=project", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("project apply error: %v", err)
	}

	if _, found, err := activation.LoadRuntimeState(home, "claude"); err != nil || found {
		t.Fatalf("project apply created global runtime fallback: found %v, err %v", found, err)
	}
}

func TestPersistRuntimeStateGuardsAndFailureCleanup(t *testing.T) {
	originalScope := applyFlags.scope
	t.Cleanup(func() { applyFlags.scope = originalScope })
	applyFlags.scope = "user"

	t.Run("provider without persistence", func(t *testing.T) {
		home := setupApplyTest(t)
		persistRuntimeState(home, provider.MustGet("codex"), authmode.IAM, provider.ConfigPlan{})
		if _, found, err := activation.LoadRuntimeState(home, "codex"); err != nil || found {
			t.Fatalf("non-persistent provider state = found %v, err %v", found, err)
		}
	})

	t.Run("case-insensitive token exclusion", func(t *testing.T) {
		home := setupApplyTest(t)
		persistRuntimeState(home, provider.MustGet("claude"), authmode.IAM, provider.ConfigPlan{
			RuntimeEnv: map[string]string{
				strings.ToLower(authmode.BedrockAuthEnvName): "secret",
				"AWS_REGION": "us-west-2",
			},
		})
		state, found, err := activation.LoadRuntimeState(home, "claude")
		if err != nil || !found {
			t.Fatalf("runtime state = found %v, err %v", found, err)
		}
		if _, ok := state.Env[strings.ToLower(authmode.BedrockAuthEnvName)]; ok {
			t.Fatal("runtime state retained a case-variant bearer token")
		}
		if state.Env["CLAUDE_CODE_USE_BEDROCK"] != "1" || state.Env["AWS_REGION"] != "us-west-2" {
			t.Fatalf("runtime state omitted routing environment: %v", state.Env)
		}
	})

	t.Run("failed save removes stale state", func(t *testing.T) {
		home := setupApplyTest(t)
		if err := activation.SaveRuntimeState(home, "claude", activation.RuntimeState{
			AuthMode: authmode.IAM,
		}); err != nil {
			t.Fatal(err)
		}

		stderr := captureStderr(t, func() {
			persistRuntimeState(home, provider.MustGet("claude"), "invalid", provider.ConfigPlan{})
		})
		if !strings.Contains(stderr, "could not save runtime fallback") {
			t.Fatalf("missing persistence warning: %q", stderr)
		}
		if _, found, err := activation.LoadRuntimeState(home, "claude"); err != nil || found {
			t.Fatalf("stale state after failed save = found %v, err %v", found, err)
		}
	})
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

// TestApply_ReapplyWithAuth_HonorsFlag is the regression for #438: re-apply
// with --auth must switch modes instead of keeping the empty named-return
// authMode and silently preserving the previous install.
func TestApply_ReapplyWithAuth_HonorsFlag(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	t.Cleanup(func() { _ = store.DeleteWithFallback(home) })

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	if got := readJuggernautAuthMode(t, home); got != authmode.IAM {
		t.Fatalf("after first apply, auth mode = %q, want %q", got, authmode.IAM)
	}

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=reapply-switch-key",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply --auth error: %v", err)
	}
	if got := readJuggernautAuthMode(t, home); got != authmode.BedrockAPIKey {
		t.Errorf("re-apply --auth did not switch mode: got %q, want %q", got, authmode.BedrockAPIKey)
	}
}
