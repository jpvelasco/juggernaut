package cmd

import (
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

// TestApply_MantleOnlyCLI_RejectsIAM: OpenCode and Grok route only through
// Mantle (bearer token required), so an explicit --auth=iam must be rejected with
// an actionable error. Codex now supports IAM via the AWS SDK credential chain.
func TestApply_MantleOnlyCLI_RejectsIAM(t *testing.T) {
	for _, cli := range []string{"opencode", "grok"} {
		t.Run(cli, func(t *testing.T) {
			_ = setupApplyTest(t)

			err := ExecuteArgs([]string{
				"apply", "--cli=" + cli, "--auth=iam",
				"--region=us-east-1", "--skip-preflight",
			})
			if err == nil {
				t.Fatalf("%s: expected --auth=iam to be rejected (Mantle needs a bearer token)", cli)
			}
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "iam") && !strings.Contains(msg, "bedrock api key") {
				t.Errorf("%s: error should explain IAM is unsupported, got: %v", cli, err)
			}
		})
	}
}

// TestApply_Codex_WritesTOMLConfig drives a full codex apply and structurally
// verifies the TOML config lands at ~/.codex/config.toml with the correct
// amazon-bedrock provider shape.
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
	if got["model"] != "openai.gpt-5.5" {
		t.Errorf("model = %v, want openai.gpt-5.5", got["model"])
	}
	if got["model_provider"] != "amazon-bedrock" {
		t.Errorf("model_provider = %v, want amazon-bedrock", got["model_provider"])
	}
	mp, ok := got["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers not a table: %T", got["model_providers"])
	}
	ab, ok := mp["amazon-bedrock"].(map[string]any)
	if !ok {
		t.Fatalf("amazon-bedrock not a table: %T", mp["amazon-bedrock"])
	}
	aws, ok := ab["aws"].(map[string]any)
	if !ok {
		t.Fatalf("aws not a table: %T", ab["aws"])
	}
	if aws["region"] != "us-east-1" {
		t.Errorf("aws.region = %v, want us-east-1", aws["region"])
	}
}

// TestApply_Codex_ModelFlag_Respected: --model=gpt-5.4 must produce a gpt-5.4
// config, not the GPT-5.5 default. Regression for the P2 bug where --model never
// reached provider.Options.Model. (Uses gpt-5.4 rather than gpt-oss because
// current Codex is Responses-only and gpt-oss — Chat-only on Mantle — is no
// longer a valid Codex model.)
func TestApply_Codex_ModelFlag_Respected(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real token; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--model=gpt-5.4",
		"--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value",
		"--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data := readFileForTest(t, filepath.Join(home, ".codex", "config.toml"))
	if !containsStr(data, "openai.gpt-5.4") {
		t.Errorf("expected gpt-5.4 model, got:\n%s", data)
	}
	if containsStr(data, "openai.gpt-5.5") {
		t.Errorf("must not fall back to gpt-5.5 when --model given:\n%s", data)
	}
	if !containsStr(data, `model_provider = "amazon-bedrock"`) {
		t.Errorf("gpt-5.4 should use amazon-bedrock provider, got:\n%s", data)
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
// model_providers.amazon-bedrock.aws table must survive — uninstall is a
// leaf-prune, not a data wipe.
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
	aws := mp["amazon-bedrock"].(map[string]any)["aws"].(map[string]any)
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
	awsAfter := mpAfter["amazon-bedrock"].(map[string]any)["aws"].(map[string]any)
	if _, ok := awsAfter["region"]; ok {
		t.Error("Juggernaut's region leaf should be removed")
	}
	if awsAfter["profile"] != "bedrock-ops" {
		t.Error("user's aws.profile sibling must survive uninstall")
	}
}

// TestUninstall_OpenCode_PreservesUserSiblingKeys: uninstall --cli=opencode
// removes ONLY Juggernaut's owned leaves — the top-level "model" key and the
// bedrock-mantle provider block. A user's own provider entries and their own
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
	// their own next to the bedrock-mantle block Juggernaut wrote.
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
	if _, ok := provAfter["bedrock-mantle"]; ok {
		t.Error("Juggernaut's bedrock-mantle provider should be removed")
	}
	if _, ok := provAfter["anthropic"]; !ok {
		t.Error("user's own provider entry lost on uninstall — data loss!")
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
