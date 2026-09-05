package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := safepath.ReadFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readFileForTestErr(path string) ([]byte, error) {
	return safepath.ReadFile(filepath.Dir(path), path)
}

func containsStr(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// TestApply_NativeCLIs_AcceptIAM: after native bedrock-runtime migration (v6),
// every CLI (including OpenCode and Grok) supports IAM via SigV4. An explicit
// --auth=iam must be accepted and should not error.
func TestApply_NativeCLIs_AcceptIAM(t *testing.T) {
	for _, cli := range []string{"opencode", "grok", "codex"} {
		t.Run(cli, func(t *testing.T) {
			_ = setupApplyTest(t)

			err := ExecuteArgs([]string{
				"apply", "--cli=" + cli, "--auth=iam",
				"--region=us-east-1", "--skip-preflight",
			})
			if err != nil {
				t.Fatalf("%s: expected --auth=iam to be accepted on native bedrock-runtime, got error: %v", cli, err)
			}
		})
	}
}

// TestApply_OpenCode_IAM_WritesJuggernautAuth: the auth-mode metadata lives in
// the user-scope sidecar (~/.config/opencode/.juggernaut.json), NOT in
// opencode.json — OpenCode's strict schema rejects unknown top-level keys.
// opencode.json must contain no juggernaut key and no "whitelist": null (RC1+RC2).
func TestApply_OpenCode_IAM_WritesJuggernautAuth(t *testing.T) {
	home := setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=opencode", "--auth=iam",
		"--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	cfg := parseJSONForTest(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	assertOpenCodeConfigShape(t, cfg, "opencode.json")
	sidecar := parseJSONForTest(t, filepath.Join(home, ".config", "opencode", ".juggernaut.json"))
	assertSidecarAuthMode(t, sidecar, authmode.IAM, "us-east-1")
}

func TestApply_Grok_IAM_OmitsAuthProviderCommand(t *testing.T) {
	home := setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=iam",
		"--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data := readFileForTest(t, filepath.Join(home, ".grok", "config.toml"))
	if containsStr(data, "auth_provider_command") {
		t.Errorf("IAM apply must not write auth_provider_command, got:\n%s", data)
	}
	if !containsStr(data, "mode") || !containsStr(data, "iam") {
		t.Errorf("expected juggernaut.auth.mode=iam, got:\n%s", data)
	}
}

func TestApply_OpenCode_ProjectScope_WritesAuthMode(t *testing.T) {
	home := setupApplyTest(t)
	configBytes, err := os.ReadFile(findBedrockConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	orig := embeddedConfigBytes
	SetEmbeddedConfig(configBytes)
	t.Cleanup(func() { SetEmbeddedConfig(orig) })
	dir := t.TempDir()
	t.Chdir(dir)
	if err := ExecuteArgs([]string{
		"apply", "--cli=opencode", "--auth=iam",
		"--scope=project", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Project-scope apply writes ./opencode.json (no juggernaut key, no
	// whitelist:null) plus the project sidecar ./.juggernaut.json — and must
	// NOT create a user-scope fallback sidecar.
	cfg := parseJSONForTest(t, filepath.Join(dir, "opencode.json"))
	assertOpenCodeConfigShape(t, cfg, "./opencode.json")
	assertSidecarAuthMode(t, parseJSONForTest(t, filepath.Join(dir, ".juggernaut.json")), authmode.IAM, "us-east-1")
	userSidecar := filepath.Join(home, ".config", "opencode", ".juggernaut.json")
	if _, err := os.Stat(userSidecar); !os.IsNotExist(err) {
		t.Errorf("project-scope apply must not create a user-scope sidecar %s (stat err=%v)", userSidecar, err)
	}
}

// parseJSONForTest reads and unmarshals a JSON file (test helper).
func parseJSONForTest(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s must be valid JSON: %v", path, err)
	}
	return doc
}

// assertOpenCodeConfigShape pins the DoD JSON shape: no top-level juggernaut
// key, and the whitelisted leaf is either a non-empty array or absent — never
// null and never an empty array (RC1+RC2). No schema library needed.
func assertOpenCodeConfigShape(t *testing.T, cfg map[string]any, label string) {
	t.Helper()
	if _, ok := cfg["juggernaut"]; ok {
		t.Errorf("%s must NOT contain a top-level juggernaut key (OpenCode schema: additionalProperties=false)", label)
	}
	prov, _ := cfg["provider"].(map[string]any)
	ab, _ := prov["amazon-bedrock"].(map[string]any)
	wl, present := ab["whitelist"]
	if !present {
		return
	}
	arr, ok := wl.([]any)
	if !ok || len(arr) == 0 {
		t.Errorf("%s: whitelist must be a non-empty array or absent, got %#v", label, wl)
	}
}

// assertSidecarAuthMode checks the sidecar document's juggernaut.auth block.
func assertSidecarAuthMode(t *testing.T, sidecar map[string]any, wantMode, wantRegion string) {
	t.Helper()
	jb, _ := sidecar["juggernaut"].(map[string]any)
	if jb == nil {
		t.Fatalf("sidecar has no juggernaut block: %#v", sidecar)
	}
	auth, _ := jb["auth"].(map[string]any)
	if mode, _ := auth["mode"].(string); mode != wantMode {
		t.Errorf("sidecar auth.mode = %q, want %q", mode, wantMode)
	}
	if region, _ := auth["region"].(string); region != wantRegion {
		t.Errorf("sidecar auth.region = %q, want %q", region, wantRegion)
	}
}

// TestApply_Codex_WritesTOMLConfig drives a full codex apply and structurally
// verifies the TOML config lands at ~/.codex/config.toml with the built-in
// amazon-bedrock-runtime provider shape (v6 routes Codex via bedrock-runtime,
// not the custom v5 amazon-bedrock table).
func TestApply_Codex_WritesTOMLConfig(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=codex: %v", err)
	}

	// Structural TOML parse — verify the nested table shape, not just strings.
	cfgPath := filepath.Join(home, ".codex", "config.toml")
	tomlFmt, _ := config.FormatByName("toml")
	mgr := config.NewManagerWithFormat(cfgPath, tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml: %v", err)
	}
	if got["model"] != "global.openai.gpt-5.6-sol" {
		t.Errorf("model = %v, want global.openai.gpt-5.6-sol", got["model"])
	}
	if got["model_provider"] != "amazon-bedrock-runtime" {
		t.Errorf("model_provider = %v, want amazon-bedrock-runtime", got["model_provider"])
	}
	mp, ok := got["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers not a table: %T", got["model_providers"])
	}
	if _, ok := mp["amazon-bedrock"]; ok {
		t.Errorf("legacy [model_providers.amazon-bedrock] table must not be written, got: %v", mp["amazon-bedrock"])
	}
	rt, ok := mp["amazon-bedrock-runtime"].(map[string]any)
	if !ok {
		t.Fatalf("amazon-bedrock-runtime not a table: %T", mp["amazon-bedrock-runtime"])
	}
	aws, ok := rt["aws"].(map[string]any)
	if !ok {
		t.Fatalf("aws not a table: %T", rt["aws"])
	}
	if aws["region"] != "us-east-1" {
		t.Errorf("aws.region = %v, want us-east-1", aws["region"])
	}
}

// TestApply_Codex_ModelFlag_Respected: --model=terra must produce a gpt-5.6-terra
// config, not the sol default. Regression for the P2 bug where --model never
// reached provider.Options.Model.
func TestApply_Codex_ModelFlag_Respected(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--model=terra",
		"--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value",
		"--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data := readFileForTest(t, filepath.Join(home, ".codex", "config.toml"))
	if !containsStr(data, "global.openai.gpt-5.6-terra") {
		t.Errorf("expected gpt-5.6-terra model, got:\n%s", data)
	}
	if containsStr(data, "gpt-5.6-sol") {
		t.Errorf("must not fall back to sol when --model=terra given:\n%s", data)
	}
	if !containsStr(data, `model_provider = "amazon-bedrock-runtime"`) {
		t.Errorf("terra should use the amazon-bedrock-runtime provider, got:\n%s", data)
	}
}

// TestApply_OpenCode_PassthroughModel_PrintsWarning locks in that provider
// ConfigPlan.Warnings are surfaced to the user. A raw (passthrough)
// OpenCode model must print the "not a known convenience alias" heads-up — the warning
// is the whole point of the honest-passthrough design and was previously
// computed but never printed.
func TestApply_OpenCode_PassthroughModel_PrintsWarning(t *testing.T) {
	_ = setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=opencode", "--model=some.exotic-v9",
			"--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})
	if !strings.Contains(out, "known convenience alias") {
		t.Errorf("expected passthrough (unverified) model warning, got:\n%s", out)
	}
}

// TestApply_OpenCode_CuratedModel_NoWarning: a curated model writes cleanly with
// no passthrough warning.
func TestApply_OpenCode_CuratedModel_NoWarning(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=opencode", "--model=glm-4.7",
			"--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})
	if strings.Contains(out, "known convenience alias") {
		t.Errorf("curated model must not warn, got:\n%s", out)
	}
	data := readFileForTest(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	if !containsStr(data, "zai.glm-4.7") {
		t.Errorf("expected glm-4.7 in config, got:\n%s", data)
	}
}

// TestUninstall_Codex_DryRun exercises the codex uninstall path without writing.
func TestUninstall_Codex_DryRun(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := ExecuteArgs([]string{
		"uninstall", "--cli=codex", "--dry-run", "--force",
	}); err != nil {
		t.Fatalf("uninstall --cli=codex --dry-run: %v", err)
	}
	// Dry run must not remove the config.
	if _, err := readFileForTestErr(filepath.Join(home, ".codex", "config.toml")); err != nil {
		t.Errorf("dry-run should not remove codex config: %v", err)
	}
}

// TestUninstall_Codex_ActuallyRemoves: a real (non-dry-run) codex uninstall must
// strip the managed keys. Regression for the P2 bug where HasJuggernautBlock
// required a Claude meta marker that Codex configs never write, so uninstall
// no-opped and left the Bedrock provider configured.
func TestUninstall_Codex_ActuallyRemoves(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := ExecuteArgs([]string{
		"uninstall", "--cli=codex", "--force",
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	// Structural TOML parse — verify specific keys are removed, not just strings.
	cfgPath := filepath.Join(home, ".codex", "config.toml")
	tomlFmt, _ := config.FormatByName("toml")
	mgr := config.NewManagerWithFormat(cfgPath, tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml: %v", err)
	}
	for _, key := range []string{"model", "model_provider", "model_providers"} {
		if _, ok := got[key]; ok {
			t.Errorf("managed key %q should be removed, got: %v", key, got)
		}
	}
}

// TestUninstall_Codex_PreservesUserSiblingKeys: uninstall removes ONLY the
// leaves Juggernaut owns. A user's own top-level keys, their own
// model_providers entries, and sibling settings inside the
// model_providers.amazon-bedrock-runtime.aws table must survive — uninstall
// is a leaf-prune, not a data wipe.
func TestUninstall_Codex_PreservesUserSiblingKeys(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // apply stores a real credential; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Inject user-owned sibling content: a top-level key, a provider entry of
	// their own, and a sibling inside the same aws table Juggernaut writes into.
	cfgPath := filepath.Join(home, ".codex", "config.toml")
	tomlFmt, _ := config.FormatByName("toml")
	mgr := config.NewManagerWithFormat(cfgPath, tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml: %v", err)
	}
	got["telemetry"] = false
	mp := got["model_providers"].(map[string]any)
	mp["my-own"] = map[string]any{"base_url": "http://192.168.0.1/v1", "env_key": "MY_OWN_KEY"}
	aws := mp["amazon-bedrock-runtime"].(map[string]any)["aws"].(map[string]any)
	aws["profile"] = "bedrock-ops"
	if err := mgr.Write(got); err != nil {
		t.Fatalf("writing config.toml: %v", err)
	}

	if err := ExecuteArgs([]string{
		"uninstall", "--cli=codex", "--force",
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml after uninstall: %v", err)
	}
	for _, key := range []string{"model", "model_provider", "juggernaut"} {
		if _, ok := after[key]; ok {
			t.Errorf("managed key %q should be removed, got: %v", key, after[key])
		}
	}
	if after["telemetry"] != false {
		t.Errorf("user's top-level 'telemetry' key must survive, got: %v", after["telemetry"])
	}
	mpAfter, ok := after["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers table vanished — user providers must survive, got: %v", after["model_providers"])
	}
	if _, ok := mpAfter["my-own"]; !ok {
		t.Error("user's own provider entry lost on uninstall — data loss!")
	}
	awsAfter := mpAfter["amazon-bedrock-runtime"].(map[string]any)["aws"].(map[string]any)
	if _, ok := awsAfter["region"]; ok {
		t.Error("Juggernaut's region leaf should be removed")
	}
	if awsAfter["profile"] != "bedrock-ops" {
		t.Error("user's aws.profile sibling must survive uninstall")
	}
}

// TestUninstall_OpenCode_PreservesUserSiblingKeys: uninstall --cli=opencode
// removes ONLY Juggernaut's owned leaves — the top-level "model" key and the
// amazon-bedrock provider block. A user's own provider entries and their own
// top-level keys must survive.
func TestUninstall_OpenCode_PreservesUserSiblingKeys(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // apply stores a real credential; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=opencode", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Inject user-owned sibling content: a top-level key and a provider of
	// their own next to the amazon-bedrock block Juggernaut wrote.
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	mgr := config.NewManager(configPath)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse opencode.json: %v", err)
	}
	got["theme"] = "dark"
	prov := got["provider"].(map[string]any)
	prov["anthropic"] = map[string]any{"npm": "@ai-sdk/anthropic", "options": map[string]any{"apiKey": "sk-ant-own-key"}}
	if err := mgr.Write(got); err != nil {
		t.Fatalf("writing opencode.json: %v", err)
	}

	if err := ExecuteArgs([]string{
		"uninstall", "--cli=opencode", "--force",
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse opencode.json after uninstall: %v", err)
	}
	if _, ok := after["model"]; ok {
		t.Error("managed 'model' key should be removed")
	}
	if after["theme"] != "dark" {
		t.Errorf("user's top-level 'theme' key must survive, got: %v", after["theme"])
	}
	provAfter, ok := after["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider table vanished — user providers must survive, got: %v", after["provider"])
	}
	if _, ok := provAfter["amazon-bedrock"]; ok {
		t.Error("Juggernaut's amazon-bedrock provider should be removed")
	}
	if _, ok := provAfter["anthropic"]; !ok {
		t.Error("user's own provider entry lost on uninstall — data loss!")
	}
}

// TestUninstall_Grok_PreservesUserSiblingKeys: uninstall --cli=grok removes
// ONLY Juggernaut's owned leaves — the bedrock-grok model profile, the
// models.default pointer, and the two auth keys — across three deep-merge
// tables. A user's own model profiles, models settings, and auth settings
// must all survive.
// TestUninstall_OpenCode_RemovesSidecarAndLegacyBlock: a user-scope uninstall
// of OpenCode must remove both the auth-metadata sidecar (.juggernaut.json)
// and a legacy in-file juggernaut block left by v6.2.0–v6.3.0, while preserving
// the rest of the config. Codex/Grok in-file blocks are untouched (they have no
// sidecar).
func TestUninstall_OpenCode_RemovesSidecarAndLegacyBlock(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	applyCLI(t, "opencode")

	configDir := filepath.Join(home, ".config", "opencode")
	// Re-seed a legacy in-file juggernaut block (v6.2.0–v6.3.0 shape) alongside
	// the current sidecar to prove uninstall cleans up the old location too.
	mgr := config.NewManager(filepath.Join(configDir, "opencode.json"))
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse opencode.json: %v", err)
	}
	got["juggernaut"] = map[string]any{
		"auth": map[string]any{"mode": authmode.IAM},
		"meta": map[string]any{"managedBy": "juggernaut"},
	}
	if err := mgr.Write(got); err != nil {
		t.Fatalf("injecting legacy block: %v", err)
	}

	// Dry-run previews the removal but changes nothing.
	if err := ExecuteArgs([]string{"uninstall", "--cli=opencode", "--dry-run", "--force"}); err != nil {
		t.Fatalf("uninstall --dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, ".juggernaut.json")); err != nil {
		t.Errorf("dry-run must not remove the sidecar: %v", err)
	}

	if err := ExecuteArgs([]string{"uninstall", "--cli=opencode", "--force"}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	// Sidecar gone.
	if _, err := os.Stat(filepath.Join(configDir, ".juggernaut.json")); !os.IsNotExist(err) {
		t.Errorf("sidecar must be removed, stat err = %v", err)
	}
	// Legacy in-file block gone, managed keys gone.
	after, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse opencode.json after uninstall: %v", err)
	}
	if _, ok := after["juggernaut"]; ok {
		t.Errorf("legacy in-file juggernaut block must be removed, got: %v", after["juggernaut"])
	}
	if _, ok := after["model"]; ok {
		t.Errorf("managed 'model' key should be removed")
	}
}

func TestUninstall_Grok_PreservesUserSiblingKeys(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // apply stores a real credential; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Inject user-owned sibling content into all three tables Juggernaut writes.
	configPath := filepath.Join(home, ".grok", "config.toml")
	tomlFmt, _ := config.FormatByName("toml")
	mgr := config.NewManagerWithFormat(configPath, tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml: %v", err)
	}
	got["model"].(map[string]any)["batwing-coder"] = map[string]any{"base_url": "http://192.168.0.1/v1"}
	got["models"].(map[string]any)["web_search"] = "batwing-coder"
	got["auth"].(map[string]any)["extra_user_setting"] = "keep-me"
	if err := mgr.Write(got); err != nil {
		t.Fatalf("writing config.toml: %v", err)
	}

	if err := ExecuteArgs([]string{
		"uninstall", "--cli=grok", "--force",
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml after uninstall: %v", err)
	}
	modelTbl, ok := after["model"].(map[string]any)
	if !ok {
		t.Fatalf("model table vanished — user profiles must survive, got: %v", after["model"])
	}
	if _, ok := modelTbl["bedrock-grok"]; ok {
		t.Error("Juggernaut's bedrock-grok profile should be removed")
	}
	if _, ok := modelTbl["batwing-coder"]; !ok {
		t.Error("user's own model profile lost on uninstall — data loss!")
	}
	modelsTbl, ok := after["models"].(map[string]any)
	if !ok {
		t.Fatalf("models table vanished — user settings must survive, got: %v", after["models"])
	}
	if _, ok := modelsTbl["default"]; ok {
		t.Error("Juggernaut's models.default pointer should be removed")
	}
	if modelsTbl["web_search"] != "batwing-coder" {
		t.Error("user's models.web_search setting lost on uninstall — data loss!")
	}
	authTbl, ok := after["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth table vanished — user settings must survive, got: %v", after["auth"])
	}
	for _, key := range []string{"auth_provider_command", "auth_provider_label"} {
		if _, ok := authTbl[key]; ok {
			t.Errorf("Juggernaut's auth.%s should be removed", key)
		}
	}
	if authTbl["extra_user_setting"] != "keep-me" {
		t.Error("user's own auth setting lost on uninstall — data loss!")
	}
}

// TestApply_Codex_IAM_Allowed: Codex now supports IAM via the AWS SDK credential
// chain, so apply --cli=codex --auth=iam must succeed (not rejected).
func TestApply_Codex_IAM_Allowed(t *testing.T) {
	home := setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=iam",
		"--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=codex --auth=iam: %v", err)
	}

	// Verify the config was written with IAM auth mode in the juggernaut block.
	cfgPath := filepath.Join(home, ".codex", "config.toml")
	tomlFmt, _ := config.FormatByName("toml")
	mgr := config.NewManagerWithFormat(cfgPath, tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse config.toml: %v", err)
	}
	juggernaut, ok := got["juggernaut"].(map[string]any)
	if !ok {
		t.Fatalf("juggernaut block not found in config")
	}
	auth, ok := juggernaut["auth"].(map[string]any)
	if !ok {
		t.Fatalf("juggernaut.auth not found")
	}
	if auth["mode"] != "iam" {
		t.Errorf("auth mode = %v, want iam", auth["mode"])
	}
}

// TestLaunchTargetFor_Claude maps the Claude provider's LaunchSpec + binaries
// onto the activation LaunchTarget.
func TestLaunchTargetFor_Claude(t *testing.T) {
	p, _ := provider.Get("claude")
	tgt := launchTargetFor(p, "")
	if len(tgt.BinaryNames) == 0 || tgt.BinaryNames[0] != "claude" && tgt.BinaryNames[0] != "claude.exe" {
		t.Errorf("claude binary names = %v", tgt.BinaryNames)
	}
	if tgt.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("token env = %q", tgt.TokenEnvVar)
	}
	if tgt.StaticEnv["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Errorf("claude should carry the use-bedrock flag, got %v", tgt.StaticEnv)
	}
	if tgt.RuntimeStateName != "claude" {
		t.Errorf("Claude runtime state name = %q, want claude", tgt.RuntimeStateName)
	}
}

// TestLaunchTargetFor_Claude_IAM_NeedsNoStaticToken is the regression guard for
// the Claude+IAM launch bug: the Claude LaunchTarget must NOT statically force a
// bearer token (NeedsToken=false). IAM auth uses SigV4, has no keychain token,
// and the launcher decides token-need from authModes. If NeedsToken were true,
// every Claude+IAM launch would fail with "bedrock API key not found".
func TestLaunchTargetFor_Claude_IAM_NeedsNoStaticToken(t *testing.T) {
	p, _ := provider.Get("claude")
	tgt := launchTargetFor(p, "")
	if tgt.NeedsToken {
		t.Error("Claude LaunchTarget.NeedsToken must be false — token need is auth-mode-dependent (IAM needs none), decided at launch by authModes")
	}
}

// TestLaunchTargetFor_Codex maps the Codex provider: codex binary, bearer token,
// and NO static enable flag (routes via config). NeedsToken is false — auth mode
// (IAM or API key) is stored in the config.toml juggernaut block and resolved at
// launch time.
func TestLaunchTargetFor_Codex(t *testing.T) {
	p, _ := provider.Get("codex")
	tgt := launchTargetFor(p, "")
	if len(tgt.BinaryNames) == 0 || (tgt.BinaryNames[0] != "codex" && tgt.BinaryNames[0] != "codex.exe") {
		t.Errorf("codex binary names = %v", tgt.BinaryNames)
	}
	if tgt.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("token env = %q", tgt.TokenEnvVar)
	}
	if len(tgt.StaticEnv) != 0 {
		t.Errorf("codex should have no static enable flag, got %v", tgt.StaticEnv)
	}
	// Codex now supports both IAM and API key auth — NeedsToken is false because
	// the launch wrapper reads the auth mode from the config file's juggernaut
	// block to decide at runtime.
	if tgt.NeedsToken {
		t.Error("codex NeedsToken must be false — auth mode is resolved from config at launch time")
	}
}

