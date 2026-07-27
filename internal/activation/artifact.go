// artifact.go — Legacy v4.2.6 artifact detection and recovery.

package activation

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// legacyNames holds platform-specific file names for v4.2.6 artifacts.
type legacyNames struct {
	claude          string
	backup          string
	legacyLaunchers []string
	commandNames    []string
}

func platformNames() legacyNames {
	if runtime.GOOS == "windows" {
		return legacyNames{
			claude:          "claude.cmd",
			backup:          "claude.juggernaut-original.cmd",
			legacyLaunchers: []string{"juggernaut-claude.cmd"},
			commandNames:    []string{"claude.exe", "claude.cmd", "claude.bat"},
		}
	}
	return legacyNames{
		claude:          "claude",
		backup:          "claude.juggernaut-original",
		legacyLaunchers: []string{"juggernaut-claude"},
		commandNames:    []string{"claude"},
	}
}

// RecoverLegacyArtifacts removes or restores only positively identified
// v4.2.6 launcher artifacts.
func RecoverLegacyArtifacts(binDir string) ([]LegacyAction, error) {
	self, _ := os.Executable()
	actions, err := recoverPlatformArtifacts(binDir, self, platformNames())
	if err != nil {
		return actions, err
	}
	return actions, nil
}

// DetectLegacyArtifacts reports recoverable or removable v4.2.6 artifacts.
func DetectLegacyArtifacts(binDir string) []LegacyAction {
	self, _ := os.Executable()
	return detectPlatformArtifacts(binDir, self, platformNames())
}

// isKnownJuggernautArtifact returns true if the file at path is a known
// v4.2.6 Juggernaut artifact (legacy shim or symlink to the juggernaut binary).
func isKnownJuggernautArtifact(path, self string) bool {
	if runtime.GOOS == "windows" {
		data, err := os.ReadFile(path) // #nosec G703,G501 -- path is resolved from known config paths, not user input // nosemgrep: go_filesystem_rule-fileread -- path derived from known config paths
		if err != nil {
			return false
		}
		content := string(data)
		// Normalize line endings for comparison — legacy shims may have LF or
		// CRLF endings, and may have trailing whitespace or extra blank lines.
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.TrimSpace(content)
		// Normalize the constants the same way for a fair comparison.
		normalizedLF := strings.TrimSpace(strings.ReplaceAll(legacyCmdShimLF, "\r\n", "\n"))
		normalizedCRLF := strings.TrimSpace(strings.ReplaceAll(legacyCmdShimCRLF, "\r\n", "\n"))
		return content == normalizedLF || content == normalizedCRLF
	}

	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	if self == "" {
		return false
	}
	return samePath(target, self)
}

// isLegacyClaudeShim returns true if the file at path is a legacy v4.2.6
// claude.cmd/claude.bat shim that invokes the removed juggernaut --launcher
// path. These shims must be rejected so they are not selected as the real
// Claude Code binary.
//
//nolint:unused // retained for future v4.2.6 artifact recovery
func isLegacyClaudeShim(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	data, err := os.ReadFile(path) // #nosec G703,G501 -- path is resolved from known config paths, not user input // nosemgrep: go_filesystem_rule-fileread -- path derived from known config paths
	if err != nil {
		return false
	}
	content := string(data)
	// Normalize line endings for comparison — legacy shims may have LF or
	// CRLF endings, and may have trailing whitespace or extra blank lines.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	return strings.Contains(content, "juggernaut --launcher")
}

func recoverPlatformArtifacts(binDir, self string, names legacyNames) ([]LegacyAction, error) {
	var actions []LegacyAction
	for _, name := range names.legacyLaunchers {
		path := filepath.Join(binDir, name)
		if isKnownJuggernautArtifact(path, self) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return actions, fmt.Errorf("removing %s: %w", path, err)
			}
			actions = append(actions, LegacyAction{Path: path, Action: "removed legacy Juggernaut launcher"})
		}
	}

	claudePath := filepath.Join(binDir, names.claude)
	backupPath := filepath.Join(binDir, names.backup)
	if isKnownJuggernautArtifact(claudePath, self) {
		if err := os.Remove(claudePath); err != nil && !os.IsNotExist(err) {
			return actions, fmt.Errorf("removing %s: %w", claudePath, err)
		}
		actions = append(actions, LegacyAction{Path: claudePath, Action: "removed legacy Juggernaut claude shim"})
	}
	if fileExists(backupPath) && !fileExists(claudePath) {
		if err := os.Rename(backupPath, claudePath); err != nil {
			return actions, fmt.Errorf("restoring %s: %w", claudePath, err)
		}
		actions = append(actions, LegacyAction{Path: claudePath, Action: "restored Claude Code binary from v4.2.6 backup"})
	}
	return actions, nil
}

func detectPlatformArtifacts(binDir, self string, names legacyNames) []LegacyAction {
	var actions []LegacyAction
	for _, name := range names.legacyLaunchers {
		path := filepath.Join(binDir, name)
		if isKnownJuggernautArtifact(path, self) {
			actions = append(actions, LegacyAction{Path: path, Action: "legacy Juggernaut launcher detected"})
		}
	}

	claudePath := filepath.Join(binDir, names.claude)
	backupPath := filepath.Join(binDir, names.backup)
	if isKnownJuggernautArtifact(claudePath, self) {
		actions = append(actions, LegacyAction{Path: claudePath, Action: "legacy Juggernaut claude shim detected"})
	}
	if fileExists(backupPath) && !fileExists(claudePath) {
		actions = append(actions, LegacyAction{Path: claudePath, Action: "Claude Code backup can be restored"})
	}
	return actions
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
