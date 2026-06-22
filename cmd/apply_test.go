package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestApply_DryRun_IAM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--dry-run",
		"--skip-preflight",
	})
	if err != nil {
		t.Fatalf("apply --dry-run error: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create settings.json")
	}
}

func TestApply_DryRun_BedrockAPIKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=test-key-value",
		"--region=us-west-2",
		"--dry-run",
		"--skip-preflight",
	})
	if err != nil {
		t.Fatalf("apply --dry-run %s error: %v", authmode.BedrockAPIKey, err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create settings.json")
	}
}

func TestApply_WritesSettings_IAM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--effort=xhigh",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	block, ok := settings["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("juggernaut block not present in settings.json")
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		t.Fatal("juggernaut.meta not present")
	}
	if meta["schemaVersion"] != float64(2) {
		t.Errorf("expected schemaVersion=2, got %v", meta["schemaVersion"])
	}
	if meta["managedBy"] != "juggernaut" {
		t.Errorf("expected managedBy=juggernaut, got %v", meta["managedBy"])
	}
	if settings["env"] == nil {
		t.Error("top-level env key should be present")
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatal("env is not a map")
	}
	if env["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Error("expected CLAUDE_CODE_USE_BEDROCK=1 in top-level env")
	}
	if env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-4-8[1m]" {
		t.Errorf("expected Opus default model to carry [1m], got %v", env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-4-6[1m]" {
		t.Errorf("expected Sonnet default model to carry [1m], got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}

	bashrcPath := filepath.Join(home, ".bashrc")
	bashrc, err := safepath.ReadFile(home, bashrcPath)
	if err != nil {
		t.Fatalf("reading activation profile: %v", err)
	}
	if !activation.HasBlock(string(bashrc)) {
		t.Error("apply should install shell activation block")
	}
}

func TestApply_No1MContextDisablesExtendedContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--no-1m-context",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	env := settings["env"].(map[string]any)
	if env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-4-8" {
		t.Errorf("expected Opus default model without [1m], got %v", env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-4-6" {
		t.Errorf("expected Sonnet default model without [1m], got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if env["CLAUDE_CODE_DISABLE_1M_CONTEXT"] != "1" {
		t.Errorf("expected CLAUDE_CODE_DISABLE_1M_CONTEXT=1, got %v", env["CLAUDE_CODE_DISABLE_1M_CONTEXT"])
	}
}

func TestApply_DefaultMantleDisabledPreservesInferenceProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	env := settings["env"].(map[string]any)
	if _, ok := env["CLAUDE_CODE_USE_MANTLE"]; ok {
		t.Fatal("Mantle should be disabled by default")
	}
	overrides := settings["modelOverrides"].(map[string]any)
	if overrides["sonnet"] != "global.anthropic.claude-sonnet-4-6" {
		t.Errorf("expected global Sonnet inference profile by default, got %v", overrides["sonnet"])
	}
	block := settings["juggernaut"].(map[string]any)
	meta := block["meta"].(map[string]any)
	if meta["useMantle"] != false {
		t.Errorf("expected juggernaut.meta.useMantle=false by default, got %v", meta["useMantle"])
	}
}

func TestApply_MantleFlagStripsInferenceProfilePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--mantle",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	env := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_USE_MANTLE"] != "1" {
		t.Fatalf("expected CLAUDE_CODE_USE_MANTLE=1 with --mantle, got %v", env["CLAUDE_CODE_USE_MANTLE"])
	}
	overrides := settings["modelOverrides"].(map[string]any)
	if overrides["sonnet"] != "anthropic.claude-sonnet-4-6" {
		t.Errorf("expected raw Sonnet model ID with --mantle, got %v", overrides["sonnet"])
	}
}