// TestLaunchTargetFor_OpenCodeAndGrok_NeedsNoStaticToken: IAM/SSO apply must
// launch without a Bedrock API key. Token need is read from juggernaut.auth.
func TestLaunchTargetFor_OpenCodeAndGrok_NeedsNoStaticToken(t *testing.T) {
	for _, name := range []string{"opencode", "grok"} {
		p, _ := provider.Get(name)
		tgt := launchTargetFor(p, "")
		if tgt.NeedsToken {
			t.Errorf("%s LaunchTarget.NeedsToken must be false — auth mode is resolved from config at launch", name)
		}
	}
}

func TestResolveLaunchConfigPaths_ProjectThenUser(t *testing.T) {
	home := setupApplyTest(t)

	codex, _ := provider.Get("codex")
	paths := resolveLaunchConfigPaths(codex, home)
	if len(paths) != 2 {
		t.Fatalf("codex paths = %v, want project then user", paths)
	}
	if filepath.ToSlash(paths[0]) != ".codex/config.toml" && filepath.ToSlash(paths[0]) != "./.codex/config.toml" {
		t.Errorf("codex project path = %q, want ./.codex/config.toml", paths[0])
	}
	if !strings.Contains(filepath.ToSlash(paths[1]), ".codex/config.toml") {
		t.Errorf("codex user path = %q, want ~/.codex/config.toml", paths[1])
	}

	opencode, _ := provider.Get("opencode")
	oPaths := resolveLaunchConfigPaths(opencode, home)
	// OpenCode keeps the auth block in a sidecar (.juggernaut.json), so the
	// launch candidates are project+user sidecars, then project+user configs.
	if len(oPaths) != 4 {
		t.Fatalf("opencode paths = %v, want 4 (project/user sidecars then project/user configs)", oPaths)
	}
	if filepath.Base(oPaths[0]) != ".juggernaut.json" {
		t.Errorf("opencode project sidecar path = %q, want ./.juggernaut.json", oPaths[0])
	}
	if filepath.Base(oPaths[2]) != "opencode.json" {
		t.Errorf("opencode project config path = %q, want ./opencode.json", oPaths[2])
	}

	grok, _ := provider.Get("grok")
	gPaths := resolveLaunchConfigPaths(grok, home)
	if len(gPaths) != 1 {
		t.Fatalf("grok is user-only, got %v", gPaths)
	}
	if !strings.Contains(filepath.ToSlash(gPaths[0]), ".grok/config.toml") {
		t.Errorf("grok path = %q, want ~/.grok/config.toml", gPaths[0])
	}
}

