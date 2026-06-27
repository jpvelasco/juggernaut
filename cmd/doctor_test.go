package cmd

import (
	"context"
	"encoding/json"
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

func TestDoctor_ActivationFalsePositive_HistoricalOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	// This test verifies that doctor does NOT report activation as healthy
	// when the current activation block only exists in a non-loaded historical
	// path (e.g., $HOME/Documents) while the actual OneDrive-redirected profile
	// contains a legacy launcher block.

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Real discovered profile (OneDrive) has legacy launcher block
	realProfile := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(realProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(realProfile), realProfile, []byte(
		"# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	// Historical hardcoded profile has the current activation block
	histProfile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(histProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(histProfile), histProfile, []byte(activation.Block(activation.ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	// Set up mock discovery
	runner := &mockDoctorCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": mockPSOutput(realProfile, realProfile),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	activation.SetPSRunnerForTesting(runner)
	defer activation.ResetPSRunnerForTesting()

	// Check activation using the shared resolver
	healthy, _, warnings := activation.CheckPowerShellActivation(home)
	if healthy {
		t.Error("doctor should NOT report activation healthy when only historical path has it")
	}
	if len(warnings) == 0 {
		t.Error("expected warnings about legacy block or historical path")
	}
}

func TestDoctor_ActivationHealthy_AfterMigration(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	realProfile := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(realProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(realProfile), realProfile, []byte(
		"# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockDoctorCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": mockPSOutput(realProfile, realProfile),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	activation.SetPSRunnerForTesting(runner)
	defer activation.ResetPSRunnerForTesting()

	// Apply migration
	_, err := activation.InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("InstallPowerShellActivation: %v", err)
	}

	// After migration, should be healthy
	healthy, path, warnings := activation.CheckPowerShellActivation(home)
	if !healthy {
		t.Error("doctor should report activation healthy after migration")
	}
	if path != realProfile {
		t.Errorf("expected path %s, got %s", realProfile, path)
	}
	if len(warnings) > 0 {
		t.Errorf("expected no warnings after migration, got %v", warnings)
	}
}

// mockDoctorCommandRunner implements activation.discoveryCommandRunner for doctor tests.
type mockDoctorCommandRunner struct {
	output map[string][]byte
	err    map[string]error
}

func (m *mockDoctorCommandRunner) RunContext(ctx context.Context, exe string, args []string) ([]byte, error) {
	if err := m.err[exe]; err != nil {
		return nil, err
	}
	return m.output[exe], nil
}

func mockPSOutput(allHosts, currentHost string) []byte {
	data, _ := json.Marshal(map[string]string{
		"CurrentUserAllHosts":    allHosts,
		"CurrentUserCurrentHost": currentHost,
	})
	return data
}
