//go:build windows

package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestInstallPowerShell_StripsStaleCurrentHostBlock: apply must remove this
// CLI's markers from CurrentHost profiles while installing only to AllHosts.
// Stale host-specific wrappers load after AllHosts and override (or break)
// the real CLI — the failure mode that left dead juggernaut wrappers in
// Microsoft.PowerShell_profile.ps1.
func TestInstallPowerShell_StripsStaleCurrentHostBlock(t *testing.T) {
	home := t.TempDir()
	allHosts := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	currentHost := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := os.MkdirAll(filepath.Dir(allHosts), 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a stale CurrentHost grok block (as older apply did) plus user content.
	stale := blockFor(ShellPowerShell, "grok",
		"# BEGIN: Juggernaut Grok Activation",
		"# END: Juggernaut Grok Activation")
	userLine := "# user customizations\n$env:FOO = 'bar'\n"
	if err := safepath.WriteFile(filepath.Dir(currentHost), currentHost,
		[]byte(userLine+stale+"\n")); err != nil {
		t.Fatal(err)
	}

	psResult := &ProfileResolverResult{
		ActiveTargets: []Target{
			{Path: allHosts, Shell: ShellPowerShell},
			{Path: currentHost, Shell: ShellPowerShell},
		},
		InstallTargets: []Target{
			{Path: allHosts, Shell: ShellPowerShell},
		},
		MigrationTargets: []string{allHosts, currentHost},
	}

	spec := CLISpec{
		Name:  "grok",
		Begin: "# BEGIN: Juggernaut Grok Activation",
		End:   "# END: Juggernaut Grok Activation",
	}
	installed, err := installPowerShellActivationForSpec(home, psResult, spec)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(installed) == 0 {
		t.Fatal("expected AllHosts install and/or CurrentHost cleanup")
	}

	// AllHosts must have the fresh block.
	data, err := safepath.ReadFile(filepath.Dir(allHosts), allHosts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "function global:grok") {
		t.Fatalf("AllHosts missing grok activation:\n%s", data)
	}
	if !strings.Contains(string(data), "Get-Command juggernaut") {
		t.Fatalf("AllHosts block must use resilient wrapper:\n%s", data)
	}

	// CurrentHost must no longer have the grok block, but user content stays.
	hostData, err := safepath.ReadFile(filepath.Dir(currentHost), currentHost)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hostData), "Juggernaut Grok Activation") {
		t.Fatalf("stale CurrentHost grok block was not removed:\n%s", hostData)
	}
	if !strings.Contains(string(hostData), "$env:FOO = 'bar'") {
		t.Fatalf("user content in CurrentHost was lost:\n%s", hostData)
	}
}

func TestResolveHistoricalTargets_IncludesAllHostsAndCurrentHost(t *testing.T) {
	docs := filepath.Join("C:", "Users", "x", "Documents")
	got := resolveHistoricalTargets(docs)
	wantSub := []string{
		filepath.Join("PowerShell", "profile.ps1"),
		filepath.Join("PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join("WindowsPowerShell", "profile.ps1"),
		filepath.Join("WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}
	if len(got) != len(wantSub) {
		t.Fatalf("expected %d paths, got %d: %v", len(wantSub), len(got), got)
	}
	for i, sub := range wantSub {
		if !strings.HasSuffix(got[i], sub) {
			t.Errorf("path %d: got %s, want suffix %s", i, got[i], sub)
		}
	}
}

func TestHistoricalPowerShellTargetsScoped_ScansOneDriveAndLocalDocs(t *testing.T) {
	home := t.TempDir()
	// Known Documents points at OneDrive; local Documents must still be scanned.
	onedriveDocs := filepath.Join(home, "OneDrive", "Documents")
	SetResolveDocumentsFolderForTesting(func() (string, error) {
		return onedriveDocs, nil
	})
	t.Cleanup(ResetResolveDocumentsFolderForTesting)

	got := historicalPowerShellTargetsScoped(home)
	var hasLocal, hasOneDrive bool
	localProfile := filepath.Join(home, "Documents", "PowerShell", "profile.ps1")
	odProfile := filepath.Join(onedriveDocs, "PowerShell", "profile.ps1")
	for _, p := range got {
		if strings.EqualFold(p, localProfile) {
			hasLocal = true
		}
		if strings.EqualFold(p, odProfile) {
			hasOneDrive = true
		}
	}
	if !hasOneDrive {
		t.Errorf("expected OneDrive AllHosts profile in historical list: %v", got)
	}
	if !hasLocal {
		t.Errorf("expected local Documents AllHosts profile in historical list: %v", got)
	}
}
