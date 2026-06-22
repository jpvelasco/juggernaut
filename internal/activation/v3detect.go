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
		targets, readErr := shimTargetsJuggernautHome(path)
		switch {
		case targets:
			found = append(found, path)
		case readErr != nil:
			// The file exists but couldn't be read; surface it conservatively
			// rather than silently missing a possible v3 artifact.
			found = append(found, path+" (unreadable; possible v3 shim)")
		}
	}

	if len(found) == 0 {
		return false, ""
	}
	return true, strings.Join(found, "; ") +
		" — legacy v3 install detected; reinstall with `npm install -g juggernaut-bedrock` and remove these stale shims"
}

// shimTargetsJuggernautHome reports whether the file at path is a shim that
// delegates to a ~/.juggernaut install tree (the v3 layout). It returns a
// non-nil error when the file exists but could not be read, so callers can
// distinguish "definitely not a v3 shim" from "couldn't tell".
func shimTargetsJuggernautHome(path string) (bool, error) {
	if !fileExists(path) {
		return false, nil
	}
	data, err := os.ReadFile(path) // #nosec G304 // nosemgrep: gosec.G304-1, go_filesystem_rule-fileread -- path = constrained binDir (DefaultBinDir via safepath) + one of three fixed shim names
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), ".juggernaut"), nil
}
