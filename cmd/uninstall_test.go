package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// autoModeCapableStub wraps stubProvider (from apply_test.go) to report
// CapAutoMode support, so warnIfAutoModeWillBeLost's error branches — which
// are only reached past the CapAutoMode guard — are actually exercised.
type autoModeCapableStub struct{ stubProvider }

func (autoModeCapableStub) Supports(c provider.Capability) bool { return c == provider.CapAutoMode }
func (autoModeCapableStub) DisplayName() string                 { return "AutoMode" }

// TestUninstall_DryRun_PreservesBlock verifies that a dry-run reports intended
// removals but leaves settings.json untouched and never prints completion.
func TestUninstall_DryRun_PreservesBlock(t *testing.T) {
	home := setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--full", "--dry-run"}); err != nil {
			t.Fatalf("uninstall --dry-run error: %v", err)
		}
	})

	if !strings.Contains(out, "Would remove juggernaut block") {
		t.Errorf("dry-run should preview settings removal, got:\n%s", out)
	}
	if !strings.Contains(out, "Would remove Juggernaut Claude activation") {
		t.Errorf("dry-run --full should preview activation removal, got:\n%s", out)
	}
	if strings.Contains(out, "Uninstall complete") {
		t.Errorf("dry-run must not print completion, got:\n%s", out)
	}

	// The juggernaut block must still be present after a dry-run.
	data := readSettingsJSON(t, home)
	settings, err := testutil.ParseJSON(data)
	if err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; !ok {
		t.Error("dry-run removed the juggernaut block; it should be preserved")
	}
	if _, found, err := activation.LoadRuntimeState(home, "claude"); err != nil || !found {
		t.Errorf("dry-run removed runtime fallback: found %v, err %v", found, err)
	}
}

// TestUninstall_ScopeFilter verifies --scope=user only touches the user-scope
// settings and prints a scoped removal message.
func TestUninstall_ScopeFilter(t *testing.T) {
	home := setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--scope=user", "--force"}); err != nil {
			t.Fatalf("uninstall --scope=user error: %v", err)
		}
	})

	if !strings.Contains(out, "user claude config") {
		t.Errorf("expected scoped removal message for user scope, got:\n%s", out)
	}

	data := readSettingsJSON(t, home)
	settings, err := testutil.ParseJSON(data)
	if err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; ok {
		t.Error("user-scope juggernaut block should have been removed")
	}
	if _, found, err := activation.LoadRuntimeState(home, "claude"); err != nil || found {
		t.Errorf("user uninstall left runtime fallback: found %v, err %v", found, err)
	}
}

