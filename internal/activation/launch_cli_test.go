package activation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLaunchWithOptions_CodexSpec: with a Codex LaunchTarget that has a config
// path containing an API-key auth mode, the launcher reads the auth mode from
// the config file's juggernaut block and injects the bearer token.
func TestLaunchWithOptions_CodexSpec(t *testing.T) {
	// Clear the ambient var: this test process runs inside Claude Code, which
	// sets CLAUDE_CODE_USE_BEDROCK in the real environment. We assert the Codex
	// launch does not itself set it.
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	home := t.TempDir()

	realDir := t.TempDir()
	codexName := "codex"
	if runtime.GOOS == "windows" {
		codexName = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, codexName), "real codex")

	// Write a config.toml with a juggernaut block that declares API key auth.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	cfgContent := `[juggernaut]
  [juggernaut.auth]
    mode = "bedrock-api-key"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
			NeedsToken:  false, // auth mode read from config
			ConfigPath:  cfgPath,
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if !strings.Contains(gotPath, "codex") {
		t.Errorf("expected codex binary resolved, got %q", gotPath)
	}
	// Config says bedrock-api-key → token injected
	if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "tok-123" {
		t.Errorf("expected bearer token injected, env=%v", gotEnv)
	}
	if envValue(gotEnv, "CLAUDE_CODE_USE_BEDROCK") != "" {
		t.Errorf("Codex must NOT set CLAUDE_CODE_USE_BEDROCK, env=%v", gotEnv)
	}
}

// TestLaunchWithOptions_CodexOnly_InjectsToken is the P1 regression: a Codex-only
// user has NO ~/.claude/settings.json, so authModes is empty. The token must
// still be injected because the config file's juggernaut block declares
// bedrock-api-key auth (the bearer token is shared).
func TestLaunchWithOptions_CodexOnly_InjectsToken(t *testing.T) {
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	home := t.TempDir() // no .claude settings written

	realDir := t.TempDir()
	codexName := "codex"
	if runtime.GOOS == "windows" {
		codexName = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, codexName), "real codex")

	// Write a config.toml with a juggernaut block that declares API key auth.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	cfgContent := `[juggernaut]
  [juggernaut.auth]
    mode = "bedrock-api-key"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var gotEnv []string
	err := LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        []string{"--version"},
		Path:        realDir,
		TokenGetter: func() (string, error) { return "codex-tok", nil },
		Runner: func(_ string, _ []string, env []string) error {
			gotEnv = env
			return nil
		},
		Target: LaunchTarget{
			BinaryNames: []string{codexName},
			TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
			NeedsToken:  false, // auth mode read from config
			ConfigPath:  cfgPath,
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "codex-tok" {
		t.Errorf("Codex-only launch must inject the shared token, env=%v", gotEnv)
	}
}

// TestLaunchWithOptions_Codex_IAM_NoToken: when the config file's juggernaut
// block declares IAM auth, the launcher must NOT inject a bearer token (IAM
// uses the AWS SDK credential chain, not a keychain token).
func TestLaunchWithOptions_Codex_IAM_NoToken(t *testing.T) {
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	home := t.TempDir()

	realDir := t.TempDir()
	codexName := "codex"
	if runtime.GOOS == "windows" {
		codexName = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, codexName), "real codex")

	// Write a config.toml with a juggernaut block that declares IAM auth.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	cfgContent := `[juggernaut]
  [juggernaut.auth]
    mode = "iam"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tokenCalled := false
	var gotEnv []string
	err := LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        []string{"--version"},
		Path:        realDir,
		TokenGetter: func() (string, error) { tokenCalled = true; return "", nil },
		Runner: func(_ string, _ []string, env []string) error {
			gotEnv = env
			return nil
		},
		Target: LaunchTarget{
			BinaryNames: []string{codexName},
			TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
			NeedsToken:  false,
			ConfigPath:  cfgPath,
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if tokenCalled {
		t.Error("IAM auth must NOT call TokenGetter")
	}
	if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "" {
		t.Errorf("IAM auth must NOT inject bearer token, env=%v", gotEnv)
	}
}

