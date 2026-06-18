package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestOpusplanProblem(t *testing.T) {
	if _, ok := opusplanProblem(nil); ok {
		t.Fatal("nil settings should not trigger opusplan warning")
	}

	if _, ok := opusplanProblem(map[string]any{"model": "sonnet"}); ok {
		t.Fatal("non-opusplan model should not trigger warning")
	}

	detail, ok := opusplanProblem(map[string]any{"model": "opusplan"})
	if !ok {
		t.Fatal("opusplan should trigger warning")
	}
	if !strings.Contains(detail, "opusplan") {
		t.Fatalf("warning should mention opusplan, got %q", detail)
	}
}

func TestClaudeCommandStatus_OKWhenRealClaudeFound(t *testing.T) {
	dir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.cmd"
	}
	claude := filepath.Join(dir, name)
	writeExecutableStub(t, claude)
	t.Setenv("PATH", dir)

	status, detail := claudeCommandStatus()
	if status != doctor.OK {
		t.Fatalf("expected OK, got %s (%s)", status, detail)
	}
	if detail != claude {
		t.Fatalf("expected claude path %s, got %s", claude, detail)
	}
}

func TestLegacyArtifactStatusWarnsWhenRecoverable(t *testing.T) {
	home := t.TempDir()
	binDir := activation.DefaultBinDir(home)
	if err := safepath.MkdirAll(binDir); err != nil {
		t.Fatalf("creating bin dir: %v", err)
	}
	backupName := "claude.juggernaut-original"
	if runtime.GOOS == "windows" {
		backupName = "claude.juggernaut-original.cmd"
	}
	backup := filepath.Join(binDir, backupName)
	if err := os.WriteFile(backup, []byte("real claude"), 0o600); err != nil {
		t.Fatalf("creating backup: %v", err)
	}

	status, detail := legacyArtifactStatus(home)
	if status != doctor.Warn {
		t.Fatalf("expected WARN, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, "backup can be restored") {
		t.Fatalf("expected recoverable backup detail, got %q", detail)
	}
}

func TestCheckSettingsScope_DefaultProjectMissingIsOK(t *testing.T) {
	home := t.TempDir()

	status, detail := checkSettingsScope(home, "project", false)
	if status != doctor.OK {
		t.Fatalf("expected OK for missing project scope, got %s (%s)", status, detail)
	}
	if detail != "not configured" {
		t.Fatalf("expected not configured detail, got %q", detail)
	}
}

func TestCheckSettingsScope_RequiredScopeMissingFails(t *testing.T) {
	home := t.TempDir()

	status, detail := checkSettingsScope(home, "user", true)
	if status != doctor.Fail {
		t.Fatalf("expected FAIL for missing required scope, got %s (%s)", status, detail)
	}
}

func TestMigrateCommandIsNotRegistered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "migrate" {
			t.Fatal("migrate command should not be registered in v5")
		}
	}
}

func TestLaunchCommandIsHidden(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "launch" {
			found = true
			if !cmd.Hidden {
				t.Fatal("launch command should be hidden")
			}
		}
	}
	if !found {
		t.Fatal("launch command should be registered")
	}
}

func writeExecutableStub(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("real claude"), 0o600); err != nil {
		t.Fatalf("creating executable stub: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission, go_file-permissions_rule-fileperm
			t.Fatalf("making executable stub runnable: %v", err)
		}
	}
}