// TestUninstall_ConfirmYes exercises the interactive prompt accepting "y".
func TestUninstall_ConfirmYes(t *testing.T) {
	home := setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	var out string
	withStdin(t, "y\n", func() {
		out = captureStdout(t, func() {
			if err := ExecuteArgs([]string{"uninstall"}); err != nil {
				t.Fatalf("uninstall error: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Uninstall complete") {
		t.Errorf("confirming with 'y' should complete uninstall, got:\n%s", out)
	}

	data := readSettingsJSON(t, home)
	settings, err := testutil.ParseJSON(data)
	if err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; ok {
		t.Error("confirming uninstall should have removed the juggernaut block")
	}
}

// TestUninstall_AbortPaths covers the two ways the confirmation prompt can
// decline: an explicit "n" and a closed stdin (EOF on the first Scan, e.g.
// Ctrl+D or a closed pipe). Both must abort without completing and preserve
// the juggernaut block.
func TestUninstall_AbortPaths(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		wantErrMsg string
	}{
		{name: "declined", stdin: "n\n", wantErrMsg: "declining the prompt should abort"},
		{name: "eof", stdin: "", wantErrMsg: "EOF on the prompt should abort"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := setupApplyTest(t)

			if err := ExecuteArgs([]string{
				"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
			}); err != nil {
				t.Fatalf("apply error: %v", err)
			}

			var out string
			withStdin(t, tt.stdin, func() {
				out = captureStdout(t, func() {
					if err := ExecuteArgs([]string{"uninstall"}); err != nil {
						t.Fatalf("uninstall error: %v", err)
					}
				})
			})

			if !strings.Contains(out, "Aborted") {
				t.Errorf("%s, got:\n%s", tt.wantErrMsg, out)
			}
			if strings.Contains(out, "Uninstall complete") {
				t.Errorf("aborted uninstall must not complete, got:\n%s", out)
			}

			// Block must survive an aborted uninstall.
			data := readSettingsJSON(t, home)
			settings, err := testutil.ParseJSON(data)
			if err != nil {
				t.Fatalf("parsing settings.json: %v", err)
			}
			if _, ok := settings["juggernaut"]; !ok {
				t.Error("aborted uninstall should preserve the juggernaut block")
			}
		})
	}
}

// TestUninstall_WarnsWhenAutoModeWillBeLost reproduces the real-world
// incident: apply --mode=auto leaves the config in auto mode, and uninstall
// wipes that mode with nothing left for a future apply to restore it from.
// Uninstall must warn about this before removing anything.
func TestUninstall_WarnsWhenAutoModeWillBeLost(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=auto", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	if !strings.Contains(out, "Auto mode is currently enabled") {
		t.Errorf("expected auto-mode-loss warning before uninstall, got:\n%s", out)
	}
}

// TestUninstall_NoAutoModeWarningWhenNotAuto verifies the warning is silent
// when permission mode was never auto — no false positives on every uninstall.
func TestUninstall_NoAutoModeWarningWhenNotAuto(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	if strings.Contains(out, "Auto mode is currently enabled") {
		t.Errorf("did not expect auto-mode-loss warning when mode was never auto, got:\n%s", out)
	}
}

// TestWarnIfAutoModeWillBeLost_ManagerErrorSkipped covers the newProviderManager
// error branch: a provider whose ConfigPath fails must be skipped (continue),
// not treated as a crash or a false "auto mode found" positive.
func TestWarnIfAutoModeWillBeLost_ManagerErrorSkipped(t *testing.T) {
	home := testutil.NewTestHome(t)
	prov := autoModeCapableStub{stubProvider{formatName: "json", pathErr: fmt.Errorf("bad path")}}

	out := captureStdout(t, func() {
		warnIfAutoModeWillBeLost(home, prov)
	})

	if strings.Contains(out, "Auto mode is currently enabled") {
		t.Errorf("a ConfigPath error must not be reported as auto mode found, got:\n%s", out)
	}
}

// TestWarnIfAutoModeWillBeLost_ReadErrorSkipped covers the mgr.Read() error
// branch: a config path pointing at a directory (unreadable as a file) must
// be skipped (continue), not crash or falsely report auto mode.
func TestWarnIfAutoModeWillBeLost_ReadErrorSkipped(t *testing.T) {
	home := testutil.NewTestHome(t)
	dir := filepath.Join(home, "isadir")
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dp := dirPathProvider{stubProvider: stubProvider{formatName: "json"}, dir: dir}

	out := captureStdout(t, func() {
		warnIfAutoModeWillBeLost(home, autoModeReadErrorStub{dp})
	})

	if strings.Contains(out, "Auto mode is currently enabled") {
		t.Errorf("a Read() error must not be reported as auto mode found, got:\n%s", out)
	}
}

// autoModeReadErrorStub wraps dirPathProvider to report CapAutoMode support.
type autoModeReadErrorStub struct{ dirPathProvider }

func (autoModeReadErrorStub) Supports(c provider.Capability) bool { return c == provider.CapAutoMode }
func (autoModeReadErrorStub) DisplayName() string                 { return "AutoModeError" }

// TestUninstall_NothingInstalled is a clean no-op: uninstall on a fresh home
// should succeed and still print completion without errors.
func TestUninstall_NothingInstalled(t *testing.T) {
	_ = setupApplyTest(t)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--force"}); err != nil {
			t.Fatalf("uninstall on clean home error: %v", err)
		}
	})

	if !strings.Contains(out, "Uninstall complete") {
		t.Errorf("expected completion message on clean uninstall, got:\n%s", out)
	}
	if strings.Contains(out, "Removed juggernaut block") {
		t.Errorf("clean home should not report a removed block, got:\n%s", out)
	}
}

