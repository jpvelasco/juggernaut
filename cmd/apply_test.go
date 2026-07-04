package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestApply_StorageFlagUnrecognized(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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

// TestApply_DryRun_BedrockAPIKey_NoPromptWithoutKey verifies dry-run is
// non-interactive: with bedrock-api-key auth but NO --bedrock-key and no stored
// credential, a dry-run must not trigger the interactive key prompt. We feed a
// closed stdin (EOF); if the prompt fired it would error, and a dry-run must
// never prompt at all. The command must succeed and write nothing.
func TestApply_DryRun_BedrockAPIKey_NoPromptWithoutKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	var err error
	withStdin(t, "", func() { // closed stdin: a prompt here would fail, not hang
		err = ExecuteArgs([]string{
			"apply",
			"--auth=" + authmode.BedrockAPIKey,
			"--region=us-west-2",
			"--dry-run",
			"--skip-preflight",
		})
	})
	if err != nil {
		t.Fatalf("dry-run must not prompt for a key; got error: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(statErr) {
		t.Error("dry-run should not create settings.json")
	}
}

func TestApply_WritesSettings_IAM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	if _, ok := env["ANTHROPIC_DEFAULT_FABLE_MODEL"]; ok {
		t.Errorf("Fable default model should be omitted unless configured, got %v", env["ANTHROPIC_DEFAULT_FABLE_MODEL"])
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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	if overrides["fable"] != customModel {
		t.Errorf("expected fable=%s, got %v", customModel, overrides["fable"])
	}
}

func TestUninstall_RemovesBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Apply with all new flags to ensure they get cleaned up.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--mode=auto", "--always-thinking", "--effort=max",
		"--fallback-model=global.anthropic.claude-opus-4-8,global.anthropic.claude-sonnet-4-6", "--skip-preflight",
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
		"juggernaut", "env", "model", "modelOverrides", "fallbackModel",
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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)
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
	setupMockPSRunner(t, home)
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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=bogus", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error for invalid --mode value")
	}
}

func TestApply_FableAndFallbackWritesNativeKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	const fableModel = "custom.fable.model"
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--fable-model=" + fableModel,
		"--fallback-model=global.anthropic.claude-opus-4-8, global.anthropic.claude-sonnet-4-6",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	env, _ := settings["env"].(map[string]any)
	if env["ANTHROPIC_DEFAULT_FABLE_MODEL"] != fableModel {
		t.Errorf("expected Fable env model override, got %v", env["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
	overrides := settings["modelOverrides"].(map[string]any)
	if overrides["fable"] != fableModel {
		t.Errorf("expected native fable alias=%s, got %v", fableModel, overrides["fable"])
	}
	fallbacks, ok := settings["fallbackModel"].([]any)
	if !ok || len(fallbacks) != 2 {
		t.Fatalf("expected fallbackModel array with two entries, got %#v", settings["fallbackModel"])
	}
	if fallbacks[0] != "global.anthropic.claude-opus-4-8" || fallbacks[1] != "global.anthropic.claude-sonnet-4-6" {
		t.Errorf("unexpected fallbackModel chain: %#v", fallbacks)
	}

	block := settings["juggernaut"].(map[string]any)
	meta := block["meta"].(map[string]any)
	metaFallbacks, ok := meta["fallbackModels"].([]any)
	if !ok || len(metaFallbacks) != 2 {
		t.Fatalf("expected juggernaut.meta.fallbackModels with two entries, got %#v", meta["fallbackModels"])
	}
}

func TestApply_FallbackModelRejectsEmptyEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2",
		"--fallback-model=global.anthropic.claude-opus-4-8,,global.anthropic.claude-sonnet-4-6",
		"--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected empty --fallback-model entry to error")
	}
	if !strings.Contains(err.Error(), "--fallback-model contains an empty model ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApply_ServiceTier_WritesEnvVar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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
	setupMockPSRunner(t, home)

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

func TestApply_EffortAutoUsesEnvOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--effort=auto", "--skip-preflight"})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["effortLevel"]; ok {
		t.Errorf("effortLevel should be omitted for auto because Claude Code settings only accept persisted fixed levels, got %v", settings["effortLevel"])
	}
	env, _ := settings["env"].(map[string]any)
	if got := env["CLAUDE_CODE_EFFORT_LEVEL"]; got != "auto" {
		t.Errorf("expected CLAUDE_CODE_EFFORT_LEVEL=auto, got %v", got)
	}
}

