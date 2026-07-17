package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestUninstall_DryRun_PreservesBlock verifies that a dry-run reports intended
// removals but leaves settings.json untouched and never prints completion.
func TestUninstall_DryRun_PreservesBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; !ok {
		t.Error("dry-run removed the juggernaut block; it should be preserved")
	}
}

// TestUninstall_ScopeFilter verifies --scope=user only touches the user-scope
// settings and prints a scoped removal message.
func TestUninstall_ScopeFilter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; ok {
		t.Error("user-scope juggernaut block should have been removed")
	}
}

// TestUninstall_ConfirmYes exercises the interactive prompt accepting "y".
func TestUninstall_ConfirmYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; ok {
		t.Error("confirming uninstall should have removed the juggernaut block")
	}
}

// TestUninstall_ConfirmAbort exercises the prompt being declined (anything but y).
func TestUninstall_ConfirmAbort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	var out string
	withStdin(t, "n\n", func() {
		out = captureStdout(t, func() {
			if err := ExecuteArgs([]string{"uninstall"}); err != nil {
				t.Fatalf("uninstall error: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Aborted") {
		t.Errorf("declining the prompt should abort, got:\n%s", out)
	}
	if strings.Contains(out, "Uninstall complete") {
		t.Errorf("aborted uninstall must not complete, got:\n%s", out)
	}

	// Block must survive an aborted uninstall.
	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; !ok {
		t.Error("aborted uninstall should preserve the juggernaut block")
	}
}

// TestUninstall_EOFAborts verifies that closing stdin without input (e.g. Ctrl+D
// or a closed pipe) is treated as a decline: the uninstall aborts and the block
// survives. Covers the scanner.Scan()==false EOF branch of confirmUninstallAborted.
func TestUninstall_EOFAborts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	var out string
	withStdin(t, "", func() { // empty input -> EOF on first Scan
		out = captureStdout(t, func() {
			if err := ExecuteArgs([]string{"uninstall"}); err != nil {
				t.Fatalf("uninstall error: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Aborted") {
		t.Errorf("EOF on the prompt should abort, got:\n%s", out)
	}
	if strings.Contains(out, "Uninstall complete") {
		t.Errorf("EOF-aborted uninstall must not complete, got:\n%s", out)
	}

	data := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	if _, ok := settings["juggernaut"]; !ok {
		t.Error("EOF-aborted uninstall should preserve the juggernaut block")
	}
}

// TestUninstall_NothingInstalled is a clean no-op: uninstall on a fresh home
// should succeed and still print completion without errors.
func TestUninstall_NothingInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

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
