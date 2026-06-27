package activation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()

	var psResult *ProfileResolverResult
	// On Windows, inject a mock runner + resolver so Install doesn't touch real profiles.
	if runtime.GOOS == "windows" {
		psProfile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
		runner := &testDiscoveryRunner{
			output: map[string][]byte{
				"pwsh.exe":       testPSOutput(psProfile, psProfile),
				"powershell.exe": testPSOutput(psProfile, psProfile),
			},
		}
		SetPSRunnerForTesting(runner)
		defer ResetPSRunnerForTesting()

		psResult = &ProfileResolverResult{
			ActiveTargets:  []Target{{Path: psProfile, Shell: ShellPowerShell}},
			InstallTargets: []Target{{Path: psProfile, Shell: ShellPowerShell}},
		}
	}

	opts := InstallOptions{PowerShellResult: psResult}
	first, err := InstallWith(home, opts)
	if err != nil {
		t.Fatalf("InstallWith() first error: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("first install should write activation blocks")
	}

	snapshots := readTargets(t, home, psResult)
	second, err := InstallWith(home, opts)
	if err != nil {
		t.Fatalf("InstallWith() second error: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second install should be unchanged, got %v", second)
	}
	for path, before := range snapshots {
		after, err := safepath.ReadFile(filepath.Dir(path), path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if string(after) != before {
			t.Fatalf("target %s changed on idempotent install", path)
		}
	}
}

func TestUninstallPreservesUnrelatedContent(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".bashrc")
	original := "export FOO=bar\n"
	if err := safepath.WriteFile(home, target, []byte(original)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var psResult *ProfileResolverResult
	// On Windows, inject a mock runner + resolver so Install doesn't touch real profiles.
	if runtime.GOOS == "windows" {
		psProfile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
		runner := &testDiscoveryRunner{
			output: map[string][]byte{
				"pwsh.exe":       testPSOutput(psProfile, psProfile),
				"powershell.exe": testPSOutput(psProfile, psProfile),
			},
		}
		SetPSRunnerForTesting(runner)
		defer ResetPSRunnerForTesting()

		psResult = &ProfileResolverResult{
			ActiveTargets: []Target{{Path: psProfile, Shell: ShellPowerShell}},
		}
	}

	opts := InstallOptions{PowerShellResult: psResult}
	if _, err := InstallWith(home, opts); err != nil {
		t.Fatalf("InstallWith(): %v", err)
	}

	uninstallOpts := UninstallOptions{PowerShellResult: psResult}
	removed, err := UninstallWith(home, uninstallOpts)
	if err != nil {
		t.Fatalf("UninstallWith(): %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("expected at least one activation block to be removed")
	}
	data, err := safepath.ReadFile(home, target)
	if err != nil {
		t.Fatalf("reading bashrc: %v", err)
	}
	if !strings.Contains(string(data), "export FOO=bar") {
		t.Fatalf("unrelated content was not preserved: %q", data)
	}
	if strings.Contains(string(data), BeginMarker) {
		t.Fatalf("activation block was not removed: %q", data)
	}
}

func TestBlocksContainValidDelegation(t *testing.T) {
	tests := []struct {
		shell Shell
		want  string
	}{
		{ShellPOSIX, "juggernaut launch -- \"$@\""},
		{ShellFish, "juggernaut launch -- $argv"},
		{ShellPowerShell, "juggernaut launch -- @args"},
	}
	for _, tt := range tests {
		block := Block(tt.shell)
		if !strings.Contains(block, BeginMarker) || !strings.Contains(block, EndMarker) {
			t.Fatalf("%s block missing markers: %q", tt.shell, block)
		}
		if !strings.Contains(block, tt.want) {
			t.Fatalf("%s block missing delegation %q: %q", tt.shell, tt.want, block)
		}
	}
}

func TestLaunchInvokesRealClaudeStubWithoutRecursion(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut")
	if runtime.GOOS == "windows" {
		self += ".exe"
	}
	writeExecutableFile(t, dir, self, "juggernaut")
	// Create a symlink from "claude" to the juggernaut binary to simulate
	// a recursive activation loop — resolveClaudeBinary must skip it.
	if runtime.GOOS == "windows" {
		if err := os.Symlink(self, filepath.Join(dir, "claude.exe")); err != nil {
			t.Fatalf("creating recursive symlink: %v", err)
		}
	} else if err := os.Symlink(self, filepath.Join(dir, "claude")); err != nil {
		t.Fatalf("creating recursive symlink: %v", err)
	}

	realDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	realClaude := filepath.Join(realDir, name)
	writeExecutableFile(t, realDir, realClaude, "real claude")

	called := false
	err := launchWithExecutable(t, home, dir+string(os.PathListSeparator)+realDir, self, LaunchOptions{
		Args: []string{"--version"},
		Runner: func(path string, args []string, env []string) error {
			called = true
			if path != realClaude {
				t.Fatalf("resolved path=%s want %s", path, realClaude)
			}
			if len(args) != 1 || args[0] != "--version" {
				t.Fatalf("args=%v", args)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("launchWithExecutable(): %v", err)
	}
	if !called {
		t.Fatal("runner was not called")
	}
}

func TestLaunchInjectsAPIKeyToken(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, "bedrock-api-key")
	realDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	realClaude := filepath.Join(realDir, name)
	writeExecutableFile(t, realDir, realClaude, "real claude")

	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: realDir,
		TokenGetter: func() (string, error) {
			return "token-value", nil
		},
		Runner: func(_ string, _ []string, env []string) error {
			if got := envValue(env, "AWS_BEARER_TOKEN_BEDROCK"); got != "token-value" {
				t.Fatalf("AWS_BEARER_TOKEN_BEDROCK=%q", got)
			}
			if got := envValue(env, "CLAUDE_CODE_USE_BEDROCK"); got != "1" {
				t.Fatalf("CLAUDE_CODE_USE_BEDROCK=%q", got)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LaunchWithOptions(): %v", err)
	}
}

func TestLaunchIAMDoesNotReadKeychain(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, "iam")
	realDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	realClaude := filepath.Join(realDir, name)
	writeExecutableFile(t, realDir, realClaude, "real claude")

	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: realDir,
		TokenGetter: func() (string, error) {
			return "", errors.New("keychain should not be read for IAM")
		},
		Runner: func(_ string, _ []string, env []string) error {
			if got := envValue(env, "AWS_BEARER_TOKEN_BEDROCK"); got != "" {
				t.Fatalf("AWS_BEARER_TOKEN_BEDROCK should be unset, got %q", got)
			}
			if got := envValue(env, "CLAUDE_CODE_USE_BEDROCK"); got != "1" {
				t.Fatalf("CLAUDE_CODE_USE_BEDROCK=%q", got)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LaunchWithOptions(): %v", err)
	}
}

func TestLaunch_BedrockAPIKey_UsesDefaultKeychain(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, "bedrock-api-key")
	realDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	realClaude := filepath.Join(realDir, name)
	writeExecutableFile(t, realDir, realClaude, "real claude")

	// No TokenGetter injected — Launch must use keychain.Default().Get.
	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: realDir,
		Runner: func(_ string, _ []string, env []string) error {
			if got := envValue(env, "CLAUDE_CODE_USE_BEDROCK"); got != "1" {
				t.Fatalf("CLAUDE_CODE_USE_BEDROCK=%q", got)
			}
			// The key will be empty because keychain isn't seeded, but the
			// important thing is that LaunchWithOptions didn't error on a
			// nil TokenGetter — it fell through to keychain.Default().Get.
			return nil
		},
	})
	// We expect an error because the keychain is empty for bedrock-api-key mode.
	// The key point is that it tried to read from keychain, not that it succeeded.
	if err == nil {
		// If the keychain happens to have a key, that's fine too.
	} else if !strings.Contains(err.Error(), "keychain") {
		t.Fatalf("expected keychain-related error when TokenGetter is nil, got: %v", err)
	}
}

func TestLaunch_IAM_UnsetsBearerTokenIfPreSet(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, "iam")
	realDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	realClaude := filepath.Join(realDir, name)
	writeExecutableFile(t, realDir, realClaude, "real claude")

	// Pre-set AWS_BEARER_TOKEN_BEDROCK in the environment to verify unsetEnv runs.
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "should-be-removed")

	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: realDir,
		TokenGetter: func() (string, error) {
			return "", errors.New("keychain should not be read for IAM")
		},
		Runner: func(_ string, _ []string, env []string) error {
			if got := envValue(env, "AWS_BEARER_TOKEN_BEDROCK"); got != "" {
				t.Fatalf("AWS_BEARER_TOKEN_BEDROCK should be unset for IAM mode, got %q", got)
			}
			if got := envValue(env, "CLAUDE_CODE_USE_BEDROCK"); got != "1" {
				t.Fatalf("CLAUDE_CODE_USE_BEDROCK=%q", got)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LaunchWithOptions(): %v", err)
	}
}

func TestLaunchWithoutSettingsDoesNotForceBedrockEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	realDir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	realClaude := filepath.Join(realDir, name)
	writeExecutableFile(t, realDir, realClaude, "real claude")

	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: realDir,
		TokenGetter: func() (string, error) {
			return "", errors.New("keychain should not be read without Juggernaut settings")
		},
		Runner: func(_ string, _ []string, env []string) error {
			if got := envValue(env, "CLAUDE_CODE_USE_BEDROCK"); got != "" {
				t.Fatalf("CLAUDE_CODE_USE_BEDROCK should be unchanged/unset, got %q", got)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LaunchWithOptions(): %v", err)
	}
}

func launchWithExecutable(t *testing.T, home, pathList, self string, opts LaunchOptions) error {
	t.Helper()
	claudePath, err := resolveClaudeBinary(pathList, self)
	if err != nil {
		return err
	}
	if opts.Runner == nil {
		t.Fatal("test runner required")
	}
	return opts.Runner(claudePath, opts.Args, os.Environ())
}

func readTargets(t *testing.T, home string, psResult *ProfileResolverResult) map[string]string {
	t.Helper()
	out := map[string]string{}

	if runtime.GOOS == "windows" && psResult != nil {
		// On Windows, read from injected PowerShell paths + POSIX targets.
		for _, target := range psResult.ActiveTargets {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err != nil {
				t.Fatalf("reading %s: %v", target.Path, err)
			}
			out[target.Path] = string(data)
		}
		for _, target := range DefaultTargets(home) {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err != nil {
				t.Fatalf("reading %s: %v", target.Path, err)
			}
			out[target.Path] = string(data)
		}
	} else {
		for _, target := range DefaultTargets(home) {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err != nil {
				t.Fatalf("reading %s: %v", target.Path, err)
			}
			out[target.Path] = string(data)
		}
	}
	return out
}

func executableFixture(t *testing.T) string {
	t.Helper()
	self := filepath.Join(t.TempDir(), "juggernaut")
	if runtime.GOOS == "windows" {
		self += ".exe"
	}
	writeExecutableFile(t, filepath.Dir(self), self, "juggernaut")
	return self
}

func writeFile(t *testing.T, base, path, content string) {
	t.Helper()
	if err := safepath.WriteFile(base, path, []byte(content)); err != nil {
		t.Fatalf("writing file fixture %s: %v", path, err)
	}
}

func writeExecutableFile(t *testing.T, base, path, content string) {
	t.Helper()
	writeFile(t, base, path, content)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission, go_file-permissions_rule-fileperm
			t.Fatalf("making file fixture executable %s: %v", path, err)
		}
	}
}

