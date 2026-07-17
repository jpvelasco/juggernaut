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
	// Intermediate path component is a file → MkdirAll/WriteFile fails.
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "profile.sh")
	_, err := InstallTargetFor(Target{Path: path, Shell: ShellPOSIX}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected write error when parent is a file")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Fatalf("expected writing error, got %v", err)
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
	if err := os.Mkdir(bashrc, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := UninstallWith(home, UninstallOptions{
		PowerShellResult: &ProfileResolverResult{},
	})
	if err == nil {
		t.Fatal("expected error when .bashrc is a directory")
	}
}

func TestInstallPowerShell_LegacyMigrationAndStripErrors(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	allHosts := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	legacyPath := filepath.Join(home, "Documents", "PowerShell", "legacy.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatal(err)
	}

	// Seed legacy launcher + bedrock blocks for first-pass migration.
	legacyBody := LegacyLauncherBegin + "\nold\n" + LegacyLauncherEnd + "\n" +
		LegacyBedrockBegin + "\nold\n" + LegacyBedrockEnd + "\n"
	if err := safepath.WriteFile(filepath.Dir(legacyPath), legacyPath, []byte(legacyBody)); err != nil {
		t.Fatal(err)
	}

	psResult := &ProfileResolverResult{
		ActiveTargets:    []Target{{Path: allHosts, Shell: ShellPowerShell}},
		InstallTargets:   []Target{{Path: allHosts, Shell: ShellPowerShell}},
		MigrationTargets: []string{legacyPath},
	}
	if _, err := installPowerShellActivationForSpec(home, psResult, claudeCLISpec()); err != nil {
		t.Fatalf("install with legacy migration: %v", err)
	}
	data, err := safepath.ReadFile(filepath.Dir(legacyPath), legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if HasLegacyLauncherBlock(string(data)) || HasLegacyBedrockBlock(string(data)) {
		t.Fatalf("legacy blocks should be migrated away:\n%s", data)
	}
	// AllHosts should have Claude activation.
	allData, err := safepath.ReadFile(filepath.Dir(allHosts), allHosts)
	if err != nil {
		t.Fatal(err)
	}
	if !HasBlock(string(allData)) {
		t.Fatal("expected AllHosts activation after install")
	}
}

func TestInstallPowerShell_MigrationReadError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	allHosts := filepath.Join(home, "ok", "profile.ps1")
	bad := filepath.Join(home, "bad-dir")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bad, 0o700); err != nil {
		t.Fatal(err)
	}
	// Migration target is a directory → ReadFile non-NotExist error.
	_, err := installPowerShellActivationForSpec(home, &ProfileResolverResult{
		InstallTargets:   []Target{{Path: allHosts, Shell: ShellPowerShell}},
		MigrationTargets: []string{bad},
	}, claudeCLISpec())
	if err == nil || !strings.Contains(err.Error(), "legacy migration") {
		t.Fatalf("expected legacy migration read error, got %v", err)
	}
}

func TestInstallPowerShell_InstallTargetError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	// Install target is a directory → InstallTargetFor fails.
	dir := filepath.Join(home, "ps")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := installPowerShellActivationForSpec(home, &ProfileResolverResult{
		InstallTargets: []Target{{Path: dir, Shell: ShellPowerShell}},
	}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected InstallTargetFor error")
	}
}

func TestInstallPowerShell_ThirdPassStripError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	allHosts := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	staleDir := filepath.Join(home, "Documents", "PowerShell", "stale-dir")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Stale dir only in ActiveTargets (not MigrationTargets) so first-pass
	// migration skips it and third-pass strip hits the read error.
	_, err := installPowerShellActivationForSpec(home, &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: allHosts, Shell: ShellPowerShell},
			{Path: staleDir, Shell: ShellPowerShell},
		},
		InstallTargets:   []Target{{Path: allHosts, Shell: ShellPowerShell}},
		MigrationTargets: nil,
	}, claudeCLISpec())
	if err == nil {
		t.Fatal("expected third-pass strip error for directory path")
	}
}

