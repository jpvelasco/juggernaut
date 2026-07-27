package activation

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// setupActivationFixture returns a temp home and an injected PowerShell result
// so the install/uninstall wrappers never touch real shell profiles. On Windows
// it also wires a mock discovery runner.
func setupActivationFixture(t *testing.T) (string, *ProfileResolverResult) {
	t.Helper()
	home := t.TempDir()

	var psResult *ProfileResolverResult
	if runtime.GOOS == "windows" {
		psProfile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
		runner := &testDiscoveryRunner{
			output: map[string][]byte{
				"pwsh.exe":       testPSOutput(psProfile, psProfile),
				"powershell.exe": testPSOutput(psProfile, psProfile),
			},
		}
		SetPSRunnerForTesting(runner)
		t.Cleanup(ResetPSRunnerForTesting)

		psResult = &ProfileResolverResult{
			ActiveTargets:  []Target{{Path: psProfile, Shell: ShellPowerShell}},
			InstallTargets: []Target{{Path: psProfile, Shell: ShellPowerShell}},
		}
	}
	return home, psResult
}

func TestSamePath(t *testing.T) {
	if !samePath("/a/b/../b/c", "/a/b/c") {
		t.Error("samePath should treat cleaned-equal paths as equal")
	}
	if samePath("/a/b/c", "/a/b/d") {
		t.Error("samePath should treat distinct paths as unequal")
	}
	if runtime.GOOS == "windows" {
		if !samePath(`C:\Foo\Bar`, `c:\foo\bar`) {
			t.Error("samePath should be case-insensitive on Windows")
		}
	}
}

func TestDefaultBinDir_EmptyHomeFallsBack(t *testing.T) {
	testutil.NewTestHome(t)
	// Empty arg forces the env/UserHomeDir fallback chain.
	if got := DefaultBinDir(""); got == "" {
		t.Error("DefaultBinDir(\"\") should resolve a non-empty path via fallbacks")
	}
}

func TestHasBlockWithMarkers(t *testing.T) {
	if !HasBlockWithMarkers("# BEGIN: X\nbody\n# END: X\n", "# BEGIN: X", "# END: X") {
		t.Fatal("expected markers present")
	}
	if HasBlockWithMarkers("# BEGIN: X\nbody\n", "# BEGIN: X", "# END: X") {
		t.Fatal("expected missing end marker to fail")
	}
	if HasBlockWithMarkers("body", "", "# END: X") {
		t.Fatal("empty begin must fail closed")
	}
}

func TestInstalledTargets_ReflectsInstallState(t *testing.T) {
	home, psResult := setupActivationFixture(t)

	// Nothing installed yet.
	if got := InstalledTargetsWith(home, psResult); len(got) != 0 {
		t.Fatalf("expected no installed targets before install, got %v", got)
	}

	installed, err := InstallWith(home, InstallOptions{PowerShellResult: psResult})
	if err != nil {
		t.Fatalf("InstallWith() error: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("install should report written targets")
	}

	found := InstalledTargetsWith(home, psResult)
	if len(found) != len(installed) {
		t.Fatalf("InstalledTargetsWith reported %d targets, want %d (installed=%v found=%v)",
			len(found), len(installed), installed, found)
	}
}

func TestUninstallWith_RemovesInstalledBlocks(t *testing.T) {
	home, psResult := setupActivationFixture(t)

	if _, err := InstallWith(home, InstallOptions{PowerShellResult: psResult}); err != nil {
		t.Fatalf("InstallWith() error: %v", err)
	}

	removed, err := UninstallWith(home, UninstallOptions{PowerShellResult: psResult})
	if err != nil {
		t.Fatalf("UninstallWith() error: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("uninstall should report removed targets")
	}

	// After uninstall, no target should still contain a block.
	if got := InstalledTargetsWith(home, psResult); len(got) != 0 {
		t.Fatalf("expected no installed targets after uninstall, got %v", got)
	}
}

// TestNoArgWrappers_RoundTrip exercises the no-arg Install/InstalledTargets/
// Uninstall entry points. On non-Windows these only touch DefaultTargets(home),
// which live entirely under the temp HOME, so they're safe to call directly.
func TestNoArgWrappers_RoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no-arg wrappers resolve real PowerShell profiles on Windows")
	}
	home := t.TempDir()

	installed, err := Install(home)
	if err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("Install() should write activation blocks")
	}

	if got := InstalledTargets(home); len(got) != len(installed) {
		t.Fatalf("InstalledTargets() = %d, want %d", len(got), len(installed))
	}

	removed, err := Uninstall(home)
	if err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("Uninstall() should remove the installed blocks")
	}
	if got := InstalledTargets(home); len(got) != 0 {
		t.Fatalf("InstalledTargets() after uninstall = %d, want 0", len(got))
	}
}

func TestUninstallWith_NoBlocksIsNoop(t *testing.T) {
	home, psResult := setupActivationFixture(t)

	removed, err := UninstallWith(home, UninstallOptions{PowerShellResult: psResult})
	if err != nil {
		t.Fatalf("UninstallWith() on clean home error: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("uninstall on clean home should remove nothing, got %v", removed)
	}
}

func TestRemoveTarget_MissingFileIsNoop(t *testing.T) {
	home := t.TempDir()
	missing := filepath.Join(home, ".bashrc")

	ok, err := RemoveTarget(missing)
	if err != nil {
		t.Fatalf("RemoveTarget on missing file should not error, got %v", err)
	}
	if ok {
		t.Fatal("RemoveTarget on missing file should report no change")
	}
}

func TestRemoveTarget_RemovesBlock(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".bashrc")
	content := "export FOO=bar\n" + Block(ShellPOSIX) + "\nalias ll='ls -la'\n"
	writeFile(t, home, path, content)

	ok, err := RemoveTarget(path)
	if err != nil {
		t.Fatalf("RemoveTarget() error: %v", err)
	}
	if !ok {
		t.Fatal("RemoveTarget should report a change when a block is present")
	}

	raw, err := safepath.ReadFile(home, path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	data := string(raw)
	if HasBlock(data) {
		t.Fatalf("block should be gone after RemoveTarget, content:\n%s", data)
	}
	if !strings.Contains(data, "export FOO=bar") || !strings.Contains(data, "alias ll='ls -la'") {
		t.Fatalf("unrelated content must be preserved, content:\n%s", data)
	}

	// Second removal is a no-op.
	ok, err = RemoveTarget(path)
	if err != nil {
		t.Fatalf("second RemoveTarget() error: %v", err)
	}
	if ok {
		t.Fatal("second RemoveTarget should report no change")
	}
}
