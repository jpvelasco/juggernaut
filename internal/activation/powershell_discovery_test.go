//go:build windows

package activation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// mockCommandRunner returns pre-configured output for test scenarios.
type mockCommandRunner struct {
	output map[string][]byte
	err    map[string]error
}

func (m *mockCommandRunner) RunContext(_ context.Context, exe string, _ []string) ([]byte, error) {
	if err := m.err[exe]; err != nil {
		return nil, err
	}
	return m.output[exe], nil
}

func makePSOutput(allHosts, currentHost string) []byte {
	paths := psProfilePaths{
		CurrentUserAllHosts:    allHosts,
		CurrentUserCurrentHost: currentHost,
	}
	data, _ := json.Marshal(paths)
	return data
}

func makeHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestDiscoverPowerShellProfiles_PS7Only(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	currentHost := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, currentHost),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if len(result.ActiveTargets) != 1 {
		t.Fatalf("expected 1 active target, got %d", len(result.ActiveTargets))
	}
	if result.ActiveTargets[0].Path != allHosts {
		t.Errorf("expected active target %s, got %s", allHosts, result.ActiveTargets[0].Path)
	}
	if result.InstallTarget.Path != allHosts {
		t.Errorf("expected install target %s, got %s", allHosts, result.InstallTarget.Path)
	}
	if !containsPathCI(result.MigrationTargets, allHosts) {
		t.Error("expected allHosts in migration targets")
	}
	
}

func TestDiscoverPowerShellProfiles_BothEditions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	ps7All := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	ps5All := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       makePSOutput(ps7All, ps7All),
			"powershell.exe": makePSOutput(ps5All, ps5All),
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if len(result.ActiveTargets) != 2 {
		t.Fatalf("expected 2 active targets, got %d", len(result.ActiveTargets))
	}
	if len(result.EditionsDiscovered) != 2 {
		t.Fatalf("expected 2 editions discovered, got %d", len(result.EditionsDiscovered))
	}
}

func TestDiscoverPowerShellProfiles_OneDriveRedirected(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Simulate OneDrive-redirected path
	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// Active target should be the OneDrive path, NOT the historical $HOME/Documents path.
	if !strings.Contains(result.ActiveTargets[0].Path, "OneDrive") {
		t.Errorf("active target should contain OneDrive, got %s", result.ActiveTargets[0].Path)
	}
}

func TestDiscoverPowerShellProfiles_UNCPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	uncPath := `\\server\share\PowerShell\Microsoft.PowerShell_profile.ps1`

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(uncPath, uncPath),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if !strings.HasPrefix(result.ActiveTargets[0].Path, `\\`) {
		t.Errorf("active target should be UNC path, got %s", result.ActiveTargets[0].Path)
	}
}

func TestDiscoverPowerShellProfiles_PathWithSpaces(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "My Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if !strings.Contains(result.ActiveTargets[0].Path, "My Documents") {
		t.Errorf("active target should contain spaces, got %s", result.ActiveTargets[0].Path)
	}
}

func TestDiscoverPowerShellProfiles_PathWithUnicode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "Dokumente_über", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if !strings.Contains(result.ActiveTargets[0].Path, "über") {
		t.Errorf("active target should contain Unicode, got %s", result.ActiveTargets[0].Path)
	}
}

func TestDiscoverPowerShellProfiles_MalformedJSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": []byte("not json at all"),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// Should report discovery failure for malformed JSON.
	if len(result.DiscoveryWarnings) == 0 {
		t.Error("expected discovery warnings for malformed JSON")
	}
	if len(result.ActiveTargets) != 0 {
		t.Errorf("expected 0 active targets on malformed JSON, got %d", len(result.ActiveTargets))
	}
}

func TestDiscoverPowerShellProfiles_DiscoveryTimeout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Simulate a timeout by returning context.DeadlineExceeded
	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       context.DeadlineExceeded,
			"powershell.exe": context.DeadlineExceeded,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// No active targets when both editions time out.
	if len(result.ActiveTargets) != 0 {
		t.Errorf("expected 0 active targets on timeout, got %d", len(result.ActiveTargets))
	}
	if len(result.DiscoveryWarnings) < 2 {
		t.Errorf("expected at least 2 discovery warnings, got %d", len(result.DiscoveryWarnings))
	}
}

