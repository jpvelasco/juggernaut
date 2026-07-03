package cmd

import (
	"path/filepath"
	"strings"
	"testing"

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

// TestApply_Codex_WritesTOMLConfig drives a full codex apply and asserts the
// TOML config lands at ~/.codex/config.toml with the Mantle provider block.
func TestApply_Codex_WritesTOMLConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=iam", "--region=us-east-1", "--skip-preflight",
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

// TestUninstall_Codex_DryRun exercises the codex uninstall path without writing.
func TestUninstall_Codex_DryRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=iam", "--region=us-east-1", "--skip-preflight",
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
