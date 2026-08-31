package activation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestInstallTargetFor_SecondCLIInstallPreservesFirst is the regression for
// #388: an `apply` for a second CLI must not clobber a block an earlier `apply`
// already wrote to the same profile. The locked read-modify-write re-reads the
// current file content before computing the update, so the first CLI's block
// survives the second CLI's write. (Without the lock, the second installer
// would act on a stale read and overwrite the first's block.)
func TestInstallTargetFor_SecondCLIInstallPreservesFirst(t *testing.T) {
	home := testutil.NewTestHome(t)
	profile := filepath.Join(home, ".bashrc")

	codexSpec := CLISpec{Name: "codex", Begin: "# BEGIN: Juggernaut Codex Activation", End: "# END: Juggernaut Codex Activation"}

	if _, err := InstallTargetFor(Target{Path: profile, Shell: ShellPOSIX}, claudeCLISpec()); err != nil {
		t.Fatalf("claude install: %v", err)
	}
	if _, err := InstallTargetFor(Target{Path: profile, Shell: ShellPOSIX}, codexSpec); err != nil {
		t.Fatalf("codex install: %v", err)
	}

	data, err := safepath.ReadFile(home, profile)
	if err != nil {
		t.Fatalf("reading profile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, claudeCLISpec().Begin) {
		t.Errorf("claude block lost after codex install:\n%s", content)
	}
	if !strings.Contains(content, codexSpec.Begin) {
		t.Errorf("codex block missing:\n%s", content)
	}
}

// TestInstallTargetFor_LockedByOtherFailsCleanly verifies that when the
// profile lock is already held by another process, the install fails with a
// clear error instead of guessing a write (which would be the data-loss path).
func TestInstallTargetFor_LockedByOtherFailsCleanly(t *testing.T) {
	home := testutil.NewTestHome(t)
	profile := filepath.Join(home, ".bashrc")
	// Create the profile so the lock file's directory exists.
	if err := safepath.WriteFile(home, profile, []byte("export A=1\n")); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	// Hold the lock out-of-band (simulating a concurrent juggernaut process).
	other := flock.New(profile + ".lock")
	locked, err := other.TryLock()
	if err != nil {
		t.Fatalf("holding lock: %v", err)
	}
	if !locked {
		t.Fatal("failed to hold the profile lock for the test")
	}
	defer func() { _ = other.Unlock() }()

	_, err = InstallTargetFor(Target{Path: profile, Shell: ShellPOSIX}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected lock-contention error, got nil")
	}
	if !strings.Contains(err.Error(), "locked by another process") {
		t.Fatalf("expected a lock-contention message, got %v", err)
	}
}

// TestWithProfileLock_LockFnErrorSurfaces covers the withProfileLock branch
// where the flock operation itself errors (injected via profileLockFn), so the
// real OS error is surfaced rather than swallowed.
func TestWithProfileLock_LockFnErrorSurfaces(t *testing.T) {
	home := testutil.NewTestHome(t)
	profile := filepath.Join(home, ".profile")

	orig := profileLockFn
	profileLockFn = func(p string, _ ...flock.Option) *flock.Flock {
		// Point the flock at a path under a directory that does not exist, so
		// TryLock fails with an OS error.
		return flock.New(filepath.Join(home, "missing-dir", p))
	}
	defer func() { profileLockFn = orig }()

	err := withProfileLock(profile, func() error { return nil })
	if err == nil {
		t.Fatal("expected lock acquisition error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "lock") {
		t.Fatalf("expected a lock error, got %v", err)
	}
}

// TestInstallTargetFor_MkdirFailureIsHardError covers the withProfileLock
// branch where creating the lock file's parent directory fails: the error must
// surface as a hard failure, never a silent skip.
func TestInstallTargetFor_MkdirFailureIsHardError(t *testing.T) {
	home := testutil.NewTestHome(t)
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("creating blocker file: %v", err)
	}
	profile := filepath.Join(blocker, "profile.sh")

	_, err := InstallTargetFor(Target{Path: profile, Shell: ShellPOSIX}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected hard error when the lock directory cannot be created")
	}
	if !strings.Contains(err.Error(), "creating lock directory") {
		t.Fatalf("expected a lock-directory error, got %v", err)
	}
}
