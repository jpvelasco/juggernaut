package activation

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestResolveClaudeBinary_NotFound covers the fallthrough: no claude on PATH
// yields exec.ErrNotFound.
func TestResolveClaudeBinary_NotFound(t *testing.T) {
	empty := t.TempDir() // a dir with no claude binary
	_, err := resolveClaudeBinary(empty, "")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("expected exec.ErrNotFound for an empty PATH, got %v", err)
	}
}

// TestResolveClaudeBinary_SkipsNonExecutable covers the isExecutable==false
// branch on POSIX: a non-executable file named claude must be skipped, and the
// real executable in a later PATH entry chosen instead.
func TestResolveClaudeBinary_SkipsNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit check is POSIX-only; Windows treats any file as executable")
	}
	dirA := t.TempDir()
	dirB := t.TempDir()

	// Non-executable claude in dirA (mode 0o644).
	nonExec := filepath.Join(dirA, "claude")
	if err := os.WriteFile(nonExec, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission
		t.Fatal(err)
	}
	// Executable claude in dirB.
	realClaude := filepath.Join(dirB, "claude")
	writeExecutableFile(t, dirB, realClaude, "#!/bin/sh\necho real\n")

	pathList := strings.Join([]string{dirA, dirB}, string(os.PathListSeparator))
	got, err := resolveClaudeBinary(pathList, "")
	if err != nil {
		t.Fatalf("resolveClaudeBinary: %v", err)
	}
	if got != realClaude {
		t.Errorf("expected the executable claude at %q, got %q", realClaude, got)
	}
}

// TestResolveClaudeBinary_SkipsSelf covers the recursion-avoidance branch: a
// claude entry that is the SAME file as the juggernaut binary (self) must be
// skipped so launch never re-invokes itself.
func TestResolveClaudeBinary_SkipsSelf(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	// "self" is the juggernaut binary; a claude in dirA is a hardlink/copy of it.
	self := filepath.Join(dirA, "juggernaut-self")
	writeExecutableFile(t, dirA, self, "self-binary")

	// Make dirA's claude the same file as self via a hardlink so os.SameFile
	// treats them as identical (symlinks work too; hardlink avoids readlink).
	selfClaude := filepath.Join(dirA, claudeName())
	if err := os.Link(self, selfClaude); err != nil {
		// Fall back to a symlink if hardlinks are unavailable on the FS.
		if err := os.Symlink(self, selfClaude); err != nil {
			t.Skipf("cannot link/symlink to simulate self: %v", err)
		}
	}

	realClaude := filepath.Join(dirB, claudeName())
	writeExecutableFile(t, dirB, realClaude, "real-claude")

	pathList := strings.Join([]string{dirA, dirB}, string(os.PathListSeparator))
	got, err := resolveClaudeBinary(pathList, self)
	if err != nil {
		t.Fatalf("resolveClaudeBinary: %v", err)
	}
	if got == selfClaude {
		t.Error("resolveClaudeBinary must not select the juggernaut binary itself (recursion)")
	}
	if got != realClaude {
		t.Errorf("expected the real claude at %q, got %q", realClaude, got)
	}
}

func claudeName() string {
	if runtime.GOOS == "windows" {
		return "claude.exe"
	}
	return "claude"
}

// TestAuthModes_SkipsMalformedSettings covers the guard branches: a juggernaut
// block missing the managedBy marker, or with wrong-typed auth, contributes no
// mode instead of erroring.
func TestAuthModes_SkipsMalformedSettings(t *testing.T) {
	home := t.TempDir()
	// Change into a scratch dir so the "./.claude/settings.json" probe is empty.
	t.Chdir(t.TempDir())

	writeAuthSettings(t, home, `{
		"juggernaut": {
			"meta": {"managedBy": "someone-else"},
			"auth": {"mode": "iam"}
		}
	}`)

	modes, err := authModes(home)
	if err != nil {
		t.Fatalf("authModes: %v", err)
	}
	if len(modes) != 0 {
		t.Errorf("a block without managedBy==juggernaut must be ignored, got %v", modes)
	}
}

// TestAuthModes_ReadsManagedMode covers the happy path: a properly-managed
// block contributes its auth mode.
func TestAuthModes_ReadsManagedMode(t *testing.T) {
	home := t.TempDir()
	t.Chdir(t.TempDir())

	writeAuthSettings(t, home, `{
		"juggernaut": {
			"meta": {"managedBy": "juggernaut"},
			"auth": {"mode": "iam"}
		}
	}`)

	modes, err := authModes(home)
	if err != nil {
		t.Fatalf("authModes: %v", err)
	}
	if len(modes) != 1 || modes[0] != "iam" {
		t.Errorf("expected [iam], got %v", modes)
	}
}

func writeAuthSettings(t *testing.T, home, content string) {
	t.Helper()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := safepath.WriteFile(home, path, []byte(content)); err != nil {
		t.Fatalf("writing settings: %v", err)
	}
}
