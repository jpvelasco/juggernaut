package activation

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// pathKey normalizes a path for set membership. On Windows, comparison is
// case-insensitive after Clean so paths that differ only by casing still match.
func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

// containsPathCI checks if a path exists in a list, case-insensitive on Windows.
func containsPathCI(paths []string, path string) bool {
	key := pathKey(path)
	for _, p := range paths {
		if pathKey(p) == key {
			return true
		}
	}
	return false
}

// containsTargetPathCI checks if a path exists in a []Target,
// case-insensitive on Windows.
func containsTargetPathCI(targets []Target, path string) bool {
	key := pathKey(path)
	for _, t := range targets {
		if pathKey(t.Path) == key {
			return true
		}
	}
	return false
}

// deduplicatePathsCI removes duplicate paths, case-insensitive on Windows.
func deduplicatePathsCI(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var result []string
	for _, p := range paths {
		key := pathKey(p)
		if !seen[key] {
			seen[key] = true
			result = append(result, p)
		}
	}
	return result
}

// validateAndCanonicalizePath trims and cleans a path, returning empty string
// for invalid paths. If baseDir is non-empty, the path must be under baseDir
// (checked via filepath.Rel) to prevent path traversal attacks from a
// compromised PowerShell output.
func validateAndCanonicalizePath(p, baseDir string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if p == "." {
		return ""
	}
	if baseDir != "" && !safepath.IsUnderBase(baseDir, p) {
		return ""
	}
	return p
}