func TestApply_ModelFlag_OverridesAll(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	const customModel = "custom.model.id"
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--model=" + customModel,
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	block := settings["juggernaut"].(map[string]any)
	overrides := block["modelOverrides"].(map[string]any)
	if overrides["opus"] != customModel {
		t.Errorf("expected opus=%s, got %v", customModel, overrides["opus"])
	}
	if overrides["sonnet"] != customModel {
		t.Errorf("expected sonnet=%s, got %v", customModel, overrides["sonnet"])
	}
	if overrides["haiku"] != customModel {
		t.Errorf("expected haiku=%s, got %v", customModel, overrides["haiku"])
	}
}

func TestUninstall_RemovesBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Apply with all new flags to ensure they get cleaned up.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--mode=auto", "--always-thinking", "--effort=max", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	if err := ExecuteArgs([]string{"uninstall", "--force"}); err != nil {
		t.Fatalf("uninstall error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	for _, k := range []string{
		"juggernaut", "env", "model", "modelOverrides",
		"effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight", "permissions",
	} {
		if _, ok := settings[k]; ok {
			t.Errorf("key %q should be removed after uninstall", k)
		}
	}
}

func TestUninstall_RemovesTokenFromConfiguredProfileStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tokenPath := isolateCredentialEnv(t, home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=key-to-remove",
		"--storage=profile",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("profile token should exist after apply: %v", err)
	}

	if err := ExecuteArgs([]string{"uninstall", "--force"}); err != nil {
		t.Fatalf("uninstall error: %v", err)
	}

	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("profile token should be removed after uninstall, stat err=%v", err)
	}
}

func TestUninstallFull_RemovesActivationBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	bashrc := filepath.Join(home, ".bashrc")
	if err := safepath.WriteFile(home, bashrc, []byte("export KEEP=1\n")); err != nil {
		t.Fatalf("writing bashrc: %v", err)
	}
	if err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight"}); err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if err := ExecuteArgs([]string{"uninstall", "--full", "--force"}); err != nil {
		t.Fatalf("uninstall --full error: %v", err)
	}

	data, err := safepath.ReadFile(home, bashrc)
	if err != nil {
		t.Fatalf("reading bashrc: %v", err)
	}
	if activation.HasBlock(string(data)) {
		t.Fatal("uninstall --full should remove shell activation block")
	}
	if !strings.Contains(string(data), "export KEEP=1") {
		t.Fatal("uninstall --full should preserve unrelated profile content")
	}
}

func TestApply_ReApply_PermissionMode_Preserved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// First apply sets mode=auto.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=auto", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}

	// Second apply without --mode should preserve auto.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("second apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key should be preserved on re-apply")
	}
	if perms["defaultMode"] != "auto" {
		t.Errorf("expected permissions.defaultMode=auto to be preserved, got %v", perms["defaultMode"])
	}
	env, _ := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_ENABLE_AUTO_MODE"] != "1" {
		t.Error("CLAUDE_CODE_ENABLE_AUTO_MODE=1 should be preserved on re-apply")
	}
}

func TestApply_ReApply_AlwaysThinkingOff_DeletesKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// First apply with --always-thinking.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--always-thinking", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}

	// Second apply without --always-thinking should remove the key entirely.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("second apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["alwaysThinkingEnabled"]; ok {
		t.Error("alwaysThinkingEnabled should be removed when --always-thinking is not passed on re-apply")
	}
}

// setupIsolatedKeychain sets JUGGERNAUT_KEYCHAIN_SERVICE to a short fixed
// service name and skips the test if the keychain backend is unavailable.
// The name is intentionally short — macOS security(1) hangs on long names.
func setupIsolatedKeychain(t *testing.T) *keychain.Store {
	t.Helper()
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-cmd-test")
	store := keychain.Default()
	// Probe with a timeout: if Set hangs, the test would block for 10 min.
	done := make(chan error, 1)
	go func() { done <- store.Set("probe") }()
	select {
	case err := <-done:
		if err != nil {
			t.Skipf("keychain backend unavailable: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Skip("keychain backend timed out")
	}
	_ = store.Delete()
	return store
}

func TestApply_BedrockKey_FromKeychainNoReprompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := setupIsolatedKeychain(t)

	// Pre-seed the keychain (simulates post-migration state).
	if err := store.Set("migrated-api-key"); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete() })

	// apply without --bedrock-key — should use keychain, not prompt.
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	// Settings should have been written.
	readSettingsJSON(t, home)
}