func TestDiscoverPowerShellProfiles_BothMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// No active targets when both executables missing.
	if len(result.ActiveTargets) != 0 {
		t.Errorf("expected 0 active targets, got %d", len(result.ActiveTargets))
	}
	if len(result.DiscoveryWarnings) == 0 {
		t.Error("expected discovery warnings when both executables missing")
	}
}

func TestDiscoverPowerShellProfiles_CaseInsensitiveDedup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Both editions return the same path with different casing
	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	currentHost5 := filepath.Join(home, "ONEDRIVE", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       makePSOutput(allHosts, allHosts),
			"powershell.exe": makePSOutput(currentHost5, currentHost5),
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// MigrationTargets should not contain duplicates (case-insensitive)
	seen := make(map[string]bool)
	var dup bool
	for _, p := range result.MigrationTargets {
		key := strings.ToLower(p)
		if seen[key] {
			dup = true
		}
		seen[key] = true
	}
	if dup {
		t.Error("migration targets should be deduplicated case-insensitively")
	}
}

func TestDiscoverPowerShellProfiles_NoExecutable_DiscoveryFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// No active targets, no fallback — discovery failure is reported.
	if len(result.ActiveTargets) != 0 {
		t.Errorf("expected 0 active targets, got %d", len(result.ActiveTargets))
	}
	if len(result.DiscoveryWarnings) == 0 {
		t.Error("expected discovery warnings when no executables found")
	}
	if len(result.EditionsDiscovered) != 0 {
		t.Errorf("expected 0 editions discovered, got %d", len(result.EditionsDiscovered))
	}
}

func TestParseDiscoveryOutput_Valid(t *testing.T) {
	home := t.TempDir()
	allHosts := filepath.Join(home, "redirected", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	output := makePSOutput(allHosts, allHosts)

	result := parseDiscoveryOutput("PowerShell 7", output)

	if !result.DiscoveryOK {
		t.Fatal("expected discovery to succeed")
	}
	if result.AllHosts != allHosts {
		t.Errorf("expected %s, got %s", allHosts, result.AllHosts)
	}
}

func TestParseDiscoveryOutput_EmptyPaths(t *testing.T) {
	output := makePSOutput("", "")

	result := parseDiscoveryOutput("PowerShell 7", output)

	if !result.DiscoveryOK {
		t.Fatal("expected discovery to succeed even with empty paths")
	}
	if result.AllHosts != "" {
		t.Errorf("expected empty AllHosts, got %q", result.AllHosts)
	}
}

func TestParseDiscoveryOutput_BOM(t *testing.T) {
	home := t.TempDir()
	allHosts := filepath.Join(home, "profile.ps1")
	output := makePSOutput(allHosts, allHosts)
	output = append([]byte("\xef\xbb\xbf"), output...) // prepend UTF-8 BOM

	result := parseDiscoveryOutput("PowerShell 7", output)

	if !result.DiscoveryOK {
		t.Fatal("expected discovery to succeed with BOM")
	}
	if result.AllHosts != allHosts {
		t.Errorf("expected clean path, got %q", result.AllHosts)
	}
}

func TestLegacyMarkers_Detection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "legacy launcher block",
			content: "# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher",
			want:    true,
		},
		{
			name:    "legacy bedrock block",
			content: "# BEGIN: Claude Code Bedrock Configuration\nstuff\n# END: Claude Code Bedrock Configuration",
			want:    true,
		},
		{
			name:    "current block only",
			content: "# BEGIN: Juggernaut Claude Activation\nstuff\n# END: Juggernaut Claude Activation",
			want:    false,
		},
		{
			name:    "no blocks",
			content: "export FOO=bar",
			want:    false,
		},
		{
			name:    "incomplete legacy block",
			content: "# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasAnyLegacyBlock(tt.content) != tt.want {
				t.Errorf("HasAnyLegacyBlock() = %v, want %v", !tt.want, tt.want)
			}
		})
	}
}

