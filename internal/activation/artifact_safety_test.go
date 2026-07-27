package activation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestRecoverLegacyArtifacts_PreservesRealClaude is the safety invariant: a real
// claude binary whose contents are NOT a known Juggernaut shim must never be
// removed by recovery. A false positive here deletes a user's real Claude Code.
func TestRecoverLegacyArtifacts_PreservesRealClaude(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows uses content matching; POSIX safety is covered by the symlink tests")
	}
	home := testutil.NewTestHome(t)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
		t.Fatal(err)
	}
	self := filepath.Join(binDir, "juggernaut.exe")
	writeExecutableFile(t, binDir, self, "juggernaut")

	// A genuine claude.cmd whose body is NOT the legacy launcher shim.
	realClaude := filepath.Join(binDir, "claude.cmd")
	if err := os.WriteFile(realClaude, []byte("@echo off\n\"%~dp0\\node.exe\" claude.js %*\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	actions, err := RecoverLegacyArtifacts(binDir)
	if err != nil {
		t.Fatalf("RecoverLegacyArtifacts() error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected no recovery actions for a real claude.cmd, got %v", actions)
	}
	if _, err := os.Stat(realClaude); err != nil {
		t.Errorf("real claude.cmd must be preserved, but it is gone: %v", err)
	}
}

// TestRecoverLegacyArtifacts_RestoresBackup covers the restore branch: when a
// v4.2.6 backup exists and no claude is present, the backup is renamed back
// into place.
func TestRecoverLegacyArtifacts_RestoresBackup(t *testing.T) {
	home := testutil.NewTestHome(t)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
		t.Fatal(err)
	}
	self := filepath.Join(binDir, "juggernaut")
	writeExecutableFile(t, binDir, self, "juggernaut")

	names := platformNames()
	backup := filepath.Join(binDir, names.backup)
	claude := filepath.Join(binDir, names.claude)
	writeExecutableFile(t, binDir, backup, "real-claude-binary")

	actions, err := RecoverLegacyArtifacts(binDir)
	if err != nil {
		t.Fatalf("RecoverLegacyArtifacts() error: %v", err)
	}

	if _, err := os.Stat(claude); err != nil {
		t.Errorf("claude should have been restored from backup: %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Errorf("backup should be gone after restore, stat err = %v", err)
	}
	if len(actions) == 0 {
		t.Error("expected a restore action to be reported")
	}
}

// TestRecoverLegacyArtifacts_KeepsExistingClaudeOverBackup verifies restore does
// NOT clobber an existing claude when a backup is also present.
func TestRecoverLegacyArtifacts_KeepsExistingClaudeOverBackup(t *testing.T) {
	home := testutil.NewTestHome(t)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
		t.Fatal(err)
	}
	self := filepath.Join(binDir, "juggernaut")
	writeExecutableFile(t, binDir, self, "juggernaut")

	names := platformNames()
	backup := filepath.Join(binDir, names.backup)
	claude := filepath.Join(binDir, names.claude)
	writeExecutableFile(t, binDir, backup, "old-backup")
	writeExecutableFile(t, binDir, claude, "current-claude")

	if _, err := RecoverLegacyArtifacts(binDir); err != nil {
		t.Fatalf("RecoverLegacyArtifacts() error: %v", err)
	}

	data, err := os.ReadFile(claude) // nosemgrep: go_filesystem_rule-fileread — path under t.TempDir()
	if err != nil {
		t.Fatalf("reading claude: %v", err)
	}
	if string(data) != "current-claude" {
		t.Errorf("existing claude must not be overwritten by backup, got %q", string(data))
	}
}

// TestIsKnownJuggernautArtifact_SymlinkTargets covers the POSIX branch: a
// symlink to the juggernaut binary is a known artifact; a symlink elsewhere is
// not; a regular file is not.
func TestIsKnownJuggernautArtifact_SymlinkTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink branch is POSIX-only")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "juggernaut")
	writeExecutableFile(t, dir, self, "juggernaut")

	// Symlink pointing at the juggernaut binary -> known artifact.
	link := filepath.Join(dir, "claude")
	if err := os.Symlink(self, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if !isKnownJuggernautArtifact(link, self) {
		t.Error("a symlink to the juggernaut binary should be recognized as a known artifact")
	}

	// Symlink pointing somewhere else -> NOT a known artifact.
	other := filepath.Join(dir, "some-other-binary")
	writeExecutableFile(t, dir, other, "other")
	link2 := filepath.Join(dir, "claude2")
	if err := os.Symlink(other, link2); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if isKnownJuggernautArtifact(link2, self) {
		t.Error("a symlink to an unrelated binary must NOT be treated as a Juggernaut artifact")
	}

	// A regular file (not a symlink) -> NOT a known artifact.
	if isKnownJuggernautArtifact(other, self) {
		t.Error("a regular file must NOT be treated as a Juggernaut artifact on POSIX")
	}

	// Empty self -> never a known artifact (avoids matching on empty target).
	if isKnownJuggernautArtifact(link, "") {
		t.Error("with empty self, nothing should be recognized as a known artifact")
	}
}
