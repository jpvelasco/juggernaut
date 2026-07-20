package activation

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func skipIf(t *testing.T, cond bool, msg string) {
	t.Helper()
	if cond {
		t.Skip(msg)
	}
}

func writeShellBlock(t *testing.T, home, filename string) string {
	t.Helper()
	path := filepath.Join(home, filename)
	block := Block(ShellPOSIX)
	if err := os.WriteFile(path, []byte(block), 0o600); err != nil {
		t.Fatalf("writeShellBlock(%s): %v", path, err)
	}
	return path
}

func createClaudeDir(t *testing.T, home string) string {
	t.Helper()
	d := filepath.Join(home, ".claude")
	if err := os.MkdirAll(d, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission — 0o700 is correct for directories, test under t.TempDir()
		t.Fatalf("createClaudeDir: %v", err)
	}
	return d
}

func mustMarshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func defaultLaunchTarget() LaunchTarget {
	return LaunchTarget{
		BinaryNames: []string{"test"},
		NeedsToken:  true,
	}
}

func assertEmptyActions(t *testing.T, name string, actions []LegacyAction) {
	t.Helper()
	if len(actions) != 0 {
		t.Errorf("%s: expected no actions in clean dir, got %d", name, len(actions))
	}
}

// ---------------------------------------------------------------------------
// Thin wrapper functions — 0% coverage because they delegate to the *With/*For
// variants which are already tested. These exercises confirm the delegation
// contracts and cover the lines themselves.
// ---------------------------------------------------------------------------

func TestWrapperDelegation(t *testing.T) {
	t.Run("Install", func(t *testing.T) {
		skipIf(t, runtime.GOOS == "windows", "requires POSIX targets")
		home := t.TempDir()
		bashrc := filepath.Join(home, ".bashrc")
		if err := os.WriteFile(bashrc, []byte("# existing"), 0o600); err != nil {
			t.Fatal(err)
		}
		installed, err := Install(home)
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
		found := false
		for _, p := range installed {
			if p == bashrc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in installed targets %v", bashrc, installed)
		}
	})

	t.Run("Uninstall", func(t *testing.T) {
		skipIf(t, runtime.GOOS == "windows", "requires POSIX targets")
		home := t.TempDir()
		bashrc := writeShellBlock(t, home, ".bashrc")
		removed, err := Uninstall(home)
		if err != nil {
			t.Fatalf("Uninstall: %v", err)
		}
		found := false
		for _, p := range removed {
			if p == bashrc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s in removed targets %v", bashrc, removed)
		}
	})

	t.Run("InstalledTargets", func(t *testing.T) {
		skipIf(t, runtime.GOOS == "windows", "requires POSIX targets")
		home := t.TempDir()
		bashrc := writeShellBlock(t, home, ".bashrc")
		paths := InstalledTargets(home)
		if len(paths) == 0 {
			t.Error("should find .bashrc with block")
		}
		if len(paths) > 0 && paths[0] != bashrc {
			t.Errorf("expected %s, got %v", bashrc, paths)
		}
	})

	t.Run("InstalledTargetsWith", func(t *testing.T) {
		skipIf(t, runtime.GOOS == "windows", "requires POSIX targets")
		home := t.TempDir()
		writeShellBlock(t, home, ".zshrc")
		paths := InstalledTargetsWith(home, nil)
		if len(paths) == 0 {
			t.Error("should find .zshrc with block")
		}
	})

	t.Run("Launch", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("PATH", "")
		if err := Launch(home, []string{}); err == nil {
			t.Error("expected error when no binary found")
		}
	})

	t.Run("ResolveClaudeBinary", func(t *testing.T) {
		if _, err := ResolveClaudeBinary("/nonexistent"); err == nil {
			t.Error("expected error for nonexistent path")
		}
	})

	t.Run("ResolveBinary", func(t *testing.T) {
		if _, err := ResolveBinary("/nonexistent", []string{"nonexistent"}); err == nil {
			t.Error("expected error for nonexistent binary")
		}
	})

	t.Run("ResolvePowerShellProfiles", func(t *testing.T) {
		skipIf(t, runtime.GOOS == "windows", "Windows requires real PowerShell discovery")
		r := ResolvePowerShellProfiles()
		if len(r.ActiveTargets) != 0 {
			t.Error("should return empty on non-Windows")
		}
	})
}

