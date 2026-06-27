//go:build !windows

package activation

import (
	"path/filepath"
)

// ProfileResolverResult is the shared, authoritative result of profile
// discovery. On non-Windows platforms it returns the same targets as
// DefaultTargets (no dynamic discovery is needed).
type ProfileResolverResult struct {
	ActiveTargets      []Target
	InstallTarget      Target
	MigrationTargets   []string
	DiscoveryWarnings  []string
	UsedFallback       bool
	EditionsDiscovered []string
}

// discoverPowerShellProfiles is a no-op on non-Windows; it returns the same
// targets as DefaultTargets.
func discoverPowerShellProfiles() ProfileResolverResult {
	targets := DefaultTargets("")
	result := ProfileResolverResult{
		ActiveTargets: targets,
	}
	if len(targets) > 0 {
		result.InstallTarget = targets[0]
		for _, t := range targets {
			result.MigrationTargets = append(result.MigrationTargets, t.Path)
		}
	}
	result.EditionsDiscovered = []string{"non-windows"}
	return result
}

// historicalPowerShellTargets returns nil on non-Windows.
func historicalPowerShellTargets() []string {
	return nil
}

// historicalPowerShellTargetsScoped returns nil on non-Windows.
func historicalPowerShellTargetsScoped(home string) []string {
	return nil
}

// discoverPowerShellProfilesScoped is a no-op on non-Windows.
func discoverPowerShellProfilesScoped(home string) ProfileResolverResult {
	return discoverPowerShellProfiles()
}

// containsPathCI checks if a path exists in a list.
func containsPathCI(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

// deduplicatePathsCI removes duplicate paths.
func deduplicatePathsCI(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var result []string
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}

// validateAndCanonicalizePath trims and cleans a path.
func validateAndCanonicalizePath(p string) string {
	p = filepath.Clean(p)
	if p == "." || p == "\"\"" {
		return ""
	}
	return p
}

// SetPSRunnerForTesting and ResetPSRunnerForTesting are no-ops on non-Windows.
func SetPSRunnerForTesting(r interface{}) {
}

func ResetPSRunnerForTesting() {
}
