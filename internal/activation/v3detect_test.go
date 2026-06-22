package activation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectV3Install_FindsInstallDirMarker(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "juggernaut-install-dir.txt")
	if err := os.WriteFile(marker, []byte("C:\\Users\\x\\.juggernaut\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	detected, detail := DetectV3Install(binDir)
	if !detected {
		t.Fatal("expected v3 install to be detected from install-dir marker")
	}
	if detail == "" {
		t.Error("expected a non-empty detail describing the v3 artifacts")
	}
}

func TestDetectV3Install_NoMarkerNotDetected(t *testing.T) {
	binDir := t.TempDir()
	detected, _ := DetectV3Install(binDir)
	if detected {
		t.Error("expected no v3 install in an empty bin dir")
	}
}

func TestDetectV3Install_FindsShimPointingAtJuggernautHome(t *testing.T) {
	binDir := t.TempDir()
	shim := filepath.Join(binDir, "juggernaut.ps1")
	content := "#!/usr/bin/env pwsh\n& \"$HOME\\.juggernaut\\juggernaut.ps1\" @args\n"
	if err := os.WriteFile(shim, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	detected, _ := DetectV3Install(binDir)
	if !detected {
		t.Fatal("expected v3 install detected from .juggernaut shim")
	}
}

// A shim that exists but can't be read (here: the path is a directory, so
// os.ReadFile fails) should be reported conservatively rather than silently
// ignored — a possible v3 artifact is worth surfacing in doctor.
func TestDetectV3Install_ReportsUnreadableShim(t *testing.T) {
	binDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(binDir, "juggernaut.ps1"), 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission, go_file-permissions_rule-fileperm -- test fixture under t.TempDir()
		t.Fatal(err)
	}

	detected, detail := DetectV3Install(binDir)
	if !detected {
		t.Fatal("expected an unreadable shim to be reported as a possible v3 artifact")
	}
	if detail == "" {
		t.Error("expected a non-empty detail for the unreadable shim")
	}
}