func TestRemoveRuntimeStateGuardsAndFailures(t *testing.T) {
	originalFlags := uninstallFlags
	t.Cleanup(func() { uninstallFlags = originalFlags })

	t.Run("project scope preserves user state", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		if err := activation.SaveRuntimeState(home, "claude", activation.RuntimeState{AuthMode: "iam"}); err != nil {
			t.Fatal(err)
		}
		uninstallFlags.scope = "project"
		uninstallFlags.dryRun = false

		removeRuntimeState(home, provider.MustGet("claude"))
		if _, found, err := activation.LoadRuntimeState(home, "claude"); err != nil || !found {
			t.Fatalf("project uninstall state = found %v, err %v", found, err)
		}
	})

	t.Run("provider without persistence preserves state", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		if err := activation.SaveRuntimeState(home, "codex", activation.RuntimeState{AuthMode: "iam"}); err != nil {
			t.Fatal(err)
		}
		uninstallFlags.scope = "user"
		uninstallFlags.dryRun = false

		removeRuntimeState(home, provider.MustGet("codex"))
		if _, found, err := activation.LoadRuntimeState(home, "codex"); err != nil || !found {
			t.Fatalf("non-persistent provider state = found %v, err %v", found, err)
		}
	})

	t.Run("dry run ignores missing state", func(t *testing.T) {
		uninstallFlags.scope = "user"
		uninstallFlags.dryRun = true
		out := captureStdout(t, func() {
			removeRuntimeState(testutil.NewTestHome(t), provider.MustGet("claude"))
		})
		if out != "" {
			t.Fatalf("missing fallback produced dry-run output: %q", out)
		}
	})

	t.Run("dry run ignores invalid state", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		path, err := activation.RuntimeStatePath(home, "claude")
		if err != nil {
			t.Fatal(err)
		}
		if err := safepath.WriteFile(home, path, []byte(`{`)); err != nil {
			t.Fatal(err)
		}
		uninstallFlags.scope = "user"
		uninstallFlags.dryRun = true
		out := captureStdout(t, func() {
			removeRuntimeState(home, provider.MustGet("claude"))
		})
		if out != "" {
			t.Fatalf("invalid fallback produced dry-run output: %q", out)
		}
	})

	t.Run("remove failure warns", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		path, err := activation.RuntimeStatePath(home, "claude")
		if err != nil {
			t.Fatal(err)
		}
		if err := safepath.MkdirAll(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "keep"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		uninstallFlags.scope = "user"
		uninstallFlags.dryRun = false

		stderr := captureStderr(t, func() {
			removeRuntimeState(home, provider.MustGet("claude"))
		})
		if !strings.Contains(stderr, "could not remove runtime fallback") {
			t.Fatalf("missing removal warning: %q", stderr)
		}
	})
}

// TestUninstallSettingsBlock_DryRun previews removal without writing.
func TestUninstallSettingsBlock_DryRun(t *testing.T) {
	home := setupApplyTest(t)
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}

	// First apply so juggernaut-managed keys exist.
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	uninstallFlags.dryRun = true
	defer func() { uninstallFlags.dryRun = false }()

	out := captureStdout(t, func() {
		uninstallSettingsBlock(home, "user", prov)
	})
	if !strings.Contains(out, "Would remove juggernaut block") {
		t.Errorf("dry-run should preview removal, got: %q", out)
	}
}

// TestUninstallSettingsBlock_NoManagedKeys skips when the config has no
// juggernaut-managed keys.
func TestUninstallSettingsBlock_NoManagedKeys(t *testing.T) {
	home := setupApplyTest(t)
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}

	out := captureStdout(t, func() {
		uninstallSettingsBlock(home, "user", prov)
	})
	if out != "" {
		t.Errorf("expected no output when no managed keys present, got: %q", out)
	}
}