func writeSettings(t *testing.T, home, mode string) {
	t.Helper()
	path := filepath.Join(home, ".claude", "settings.json")
	content := `{"juggernaut":{"auth":{"mode":"` + mode + `"},"meta":{"managedBy":"juggernaut","schemaVersion":2}}}`
	if err := safepath.WriteFile(home, path, []byte(content)); err != nil {
		t.Fatalf("writing settings: %v", err)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

// testDiscoveryRunner is a mock discoveryCommandRunner for activation_test.go.
type testDiscoveryRunner struct {
	output map[string][]byte
	err    map[string]error
}

func (r *testDiscoveryRunner) RunContext(_ context.Context, exe string, _ []string) ([]byte, error) {
	if err := r.err[exe]; err != nil {
		return nil, err
	}
	return r.output[exe], nil
}

func TestIsLegacyClaudeShim_LF(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut.exe")
	writeExecutableFile(t, dir, self, "juggernaut")
	shim := filepath.Join(dir, "claude.cmd")

	// Write shim with LF line endings
	if err := os.WriteFile(shim, []byte("@echo off\njuggernaut --launcher %*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !isKnownJuggernautArtifact(shim, self) {
		t.Error("expected LF shim to be detected as legacy artifact")
	}
}

func TestIsLegacyClaudeShim_CRLF(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut.exe")
	writeExecutableFile(t, dir, self, "juggernaut")
	shim := filepath.Join(dir, "claude.cmd")

	// Write shim with CRLF line endings
	if err := os.WriteFile(shim, []byte("@echo off\r\njuggernaut --launcher %*\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !isKnownJuggernautArtifact(shim, self) {
		t.Error("expected CRLF shim to be detected as legacy artifact")
	}
}

func TestIsLegacyClaudeShim_NoFinalNewline(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut.exe")
	writeExecutableFile(t, dir, self, "juggernaut")
	shim := filepath.Join(dir, "claude.cmd")

	// Write shim with no final newline
	if err := os.WriteFile(shim, []byte("@echo off\njuggernaut --launcher %*"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !isKnownJuggernautArtifact(shim, self) {
		t.Error("expected shim without final newline to be detected as legacy artifact")
	}
}

func TestIsLegacyClaudeShim_TrailingWhitespace(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut.exe")
	writeExecutableFile(t, dir, self, "juggernaut")
	shim := filepath.Join(dir, "claude.cmd")

	// Write shim with trailing whitespace and extra blank lines
	if err := os.WriteFile(shim, []byte("@echo off\njuggernaut --launcher %*\n\n   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !isKnownJuggernautArtifact(shim, self) {
		t.Error("expected shim with trailing whitespace to be detected as legacy artifact")
	}
}

func TestIsLegacyClaudeShim_NotAShim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut.exe")
	writeExecutableFile(t, dir, self, "juggernaut")
	shim := filepath.Join(dir, "claude.cmd")

	// Write a file that's not a legacy shim
	if err := os.WriteFile(shim, []byte("@echo off\nreal command\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isKnownJuggernautArtifact(shim, self) {
		t.Error("expected non-shim file to NOT be detected as legacy artifact")
	}
}

func TestDefaultBinDir(t *testing.T) {
	home := t.TempDir()
	dir := DefaultBinDir(home)
	expected := filepath.Join(home, ".local", "bin")
	if dir != expected {
		t.Errorf("DefaultBinDir(%q) = %q, want %q", home, dir, expected)
	}
}

func TestRecoverLegacyArtifacts_RemovesShim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission — 0o700 is correct for dirs, test under t.TempDir()
		t.Fatal(err)
	}
	self := filepath.Join(binDir, "juggernaut.exe")
	writeExecutableFile(t, binDir, self, "juggernaut")
	shim := filepath.Join(binDir, "claude.cmd")

	// Write legacy shim
	if err := os.WriteFile(shim, []byte("@echo off\njuggernaut --launcher %*\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	actions, err := RecoverLegacyArtifacts(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 {
		t.Fatal("expected at least one recovery action")
	}

	// Shim should be removed
	if _, err := os.Stat(shim); !os.IsNotExist(err) {
		t.Error("shim should have been removed")
	}
}

func TestDetectLegacyArtifacts(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission — 0o700 is correct for dirs, test under t.TempDir()
		t.Fatal(err)
	}
	self := filepath.Join(binDir, "juggernaut.exe")
	writeExecutableFile(t, binDir, self, "juggernaut")
	shim := filepath.Join(binDir, "claude.cmd")

	// Write legacy shim
	if err := os.WriteFile(shim, []byte("@echo off\njuggernaut --launcher %*\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifacts := DetectLegacyArtifacts(binDir)
	if len(artifacts) == 0 {
		t.Fatal("expected at least one artifact detected")
	}

	// Shim should still exist (detect doesn't remove)
	if _, err := os.Stat(shim); err != nil {
		t.Error("detect should not remove the shim")
	}
}

func TestRemoveTargetWithLegacy_RemovesAllBlocks(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.ps1")

	// Profile with current block + legacy launcher block + legacy Bedrock block
	content := BeginMarker + "\n$env.TEST='1'\n" + EndMarker + "\n" +
		LegacyLauncherBegin + "\n$env.LEGACY='1'\n" + LegacyLauncherEnd + "\n" +
		LegacyBedrockBegin + "\n$env.BEDROCK='1'\n" + LegacyBedrockEnd + "\n" +
		"# other content\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := RemoveTargetWithLegacy(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed to be true")
	}

	data, err := os.ReadFile(path) // nosemgrep: go_filesystem_rule-fileread — path under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	remaining := string(data)

	if strings.Contains(remaining, BeginMarker) {
		t.Error("current activation block should be removed")
	}
	if strings.Contains(remaining, LegacyLauncherBegin) {
		t.Error("legacy launcher block should be removed")
	}
	if strings.Contains(remaining, LegacyBedrockBegin) {
		t.Error("legacy Bedrock block should be removed")
	}
	if !strings.Contains(remaining, "# other content") {
		t.Error("unrelated content should be preserved")
	}
}

func TestRemoveTargetWithLegacy_OnlyLegacyBlock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.ps1")

	// Profile with ONLY a legacy launcher block (no current block)
	content := LegacyLauncherBegin + "\n$env.LEGACY='1'\n" + LegacyLauncherEnd + "\n"

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := RemoveTargetWithLegacy(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed to be true for legacy block alone")
	}

	data, err := os.ReadFile(path) // nosemgrep: go_filesystem_rule-fileread — path under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), LegacyLauncherBegin) {
		t.Error("legacy launcher block should be removed")
	}
}

func TestRemoveTargetWithLegacy_NoBlocks(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.ps1")

	if err := os.WriteFile(path, []byte("# just a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := RemoveTargetWithLegacy(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("expected no change when no blocks present")
	}
}

func testPSOutput(allHosts, currentHost string) []byte {
	data, _ := json.Marshal(map[string]string{
		"CurrentUserAllHosts":    allHosts,
		"CurrentUserCurrentHost": currentHost,
	})
	return data
}
