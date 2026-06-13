package launcher_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/juggernaut/v4/internal/launcher"
	"github.com/jpvelasco/juggernaut/v4/internal/safepath"
)

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	if err := launcher.Install(dir); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if runtime.GOOS == "windows" {
		shim := filepath.Join(dir, "claude.cmd")
		data, err := safepath.ReadFile(dir, shim)
		if err != nil {
			t.Fatalf("claude.cmd not created: %v", err)
		}
		if len(data) == 0 {
			t.Error("claude.cmd should not be empty")
		}
	} else {
		shim := filepath.Join(dir, "claude")
		info, err := os.Lstat(shim)
		if err != nil {
			t.Fatalf("claude symlink not created: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("expected symlink for claude on Unix")
		}
	}
}

func TestIsInstalled(t *testing.T) {
	dir := t.TempDir()
	if launcher.IsInstalled(dir) {
		t.Error("should not be installed in empty dir")
	}
	_ = launcher.Install(dir)
	if !launcher.IsInstalled(dir) {
		t.Error("should be installed after Install()")
	}
}

func TestUninstall(t *testing.T) {
	dir := t.TempDir()
	_ = launcher.Install(dir)
	if err := launcher.Uninstall(dir); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if launcher.IsInstalled(dir) {
		t.Error("should not be installed after Uninstall()")
	}
}

func TestDefaultBinDir(t *testing.T) {
	dir := launcher.DefaultBinDir()
	if dir == "" {
		t.Error("DefaultBinDir() returned empty string")
	}
}
