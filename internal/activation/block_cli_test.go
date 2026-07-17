package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
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

// TestBlockFor_Codex: a codex wrapper calls the multi-CLI-only launch command,
// names the function `codex`, and uses the Codex markers.
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
	if !strings.Contains(got, "juggernaut launch-cli codex --") {
		t.Errorf("expected `juggernaut launch-cli codex --` delegation, got:\n%s", got)
	}
	if strings.Contains(got, "juggernaut launch codex --") {
		t.Errorf("version-skew-unsafe launch form must not be emitted:\n%s", got)
	}
	if !strings.Contains(got, "command codex \"$@\"") {
		t.Errorf("codex block must fall through to real codex when juggernaut is missing:\n%s", got)
	}
	if strings.Contains(got, "claude") {
		t.Errorf("codex block must not mention claude:\n%s", got)
	}
}

// TestBlockFor_CodexFish/PowerShell smoke-check the other shells name the fn
// and include multi-CLI fallthrough (same breakage mode as Claude on Windows).
func TestBlockFor_CodexOtherShells(t *testing.T) {
	begin, end := "# B", "# E"
	fish := blockFor(ShellFish, "codex", begin, end)
	if !strings.Contains(fish, "function codex") {
		t.Errorf("fish: expected `function codex`, got:\n%s", fish)
	}
	if !strings.Contains(fish, "command -q juggernaut") || !strings.Contains(fish, "command codex $argv") {
		t.Errorf("fish: expected juggernaut check + codex fallthrough, got:\n%s", fish)
	}
	ps := blockFor(ShellPowerShell, "codex", begin, end)
	if !strings.Contains(ps, "function global:codex {") {
		t.Errorf("powershell: expected `function global:codex {`, got:\n%s", ps)
	}
	if !strings.Contains(ps, "Get-Command juggernaut") || !strings.Contains(ps, "CommandType Application") {
		t.Errorf("powershell: expected resilient juggernaut check, got:\n%s", ps)
	}
	if !strings.Contains(ps, "Get-Command codex") || !strings.Contains(ps, "$app.Path") {
		t.Errorf("powershell: expected Application Path fallthrough for codex, got:\n%s", ps)
	}
	if strings.Contains(ps, "$app.Source") {
		t.Errorf("powershell: must not use $app.Source for Application fallback:\n%s", ps)
	}
}

// TestInstallTargetFor_RejectsShellInjection rejects CLI names that would be
// interpolated into generated shell profiles (function names / command tokens).
func TestInstallTargetFor_RejectsShellInjection(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".bashrc")
	if err := safepath.WriteFile(home, path, []byte("# seed\n")); err != nil {
		t.Fatal(err)
	}
	evil := []CLISpec{
		{Name: "claude; rm -rf /", Begin: "# B", End: "# E"},
		{Name: "x$(id)", Begin: "# B", End: "# E"},
		{Name: "x`id`", Begin: "# B", End: "# E"},
		{Name: "../evil", Begin: "# B", End: "# E"},
		{Name: "ok", Begin: "# B\nevil", End: "# E"},
		{Name: "ok", Begin: "# B", End: "# B"}, // identical markers
	}
	for _, spec := range evil {
		_, err := InstallTargetFor(Target{Path: path, Shell: ShellPOSIX}, spec)
		if err == nil {
			t.Errorf("expected rejection for name=%q begin=%q end=%q", spec.Name, spec.Begin, spec.End)
		}
	}
	// Valid names must still install.
	ok, err := InstallTargetFor(Target{Path: path, Shell: ShellPOSIX}, CLISpec{
		Name: "claude", Begin: BeginMarker, End: EndMarker,
	})
	if err != nil || !ok {
		t.Fatalf("valid install failed: changed=%v err=%v", ok, err)
	}
}