// isolateCredentialEnv points every credential side-channel (keychain service,
// DPAPI home, profile token path) at test-scoped locations so a test that runs
// `apply --storage=...` (which clears the non-selected backends) can never touch
// the developer's real credentials.
func isolateCredentialEnv(t *testing.T, home string) string {
	t.Helper()
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-cred-isolated")
	t.Setenv("JUGGERNAUT_HOME", filepath.Join(home, "iso-juggernaut-home"))
	tokenPath := filepath.Join(home, "profile-token")
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)
	return tokenPath
}

func TestApply_StorageProfile_WritesTokenToProfileFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tokenPath := isolateCredentialEnv(t, home)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=profile-key-123",
		"--storage=profile",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("expected token at profile path: %v", err)
	}
	if string(data) != "profile-key-123" {
		t.Errorf("expected profile-key-123, got %q", string(data))
	}
}

func TestApply_StorageProfile_PreserveKeyReadsProfileFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tokenPath := isolateCredentialEnv(t, home)
	if err := os.WriteFile(tokenPath, []byte("preexisting-profile-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--preserve-key",
		"--storage=profile",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --preserve-key with profile storage error: %v", err)
	}
}

func TestApply_ReapplyPreservesStorageMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Isolate keychain/CredMan so migration can't pull a real machine credential.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-reapply-isolated")
	t.Setenv("JUGGERNAUT_HOME", filepath.Join(home, "no-juggernaut-home"))
	tokenPath := filepath.Join(home, "profile-token")
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)

	// First apply with profile storage.
	if err := ExecuteArgs([]string{
		"apply", "--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=profile-key", "--storage=profile",
		"--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}

	// Re-apply WITHOUT --storage (e.g. just changing region). Storage must be
	// preserved as profile, not silently reset to the keychain default — which
	// would also wipe the profile token via ClearOthers.
	if err := ExecuteArgs([]string{
		"apply", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}
	jb, _ := settings["juggernaut"].(map[string]any)
	auth, _ := jb["auth"].(map[string]any)
	if auth["storage"] != "profile" {
		t.Errorf("expected storage preserved as profile on re-apply, got %v", auth["storage"])
	}
	if got, _ := os.ReadFile(tokenPath); string(got) != "profile-key" {
		t.Errorf("expected profile token preserved on re-apply, got %q", string(got))
	}
}

func TestApply_SwitchingStorage_ClearsPreviousBackend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := setupIsolatedKeychain(t)
	t.Cleanup(func() { _ = store.Delete() })

	tokenPath := filepath.Join(home, "profile-token")
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)

	// First apply: store in keychain.
	if err := ExecuteArgs([]string{
		"apply", "--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=keychain-key", "--storage=keychain",
		"--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	if got, _ := store.Get(); got != "keychain-key" {
		t.Fatalf("expected keychain-key in keychain, got %q", got)
	}

	// Second apply: switch to profile storage.
	if err := ExecuteArgs([]string{
		"apply", "--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=profile-key", "--storage=profile",
		"--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("second apply error: %v", err)
	}

	// The keychain token must be cleared to avoid orphaning a stale credential.
	if got, _ := store.Get(); got != "" {
		t.Errorf("expected keychain cleared after switching to profile storage, got %q", got)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil || string(data) != "profile-key" {
		t.Errorf("expected profile-key in profile file, got %q (err %v)", string(data), err)
	}
}

func TestApply_PreserveKey_MigratesV3ProfileTokenIntoKeychain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := setupIsolatedKeychain(t)
	t.Cleanup(func() { _ = store.Delete() })

	// Simulate a v3 profile token left on disk, with the v5 keychain empty.
	tokenPath := filepath.Join(home, "v3-profile-token")
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)
	if err := os.WriteFile(tokenPath, []byte("v3-legacy-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Default --storage is keychain; --preserve-key must migrate rather than error.
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--preserve-key",
		"--region=us-west-2",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --preserve-key should migrate v3 token, got error: %v", err)
	}

	got, err := store.Get()
	if err != nil {
		t.Fatalf("reading keychain after migration: %v", err)
	}
	if got != "v3-legacy-key" {
		t.Errorf("expected migrated key v3-legacy-key in keychain, got %q", got)
	}
}

func TestApply_KeychainOversizeKey_GivesActionableError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupIsolatedKeychain(t)

	// Windows Credential Manager / go-keyring caps blobs at 2560 bytes.
	bigKey := strings.Repeat("A", 4096)

	err := ExecuteArgs([]string{
		"apply", "--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=" + bigKey, "--storage=keychain",
		"--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error storing oversize key in keychain")
	}
	// The error must point the user at an alternative storage backend.
	if !strings.Contains(err.Error(), "--storage") {
		t.Errorf("expected actionable guidance mentioning --storage, got: %v", err)
	}
}

func TestApply_PreserveKey_ErrorsIfKeychainEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := setupIsolatedKeychain(t)
	t.Cleanup(func() { _ = store.Delete() })

	err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--preserve-key",
		"--region=us-west-2",
		"--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error when keychain is empty and --preserve-key is set")
	}
	if !strings.Contains(err.Error(), "no existing key found in keychain") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestApply_PermissionMode_AutoSetsBedrockEnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=auto", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	env, _ := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_ENABLE_AUTO_MODE"] != "1" {
		t.Error("expected CLAUDE_CODE_ENABLE_AUTO_MODE=1 when --mode=auto")
	}

	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatal("expected permissions key in settings.json")
	}
	if perms["defaultMode"] != "auto" {
		t.Errorf("expected permissions.defaultMode=auto, got %v", perms["defaultMode"])
	}
}

