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

// A v4-era npm-bridge shim delegates to an absolute target path (not ~/.juggernaut).
// When that target no longer exists, the shim is stale and shadows the working npm
// binary on PATH — doctor must flag it even though it doesn't mention ".juggernaut".
func TestDetectV3Install_FindsV4BridgeShimWithMissingTarget(t *testing.T) {
	binDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone", "juggernaut.ps1")
	shim := "param([Parameter(ValueFromRemainingArguments=$true)][string[]]$PassArgs)\n" +
		"$target = '" + missing + "'\n" +
		"if (-not (Test-Path $target)) { Write-Error 'not found'; exit 1 }\n" +
		"& $target @PassArgs\n"
	if err := os.WriteFile(filepath.Join(binDir, "juggernaut.ps1"), []byte(shim), 0o600); err != nil {
		t.Fatal(err)
	}

	detected, detail := DetectV3Install(binDir)
	if !detected {
		t.Fatal("expected a bridge shim with a missing target to be detected")
	}
	if detail == "" {
		t.Error("expected a non-empty detail")
	}
}

// A .cmd bridge shim pointing at a missing target must also be flagged.
func TestDetectV3Install_FindsCmdBridgeShimWithMissingTarget(t *testing.T) {
	binDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone", "juggernaut.cmd")
	shim := "@echo off\r\n\"" + missing + "\" %*\r\nexit /b %ERRORLEVEL%\r\n"
	if err := os.WriteFile(filepath.Join(binDir, "juggernaut.cmd"), []byte(shim), 0o600); err != nil {
		t.Fatal(err)
	}

	detected, _ := DetectV3Install(binDir)
	if !detected {
		t.Fatal("expected a .cmd bridge shim with a missing target to be detected")
	}
}

// A shim whose delegation target still exists is NOT stale — don't flag a working
// bridge (avoids false positives on a healthy install layout).
func TestDetectV3Install_IgnoresBridgeShimWithExistingTarget(t *testing.T) {
	binDir := t.TempDir()
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "juggernaut.ps1")
	if err := os.WriteFile(target, []byte("# real target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := "$target = '" + target + "'\n& $target @args\n"
	if err := os.WriteFile(filepath.Join(binDir, "juggernaut.ps1"), []byte(shim), 0o600); err != nil {
		t.Fatal(err)
	}

	detected, _ := DetectV3Install(binDir)
	if detected {
		t.Error("a bridge shim whose target exists should not be flagged as stale")
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