func TestApply_EffortUltracodeInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--effort=ultracode", "--skip-preflight"})
	if err == nil {
		t.Fatal("expected unsupported ultracode effort value to fail")
	}
	if !strings.Contains(err.Error(), "invalid effort") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApply_SkipWebFetchPreflight_AlwaysSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	verFile, err := os.ReadFile(filepath.Join(repoRoot, "VERSION")) // nosemgrep: go_filesystem_rule-fileread — test reads hardcoded file from repo root
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

func TestCredentialEchoMode_IsEchoNone(t *testing.T) {
	// EchoModePassword breaks on Windows (keystrokes are dropped by the TUI
	// input loop). The credential prompt must use EchoNone instead.
	// Regression test for: https://github.com/charmbracelet/bubbles/issues/865
	if credentialEchoMode != huh.EchoMode(textinput.EchoNone) {
		t.Fatalf("credential prompt must use EchoModeNone, got %v", credentialEchoMode)
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

// setupMockPSRunner sets a mock PowerShell discovery runner on Windows so
// that command-level tests never touch real PowerShell profiles.
// It returns a cleanup function that should be deferred.
func setupMockPSRunner(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	psProfile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	runner := &mockApplyCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockApplyPSOutput(psProfile, psProfile),
			"powershell.exe": mockApplyPSOutput(psProfile, psProfile),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	t.Cleanup(activation.ResetPSRunnerForTesting)
}

// mockApplyCommandRunner implements activation.discoveryCommandRunner for apply tests.
type mockApplyCommandRunner struct {
	output map[string][]byte
	err    map[string]error
}

func (m *mockApplyCommandRunner) RunContext(_ context.Context, exe string, _ []string) ([]byte, error) {
	if err := m.err[exe]; err != nil {
		return nil, err
	}
	return m.output[exe], nil
}

func mockApplyPSOutput(allHosts, currentHost string) []byte {
	data, _ := json.Marshal(map[string]string{
		"CurrentUserAllHosts":    allHosts,
		"CurrentUserCurrentHost": currentHost,
	})
	return data
}

// captureStdout runs fn while redirecting os.Stdout and returns what was printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}

func TestApply_AutoMode_WarnsWhenModelNotOpus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2", "--mode=auto", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if !strings.Contains(out, "Auto mode on Bedrock requires Opus") {
		t.Errorf("expected auto-mode model warning with default Sonnet model, got:\n%s", out)
	}
}

func TestApply_AutoMode_NoWarnWhenModelIsOpus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2", "--mode=auto",
			"--model", "global.anthropic.claude-opus-4-8", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if strings.Contains(out, "Auto mode on Bedrock requires Opus") {
		t.Errorf("did not expect auto-mode warning when model is Opus, got:\n%s", out)
	}
}

func TestApply_NonAutoMode_NoAutoModeWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2", "--mode=plan", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if strings.Contains(out, "Auto mode on Bedrock requires Opus") {
		t.Errorf("did not expect auto-mode warning for --mode=plan, got:\n%s", out)
	}
}

func TestApply_Mantle_WarnsAboutPromptCaching(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2", "--mantle", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if !strings.Contains(out, "prompt caching") {
		t.Errorf("expected Mantle warning about prompt caching, got:\n%s", out)
	}
}

func TestApply_MantleURL_WarnsAboutPromptCaching(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2",
			"--mantle-url=https://example.test", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if !strings.Contains(out, "prompt caching") {
		t.Errorf("expected Mantle warning about prompt caching with --mantle-url, got:\n%s", out)
	}
}

