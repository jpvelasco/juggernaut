package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

const (
	codexBegin = "# BEGIN: Juggernaut Codex Activation"
	codexEnd   = "# END: Juggernaut Codex Activation"
)

// TestUpsertBlockWithMarkers_Coexist: installing a codex block into content that
// already holds a claude block must PRESERVE the claude block (both coexist).
func TestUpsertBlockWithMarkers_Coexist(t *testing.T) {
	claudeBlk := blockFor(ShellPOSIX, "claude", BeginMarker, EndMarker)
	base := "export PATH=/x\n\n" + claudeBlk + "\n"

	codexBlk := blockFor(ShellPOSIX, "codex", codexBegin, codexEnd)
	got := upsertBlockWithMarkers(base, codexBlk, codexBegin, codexEnd)

	if !strings.Contains(got, BeginMarker) {
		t.Error("claude block must be preserved when adding codex")
	}
	if !strings.Contains(got, codexBegin) {
		t.Error("codex block must be added")
	}
	if !strings.Contains(got, "export PATH=/x") {
		t.Error("user content must be preserved")
	}
}

// TestUpsertBlockWithMarkers_ReplacesOwn: re-installing codex replaces only the
// codex block (idempotent), never duplicating it.
func TestUpsertBlockWithMarkers_ReplacesOwn(t *testing.T) {
	codexBlk := blockFor(ShellPOSIX, "codex", codexBegin, codexEnd)
	once := upsertBlockWithMarkers("", codexBlk, codexBegin, codexEnd)
	twice := upsertBlockWithMarkers(once, codexBlk, codexBegin, codexEnd)
	if strings.Count(twice, codexBegin) != 1 {
		t.Errorf("codex block should appear exactly once after re-install, got %d:\n%s",
			strings.Count(twice, codexBegin), twice)
	}
}

// TestUninstallWith_CodexSpec_PreservesClaude: uninstalling the Codex block via
// UninstallWith{Spec} removes only Codex, leaving Claude's block in the profile.
func TestUninstallWith_CodexSpec_PreservesClaude(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".bashrc")
	claudeBlk := blockFor(ShellPOSIX, "claude", BeginMarker, EndMarker)
	codexBlk := blockFor(ShellPOSIX, "codex", codexBegin, codexEnd)
	seed := "export X=1\n\n" + claudeBlk + "\n\n" + codexBlk + "\n"
	if err := safepath.WriteFile(home, profile, []byte(seed)); err != nil {
		t.Fatal(err)
	}

	removed, err := UninstallWith(home, UninstallOptions{
		Spec:             CLISpec{Name: "codex", Begin: codexBegin, End: codexEnd},
		PowerShellResult: &ProfileResolverResult{}, // avoid touching real PS profiles
	})
	if err != nil {
		t.Fatalf("UninstallWith: %v", err)
	}
	if len(removed) == 0 {
		t.Error("expected the codex block to be removed from at least one profile")
	}

	data, _ := safepath.ReadFile(home, profile)
	got := string(data)
	if strings.Contains(got, codexBegin) {
		t.Error("codex block should be gone")
	}
	if !strings.Contains(got, BeginMarker) {
		t.Error("claude block must be preserved")
	}
	if !strings.Contains(got, "export X=1") {
		t.Error("user content must be preserved")
	}
}

// TestUpsertBlock_ClaudeUnchanged: the legacy upsertBlock (Claude) still behaves
// identically — delegating to the marker-parameterized version must not drift.
func TestUpsertBlock_ClaudeUnchanged(t *testing.T) {
	base := "line1\n"
	legacy := base + "\n" + Block(ShellPOSIX) + "\n"
	got := upsertBlock(base, Block(ShellPOSIX))
	if got != legacy {
		t.Errorf("legacy upsertBlock drifted:\n got:\n%q\n want:\n%q", got, legacy)
	}
}
