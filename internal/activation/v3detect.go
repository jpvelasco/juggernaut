package activation

import (
	"os"
	"path/filepath"
	"strings"
)

// v3InstallDirMarker is the file the v3 PowerShell installer wrote alongside its
// shim to record the install directory.
const v3InstallDirMarker = "juggernaut-install-dir.txt"

// DetectV3Install reports whether binDir contains artifacts from a pre-v5
// (PowerShell/Bash installer) Juggernaut install. v5 ships only as an npm
// package, so a leftover v3 shim shadows the npm binary on PATH and breaks once
// the old install tree is upgraded. Returns a human-readable detail describing
// what was found and how to migrate.
func DetectV3Install(binDir string) (bool, string) {
	var found []string

	marker := filepath.Join(binDir, v3InstallDirMarker)
	if fileExists(marker) {
		found = append(found, marker)
	}

	for _, name := range []string{"juggernaut.ps1", "juggernaut.cmd", "juggernaut"} {
		path := filepath.Join(binDir, name)
		if shimTargetsJuggernautHome(path) {
			found = append(found, path)
		}
	}

	if len(found) == 0 {
		return false, ""
	}
	return true, strings.Join(found, "; ") +
		" — legacy v3 install detected; reinstall with `npm install -g juggernaut-bedrock` and remove these stale shims"
}

// shimTargetsJuggernautHome reports whether the file at path is a shim that
// delegates to a ~/.juggernaut install tree (the v3 layout).
func shimTargetsJuggernautHome(path string) bool {
	if !fileExists(path) {
		return false
	}
	data, err := os.ReadFile(path) // #nosec G304 // nosemgrep: gosec.G304-1, go_filesystem_rule-fileread -- path is a fixed bin dir + fixed shim names
	if err != nil {
		return false
	}
	return strings.Contains(string(data), ".juggernaut")
}
