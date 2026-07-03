package activation

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLaunchWithOptions_CodexSpec: with a Codex LaunchTarget, the launcher must
// resolve the `codex` binary, inject the bearer token into the spec's env var,
// and NOT set CLAUDE_CODE_USE_BEDROCK (Codex routes via config, not an env flag).
func TestLaunchWithOptions_CodexSpec(t *testing.T) {
	// Clear the ambient var: this test process runs inside Claude Code, which
	// sets CLAUDE_CODE_USE_BEDROCK in the real environment. We assert the Codex
	// launch does not itself set it.
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	home := t.TempDir()
	writeSettings(t, home, "bedrock-api-key") // token needed

	realDir := t.TempDir()
	codexName := "codex"
	if runtime.GOOS == "windows" {
		codexName = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, codexName), "real codex")

	var gotPath string
	var gotEnv []string
	err := LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        []string{"--version"},
		Path:        realDir,
		TokenGetter: func() (string, error) { return "tok-123", nil },
		Runner: func(path string, args []string, env []string) error {
			gotPath, gotEnv = path, env
			return nil
		},
		Target: LaunchTarget{
			BinaryNames: []string{codexName},
			TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
			NeedsToken:  true,
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if !strings.Contains(gotPath, "codex") {
		t.Errorf("expected codex binary resolved, got %q", gotPath)
	}
	if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "tok-123" {
		t.Errorf("expected bearer token injected, env=%v", gotEnv)
	}
	if envValue(gotEnv, "CLAUDE_CODE_USE_BEDROCK") != "" {
		t.Errorf("Codex must NOT set CLAUDE_CODE_USE_BEDROCK, env=%v", gotEnv)
	}
}

// TestLaunchWithOptions_ClaudeDefault: an empty Target defaults to Claude
// behavior (claude binary + CLAUDE_CODE_USE_BEDROCK=1) — back-compat.
func TestLaunchWithOptions_ClaudeDefault(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, "iam")

	realDir := t.TempDir()
	claudeName := "claude"
	if runtime.GOOS == "windows" {
		claudeName = "claude.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, claudeName), "real claude")

	var gotPath string
	var gotEnv []string
	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Args: []string{"--version"},
		Path: realDir,
		Runner: func(path string, args []string, env []string) error {
			gotPath, gotEnv = path, env
			return nil
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if !strings.Contains(gotPath, "claude") {
		t.Errorf("expected claude binary, got %q", gotPath)
	}
	if envValue(gotEnv, "CLAUDE_CODE_USE_BEDROCK") != "1" {
		t.Errorf("Claude default must set CLAUDE_CODE_USE_BEDROCK=1, env=%v", gotEnv)
	}
}
