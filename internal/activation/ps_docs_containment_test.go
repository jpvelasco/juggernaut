//go:build windows

package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// Given Documents is redirected outside the user profile (enterprise folder
// redirection, another volume) and PowerShell discovery succeeds,
// When profiles are resolved,
// Then the discovered paths must be accepted — the OS-resolved Known Folder
// root is the containment boundary — instead of being silently dropped,
// which forces the false "discovery failed" fallback on every run.
func TestDiscoverPowerShellProfiles_DocumentsOutsideHome(t *testing.T) {
	home := testutil.NewTestHome(t)
	docs := t.TempDir() // sibling of home — NOT under it

	psProfile := filepath.Join(docs, "PowerShell", "profile.ps1")
	SetPSRunnerForTesting(&mockCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       makePSOutput(psProfile, psProfile),
			"powershell.exe": makePSOutput(psProfile, psProfile),
		},
	})
	t.Cleanup(ResetPSRunnerForTesting)
	SetResolveDocumentsFolderForTesting(func() (string, error) { return docs, nil })
	t.Cleanup(ResetResolveDocumentsFolderForTesting)

	res := discoverPowerShellProfilesScoped(home)

	if res.UsedFallback {
		t.Errorf("discovery reported failure despite successful PS queries; warnings=%v", res.DiscoveryWarnings)
	}
	if len(res.ActiveTargets) == 0 {
		t.Fatalf("no active targets discovered; warnings=%v", res.DiscoveryWarnings)
	}
	found := false
	for _, tgt := range res.ActiveTargets {
		if !strings.HasPrefix(strings.ToLower(tgt.Path), strings.ToLower(docs)) {
			t.Errorf("target %s escapes the Documents boundary %s", tgt.Path, docs)
		}
		if strings.EqualFold(tgt.Path, psProfile) {
			found = true
		}
	}
	if !found {
		t.Errorf("discovered profile %s missing from active targets: %+v", psProfile, res.ActiveTargets)
	}
}