// ---------------------------------------------------------------------------
// Miscellaneous helpers — resolveBinaryFrom, DefaultBinDir, platformNames, etc.
// ---------------------------------------------------------------------------

func TestMiscHelpers(t *testing.T) {
	t.Run("ResolveBinaryFrom_SelfPathsSkip", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "test-bin")
		if runtime.GOOS == "windows" {
			binPath += ".exe"
		}
		if err := os.WriteFile(binPath, []byte("fake"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveBinaryFrom(dir, []string{filepath.Base(binPath)}, "", []string{binPath}); err == nil {
			t.Error("expected error when the only candidate is in SelfPaths")
		}
	})

	t.Run("DefaultBinDir_UserProfileFallback", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "/tmp/up-home")
		got := DefaultBinDir("")
		want := filepath.Join("/tmp/up-home", ".local", "bin")
		if got != want {
			t.Errorf("DefaultBinDir = %q, want %q", got, want)
		}
	})

	t.Run("PlatformNames", func(t *testing.T) {
		names := platformNames()
		wantClaude := "claude"
		if runtime.GOOS == "windows" {
			wantClaude = "claude.cmd"
		}
		if names.claude != wantClaude {
			t.Errorf("platformNames.claude = %q, want %q", names.claude, wantClaude)
		}
		if len(names.commandNames) == 0 {
			t.Error("commandNames should not be empty")
		}
	})

	t.Run("ClaudeLaunchTarget", func(t *testing.T) {
		tgt := claudeLaunchTarget()
		if len(tgt.BinaryNames) == 0 {
			t.Error("BinaryNames should not be empty")
		}
		if tgt.TokenEnvVar != bedrockAuthEnvName {
			t.Errorf("TokenEnvVar = %q, want %q", tgt.TokenEnvVar, bedrockAuthEnvName)
		}
		if tgt.StaticEnv["CLAUDE_CODE_USE_BEDROCK"] != "1" {
			t.Error("StaticEnv should contain CLAUDE_CODE_USE_BEDROCK=1")
		}
	})

	t.Run("DefaultTargets", func(t *testing.T) {
		home := "/home/user"
		targets := DefaultTargets(home)
		expected := map[string]Shell{
			filepath.Join(home, ".bashrc"):                        ShellPOSIX,
			filepath.Join(home, ".zshrc"):                         ShellPOSIX,
			filepath.Join(home, ".profile"):                       ShellPOSIX,
			filepath.Join(home, ".config", "fish", "config.fish"): ShellFish,
		}
		if len(targets) != len(expected) {
			t.Fatalf("expected %d targets, got %d", len(expected), len(targets))
		}
		for _, tgt := range targets {
			wantShell, ok := expected[tgt.Path]
			if !ok {
				t.Errorf("unexpected target path: %s", tgt.Path)
				continue
			}
			if tgt.Shell != wantShell {
				t.Errorf("shell for %s = %s, want %s", tgt.Path, tgt.Shell, wantShell)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// isKnownJuggernautArtifact — non-Windows symlink paths
// ---------------------------------------------------------------------------

func TestIsKnownJuggernautArtifact_NonWindows(t *testing.T) {
	skipIf(t, runtime.GOOS == "windows", "symlink test for non-Windows")

	tests := []struct {
		name     string
		setup    func(dir string) (link, self string)
		wantTrue bool
	}{
		{
			name: "symlink target differs from self",
			setup: func(dir string) (string, string) {
				target := filepath.Join(dir, "target")
				self := filepath.Join(dir, "self")
				link := filepath.Join(dir, "link")
				if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink not supported: %v", err)
				}
				return link, self
			},
			wantTrue: false,
		},
		{
			name: "empty self",
			setup: func(dir string) (string, string) {
				target := filepath.Join(dir, "target")
				link := filepath.Join(dir, "link")
				if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink not supported: %v", err)
				}
				return link, ""
			},
			wantTrue: false,
		},
		{
			name: "regular file not symlink",
			setup: func(dir string) (string, string) {
				path := filepath.Join(dir, "regular")
				if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
				return path, "/some/self"
			},
			wantTrue: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			link, self := tc.setup(dir)
			got := isKnownJuggernautArtifact(link, self)
			if got != tc.wantTrue {
				t.Errorf("isKnownJuggernautArtifact(%q, %q) = %v, want %v", link, self, got, tc.wantTrue)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Legacy artifact detection — clean directory (no artifacts expected)
// ---------------------------------------------------------------------------

func TestLegacyArtifactDetection_CleanDir(t *testing.T) {
	home := t.TempDir()
	assertEmptyActions(t, "DetectLegacyArtifacts", DetectLegacyArtifacts(home))

	skipIf(t, runtime.GOOS == "windows", "non-Windows only")
	actions, err := RecoverLegacyArtifacts(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEmptyActions(t, "RecoverLegacyArtifacts", actions)

	actions, err = recoverPlatformArtifacts(t.TempDir(), "", platformNames())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEmptyActions(t, "recoverPlatformArtifacts", actions)

	assertEmptyActions(t, "detectPlatformArtifacts", detectPlatformArtifacts(t.TempDir(), "", platformNames()))
}

// ---------------------------------------------------------------------------
// authModes — empty config path
// ---------------------------------------------------------------------------

func TestAuthModes_EmptyConfig(t *testing.T) {
	home := t.TempDir()
	claudeDir := createClaudeDir(t, home)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	modes, err := authModes(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(modes) != 0 {
		t.Errorf("expected no auth modes from empty config, got %v", modes)
	}
}

// ---------------------------------------------------------------------------
// LaunchWithOptions — IAM, expired key, and error paths
// ---------------------------------------------------------------------------

func TestLaunchWithOptions(t *testing.T) {
	t.Run("ManagedIAM_NoToken", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
		home := t.TempDir()
		claudeDir := createClaudeDir(t, home)
		settings := map[string]any{
			"juggernaut": map[string]any{
				"auth": map[string]any{"mode": "iam"},
				"meta": map[string]any{"managedBy": "juggernaut"},
			},
		}
		if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), mustMarshalJSON(settings), 0o600); err != nil {
			t.Fatal(err)
		}

		realDir := t.TempDir()
		claudeName := "claude"
		if runtime.GOOS == "windows" {
			claudeName = "claude.exe"
		}
		writeExecutableFile(t, realDir, filepath.Join(realDir, claudeName), "real claude")

		tokenCalled := false
		var gotEnv []string
		err := LaunchWithOptions(LaunchOptions{
			Home: home,
			Args: []string{"--version"},
			Path: realDir,
			TokenGetter: func() (string, error) {
				tokenCalled = true
				return "", nil
			},
			Runner: func(_ string, _ []string, env []string) error {
				gotEnv = env
				return nil
			},
			Target: LaunchTarget{
				BinaryNames: []string{claudeName},
				TokenEnvVar: bedrockAuthEnvName,
				StaticEnv:   map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"},
				NeedsToken:  false,
			},
		})
		if err != nil {
			t.Fatalf("launch: %v", err)
		}
		if tokenCalled {
			t.Error("IAM auth must NOT call TokenGetter")
		}
		if envValue(gotEnv, "CLAUDE_CODE_USE_BEDROCK") != "1" {
			t.Errorf("expected CLAUDE_CODE_USE_BEDROCK=1, env=%v", gotEnv)
		}
		if envValue(gotEnv, "AWS_BEARER_TOKEN_BEDROCK") != "" {
			t.Errorf("IAM auth must NOT inject bearer token, env=%v", gotEnv)
		}
	})

	t.Run("ExpiredKeyWarning", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		expiredURL := "https://bedrock.amazonaws.com/?Action=CallWithBearerToken" +
			"&X-Amz-Date=20200101T000000Z&X-Amz-Expires=3600"
		expiredKey := "bedrock-" + "api-key-" + base64.StdEncoding.EncodeToString([]byte(expiredURL))

		warned := ""
		err := LaunchWithOptions(LaunchOptions{
			Home: home,
			Path: "/nonexistent",
			Target: LaunchTarget{
				BinaryNames: []string{"test"},
				NeedsToken:  true,
			},
			TokenGetter: func() (string, error) {
				return expiredKey, nil
			},
			Warner: func(msg string) {
				warned = msg
			},
			Runner: func(path string, args []string, env []string) error {
				return nil
			},
		})
		_ = err
		// The 2020 key is permanently expired; the warning must fire.
		if warned == "" {
			t.Error("expected expiry warning for a 2020 key, but Warner was never called")
		} else if !strings.Contains(warned, "expired") {
			t.Errorf("warning should mention expired, got: %s", warned)
		}
	})

	t.Run("ErrorPaths", func(t *testing.T) {
		tests := []struct {
			name       string
			opts       LaunchOptions
			wantErr    bool
			errContain string
		}{
			{
				name:       "tokenGetter returns error",
				opts:       LaunchOptions{Home: "", Path: "/nonexistent", Target: defaultLaunchTarget(), TokenGetter: func() (string, error) { return "", os.ErrNotExist }},
				wantErr:    true,
				errContain: "reading Bedrock API key",
			},
			{
				name:       "tokenGetter returns empty token",
				opts:       LaunchOptions{Home: "", Path: "/nonexistent", Target: defaultLaunchTarget(), TokenGetter: func() (string, error) { return "", nil }},
				wantErr:    true,
				errContain: "not found in keychain",
			},
			{
				name:    "default Warner (no crash)",
				opts:    LaunchOptions{Home: "", Path: "/nonexistent", Target: LaunchTarget{BinaryNames: []string{"test"}, NeedsToken: false}},
				wantErr: true,
			},
			{
				name:    "default TokenGetter (no crash)",
				opts:    LaunchOptions{Home: "", Path: "/nonexistent", Target: defaultLaunchTarget()},
				wantErr: true,
			},
			{
				name:    "default Path from environment",
				opts:    LaunchOptions{Home: "", Path: "", Target: LaunchTarget{BinaryNames: []string{"this-binary-does-not-exist-xyz"}, NeedsToken: false}},
				wantErr: true,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				opts := tc.opts
				opts.Home = home

				err := LaunchWithOptions(opts)
				if tc.wantErr {
					if err == nil {
						t.Fatal("expected error")
					}
					if tc.errContain != "" && !strings.Contains(err.Error(), tc.errContain) {
						t.Errorf("expected error containing %q, got: %v", tc.errContain, err)
					}
				} else if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Non-Windows helper functions — consolidated subtests
// ---------------------------------------------------------------------------

func TestNonWindowsHelpers(t *testing.T) {
	skipIf(t, runtime.GOOS == "windows", "non-Windows only")

	t.Run("CheckPowerShellActivationWith", func(t *testing.T) {
		healthy, path, warnings := CheckPowerShellActivationWith(t.TempDir(), nil)
		if !healthy {
			t.Error("should be healthy on non-Windows")
		}
		if path != "" {
			t.Error("path should be empty on non-Windows")
		}
		if len(warnings) != 0 {
			t.Error("warnings should be empty on non-Windows")
		}
	})

	t.Run("profilePathKey", func(t *testing.T) {
		got := profilePathKey("/Users/test/profile.ps1")
		if got != "/Users/test/profile.ps1" {
			t.Errorf("profilePathKey = %q, want /Users/test/profile.ps1", got)
		}
	})

	t.Run("samePath", func(t *testing.T) {
		if !samePath("/a/b", "/a/b") {
			t.Error("same paths should match")
		}
		if samePath("/a/b", "/a/c") {
			t.Error("different paths should not match")
		}
	})

	t.Run("isLegacyClaudeShim", func(t *testing.T) {
		if isLegacyClaudeShim("/some/path") {
			t.Error("should return false on non-Windows")
		}
	})
}