func TestApply_MantleDryRun_WarnsAboutPromptCaching(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2",
			"--mantle", "--dry-run", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if !strings.Contains(out, "Dry run — no changes written.") {
		t.Errorf("expected dry-run output, got:\n%s", out)
	}
	if !strings.Contains(out, "prompt caching") {
		t.Errorf("expected Mantle warning on dry run, got:\n%s", out)
	}
}

func TestApply_NoMantleDryRun_NoMantleWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2", "--dry-run", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if strings.Contains(out, "prompt caching") {
		t.Errorf("did not expect Mantle warning on dry run without Mantle, got:\n%s", out)
	}
}

func TestUninstall_Codex_PreservesSharedToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Uninstalling a non-Claude CLI must NOT remove the shared bearer token,
	// even with --full (that would break a still-configured Claude).
	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"uninstall", "--cli=codex", "--full", "--force",
		}); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
	})
	if strings.Contains(out, "Removed bearer token from keychain") {
		t.Errorf("uninstall --cli=codex must NOT remove the shared keychain token, got:\n%s", out)
	}
}

func TestApply_Codex_NoClaudeWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=codex", "--auth=bedrock-api-key", "--bedrock-key=dummy",
			"--region=us-east-1", "--mode=auto", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})
	// The Claude auto-mode warning is nonsensical for Codex and must not appear.
	if strings.Contains(out, "Auto mode on Bedrock requires Opus") {
		t.Errorf("Codex apply must not print the Claude auto-mode warning, got:\n%s", out)
	}
}

func TestApply_CodexDryRun_ReapplySkipsPromptWhenCodexConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	codexDir := filepath.Join(home, ".codex")
	if err := safepath.MkdirAll(codexDir); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	configPath := filepath.Join(codexDir, "config.toml")
	content := []byte("model = \"openai.gpt-5.5\"\nmodel_provider = \"bedrock-mantle\"\n\n[model_providers.bedrock-mantle]\nname = \"Amazon Bedrock (Mantle)\"\nbase_url = \"https://bedrock-mantle.us-west-2.api.aws/openai/v1\"\nwire_api = \"responses\"\nenv_key = \"AWS_BEARER_TOKEN_BEDROCK\"\n")
	if err := safepath.WriteFile(codexDir, configPath, content); err != nil {
		t.Fatalf("write codex config: %v", err)
	}

	var err error
	withStdin(t, "", func() {
		err = ExecuteArgs([]string{
			"apply",
			"--cli=codex",
			"--dry-run",
			"--skip-preflight",
		})
	})
	if err != nil {
		t.Fatalf("codex re-apply dry-run should not prompt when codex config exists: %v", err)
	}
}

func TestApply_ClaudeDryRun_ReapplySkipsPromptWhenClaudeConfigExists(t *testing.T) {
	// Cross-provider guard: the reapply-detection refactor (threading the
	// provider through resolveApplyInputs) must NOT break Claude's own detection.
	// A first apply writes ~/.claude/settings.json; a second dry-run with closed
	// stdin must skip the prompt (detect existing config) and return cleanly.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	var err error
	withStdin(t, "", func() {
		err = ExecuteArgs([]string{
			"apply", "--cli=claude", "--dry-run", "--skip-preflight",
		})
	})
	if err != nil {
		t.Fatalf("claude re-apply dry-run should not prompt when settings.json exists: %v", err)
	}
}

// stubProvider is a minimal provider.Provider for exercising resolveApplyInputs
// error branches (bad config format, config-path failure) that a real
// registered provider never hits.
type stubProvider struct {
	formatName string
	pathErr    error
}

