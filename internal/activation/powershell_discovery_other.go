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
func validateAndCanonicalizePath(p, baseDir string) string {
	p = filepath.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

// SetPSRunnerForTesting and ResetPSRunnerForTesting are no-ops on non-Windows.
func SetPSRunnerForTesting(r interface{}) {
}

func ResetPSRunnerForTesting() {
}
