package activation

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// v3InstallDirMarker is the file the v3 PowerShell installer wrote alongside its
// shim to record the install directory.
const v3InstallDirMarker = "juggernaut-install-dir.txt"

// shimDelegationTarget matches the absolute path a legacy launcher shim delegates
// to — either PowerShell `$target = '...'` or a `.cmd`'s leading `"..."` call.
// Both v3 (~/.juggernaut) and v4 npm-bridge shims use one of these forms.
var shimDelegationTargets = []*regexp.Regexp{
	regexp.MustCompile(`(?m)\$target\s*=\s*['"]([^'"]+)['"]`),
	regexp.MustCompile(`(?m)^\s*"([^"]+\.(?:ps1|cmd|exe))"`),
}

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
		stale, readErr := staleShimReason(path)
		switch {
		case stale != "":
			found = append(found, path+" ("+stale+")")
		case readErr != nil:
			// The file exists but couldn't be read; surface it conservatively
			// rather than silently missing a possible stale shim.
			found = append(found, path+" (unreadable; possible legacy shim)")
		}
	}

	if len(found) == 0 {
		return false, ""
	}
	return true, strings.Join(found, "; ") +
		" — legacy launcher shim(s) shadowing the npm binary; remove the listed file(s), then run `npm install -g juggernaut-bedrock` if needed"
}

// staleShimReason returns a short reason when the file at path is a stale
// Juggernaut launcher shim, or "" when it is not (or doesn't exist). A shim is
// stale if it delegates to a ~/.juggernaut tree (the v3 layout) or to an
// absolute target path that no longer exists (the v4 npm-bridge layout, whose
// hardcoded target breaks after the real binary moves). It returns a non-nil
// error only when the file exists but cannot be read.
func staleShimReason(path string) (string, error) {
	if !fileExists(path) {
		return "", nil
	}
	data, err := os.ReadFile(path) // #nosec G304 // nosemgrep: gosec.G304-1, go_filesystem_rule-fileread -- path = constrained binDir (DefaultBinDir via safepath) + one of three fixed shim names
	if err != nil {
		return "", err
	}
	body := string(data)
	if strings.Contains(body, ".juggernaut") {
		return "legacy v3 shim", nil
	}
	if target := shimDelegationTarget(body); target != "" && !fileExists(target) {
		return "delegates to missing target " + target, nil
	}
	return "", nil
}

// shimDelegationTarget extracts the path a launcher shim delegates to, or ""
// if the body isn't a recognizable delegating shim.
func shimDelegationTarget(body string) string {
	for _, re := range shimDelegationTargets {
		if m := re.FindStringSubmatch(body); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}