func TestApply_PermissionMode_InvalidErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=bogus", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error for invalid --mode value")
	}
}

func TestApply_ServiceTier_WritesEnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--service-tier=flex", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	env, _ := settings["env"].(map[string]any)
	if env["ANTHROPIC_BEDROCK_SERVICE_TIER"] != "flex" {
		t.Errorf("expected ANTHROPIC_BEDROCK_SERVICE_TIER=flex, got %v", env["ANTHROPIC_BEDROCK_SERVICE_TIER"])
	}
}

func TestApply_AlwaysThinking_WritesNativeKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--always-thinking", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if settings["alwaysThinkingEnabled"] != true {
		t.Errorf("expected alwaysThinkingEnabled=true in settings.json, got %v", settings["alwaysThinkingEnabled"])
	}
}

func TestApply_EffortLevel_WritesNativeKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--effort=medium", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if settings["effortLevel"] != "medium" {
		t.Errorf("expected effortLevel=medium as native key in settings.json, got %v", settings["effortLevel"])
	}
}

func TestApply_MaxEffortUsesEnvOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--effort=max", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["effortLevel"]; ok {
		t.Error("effortLevel should be omitted for max because Claude Code only accepts max via env/session")
	}
	env, _ := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("expected CLAUDE_CODE_EFFORT_LEVEL=max, got %v", env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
}

func TestApply_SkipWebFetchPreflight_AlwaysSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if settings["skipWebFetchPreflight"] != true {
		t.Errorf("expected skipWebFetchPreflight=true for all Bedrock configs, got %v", settings["skipWebFetchPreflight"])
	}
}

func readSettingsJSON(t *testing.T, home string) []byte {
	t.Helper()
	data, err := safepath.ReadFile(home, filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	return data
}
