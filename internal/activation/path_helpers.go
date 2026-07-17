package activation

import (
	"path/filepath"
	"runtime"
	"strings"
)

// containsPathCI checks if a path exists in a list, case-insensitive on Windows.
func containsPathCI(paths []string, path string) bool {
	for _, p := range paths {
		if runtime.GOOS == "windows" {
			if strings.EqualFold(p, path) {
				return true
			}
		} else if p == path {
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
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(p)
		}
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
	if baseDir != "" {
		rel, err := filepath.Rel(baseDir, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
	}
	return p
}

// isPathUnderBase checks if a path is under the given base directory.
func isPathUnderBase(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
