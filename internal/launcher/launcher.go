// Package launcher installs and operates the claude credential-injecting shim.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jpvelasco/juggernaut/internal/keychain"
)

const cmdShim = "@echo off\njuggernaut --launcher %*\n"

// DefaultBinDir returns the platform-appropriate bin directory for the claude shim.
func DefaultBinDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("USERPROFILE"), ".local", "bin")
	}
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".local", "bin")
}

// Install creates the claude shim in binDir (symlink on Unix, .cmd on Windows).
func Install(binDir string) error {
	if err := os.MkdirAll(binDir, 0o700); err != nil {
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

	claudePath, err := findRealClaude()
	if err != nil {
		return err
	}

	cmd := exec.Command(claudePath, args...) //nolint:gosec
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findRealClaude() (string, error) {
	self, _ := os.Executable()
	selfBase := strings.TrimSuffix(filepath.Base(self), ".exe")

	paths := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range paths {
		candidate := filepath.Join(dir, "claude")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(candidate)
			if strings.TrimSuffix(filepath.Base(target), ".exe") == selfBase {
				continue
			}
		}
		return candidate, nil
	}
	return "", fmt.Errorf("claude binary not found on PATH — is Claude Code installed?")
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