// TestReadAuthModeFromConfig covers the error paths of readAuthModeFromConfig:
// missing file, no juggernaut block, wrong managedBy, missing auth, JSON format.
func TestReadAuthModeFromConfig(t *testing.T) {
	temp := t.TempDir()

	// Non-existent file returns empty string
	path := filepath.Join(temp, "nope.toml")
	if got := readAuthModeFromConfig(path); got != "" {
		t.Errorf("non-existent file: got %q, want \"\"", got)
	}

	// File without juggernaut block
	path = filepath.Join(temp, "plain.toml")
	if err := os.WriteFile(path, []byte("foo = \"bar\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAuthModeFromConfig(path); got != "" {
		t.Errorf("no juggernaut block: got %q, want \"\"", got)
	}

	// Juggernaut block but wrong managedBy
	path = filepath.Join(temp, "other.toml")
	content := `[juggernaut]
  [juggernaut.auth]
    mode = "iam"
  [juggernaut.meta]
    managedBy = "other"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAuthModeFromConfig(path); got != "" {
		t.Errorf("wrong managedBy: got %q, want \"\"", got)
	}

	// Juggernaut block without auth sub-table
	path = filepath.Join(temp, "noauth.toml")
	content = `[juggernaut]
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAuthModeFromConfig(path); got != "" {
		t.Errorf("no auth block: got %q, want \"\"", got)
	}

	// Valid IAM auth mode
	path = filepath.Join(temp, "iam.toml")
	content = `[juggernaut]
  [juggernaut.auth]
    mode = "iam"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAuthModeFromConfig(path); got != "iam" {
		t.Errorf("iam mode: got %q, want \"iam\"", got)
	}

	// Valid API key auth mode
	path = filepath.Join(temp, "apikey.toml")
	content = `[juggernaut]
  [juggernaut.auth]
    mode = "bedrock-api-key"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAuthModeFromConfig(path); got != "bedrock-api-key" {
		t.Errorf("apikey mode: got %q, want \"bedrock-api-key\"", got)
	}

	// JSON format — valid API key auth
	path = filepath.Join(temp, "config.json")
	content = `{"juggernaut":{"auth":{"mode":"bedrock-api-key"},"meta":{"managedBy":"juggernaut"}}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readAuthModeFromConfig(path); got != "bedrock-api-key" {
		t.Errorf("json apikey: got %q, want \"bedrock-api-key\"", got)
	}
}

// TestLaunchWithOptions_Codex_NoConfigFile: when ConfigPath points to a
// nonexistent file, the launcher falls through to unmanaged behavior (no
// token injected, no static env).
func TestLaunchWithOptions_Codex_NoConfigFile(t *testing.T) {
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	home := t.TempDir()

	realDir := t.TempDir()
	codexName := "codex"
	if runtime.GOOS == "windows" {
		codexName = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, codexName), "real codex")

	tokenCalled := false
	var gotEnv []string
	err := LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        []string{"--version"},
		Path:        realDir,
		TokenGetter: func() (string, error) { tokenCalled = true; return "", nil },
		Runner: func(_ string, _ []string, env []string) error {
			gotEnv = env
			return nil
		},
		Target: LaunchTarget{
			BinaryNames: []string{codexName},
			TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
			NeedsToken:  false,
			ConfigPath:  filepath.Join(home, "nonexistent", "config.toml"),
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if tokenCalled {
		t.Error("no config file must NOT call TokenGetter")
	}
	if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "" {
		t.Errorf("no config file must NOT inject token, env=%v", gotEnv)
	}
}

// TestLaunchWithOptions_Codex_BadConfigFile: when ConfigPath points to a
// config file without a juggernaut block, the launcher falls through to
// unmanaged behavior (no token injected).
func TestLaunchWithOptions_Codex_BadConfigFile(t *testing.T) {
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	home := t.TempDir()

	realDir := t.TempDir()
	codexName := "codex"
	if runtime.GOOS == "windows" {
		codexName = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, codexName), "real codex")

	cfgPath := filepath.Join(home, "plain.toml")
	if err := os.WriteFile(cfgPath, []byte("foo = \"bar\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	tokenCalled := false
	var gotEnv []string
	err := LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        []string{"--version"},
		Path:        realDir,
		TokenGetter: func() (string, error) { tokenCalled = true; return "", nil },
		Runner: func(_ string, _ []string, env []string) error {
			gotEnv = env
			return nil
		},
		Target: LaunchTarget{
			BinaryNames: []string{codexName},
			TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
			NeedsToken:  false,
			ConfigPath:  cfgPath,
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if tokenCalled {
		t.Error("no juggernaut block must NOT call TokenGetter")
	}
	if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "" {
		t.Errorf("no juggernaut block must NOT inject token, env=%v", gotEnv)
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
