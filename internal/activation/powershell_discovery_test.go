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

// setupDocsMock mocks resolveDocumentsFolder to return the given home
// directory as the Documents folder, so that path containment checks pass
// when test paths are under t.TempDir(). This is intentionally broad — the
// real Documents folder is always under $HOME, so using $HOME as the base
// ensures all test paths are contained.
func setupDocsMock(t *testing.T, home string) {
	t.Helper()
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return home, nil
	})
	t.Cleanup(ResetResolveDocumentsFolderForTesting)
}

func TestDiscoverPowerShellProfiles_PS7Only(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	currentHost := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile_7host.ps1")

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

	// ActiveTargets should contain both AllHosts and CurrentHost.
	if len(result.ActiveTargets) != 2 {
		t.Fatalf("expected 2 active targets, got %d", len(result.ActiveTargets))
	}
	if result.ActiveTargets[0].Path != allHosts {
		t.Errorf("expected first active target %s, got %s", allHosts, result.ActiveTargets[0].Path)
	}
	if result.ActiveTargets[1].Path != currentHost {
		t.Errorf("expected second active target %s, got %s", currentHost, result.ActiveTargets[1].Path)
	}

	// InstallTargets should only contain AllHosts — CurrentHost is never installed.
	if len(result.InstallTargets) != 1 {
		t.Fatalf("expected 1 install target, got %d", len(result.InstallTargets))
	}
	if result.InstallTargets[0].Path != allHosts {
		t.Errorf("expected install target %s, got %s", allHosts, result.InstallTargets[0].Path)
	}

}

func TestDiscoverPowerShellProfiles_BothEditions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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
	setupDocsMock(t, home)

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
	setupDocsMock(t, home)

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

	// UNC paths are outside the user's home directory and should be rejected
	// by the path containment check. The Known Documents fallback should then
	// provide valid targets.
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets when UNC path rejected")
	}
	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true when UNC path rejected")
	}
	// Active targets should NOT be the UNC path
	for _, target := range result.ActiveTargets {
		if strings.HasPrefix(target.Path, `\\`) {
			t.Errorf("UNC path should have been rejected, got %s", target.Path)
		}
	}
}

func TestDiscoverPowerShellProfiles_PathWithSpaces(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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
	setupDocsMock(t, home)

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
	setupDocsMock(t, home)

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
	// Known Documents fallback should provide targets when discovery fails.
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets when discovery fails")
	}
	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true")
	}
	// InstallTargets must also be populated so installation can proceed.
	if len(result.InstallTargets) == 0 {
		t.Error("expected fallback install targets when discovery fails")
	}
}

func TestDiscoverPowerShellProfiles_DiscoveryTimeout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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

	// Known Documents fallback should provide targets when both editions time out.
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets when discovery times out")
	}
	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true")
	}
	if len(result.DiscoveryWarnings) < 2 {
		t.Errorf("expected at least 2 discovery warnings, got %d", len(result.DiscoveryWarnings))
	}
	// InstallTargets must also be populated so installation can proceed.
	if len(result.InstallTargets) == 0 {
		t.Error("expected fallback install targets when discovery times out")
	}
}

func TestDiscoverPowerShellProfiles_BothMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// Known Documents fallback should provide targets when both executables missing.
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets when both executables missing")
	}
	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true")
	}
	if len(result.DiscoveryWarnings) == 0 {
		t.Error("expected discovery warnings when both executables missing")
	}
	// InstallTargets must also be populated so installation can proceed.
	if len(result.InstallTargets) == 0 {
		t.Error("expected fallback install targets when both executables missing")
	}
}

func TestDiscoverPowerShellProfiles_CaseInsensitiveDedup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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

	// ActiveTargets should not contain duplicates (case-insensitive)
	seen := make(map[string]bool)
	var dup bool
	for _, t := range result.ActiveTargets {
		key := strings.ToLower(t.Path)
		if seen[key] {
			dup = true
		}
		seen[key] = true
	}
	if dup {
		t.Error("active targets should be deduplicated case-insensitively")
	}
}

func TestDiscoverPowerShellProfiles_NoExecutable_DiscoveryFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	// Known Documents fallback should provide targets when no executables found.
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets when no executables found")
	}
	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true")
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

func TestCheckPowerShellActivation_Healthy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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

func TestCheckPowerShellActivation_NoActivation_DiscoveredProfileOnly(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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
	setupDocsMock(t, home)

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
	docsDir := filepath.Join(home, "Documents")
	validPath := filepath.Join(docsDir, "PowerShell", "profile.ps1")
	validSpaces := filepath.Join(docsDir, "My PowerShell", "profile.ps1")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"valid path", validPath, validPath},
		{"empty", "", ""},
		{"whitespace", "  ", ""},
		{"dot", ".", ""},
		{"with spaces", validSpaces, validSpaces},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateAndCanonicalizePath(tt.in, docsDir)
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

