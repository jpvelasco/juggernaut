package activation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestShouldWritePOSIXTarget_DefaultAndStatError(t *testing.T) {
	home := t.TempDir()

	// Unknown basename with missing file → default false.
	unknown := Target{Path: filepath.Join(home, "custom.rc"), Shell: ShellPOSIX}
	if shouldWritePOSIXTarget(unknown) {
		t.Fatal("unknown missing profile must not be created")
	}

	// Non-NotExist Stat (e.g. permission) must still attempt install. Windows
	// maps many bad paths to IsNotExist, so inject the error via statProfile.
	statProfile = func(string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}
	t.Cleanup(func() { statProfile = os.Stat })
	if !shouldWritePOSIXTarget(Target{Path: filepath.Join(home, ".zshrc"), Shell: ShellPOSIX}) {
		t.Fatal("non-NotExist Stat errors must still attempt install")
	}
}

func TestValidateMarkers_EmptyAndTooLong(t *testing.T) {
	if err := validateMarkers("", "# E"); err == nil {
		t.Fatal("empty begin must fail")
	}
	if err := validateMarkers("# B", ""); err == nil {
		t.Fatal("empty end must fail")
	}
	long := "#" + strings.Repeat("x", 200)
	if err := validateMarkers(long, "# E"); err == nil {
		t.Fatal("marker longer than 200 must fail")
	}
	if err := validateMarkers("# B", long); err == nil {
		t.Fatal("end marker longer than 200 must fail")
	}
}

func TestBlockFor_PanicsOnInvalidInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid CLI name")
		}
	}()
	_ = blockFor(ShellPOSIX, "bad;name", BeginMarker, EndMarker)
}

func TestBlockFor_PanicsOnInvalidMarkers(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid markers")
		}
	}()
	_ = blockFor(ShellPOSIX, "claude", "# same", "# same")
}

func TestInstallTargetFor_ReadError(t *testing.T) {
	home := t.TempDir()
	// Target path is a directory — ReadFile fails with a non-NotExist error.
	_, err := InstallTargetFor(Target{Path: home, Shell: ShellPOSIX}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected read error when profile path is a directory")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Fatalf("expected reading error, got %v", err)
	}
}

func TestInstallTargetFor_WriteError(t *testing.T) {
	home := t.TempDir()
	// Intermediate path component is a file. On Unix, ReadFile fails first
	// ("not a directory"); on some platforms WriteFile fails later. Either is
	// a hard error — the important contract is we do not silently succeed.
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "profile.sh")
	_, err := InstallTargetFor(Target{Path: path, Shell: ShellPOSIX}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected error when parent path component is a file")
	}
	msg := err.Error()
	if !strings.Contains(msg, "reading") && !strings.Contains(msg, "writing") {
		t.Fatalf("expected reading or writing error, got %v", err)
	}
}

func TestUninstallCLIBlocks_DuplicatePathKeySkipped(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "prof.ps1")
	block := blockFor(ShellPowerShell, "codex", codexBegin, codexEnd)
	if err := safepath.WriteFile(home, profile, []byte(block+"\n")); err != nil {
		t.Fatal(err)
	}
	// Same path twice via ActiveTargets + MigrationTargets — seen map must dedupe.
	removed, err := UninstallWith(home, UninstallOptions{
		Spec: CLISpec{Name: "codex", Begin: codexBegin, End: codexEnd},
		PowerShellResult: &ProfileResolverResult{
			ActiveTargets:    []Target{{Path: profile, Shell: ShellPowerShell}},
			MigrationTargets: []string{profile},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected single removal after dedupe, got %v", removed)
	}
}

func TestUninstallCLIBlocks_RemoveError(t *testing.T) {
	home := t.TempDir()
	// Directory path makes RemoveTargetForMarkers fail on read.
	_, err := UninstallWith(home, UninstallOptions{
		Spec: CLISpec{Name: "codex", Begin: codexBegin, End: codexEnd},
		PowerShellResult: &ProfileResolverResult{
			ActiveTargets: []Target{{Path: home, Shell: ShellPowerShell}},
		},
	})
	if err == nil {
		t.Fatal("expected remove error for directory path")
	}
}

func TestUninstallWith_RemoveTargetWithLegacyError(t *testing.T) {
	home := t.TempDir()
	// Claude uninstall walks DefaultTargets; seed .bashrc as a directory so
	// RemoveTargetWithLegacy returns a read error.
	bashrc := filepath.Join(home, ".bashrc")
	if err := safepath.MkdirAll(bashrc); err != nil {
		t.Fatal(err)
	}
	_, err := UninstallWith(home, UninstallOptions{
		PowerShellResult: &ProfileResolverResult{},
	})
	if err == nil {
		t.Fatal("expected error when .bashrc is a directory")
	}
}

func TestProfilePathKey_Clean(t *testing.T) {
	// Smoke: key is stable under Clean; on Windows casing folds.
	a := profilePathKey(filepath.Join("C:", "Users", "X", "a.ps1"))
	b := profilePathKey(filepath.Join("C:", "Users", "X", "a.ps1"))
	if a != b {
		t.Fatalf("keys differ: %q vs %q", a, b)
	}
	if runtime.GOOS == "windows" {
		upper := profilePathKey(filepath.Join("C:", "USERS", "X", "A.PS1"))
		if upper != strings.ToLower(upper) {
			t.Fatalf("windows key should be lowercased: %q", upper)
		}
	}
}