func (s stubProvider) Name() string             { return "stub" }
func (s stubProvider) BinaryNames() []string    { return []string{"stub"} }
func (s stubProvider) ConfigFormatName() string { return s.formatName }
func (s stubProvider) ConfigPath(home, scope string) (string, error) {
	if s.pathErr != nil {
		return "", s.pathErr
	}
	return filepath.Join(home, ".stub", "config"), nil
}
func (s stubProvider) NativeManagedKeys() []string         { return []string{"model"} }
func (s stubProvider) OwnsConfig(map[string]any) bool      { return false }
func (s stubProvider) ActivationMarkers() (string, string) { return "# B", "# E" }
func (s stubProvider) BuildConfig(*bedrock.Config, provider.Options) (provider.ConfigPlan, error) {
	return provider.ConfigPlan{}, nil
}
func (s stubProvider) LaunchSpec() provider.LaunchSpec   { return provider.LaunchSpec{} }
func (s stubProvider) Supports(provider.Capability) bool { return false }
func (s stubProvider) DeepMergeKeys() []string           { return nil }
func (s stubProvider) OwnedSubKeys() map[string][]string { return nil }

// TestResolveApplyInputs_BadConfigFormat_Errors covers the FormatByName error
// branch: a provider reporting an unknown config format surfaces an error rather
// than silently proceeding.
func TestResolveApplyInputs_BadConfigFormat_Errors(t *testing.T) {
	home := t.TempDir()
	bCfg := &bedrock.Config{Defaults: bedrock.Defaults{Region: "us-west-2", AuthMode: "iam"}}
	_, _, _, err := resolveApplyInputs(home, bCfg, stubProvider{formatName: "yaml"})
	if err == nil {
		t.Fatal("expected error for unknown config format")
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error should name the bad format, got: %v", err)
	}
}

// TestResolveApplyInputs_ConfigPathError_Propagates covers the ConfigPath error branch.
func TestResolveApplyInputs_ConfigPathError_Propagates(t *testing.T) {
	home := t.TempDir()
	bCfg := &bedrock.Config{Defaults: bedrock.Defaults{Region: "us-west-2", AuthMode: "iam"}}
	sentinel := fmt.Errorf("bad path")
	_, _, _, err := resolveApplyInputs(home, bCfg, stubProvider{formatName: "json", pathErr: sentinel})
	if err == nil {
		t.Fatal("expected ConfigPath error to propagate")
	}
}

// dirPathProvider is a stub whose ConfigPath points at an existing DIRECTORY, so
// Manager.Read fails (reading a directory as a file), covering the
// "checking existing configuration" error branch of resolveApplyInputs.
type dirPathProvider struct {
	stubProvider
	dir string
}

func (d dirPathProvider) ConfigPath(string, string) (string, error) { return d.dir, nil }

func TestResolveApplyInputs_ReadError_Propagates(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "isadir")
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bCfg := &bedrock.Config{Defaults: bedrock.Defaults{Region: "us-west-2", AuthMode: "iam"}}
	_, _, _, err := resolveApplyInputs(home, bCfg, dirPathProvider{stubProvider: stubProvider{formatName: "json"}, dir: dir})
	if err == nil {
		t.Fatal("expected a read error when config path is a directory")
	}
	if !strings.Contains(err.Error(), "checking existing configuration") {
		t.Errorf("expected wrapped read error, got: %v", err)
	}
}

func TestApply_UnknownCLI_Errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--cli=nonesuch", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected error for unknown --cli")
	}
	if !strings.Contains(err.Error(), "nonesuch") {
		t.Errorf("error should name the bad CLI, got: %v", err)
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should list supported CLI names to guide the user, got: %v", err)
	}
}

func TestApply_DefaultCLI_IsClaude_StillWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// No --cli flag: must behave exactly as before (Claude), writing settings.json.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Errorf("expected settings.json written for default (claude) CLI: %v", err)
	}
}

func TestApply_ExplicitClaudeCLI_Works(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--cli=claude", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=claude error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Errorf("expected settings.json written for --cli=claude: %v", err)
	}
}

func TestApply_NoMantle_NoMantleWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply error: %v", err)
		}
	})

	if strings.Contains(out, "prompt caching") {
		t.Errorf("did not expect Mantle warning when Mantle is off, got:\n%s", out)
	}
}
