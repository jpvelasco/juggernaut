//go:build !windows

package activation

import (
	"os"
	"path/filepath"
)

// discoverPowerShellProfiles is a no-op on non-Windows; it returns the same
// targets as DefaultTargets.
func discoverPowerShellProfiles() ProfileResolverResult {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	targets := DefaultTargets(home)
	result := ProfileResolverResult{
		ActiveTargets: targets,
	}
	result.EditionsDiscovered = []string{"non-windows"}
	return result
}

// discoverPowerShellProfilesScoped is a no-op on non-Windows.
func discoverPowerShellProfilesScoped(home string) ProfileResolverResult {
	return discoverPowerShellProfiles()
}

// SetPSRunnerForTesting and ResetPSRunnerForTesting are no-ops on non-Windows.
func SetPSRunnerForTesting(r interface{}) {
}

func ResetPSRunnerForTesting() {
}