func TestLaunchCLI_RequiresExplicitCLIName(t *testing.T) {
	err := ExecuteArgs([]string{"launch-cli", "--"})
	if err == nil {
		t.Fatal("expected launch-cli without a CLI name to fail")
	}
	if !strings.Contains(err.Error(), "requires a CLI name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLaunchCLI_UnknownCLI returns an error from provider.Get before touching
// activation or keychain, verifying the provider lookup is the first gate.
func TestLaunchCLI_UnknownCLI(t *testing.T) {
	err := ExecuteArgs([]string{"launch-cli", "not-a-real-cli", "--"})
	if err == nil {
		t.Fatal("expected launch-cli with unknown CLI name to fail")
	}
	if !strings.Contains(err.Error(), "not-a-real-cli") {
		t.Fatalf("expected error mentioning unknown CLI, got: %v", err)
	}
}

// TestLaunchCLI_NoArgs returns the "requires a CLI name" error even when there
// is no "--" separator — zero args is the same failure.
func TestLaunchCLI_NoArgsAtAll(t *testing.T) {
	err := ExecuteArgs([]string{"launch-cli"})
	if err == nil {
		t.Fatal("expected launch-cli with no args to fail")
	}
	if !strings.Contains(err.Error(), "requires a CLI name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLaunch_HistoricalFormWithCLIName verifies the legacy `launch codex -- ...`
// syntax (positional CLI name before --) is accepted and routes through the same
// provider lookup as launch-cli.
func TestLaunch_HistoricalFormWithCLIName(t *testing.T) {
	err := ExecuteArgs([]string{"launch", "not-a-real-cli", "--"})
	if err == nil {
		t.Fatal("expected launch with unknown CLI name to fail")
	}
	if !strings.Contains(err.Error(), "not-a-real-cli") {
		t.Fatalf("expected error mentioning unknown CLI, got: %v", err)
	}
}

// TestLaunch_HistoricalFormNoArgs verifies the bare `launch -- ...` form
// (defaulting to claude) reaches provider lookup, not a parsing error.
func TestLaunch_HistoricalFormNoArgs(t *testing.T) {
	err := ExecuteArgs([]string{"launch", "--"})
	// This hits provider.Get("claude") which succeeds, then activation.LaunchCLI
	// which tries to resolve the real claude binary on PATH — it is not found in
	// tests, so we expect an exec error, not a parse error.
	if err == nil {
		t.Fatal("expected launch with no CLI name to fail (binary not found)")
	}
	// The error should be about finding the claude binary, not a parse error.
	if strings.Contains(err.Error(), "requires a CLI name") {
		t.Fatalf("bare launch should not require a CLI name, got: %v", err)
	}
}

// TestLaunch_DefaultsToClaude verifies `launch -- ...` with no positional CLI
// name defaults to "claude" by reaching the provider-get step (not a parse error).
func TestLaunch_DefaultsToClaude(t *testing.T) {
	err := ExecuteArgs([]string{"launch"})
	if err == nil {
		t.Fatal("expected launch to fail (binary not found)")
	}
	// Should reach provider.Get("claude") → activation, not a parse error.
	if strings.Contains(err.Error(), "requires a CLI name") {
		t.Fatalf("launch should not require a CLI name, got: %v", err)
	}
}

// TestResolveSelfPaths_NoEnv returns nil when JUGGERNAUT_ORIGINAL_BIN is not set.
func TestResolveSelfPaths_NoEnv(t *testing.T) {
	orig := os.Getenv("JUGGERNAUT_ORIGINAL_BIN")
	os.Unsetenv("JUGGERNAUT_ORIGINAL_BIN")
	t.Cleanup(func() { os.Setenv("JUGGERNAUT_ORIGINAL_BIN", orig) })
	result := resolveSelfPaths()
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// TestResolveSelfPaths_RelativePath returns nil for a relative (unsafe) path.
// Windows-only: resolveSelfPaths returns nil on non-Windows before checking the env var.
func TestResolveSelfPaths_RelativePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only")
	}
	t.Setenv("JUGGERNAUT_ORIGINAL_BIN", "relative/path.exe")
	result := resolveSelfPaths()
	if result != nil {
		t.Errorf("expected nil for relative path, got %v", result)
	}
}

// TestResolveSelfPaths_AbsolutePath returns the path when it's absolute.
// Windows-only: resolveSelfPaths returns nil on non-Windows before checking the env var.
func TestResolveSelfPaths_AbsolutePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only")
	}
	t.Setenv("JUGGERNAUT_ORIGINAL_BIN", "C:\\some\\path\\juggernaut.exe")
	result := resolveSelfPaths()
	if len(result) != 1 {
		t.Fatalf("expected 1 path, got %d", len(result))
	}
	if result[0] != "C:\\some\\path\\juggernaut.exe" {
		t.Errorf("expected original path, got %q", result[0])
	}
}
