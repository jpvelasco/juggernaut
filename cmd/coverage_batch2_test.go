package cmd

// coverage_batch2_test.go — coverage push batch 2: closes the remaining
// Windows-leg activation branches in runDoctor. Both paths require real
// PowerShell profile discovery, so they use the mockPSRunner and run only on
// Windows; the POSIX equivalents are exercised on the Linux/macOS CI legs.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// writePSProfile creates the parent directory for a PowerShell profile path and
// writes the given content, failing the test on any error.
func writePSProfile(t *testing.T, path, content string) {
	t.Helper()
	if err := safepath.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestDoctor_ClaudeActivationWarnings_Windows covers the Windows-only Claude
// warnings loop (doctor.go): a healthy activation block plus a legacy launcher
// block in a later-loading profile makes CheckPowerShellActivationWith emit a
// warning, which runDoctor surfaces as an "activation warning" check.
func TestDoctor_ClaudeActivationWarnings_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell profile discovery is Windows-only")
	}

	home := setupApplyTest(t)

	ps7All := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	writePSProfile(t, ps7All, activation.BeginMarker+"\n$env.TEST='1'\n"+activation.EndMarker)

	ps5Host := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile_5host.ps1")
	writePSProfile(t, ps5Host, activation.LegacyLauncherBegin+"\n$env.LEGACY='1'\n"+activation.LegacyLauncherEnd)

	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockPSOutputJSON(ps7All, ps7All),
			"powershell.exe": mockPSOutputJSON(ps5Host, ps5Host),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	t.Cleanup(activation.ResetPSRunnerForTesting)

	out := captureStdout(t, func() {
		_ = ExecuteArgs([]string{"doctor"})
	})
	if !strings.Contains(out, "activation warning") {
		t.Errorf("expected doctor activation warning output, got:\n%s", out)
	}
}

// TestDoctor_CodexActivationOK_Windows covers the Windows-only non-Claude OK
// branch: a codex activation block present in a discovered profile makes
// InstalledTargetsForMarkers return a path, so doctor reports it as active.
func TestDoctor_CodexActivationOK_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell profile discovery is Windows-only")
	}

	home := setupApplyTest(t)

	profile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	writePSProfile(t, profile, "# BEGIN: Juggernaut Codex Activation\nfunction codex {}\n# END: Juggernaut Codex Activation\n")

	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockPSOutputJSON(profile, profile),
			"powershell.exe": mockPSOutputJSON(profile, profile),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	t.Cleanup(activation.ResetPSRunnerForTesting)

	out := captureStdout(t, func() {
		_ = ExecuteArgs([]string{"doctor", "--cli", "codex"})
	})
	if !strings.Contains(out, "codex activation") || !strings.Contains(out, "active in") {
		t.Errorf("expected codex active output, got:\n%s", out)
	}
}
