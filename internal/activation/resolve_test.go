package activation

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveOrUse_PreResolvedProvided(t *testing.T) {
	expected := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: "/home/user/.config/powershell/MicrosoftProfile.ps1", Shell: ShellPowerShell},
		},
		InstallTargets: []Target{
			{Path: "/home/user/.config/powershell/MicrosoftProfile.ps1", Shell: ShellPowerShell},
		},
		MigrationTargets:   []string{"/home/user/old-profile.ps1"},
		DiscoveryWarnings:  []string{"no powershell"},
		UsedFallback:       true,
		EditionsDiscovered: []string{"core"},
	}

	got := resolveOrUse("/home/user", expected)
	if got == nil {
		t.Fatal("expected non-nil result when pre-resolved result is provided")
	}
	if got != expected {
		t.Error("expected the same pointer returned when pre-resolved result is provided")
	}
	if len(got.ActiveTargets) != 1 {
		t.Errorf("expected 1 active target, got %d", len(got.ActiveTargets))
	}
}

func TestResolveOrUse_NilOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test is for non-Windows platforms")
	}

	got := resolveOrUse("/home/user", nil)
	if got != nil {
		t.Error("expected nil result on non-Windows when psResult is nil")
	}
}

func TestResolveOrUse_NilOnWindowsCallsResolve(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test is for Windows platforms")
	}

	home := t.TempDir()
	got := resolveOrUse(home, nil)

	// On Windows with nil psResult, resolveOrUse calls
	// ResolvePowerShellProfilesScoped which internally calls
	// discoverPowerShellProfilesScoped. The result should be non-nil.
	if got == nil {
		t.Fatal("expected non-nil result on Windows (resolve was called)")
	}
	// The returned result's paths should be under the given home directory.
	for _, target := range got.ActiveTargets {
		if !filepath.IsAbs(target.Path) {
			t.Errorf("expected absolute path in active target, got %q", target.Path)
		}
	}
}

func TestResolveOrUse_PreResolvedIgnoresHome(t *testing.T) {
	// When a pre-resolved result is provided, the home parameter should be
	// irrelevant — resolveOrUse returns the pre-resolved result directly.
	expected := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: "/different/path/profile.ps1", Shell: ShellPowerShell},
		},
	}

	got := resolveOrUse("/completely/different/home", expected)
	if got != expected {
		t.Error("expected pre-resolved result returned unchanged")
	}
	if got.ActiveTargets[0].Path != "/different/path/profile.ps1" {
		t.Errorf("expected original path, got %q", got.ActiveTargets[0].Path)
	}
}

func TestResolveOrUse_PreResolvedWithEmptyResult(t *testing.T) {
	// A pre-resolved result with empty fields is still returned (not nil).
	expected := &ProfileResolverResult{}

	got := resolveOrUse("/home/user", expected)
	if got == nil {
		t.Fatal("expected non-nil result even when pre-resolved result has empty fields")
	}
	if got != expected {
		t.Error("expected the same pointer returned")
	}
}