func TestLegacyMarkers_Removal(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "remove legacy launcher",
			content: "before\n# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher\nafter",
			want:    "before\nafter",
		},
		{
			name:    "remove legacy bedrock",
			content: "# BEGIN: Claude Code Bedrock Configuration\nexport FOO=bar\n# END: Claude Code Bedrock Configuration",
			want:    "",
		},
		{
			name:    "preserve current block",
			content: "# BEGIN: Juggernaut Claude Activation\nstuff\n# END: Juggernaut Claude Activation",
			want:    "# BEGIN: Juggernaut Claude Activation\nstuff\n# END: Juggernaut Claude Activation",
		},
		{
			name:    "preserve unrelated content",
			content: "export FOO=bar\n# BEGIN: Juggernaut Launcher\nold stuff\n# END: Juggernaut Launcher\nalias x='y'",
			want:    "export FOO=bar\nalias x='y'",
		},
		{
			name:    "CRLF legacy block",
			content: "before\r\n# BEGIN: Juggernaut Launcher\r\nold stuff\r\n# END: Juggernaut Launcher\r\nafter",
			want:    "before\nafter",
		},
		{
			name: "valid pair followed by orphan begin preserves trailing content",
			content: "before\n# BEGIN: Juggernaut Launcher\nold stuff\n# END: Juggernaut Launcher\n# BEGIN: Juggernaut Launcher\nafter orphan",
			want:    "before\n# BEGIN: Juggernaut Launcher\nafter orphan",
		},
		{
			name: "end before begin does not enter deletion mode",
			content: "before\n# END: Juggernaut Launcher\n# BEGIN: Juggernaut Launcher\nstuff\n# END: Juggernaut Launcher\nafter",
			want:    "before\n# END: Juggernaut Launcher\nafter",
		},
		{
			name: "multiple complete blocks removed",
			content: "a\n# BEGIN: Juggernaut Launcher\nx\n# END: Juggernaut Launcher\nb\n# BEGIN: Juggernaut Launcher\ny\n# END: Juggernaut Launcher\nc",
			want:    "a\nb\nc",
		},
		{
			name: "orphan begin with no matching end is preserved",
			content: "before\n# BEGIN: Juggernaut Launcher\norphan begin only\nafter",
			want:    "before\n# BEGIN: Juggernaut Launcher\norphan begin only\nafter",
		},
		{
			name: "nested-like markers: first begin matches first end",
			content: "# BEGIN: Juggernaut Launcher\n# BEGIN: Juggernaut Launcher\ninner\n# END: Juggernaut Launcher\n# END: Juggernaut Launcher\nafter",
			want:    "after",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned, _ := removeLegacyBlocks(tt.content)
			if cleaned != tt.want {
				t.Errorf("removeLegacyBlocks() = %q, want %q", cleaned, tt.want)
			}
		})
	}
}

func TestMigrateLegacyAndInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()

	// Create a profile with legacy block
	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(
		"export FOO=bar\n# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher\nalias x='y'",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	modified, err := migrateLegacyAndInstall(allHosts)
	if err != nil {
		t.Fatalf("migrateLegacyAndInstall: %v", err)
	}
	if !modified {
		t.Fatal("expected file to be modified")
	}

	data, err := safepath.ReadFile(filepath.Dir(allHosts), allHosts)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	// Legacy block should be removed
	if HasLegacyLauncherBlock(content) {
		t.Error("legacy block should be removed")
	}
	// Current block should be installed
	if !HasBlock(content) {
		t.Error("current block should be installed")
	}
	// Unrelated content should be preserved
	if !strings.Contains(content, "export FOO=bar") {
		t.Error("unrelated content should be preserved")
	}
	if !strings.Contains(content, "alias x='y'") {
		t.Error("unrelated content should be preserved")
	}
}

func TestMigrateLegacyOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()

	path := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(path), path, []byte(
		"# BEGIN: Juggernaut Launcher\nold stuff\n# END: Juggernaut Launcher\nexport KEEP=1",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	modified, err := migrateLegacyOnly(path)
	if err != nil {
		t.Fatalf("migrateLegacyOnly: %v", err)
	}
	if !modified {
		t.Fatal("expected file to be modified")
	}

	data, err := safepath.ReadFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	if HasLegacyLauncherBlock(content) {
		t.Error("legacy block should be removed")
	}
	if HasBlock(content) {
		t.Error("current block should NOT be installed by migrateLegacyOnly")
	}
	if !strings.Contains(content, "export KEEP=1") {
		t.Error("unrelated content should be preserved")
	}
}

func TestCheckPowerShellActivation_Healthy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(Block(ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	healthy, path, warnings := CheckPowerShellActivation(home)
	if !healthy {
		t.Error("expected activation to be healthy")
	}
	if path != allHosts {
		t.Errorf("expected path %s, got %s", allHosts, path)
	}
	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestCheckPowerShellActivation_Unhealthy_LegacyInEffective(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(
		"# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	healthy, _, warnings := CheckPowerShellActivation(home)
	if healthy {
		t.Error("expected activation to be unhealthy with legacy block")
	}
	if len(warnings) == 0 {
		t.Error("expected warnings about legacy block")
	}
}

func TestCheckPowerShellActivation_NoActivation_DiscoveredProfileOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// The real discovered profile has NO activation
	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte("export FOO=bar")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	healthy, _, warnings := CheckPowerShellActivation(home)
	if healthy {
		t.Error("expected activation to be unhealthy when discovered profile has no activation")
	}
	// No warnings about historical paths — hardcoded paths are removed.
	if len(warnings) > 0 {
		t.Errorf("expected no warnings about historical paths, got %v", warnings)
	}
}

func TestCheckPowerShellActivation_NoFalseWarning_DiscoveredPathMatchesHistorical(t *testing.T) {
	// Regression: previously, doctor checked every guessed historical path
	// and warned whenever it contained activation, without excluding paths
	// also present in ActiveTargets. On a standard non-OneDrive setup, the
	// discovered profile can equal a historical candidate, producing a false
	// warning that PowerShell does not load it. With hardcoded paths removed,
	// only the dynamically discovered profile is checked.
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Use the standard non-OneDrive path that used to be both "active" and "historical"
	allHosts := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(Block(ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	healthy, _, warnings := CheckPowerShellActivation(home)
	if !healthy {
		t.Error("expected activation to be healthy — the discovered profile has a valid block")
	}
	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidateAndCanonicalizePath(t *testing.T) {
	home := t.TempDir()
	validPath := filepath.Join(home, "profile.ps1")
	validSpaces := filepath.Join(home, "My Documents", "profile.ps1")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"valid path", validPath, validPath},
		{"empty", "", ""},
		{"whitespace", "  ", ""},
		{"dot", ".", ""},
		{"quotes", `""`, ""},
		{"with spaces", validSpaces, validSpaces},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAndCanonicalizePath(tt.in)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeduplicatePathsCI(t *testing.T) {
	home := t.TempDir()
	path1 := filepath.Join(home, "profile.ps1")
	path2 := filepath.Join(home, "other.ps1")

	paths := []string{
		path1,
		strings.ToUpper(path1),
		path2,
	}
	deduped := deduplicatePathsCI(paths)
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique paths, got %d: %v", len(deduped), deduped)
	}
}

func TestHistoricalPowerShellTargets_ReturnsNil(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	candidates := historicalPowerShellTargets()

	// Hardcoded historical paths have been removed.
	if candidates != nil {
		t.Fatalf("expected nil, got %d candidates", len(candidates))
	}
}

func TestInstallPowerShellActivation_LegacyMigration(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(
		"export FOO=bar\n# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher\nalias x='y'",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	installed, err := InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("InstallPowerShellActivation: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one path to be installed")
	}

	data, err := safepath.ReadFile(filepath.Dir(allHosts), allHosts)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	if HasLegacyLauncherBlock(content) {
		t.Error("legacy block should be removed")
	}
	if !HasBlock(content) {
		t.Error("current block should be installed")
	}
	if !strings.Contains(content, "export FOO=bar") {
		t.Error("unrelated content should be preserved")
	}
}

func TestInstallPowerShellActivation_Idempotent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(Block(ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	_, err := InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}

	data1, _ := safepath.ReadFile(filepath.Dir(allHosts), allHosts)

	_, err = InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}

	data2, _ := safepath.ReadFile(filepath.Dir(allHosts), allHosts)

	if string(data1) != string(data2) {
		t.Error("second install should not change file")
	}
}

func TestUninstallPowerShellActivation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(
		"export FOO=bar\n"+Block(ShellPowerShell)+"\n# BEGIN: Juggernaut Launcher\nold stuff\n# END: Juggernaut Launcher\nalias x='y'",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	removed, err := UninstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("UninstallPowerShellActivation: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("expected at least one path to be removed")
	}

	data, err := safepath.ReadFile(filepath.Dir(allHosts), allHosts)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	if HasBlock(content) {
		t.Error("current block should be removed")
	}
	if HasLegacyLauncherBlock(content) {
		t.Error("legacy block should be removed")
	}
	if !strings.Contains(content, "export FOO=bar") {
		t.Error("unrelated content should be preserved")
	}
	if !strings.Contains(content, "alias x='y'") {
		t.Error("unrelated content should be preserved")
	}
}

func TestRemoveLegacyBlocks_RemovesBoth(t *testing.T) {
	content := "# BEGIN: Juggernaut Launcher\nold\n# END: Juggernaut Launcher\nexport FOO=bar\n# BEGIN: Claude Code Bedrock Configuration\nmore old\n# END: Claude Code Bedrock Configuration\nalias x='y'"

	cleaned, removed := removeLegacyBlocks(content)

	if len(removed) != 2 {
		t.Errorf("expected 2 blocks removed, got %d: %v", len(removed), removed)
	}
	if HasLegacyLauncherBlock(cleaned) {
		t.Error("legacy launcher block should be removed")
	}
	if HasLegacyBedrockBlock(cleaned) {
		t.Error("legacy bedrock block should be removed")
	}
	if !strings.Contains(cleaned, "export FOO=bar") {
		t.Error("unrelated content should be preserved")
	}
	if !strings.Contains(cleaned, "alias x='y'") {
		t.Error("unrelated content should be preserved")
	}
}

func TestInstallPowerShellActivation_HostSpecificOverride(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// PS7 AllHosts (authoritative)
	ps7All := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	// PS5.1 CurrentHost (host-specific, loads after AllHosts)
	ps5Host := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")

	if err := safepath.MkdirAll(filepath.Dir(ps7All)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.MkdirAll(filepath.Dir(ps5Host)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	// PS7 profile has current block
	if err := safepath.WriteFile(filepath.Dir(ps7All), ps7All, []byte(Block(ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	// PS5.1 host-specific profile has legacy block that could override
	if err := safepath.WriteFile(filepath.Dir(ps5Host), ps5Host, []byte(
		"# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       makePSOutput(ps7All, ps7All),
			"powershell.exe": makePSOutput(ps5Host, ps5Host),
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	installed, err := InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("InstallPowerShellActivation: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one path to be installed")
	}

	// Check PS5.1 host-specific profile was migrated
	data, err := safepath.ReadFile(filepath.Dir(ps5Host), ps5Host)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	if HasLegacyLauncherBlock(content) {
		t.Error("legacy block in host-specific profile should be removed")
	}
	if !HasBlock(content) {
		t.Error("current block should be installed in host-specific profile")
	}
}

func TestDiscoveryTimeout_WithMock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Simulate a slow executable that times out
	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       context.DeadlineExceeded,
			"powershell.exe": context.DeadlineExceeded,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// No active targets when both editions time out — no fallback.
	if len(result.ActiveTargets) != 0 {
		t.Errorf("expected 0 active targets on timeout, got %d", len(result.ActiveTargets))
	}
	// No editions should be listed as discovered because both timed out.
	if len(result.EditionsDiscovered) != 0 {
		t.Errorf("expected 0 editions discovered, got %d", len(result.EditionsDiscovered))
	}
	if len(result.DiscoveryWarnings) == 0 {
		t.Error("expected discovery warnings when both time out")
	}
}

func TestOneExecutableMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if len(result.ActiveTargets) != 1 {
		t.Fatalf("expected 1 active target, got %d", len(result.ActiveTargets))
	}
	if result.ActiveTargets[0].Path != allHosts {
		t.Errorf("expected %s, got %s", allHosts, result.ActiveTargets[0].Path)
	}
}

func TestValidateAndCanonicalizePath_Unicode(t *testing.T) {
	home := t.TempDir()
	// Paths with Unicode characters should work
	p := validateAndCanonicalizePath(filepath.Join(home, "über", "profile.ps1"))
	if p == "" {
		t.Error("Unicode path should be valid")
	}
}

func TestValidateAndCanonicalizePath_Spaces(t *testing.T) {
	home := t.TempDir()
	p := validateAndCanonicalizePath("  " + filepath.Join(home, "My Documents", "profile.ps1") + "  ")
	if p == "" {
		t.Error("path with spaces should be valid")
	}
}

// Test the exact regression scenario from the bug report.
func TestRegression_OneDriveRedirectedWithLegacyBlock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Actual discovered PowerShell profile (redirected Documents)
	realProfile := filepath.Join(home, "redirected-documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(realProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	// Real profile has the legacy launcher block
	if err := safepath.WriteFile(filepath.Dir(realProfile), realProfile, []byte(
		"export FOO=bar\n# BEGIN: Juggernaut Launcher\nfunction claude { juggernaut --launcher @args }\n# END: Juggernaut Launcher\nalias x='y'",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(realProfile, realProfile),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	// Before apply: doctor should NOT report activation as healthy.
	healthy, _, warnings := CheckPowerShellActivation(home)
	if healthy {
		t.Error("doctor should NOT report activation healthy before apply")
	}
	if len(warnings) == 0 {
		t.Error("expected warnings about legacy block")
	}

	// Apply
	installed, err := InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("InstallPowerShellActivation: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one path to be installed")
	}

	// After apply: real profile should have current block, not legacy
	data, err := safepath.ReadFile(filepath.Dir(realProfile), realProfile)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

	if HasLegacyLauncherBlock(content) {
		t.Error("legacy block should be removed from real profile")
	}
	if !HasBlock(content) {
		t.Error("current block should be in real profile")
	}
	if !strings.Contains(content, "export FOO=bar") {
		t.Error("unrelated content should be preserved")
	}
	if !strings.Contains(content, "alias x='y'") {
		t.Error("unrelated content should be preserved")
	}

	// After apply: doctor should report activation healthy
	healthy, foundPath, warnings := CheckPowerShellActivation(home)
	if !healthy {
		t.Error("doctor should report activation healthy after apply")
	}
	if foundPath != realProfile {
		t.Errorf("expected path %s, got %s", realProfile, foundPath)
	}
	if len(warnings) > 0 {
		t.Errorf("expected no warnings after apply, got %v", warnings)
	}

	// Idempotent: second apply should not change anything
	_, err = InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	data2, _ := safepath.ReadFile(filepath.Dir(realProfile), realProfile)
	if string(data) != string(data2) {
		t.Error("second apply should not change file")
	}
}

// Test that the mockCommandRunner respects the context timeout.
func TestMockCommandRunner_WithTimeout(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "profile.ps1")
	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": makePSOutput(path, path),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	data, err := runner.RunContext(ctx, "pwsh.exe", []string{"-NoProfile"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty output")
	}
}

// Test that the mockCommandRunner returns errors correctly.
func TestMockCommandRunner_ReturnsError(t *testing.T) {
	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe": os.ErrNotExist,
		},
	}

	_, err := runner.RunContext(context.Background(), "pwsh.exe", nil)
	if err != os.ErrNotExist {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

// Test the _ = time to avoid unused import
var _ = time.Second