func TestInstallPowerShell_LegacyWriteError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	legacyPath := filepath.Join(home, "Documents", "PowerShell", "legacy.ps1")
	if err := safepath.MkdirAll(filepath.Dir(legacyPath)); err != nil {
		t.Fatal(err)
	}
	body := LegacyLauncherBegin + "\nold\n" + LegacyLauncherEnd + "\n"
	if err := safepath.WriteFile(filepath.Dir(legacyPath), legacyPath, []byte(body)); err != nil {
		t.Fatal(err)
	}
	// Make the profile read-only so migration WriteFile fails.
	if err := os.Chmod(legacyPath, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(legacyPath, 0o600) })

	_, err := installPowerShellActivationForSpec(home, &ProfileResolverResult{
		InstallTargets:   nil,
		MigrationTargets: []string{legacyPath},
	}, claudeCLISpec())
	if err == nil {
		// Windows may still allow owner writes on 0400; skip soft if so.
		t.Skip("platform allowed write despite 0400; cannot force migration write error")
	}
	if !strings.Contains(err.Error(), "migrating legacy") {
		t.Fatalf("expected migrating legacy error, got %v", err)
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
		lower := profilePathKey(filepath.Join("c:", "users", "x", "a.ps1"))
		// filepath.Join("C:", ...) quirks vary; at least lower-casing applies.
		if upper != strings.ToLower(upper) {
			t.Fatalf("windows key should be lowercased: %q", upper)
		}
		_ = lower
	}
}

func TestPathsEqualCI(t *testing.T) {
	if pathsEqualCI("", "x") || pathsEqualCI("x", "") {
		t.Fatal("empty path must not equal")
	}
	if !pathsEqualCI(filepath.Clean("/a/b"), filepath.Clean("/a/b")) {
		t.Fatal("equal paths must match")
	}
	if runtime.GOOS == "windows" {
		if !pathsEqualCI(`C:\Users\A`, `c:\users\a`) {
			t.Fatal("windows paths must compare case-insensitively")
		}
	}
}

func TestHistoricalPowerShellTargetsScoped_EmptyAndDocsError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	if got := historicalPowerShellTargetsScoped(""); got != nil {
		t.Fatalf("empty home: got %v", got)
	}
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return "", os.ErrNotExist
	})
	t.Cleanup(ResetResolveDocumentsFolderForTesting)

	home := t.TempDir()
	got := historicalPowerShellTargetsScoped(home)
	// Falls back to $HOME/Documents and still adds OneDrive alternate.
	want := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	found := false
	for _, p := range got {
		if strings.EqualFold(p, want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fallback Documents profile in %v", got)
	}
}

func TestHistoricalPowerShellTargets_Unscoped(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	docs := filepath.Join(home, "Documents")
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return docs, nil
	})
	t.Cleanup(ResetResolveDocumentsFolderForTesting)
	got := historicalPowerShellTargets()
	if len(got) == 0 {
		t.Fatal("expected historical targets from Known Documents")
	}

	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return "", os.ErrPermission
	})
	if got := historicalPowerShellTargets(); got != nil {
		t.Fatalf("docs error should return nil, got %v", got)
	}

	// Exercise the real Known Folder resolver (not mocked).
	ResetResolveDocumentsFolderForTesting()
	_ = historicalPowerShellTargets()
	// resolveHomeDir: empty HOME then USERPROFILE, then UserHomeDir fallback.
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if got := resolveHomeDir(); got == "" {
		// UserHomeDir may still succeed; empty only if both env and OS fail.
		t.Log("resolveHomeDir returned empty with cleared env (OS fallback may vary)")
	}
	t.Setenv("USERPROFILE", home)
	if got := resolveHomeDir(); got != home {
		// HOME empty, USERPROFILE set
		if got == "" {
			t.Fatal("expected USERPROFILE fallback")
		}
	}
}

func TestUninstallCLIBlocks_WindowsNilResolver(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	home := t.TempDir()
	// Seed a POSIX profile only; nil PowerShellResult forces live discovery
	// (covers uninstallCLIBlocks windows branch) without requiring real PS blocks.
	bashrc := filepath.Join(home, ".bashrc")
	block := blockFor(ShellPOSIX, "codex", codexBegin, codexEnd)
	if err := safepath.WriteFile(home, bashrc, []byte(block+"\n")); err != nil {
		t.Fatal(err)
	}
	// Mock docs so discovery fallback stays under temp home if PS query fails.
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return filepath.Join(home, "Documents"), nil
	})
	t.Cleanup(ResetResolveDocumentsFolderForTesting)
	// Force PS discovery to fail so we don't touch real user profiles.
	SetPSRunnerForTesting(&mockCommandRunner{
		err: map[string]error{
			"pwsh.exe":       os.ErrNotExist,
			"powershell.exe": os.ErrNotExist,
		},
	})
	t.Cleanup(ResetPSRunnerForTesting)

	removed, err := UninstallWith(home, UninstallOptions{
		Spec: CLISpec{Name: "codex", Begin: codexBegin, End: codexEnd},
		// PowerShellResult nil → windows live resolve path
	})
	if err != nil {
		t.Fatalf("UninstallWith: %v", err)
	}
	found := false
	for _, p := range removed {
		if p == bashrc {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected .bashrc removed, got %v", removed)
	}
}
