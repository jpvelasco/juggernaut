package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
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

// TestApply_MantleOnlyCLI_RejectsIAM: Codex/OpenCode/Grok route only through
// Mantle (bearer token required), so an explicit --auth=iam must be rejected with
// an actionable error rather than writing a config that can never authenticate.
func TestApply_MantleOnlyCLI_RejectsIAM(t *testing.T) {
	for _, cli := range []string{"codex", "opencode", "grok"} {
		t.Run(cli, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			setupMockPSRunner(t, home)

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

// TestApply_Codex_WritesTOMLConfig drives a full codex apply and asserts the
// TOML config lands at ~/.codex/config.toml with the Mantle provider block.
func TestApply_Codex_WritesTOMLConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=codex: %v", err)
	}
	data := readFileForTest(t, filepath.Join(home, ".codex", "config.toml"))
	for _, want := range []string{"openai.gpt-5.5", "bedrock-mantle", "wire_api", "openai/v1"} {
		if !containsStr(data, want) {
			t.Errorf("codex config.toml missing %q; got:\n%s", want, data)
		}
	}
}

// TestApply_Codex_ModelFlag_Respected: --model=gpt-5.4 must produce a gpt-5.4
// config, not the GPT-5.5 default. Regression for the P2 bug where --model never
// reached provider.Options.Model. (Uses gpt-5.4 rather than gpt-oss because
// current Codex is Responses-only and gpt-oss — Chat-only on Mantle — is no
// longer a valid Codex model.)
func TestApply_Codex_ModelFlag_Respected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	if !containsStr(data, `wire_api = "responses"`) {
		t.Errorf("gpt-5.4 should use wire_api=responses, got:\n%s", data)
	}
}

// TestApply_OpenCode_PassthroughModel_PrintsWarning locks in that provider
// ConfigPlan.Warnings are surfaced to the user. An unverified (passthrough)
// OpenCode model must print the "not in the curated set" heads-up — the warning
// is the whole point of the honest-passthrough design and was previously
// computed but never printed.
func TestApply_OpenCode_PassthroughModel_PrintsWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=opencode", "--model=some.exotic-v9",
			"--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})
	if !strings.Contains(out, "curated set") {
		t.Errorf("expected passthrough (unverified) model warning, got:\n%s", out)
	}
}

// TestApply_OpenCode_CuratedModel_NoWarning: a curated model writes cleanly with
// no passthrough warning.
func TestApply_OpenCode_CuratedModel_NoWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=opencode", "--model=glm-4.7",
			"--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})
	if strings.Contains(out, "curated set") {
		t.Errorf("curated model must not warn, got:\n%s", out)
	}
	data := readFileForTest(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	if !containsStr(data, "zai.glm-4.7") {
		t.Errorf("expected glm-4.7 in config, got:\n%s", data)
	}
}

// TestUninstall_Codex_DryRun exercises the codex uninstall path without writing.
func TestUninstall_Codex_DryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	data := readFileForTest(t, filepath.Join(home, ".codex", "config.toml"))
	if containsStr(data, "bedrock-mantle") || containsStr(data, "model_provider") {
		t.Errorf("codex managed keys should be removed, got:\n%s", data)
	}
}

// TestLaunchTargetFor_Claude maps the Claude provider's LaunchSpec + binaries
// onto the activation LaunchTarget.
func TestLaunchTargetFor_Claude(t *testing.T) {
	p, _ := provider.Get("claude")
	tgt := launchTargetFor(p)
	if len(tgt.BinaryNames) == 0 || tgt.BinaryNames[0] != "claude" && tgt.BinaryNames[0] != "claude.exe" {
		t.Errorf("claude binary names = %v", tgt.BinaryNames)
	}
	if tgt.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("token env = %q", tgt.TokenEnvVar)
	}
	if tgt.StaticEnv["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Errorf("claude should carry the use-bedrock flag, got %v", tgt.StaticEnv)
	}
}

// TestLaunchTargetFor_Claude_IAM_NeedsNoStaticToken is the regression guard for
// the Claude+IAM launch bug: the Claude LaunchTarget must NOT statically force a
// bearer token (NeedsToken=false). IAM auth uses SigV4, has no keychain token,
// and the launcher decides token-need from authModes. If NeedsToken were true,
// every Claude+IAM launch would fail with "bedrock API key not found".
func TestLaunchTargetFor_Claude_IAM_NeedsNoStaticToken(t *testing.T) {
	p, _ := provider.Get("claude")
	tgt := launchTargetFor(p)
	if tgt.NeedsToken {
		t.Error("Claude LaunchTarget.NeedsToken must be false — token need is auth-mode-dependent (IAM needs none), decided at launch by authModes")
	}
}

// TestLaunchTargetFor_Codex maps the Codex provider: codex binary, bearer token,
// and NO static enable flag (routes via config).
func TestLaunchTargetFor_Codex(t *testing.T) {
	p, _ := provider.Get("codex")
	tgt := launchTargetFor(p)
	if len(tgt.BinaryNames) == 0 || (tgt.BinaryNames[0] != "codex" && tgt.BinaryNames[0] != "codex.exe") {
		t.Errorf("codex binary names = %v", tgt.BinaryNames)
	}
	if tgt.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("token env = %q", tgt.TokenEnvVar)
	}
	if len(tgt.StaticEnv) != 0 {
		t.Errorf("codex should have no static enable flag, got %v", tgt.StaticEnv)
	}
	if !tgt.NeedsToken {
		t.Error("codex via Mantle needs a token")
	}
}
