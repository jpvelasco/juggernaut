// Package launcher installs and operates the claude credential-injecting shim.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jpvelasco/juggernaut/v4/internal/keychain"
	"github.com/jpvelasco/juggernaut/v4/internal/safepath"
)

const cmdShim = "@echo off\njuggernaut --launcher %*\n"

// DefaultBinDir returns the platform-appropriate bin directory for the claude shim.
func DefaultBinDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	path, err := safepath.JoinUnder(home, ".local", "bin")
	if err != nil {
		return filepath.Join(home, ".local", "bin")
	}
	return path
}

// Install creates the claude shim in binDir (symlink on Unix, .cmd on Windows).
func Install(binDir string) error {
	if err := safepath.MkdirAll(binDir); err != nil {
		return fmt.Errorf("creating bin dir: %w", err)
	}

	if runtime.GOOS == "windows" {
		shimPath := filepath.Join(binDir, "claude.cmd")
		return os.WriteFile(shimPath, []byte(cmdShim), 0o600)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	shimPath := filepath.Join(binDir, "claude")
	if err := removeIfExists(shimPath); err != nil {
		return fmt.Errorf("removing existing claude shim: %w", err)
	}
	return os.Symlink(self, shimPath)
}

// Uninstall removes the claude shim from binDir.
func Uninstall(binDir string) error {
	if runtime.GOOS == "windows" {
		return removeIfExists(filepath.Join(binDir, "claude.cmd"))
	}
	return removeIfExists(filepath.Join(binDir, "claude"))
}

// IsInstalled returns true if the claude shim exists in binDir.
func IsInstalled(binDir string) bool {
	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(binDir, "claude.cmd")
	} else {
		path = filepath.Join(binDir, "claude")
	}
	_, err := os.Lstat(path)
	return err == nil
}

// RunAsLauncher injects credentials and execs the real claude binary.
func RunAsLauncher(args []string) error {
	token, err := keychain.Default().Get()
	if err != nil {
		// Keychain unavailable (e.g. headless Linux without Secret Service).
		// For IAM auth this is fine — no bearer token is needed.
		// We proceed and let Claude Code fail if it actually needs a credential.
		token = ""
	}
	if token != "" {
		_ = os.Setenv("AWS_BEARER_TOKEN_BEDROCK", token)
	}
	_ = os.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")

	originalPath := os.Getenv("PATH")
	scrubbed := scrubPathEntry(originalPath, DefaultBinDir())
	if err := os.Setenv("PATH", scrubbed); err != nil {
		return fmt.Errorf("scrubbing PATH for claude lookup: %w", err)
	}
	defer func() { _ = os.Setenv("PATH", originalPath) }()

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude binary not found on PATH — is Claude Code installed?")
	}

	cmd := exec.Command("claude", args...)
	cmd.Path = claudePath
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func scrubPathEntry(pathList, dropDir string) string {
	if pathList == "" || dropDir == "" {
		return pathList
	}
	dropDir = filepath.Clean(dropDir)
	var kept []string
	for _, dir := range filepath.SplitList(pathList) {
		if filepath.Clean(dir) == dropDir {
			continue
		}
		kept = append(kept, dir)
	}
	return strings.Join(kept, string(filepath.ListSeparator))
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}