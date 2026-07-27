package activation

import (
	"fmt"
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

// --- iterateAllTargets ---

func TestIterateAllTargets_WithPSResult(t *testing.T) {
	home := t.TempDir()
	psResult := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: "/ps/all-hosts.ps1", Shell: ShellPowerShell},
			{Path: "/ps/current-host.ps1", Shell: ShellPowerShell},
		},
	}

	var visited []string
	_, err := iterateAllTargets(home, psResult, func(target Target) (bool, error) {
		visited = append(visited, target.Path)
		return false, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PowerShell active targets come first, then POSIX defaults.
	wantLen := 2 + len(DefaultTargets(home))
	if len(visited) != wantLen {
		t.Fatalf("visited %d targets, want %d", len(visited), wantLen)
	}
	// PowerShell targets are visited before POSIX targets.
	if visited[0] != "/ps/all-hosts.ps1" {
		t.Errorf("first visited = %q, want /ps/all-hosts.ps1", visited[0])
	}
	if visited[1] != "/ps/current-host.ps1" {
		t.Errorf("second visited = %q, want /ps/current-host.ps1", visited[1])
	}
}

func TestIterateAllTargets_NilPSResult(t *testing.T) {
	home := t.TempDir()

	var visited []string
	_, err := iterateAllTargets(home, nil, func(target Target) (bool, error) {
		visited = append(visited, target.Path)
		return false, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With nil psResult, only POSIX defaults are visited.
	if len(visited) != len(DefaultTargets(home)) {
		t.Fatalf("visited %d targets, want %d (DefaultTargets)", len(visited), len(DefaultTargets(home)))
	}
}

func TestIterateAllTargets_CollectsTrueReturns(t *testing.T) {
	home := t.TempDir()
	psResult := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: "/ps/profile.ps1", Shell: ShellPowerShell},
		},
	}

	collected, err := iterateAllTargets(home, psResult, func(target Target) (bool, error) {
		// Only collect the PowerShell target; skip POSIX defaults.
		return target.Shell == ShellPowerShell, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collected) != 1 {
		t.Fatalf("collected %d paths, want 1", len(collected))
	}
	if collected[0] != "/ps/profile.ps1" {
		t.Errorf("collected[0] = %q, want /ps/profile.ps1", collected[0])
	}
}

func TestIterateAllTargets_StopsOnError(t *testing.T) {
	home := t.TempDir()
	psResult := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: "/ps/first.ps1", Shell: ShellPowerShell},
			{Path: "/ps/second.ps1", Shell: ShellPowerShell},
		},
	}

	var visited []string
	_, err := iterateAllTargets(home, psResult, func(target Target) (bool, error) {
		visited = append(visited, target.Path)
		if target.Path == "/ps/second.ps1" {
			return false, fmt.Errorf("simulated failure")
		}
		return false, nil
	})
	if err == nil {
		t.Fatal("expected error from callback")
	}
	// Iteration should stop at the error; the remaining targets are never visited.
	if len(visited) != 2 {
		t.Errorf("visited %d targets before error, want 2", len(visited))
	}
}

func TestIterateAllTargets_PowerShellBeforePOSIX(t *testing.T) {
	home := t.TempDir()
	psResult := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: "/ps/profile.ps1", Shell: ShellPowerShell},
		},
	}

	psIdx := -1
	firstPOSIXIdx := -1
	idx := 0
	_, err := iterateAllTargets(home, psResult, func(target Target) (bool, error) {
		if target.Shell == ShellPowerShell && psIdx == -1 {
			psIdx = idx
		}
		if target.Shell != ShellPowerShell && firstPOSIXIdx == -1 {
			firstPOSIXIdx = idx
		}
		idx++
		return false, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if psIdx == -1 {
		t.Fatal("expected at least one PowerShell target")
	}
	if firstPOSIXIdx == -1 {
		t.Fatal("expected at least one POSIX target")
	}
	if psIdx >= firstPOSIXIdx {
		t.Errorf("PowerShell target visited at index %d, first POSIX at %d — PowerShell should come first", psIdx, firstPOSIXIdx)
	}
}

func TestIterateAllTargets_EmptyPSResult(t *testing.T) {
	home := t.TempDir()
	psResult := &ProfileResolverResult{} // non-nil but empty ActiveTargets

	var visited []string
	_, err := iterateAllTargets(home, psResult, func(target Target) (bool, error) {
		visited = append(visited, target.Path)
		return false, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty ActiveTargets means only POSIX defaults are visited.
	if len(visited) != len(DefaultTargets(home)) {
		t.Fatalf("visited %d targets, want %d (DefaultTargets only)", len(visited), len(DefaultTargets(home)))
	}
}
