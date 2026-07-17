package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestUninstallCLIBlocks_ScansMigrationTargets: multi-CLI uninstall must
// remove blocks from historical paths that are not in ActiveTargets.
func TestUninstallCLIBlocks_ScansMigrationTargets(t *testing.T) {
	home := t.TempDir()
	active := filepath.Join(home, "active.ps1")
	historical := filepath.Join(home, "historical.ps1")
	block := blockFor(ShellPowerShell, "codex", codexBegin, codexEnd)
	for _, p := range []string{active, historical} {
		if err := safepath.WriteFile(filepath.Dir(p), p, []byte(block+"\n")); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := UninstallWith(home, UninstallOptions{
		Spec: CLISpec{Name: "codex", Begin: codexBegin, End: codexEnd},
		PowerShellResult: &ProfileResolverResult{
			ActiveTargets:    []Target{{Path: active, Shell: ShellPowerShell}},
			MigrationTargets: []string{historical},
		},
	})
	if err != nil {
		t.Fatalf("UninstallWith: %v", err)
	}
	if len(removed) < 2 {
		t.Fatalf("expected both active and historical removed, got %v", removed)
	}
	for _, p := range []string{active, historical} {
		data, err := safepath.ReadFile(filepath.Dir(p), p)
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		if strings.Contains(string(data), codexBegin) {
			t.Errorf("block still present in %s:\n%s", p, data)
		}
	}
}
