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
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestApply_StorageFlagUnrecognized(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := ExecuteArgs([]string{
		"apply", "--storage=profile", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error: --storage flag should be unrecognized")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected 'unknown flag' in error, got: %v", err)
	}
}

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

func TestVersionInSync(t *testing.T) {
	// Version must stay in sync across VERSION, bedrock-config.json, and cmd/root.go.
	// Go runs tests from the package directory, so resolve repo root relative to this file.
	repoRoot := filepath.Join("..")
	verFile, err := os.ReadFile(filepath.Join(repoRoot, "VERSION"))
	if err != nil {
		t.Fatalf("reading VERSION: %v", err)
	}
	versionFile := strings.TrimSpace(string(verFile))

	// Check cmd/root.go Version var via the version subcommand.
	err = ExecuteArgs([]string{"version"})
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	// The VERSION file is the source of truth; the Version var (set via -ldflags at build time) must match.
	if Version == "" {
		t.Fatal("Version var is empty — was it built without -ldflags?")
	}
	if Version != versionFile {
		t.Errorf("VERSION file (%s) does not match cmd/root.go Version (%s)", versionFile, Version)
	}

	// Also check bedrock-config.json version matches.
	bedrockConfig, err := bedrock.Load(filepath.Join(repoRoot, "bedrock-config.json"))
	if err != nil {
		t.Fatalf("loading bedrock-config.json: %v", err)
	}
	if bedrockConfig.Version != versionFile {
		t.Errorf("bedrock-config.json version (%s) does not match VERSION file (%s)", bedrockConfig.Version, versionFile)
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
