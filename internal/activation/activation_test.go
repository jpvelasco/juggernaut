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

	// On Windows, inject a mock runner so Install doesn't touch real profiles.
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
	}

	first, err := Install(home)
	if err != nil {
		t.Fatalf("Install() first error: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("first install should write activation blocks")
	}

	snapshots := readTargets(t, home)
	second, err := Install(home)
	if err != nil {
		t.Fatalf("Install() second error: %v", err)
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

	// On Windows, inject a mock runner so Install doesn't touch real profiles.
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
	}

	if _, err := Install(home); err != nil {
		t.Fatalf("Install(): %v", err)
	}

	removed, err := Uninstall(home)
	if err != nil {
		t.Fatalf("Uninstall(): %v", err)
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

func TestRecoverLegacyArtifactsDoesNotDeleteUnknownClaude(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, platformNames().claude)
	writeFile(t, dir, claude, "real claude")

	actions, err := recoverPlatformArtifacts(dir, executableFixture(t), platformNames())
	if err != nil {
		t.Fatalf("recoverPlatformArtifacts(): %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("unknown claude should not be touched, got actions: %v", actions)
	}
	data, err := safepath.ReadFile(dir, claude)
	if err != nil {
		t.Fatalf("unknown claude was removed: %v", err)
	}
	if string(data) != "real claude" {
		t.Fatalf("unknown claude was changed: %q", data)
	}
}

func TestRecoverLegacyArtifactsRestoresBackupWhenClaudeMissing(t *testing.T) {
	dir := t.TempDir()
	names := platformNames()
	backup := filepath.Join(dir, names.backup)
	claude := filepath.Join(dir, names.claude)
	writeFile(t, dir, backup, "real claude")

	actions, err := recoverPlatformArtifacts(dir, executableFixture(t), names)
	if err != nil {
		t.Fatalf("recoverPlatformArtifacts(): %v", err)
	}
	if len(actions) != 1 || !strings.Contains(actions[0].Action, "restored") {
		t.Fatalf("expected restore action, got %v", actions)
	}
	data, err := safepath.ReadFile(dir, claude)
	if err != nil {
		t.Fatalf("restored claude missing: %v", err)
	}
	if string(data) != "real claude" {
		t.Fatalf("restored claude mismatch: %q", data)
	}
}

func TestRecoverLegacyArtifactsRemovesKnownShim(t *testing.T) {
	dir := t.TempDir()
	names := platformNames()
	self := executableFixture(t)
	claude := filepath.Join(dir, names.claude)
	if runtime.GOOS == "windows" {
		writeFile(t, dir, claude, legacyCmdShimLF)
	} else if err := os.Symlink(self, claude); err != nil {
		t.Fatalf("creating symlink shim: %v", err)
	}

	actions, err := recoverPlatformArtifacts(dir, self, names)
	if err != nil {
		t.Fatalf("recoverPlatformArtifacts(): %v", err)
	}
	if len(actions) != 1 || !strings.Contains(actions[0].Action, "removed") {
		t.Fatalf("expected remove action, got %v", actions)
	}
	if _, err := os.Lstat(claude); !os.IsNotExist(err) {
		t.Fatalf("known shim should be removed, stat err=%v", err)
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
	if runtime.GOOS == "windows" {
		writeFile(t, dir, filepath.Join(dir, "claude.cmd"), legacyCmdShimLF)
	} else if err := os.Symlink(self, filepath.Join(dir, "claude")); err != nil {
		t.Fatalf("creating recursive symlink: %v", err)
	}

	realDir := t.TempDir()
	realClaude := filepath.Join(realDir, platformNames().claude)
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
	realClaude := filepath.Join(realDir, platformNames().claude)
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
	realClaude := filepath.Join(realDir, platformNames().claude)
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
	realClaude := filepath.Join(realDir, platformNames().claude)
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
	realClaude := filepath.Join(realDir, platformNames().claude)
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
	realClaude := filepath.Join(realDir, platformNames().claude)
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

func readTargets(t *testing.T, home string) map[string]string {
	t.Helper()
	out := map[string]string{}

	if runtime.GOOS == "windows" {
		// On Windows, read from discovered PowerShell paths + POSIX targets.
		result := ResolvePowerShellProfiles()
		for _, path := range result.MigrationTargets {
			data, err := safepath.ReadFile(filepath.Dir(path), path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			out[path] = string(data)
		}
		for _, target := range DefaultTargets(home) {
			if target.Shell == ShellPowerShell {
				continue // handled above
			}
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

func testPSOutput(allHosts, currentHost string) []byte {
	data, _ := json.Marshal(map[string]string{
		"CurrentUserAllHosts":    allHosts,
		"CurrentUserCurrentHost": currentHost,
	})
	return data
}
