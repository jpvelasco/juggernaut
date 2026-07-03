package activation

import (
	"strings"
	"testing"
)

// TestBlockFor_ClaudeByteIdentical: the per-CLI block generator must produce
// EXACTLY the legacy Block() output for Claude (name "claude", the Claude
// markers, bare `juggernaut launch`), so existing profiles are unchanged.
func TestBlockFor_ClaudeByteIdentical(t *testing.T) {
	for _, sh := range []Shell{ShellPOSIX, ShellFish, ShellPowerShell} {
		legacy := Block(sh)
		got := blockFor(sh, "claude", BeginMarker, EndMarker)
		if got != legacy {
			t.Errorf("shell %s: blockFor(claude) drifted from legacy Block():\n got:\n%s\n want:\n%s", sh, got, legacy)
		}
	}
}

// TestBlockFor_Codex: a codex wrapper calls `juggernaut launch codex --`, names
// the function `codex`, and uses the Codex markers.
func TestBlockFor_Codex(t *testing.T) {
	begin := "# BEGIN: Juggernaut Codex Activation"
	end := "# END: Juggernaut Codex Activation"
	got := blockFor(ShellPOSIX, "codex", begin, end)

	if !strings.Contains(got, begin) || !strings.Contains(got, end) {
		t.Errorf("missing codex markers:\n%s", got)
	}
	if !strings.Contains(got, "codex()") {
		t.Errorf("expected codex() function, got:\n%s", got)
	}
	if !strings.Contains(got, "juggernaut launch codex --") {
		t.Errorf("expected `juggernaut launch codex --` delegation, got:\n%s", got)
	}
	if strings.Contains(got, "claude") {
		t.Errorf("codex block must not mention claude:\n%s", got)
	}
}

// TestBlockFor_CodexFish/PowerShell smoke-check the other shells name the fn.
func TestBlockFor_CodexOtherShells(t *testing.T) {
	begin, end := "# B", "# E"
	fish := blockFor(ShellFish, "codex", begin, end)
	if !strings.Contains(fish, "function codex") {
		t.Errorf("fish: expected `function codex`, got:\n%s", fish)
	}
	ps := blockFor(ShellPowerShell, "codex", begin, end)
	if !strings.Contains(ps, "function global:codex {") {
		t.Errorf("powershell: expected `function global:codex {`, got:\n%s", ps)
	}
}