func TestInstallPowerShellActivation_Idempotent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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
	setupDocsMock(t, home)

	allHosts := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(
		"export FOO=bar\n"+Block(ShellPowerShell)+"\nalias x='y'",
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
	if !strings.Contains(content, "export FOO=bar") {
		t.Error("unrelated content should be preserved")
	}
	if !strings.Contains(content, "alias x='y'") {
		t.Error("unrelated content should be preserved")
	}
}

func TestUninstallPowerShellActivation_HistoricalProfiles(t *testing.T) {
	// When PowerShell discovers a redirected profile (e.g. OneDrive),
	// the historical WindowsPowerShell profile is placed in MigrationTargets
	// but not ActiveTargets. Uninstall should still clean up the historical
	// profile.
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Mock Documents folder to be home/OneDrive/Documents.
	docsDir := filepath.Join(home, "OneDrive", "Documents")
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return docsDir, nil
	})
	defer ResetResolveDocumentsFolderForTesting()

	// Discovered profile (OneDrive-redirected, PS7)
	allHosts := filepath.Join(docsDir, "PowerShell", "Microsoft.PowerShell_profile.ps1")
	// Historical profile (WindowsPowerShell — in MigrationTargets, not ActiveTargets)
	historical := filepath.Join(docsDir, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")

	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.MkdirAll(filepath.Dir(historical)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(Block(ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(historical), historical, []byte(Block(ShellPowerShell))); err != nil {
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

	// Historical profile should also be cleaned
	data, err := safepath.ReadFile(filepath.Dir(historical), historical)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if HasBlock(string(data)) {
		t.Error("historical profile block should be removed")
	}
}

func TestInstallPowerShellActivation_HostSpecificOverride(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

	// PS7 AllHosts (authoritative)
	ps7All := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	// PS5.1 AllHosts
	ps5All := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
	// PS5.1 CurrentHost (host-specific, loads after AllHosts — must NOT be installed)
	ps5Host := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile_5host.ps1")

	if err := safepath.MkdirAll(filepath.Dir(ps7All)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.MkdirAll(filepath.Dir(ps5All)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.MkdirAll(filepath.Dir(ps5Host)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	// PS7 profile is empty
	if err := safepath.WriteFile(filepath.Dir(ps7All), ps7All, []byte("")); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	// PS5.1 AllHosts profile is empty
	if err := safepath.WriteFile(filepath.Dir(ps5All), ps5All, []byte("")); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	// PS5.1 CurrentHost profile has unrelated content
	if err := safepath.WriteFile(filepath.Dir(ps5Host), ps5Host, []byte("export PS5HOST=value")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       makePSOutput(ps7All, ps7All),
			"powershell.exe": makePSOutput(ps5All, ps5Host),
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

	// PS7 AllHosts should have the activation block
	data, err := safepath.ReadFile(filepath.Dir(ps7All), ps7All)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !HasBlock(string(data)) {
		t.Error("current block should be installed in PS7 AllHosts profile")
	}

	// PS5.1 AllHosts should have the activation block
	data, err = safepath.ReadFile(filepath.Dir(ps5All), ps5All)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !HasBlock(string(data)) {
		t.Error("current block should be installed in PS5.1 AllHosts profile")
	}

	// PS5.1 CurrentHost must NOT have the activation block — it loads after
	// AllHosts and can override or retain a stale duplicate.
	data, err = safepath.ReadFile(filepath.Dir(ps5Host), ps5Host)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if HasBlock(string(data)) {
		t.Error("activation block must NOT be installed in CurrentHost profile")
	}
	if !strings.Contains(string(data), "export PS5HOST=value") {
		t.Error("CurrentHost unrelated content should be preserved")
	}
}

func TestDiscoveryTimeout_WithMock(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

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

	// Known Documents fallback should provide targets when both editions time out.
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets when both editions time out")
	}
	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true")
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
	setupDocsMock(t, home)

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
	p := validateAndCanonicalizePath(filepath.Join(home, "über", "profile.ps1"), "")
	if p == "" {
		t.Error("Unicode path should be valid")
	}
}

func TestValidateAndCanonicalizePath_Spaces(t *testing.T) {
	home := t.TempDir()
	p := validateAndCanonicalizePath("  "+filepath.Join(home, "My Documents", "profile.ps1")+"  ", "")
	if p == "" {
		t.Error("path with spaces should be valid")
	}
}

func TestValidateAndCanonicalizePath_PathContainment(t *testing.T) {
	home := t.TempDir()
	docsDir := filepath.Join(home, "Documents")

	// Valid path under baseDir
	validPath := filepath.Join(docsDir, "PowerShell", "Microsoft.PowerShell_profile.ps1")
	p := validateAndCanonicalizePath(validPath, docsDir)
	if p != validPath {
		t.Errorf("expected valid path, got %q", p)
	}

	// Path outside baseDir should be rejected
	escapedPath := filepath.Join(home, "AppData", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	p = validateAndCanonicalizePath(escapedPath, docsDir)
	if p != "" {
		t.Errorf("expected empty string for escaped path, got %q", p)
	}

	// Path traversal attempt should be rejected
	traversal := filepath.Join(docsDir, "..", "AppData", "PowerShell", "profile.ps1")
	p = validateAndCanonicalizePath(traversal, docsDir)
	if p != "" {
		t.Errorf("expected empty string for traversal path, got %q", p)
	}

	// No baseDir means no containment check (backwards compat)
	escapedPath2 := filepath.Join(home, "AppData", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	p = validateAndCanonicalizePath(escapedPath2, "")
	if p != escapedPath2 {
		t.Errorf("expected path without baseDir check, got %q", p)
	}
}

// Test the OneDrive redirect scenario: install to a redirected profile,
// verify doctor reports healthy after install, and idempotency.
func TestOneDriveRedirect_FullFlow(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)
	setupDocsMock(t, home)

	// Actual discovered PowerShell profile (redirected Documents)
	realProfile := filepath.Join(home, "redirected-documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(realProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}

	// Real profile has unrelated content
	if err := safepath.WriteFile(filepath.Dir(realProfile), realProfile, []byte(
		"export FOO=bar\nalias x='y'",
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

	// Before apply: doctor should NOT report activation as healthy (no activation block)
	healthy, _, _ := CheckPowerShellActivation(home)
	if healthy {
		t.Error("doctor should NOT report activation healthy before apply")
	}

	// Apply
	installed, err := InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("InstallPowerShellActivation: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one path to be installed")
	}

	// After apply: real profile should have the current block
	data, err := safepath.ReadFile(filepath.Dir(realProfile), realProfile)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)

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

// Test that when both PowerShell editions fail, the Known Documents
// fallback provides valid installation targets.
func TestKnownDocumentsFallback_InstallTargets(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Mock resolveDocumentsFolder to return a known path
	docsDir := filepath.Join(home, "Documents")
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return docsDir, nil
	})
	defer ResetResolveDocumentsFolderForTesting()

	// Both editions fail
	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	result := discoverPowerShellProfiles()

	if !result.UsedFallback {
		t.Error("expected UsedFallback to be true")
	}
	if len(result.InstallTargets) == 0 {
		t.Error("expected fallback install targets")
	}
	if len(result.ActiveTargets) == 0 {
		t.Error("expected fallback active targets")
	}

	// Install targets should be under the Known Documents path
	for _, target := range result.InstallTargets {
		if !strings.HasPrefix(target.Path, docsDir) {
			t.Errorf("expected install target under %s, got %s", docsDir, target.Path)
		}
	}
}

// Test that fallback discovery installs the activation block and that
// installation actually creates the profile blocks when both PowerShell
// probes fail.
func TestFallbackInstallation_ActualProfileCreated(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	makeHome(t, home)

	// Mock the Known Folder API to return a path under the temp dir.
	docsDir := filepath.Join(home, "Documents")
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return docsDir, nil
	})
	defer ResetResolveDocumentsFolderForTesting()

	// Both PowerShell editions fail — forces Known Documents fallback.
	runner := &mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	}
	SetPSRunnerForTesting(runner)
	defer ResetPSRunnerForTesting()

	psResult := discoverPowerShellProfiles()

	// Verify fallback install targets are populated.
	if len(psResult.InstallTargets) == 0 {
		t.Fatal("expected fallback install targets")
	}

	// Ensure the AllHosts fallback target is present.
	hasAllHosts := false
	for _, target := range psResult.InstallTargets {
		if strings.HasSuffix(target.Path, "Microsoft.PowerShell_profile.ps1") {
			hasAllHosts = true
			break
		}
	}
	if !hasAllHosts {
		t.Error("expected AllHosts fallback target in InstallTargets")
	}

	// Now verify that InstallPowerShellActivationWith actually writes the block.
	installed, err := InstallPowerShellActivationWith(home, &psResult)
	if err != nil {
		t.Fatalf("InstallPowerShellActivationWith: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected at least one path to be installed on fallback")
	}

	// Verify the file was actually created with the block.
	for _, target := range psResult.InstallTargets {
		data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
		if err != nil {
			t.Fatalf("reading fallback profile %s: %v", target.Path, err)
		}
		if !HasBlock(string(data)) {
			t.Errorf("fallback profile %s should contain activation block", target.Path)
		}
	}
}
