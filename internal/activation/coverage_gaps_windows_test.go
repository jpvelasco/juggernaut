//go:build windows

package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestInstallPowerShell_LegacyMigrationAndStripErrors(t *testing.T) {
	home := testutil.NewTestHome(t)
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
	home := testutil.NewTestHome(t)
	allHosts := filepath.Join(home, "ok", "profile.ps1")
	bad := filepath.Join(home, "bad-dir")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(bad); err != nil {
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
	home := testutil.NewTestHome(t)
	// Install target is a directory → InstallTargetFor fails.
	dir := filepath.Join(home, "ps")
	if err := safepath.MkdirAll(dir); err != nil {
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
	home := testutil.NewTestHome(t)
	allHosts := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	staleDir := filepath.Join(home, "Documents", "PowerShell", "stale-dir")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(staleDir); err != nil {
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
	home := testutil.NewTestHome(t)
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

func TestPathsEqualCI(t *testing.T) {
	if pathsEqualCI("", "x") || pathsEqualCI("x", "") {
		t.Fatal("empty path must not equal")
	}
	if !pathsEqualCI(filepath.Clean("/a/b"), filepath.Clean("/a/b")) {
		t.Fatal("equal paths must match")
	}
	if !pathsEqualCI(`C:\Users\A`, `c:\users\a`) {
		t.Fatal("windows paths must compare case-insensitively")
	}
}

func TestHistoricalPowerShellTargetsScoped_EmptyAndDocsError(t *testing.T) {
	if got := historicalPowerShellTargetsScoped(""); got != nil {
		t.Fatalf("empty home: got %v", got)
	}
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return "", os.ErrNotExist
	})
	t.Cleanup(ResetResolveDocumentsFolderForTesting)

	home := testutil.NewTestHome(t)
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
	home := testutil.NewTestHome(t)
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
		t.Log("resolveHomeDir returned empty with cleared env (OS fallback may vary)")
	}
	t.Setenv("USERPROFILE", home)
	if got := resolveHomeDir(); got == "" {
		t.Fatal("expected USERPROFILE fallback")
	}
}

func TestUninstallCLIBlocks_WindowsNilResolver(t *testing.T) {
	home := testutil.NewTestHome(t)
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

// TestInstallPowerShell_StaleStripNotCountedAsInstall: third-pass cleanup
// must not inflate the "installed" path list used by apply messaging.
func TestInstallPowerShell_StaleStripNotCountedAsInstall(t *testing.T) {
	home := testutil.NewTestHome(t)
	allHosts := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	currentHost := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatal(err)
	}
	// Pre-seed AllHosts with the current block so second pass is a no-op.
	block := blockFor(ShellPowerShell, "claude", BeginMarker, EndMarker)
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(block+"\n")); err != nil {
		t.Fatal(err)
	}
	// Stale host block to strip only.
	if err := safepath.WriteFile(filepath.Dir(currentHost), currentHost, []byte(block+"\n")); err != nil {
		t.Fatal(err)
	}

	installed, err := installPowerShellActivationForSpec(home, &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: allHosts, Shell: ShellPowerShell},
			{Path: currentHost, Shell: ShellPowerShell},
		},
		InstallTargets:   []Target{{Path: allHosts, Shell: ShellPowerShell}},
		MigrationTargets: []string{currentHost},
	}, claudeCLISpec())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range installed {
		if strings.EqualFold(p, currentHost) {
			t.Fatalf("stale-strip path must not appear in installed list: %v", installed)
		}
	}
	// Host block gone.
	data, _ := safepath.ReadFile(filepath.Dir(currentHost), currentHost)
	if HasBlock(string(data)) {
		t.Fatal("stale CurrentHost block should still be stripped")
	}
}
