package activation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestInstallTargetFor_Coexist installs a Claude block then a Codex block into
// the same profile and asserts both survive.
func TestInstallTargetFor_Coexist(t *testing.T) {
	home := testutil.NewTestHome(t)
	profile := filepath.Join(home, ".bashrc")
	if err := safepath.WriteFile(home, profile, []byte("export X=1\n")); err != nil {
		t.Fatal(err)
	}
	tgt := Target{Path: profile, Shell: ShellPOSIX}

	claudeSpec := CLISpec{Name: "claude", Begin: BeginMarker, End: EndMarker}
	codexSpec := CLISpec{
		Name:  "codex",
		Begin: "# BEGIN: Juggernaut Codex Activation",
		End:   "# END: Juggernaut Codex Activation",
	}

	if _, err := InstallTargetFor(tgt, claudeSpec); err != nil {
		t.Fatalf("install claude: %v", err)
	}
	if _, err := InstallTargetFor(tgt, codexSpec); err != nil {
		t.Fatalf("install codex: %v", err)
	}

	data, _ := safepath.ReadFile(filepath.Dir(profile), profile)
	got := string(data)
	if !strings.Contains(got, "claude()") {
		t.Error("claude block lost after installing codex")
	}
	if !strings.Contains(got, "codex()") {
		t.Error("codex block not installed")
	}
	if !strings.Contains(got, "juggernaut launch-cli codex --") {
		t.Error("codex block should delegate to `launch-cli codex`")
	}
	if !strings.Contains(got, "export X=1") {
		t.Error("user content lost")
	}
}

// TestInstallTargetFor_ClaudeMatchesLegacy: installing via the claude CLISpec
// produces the same profile content as the legacy InstallTarget.
func TestInstallTargetFor_ClaudeMatchesLegacy(t *testing.T) {
	home := testutil.NewTestHome(t)
	legacyProfile := filepath.Join(home, "legacy.sh")
	specProfile := filepath.Join(home, "spec.sh")
	seed := "export X=1\n"
	_ = safepath.WriteFile(home, legacyProfile, []byte(seed))
	_ = safepath.WriteFile(home, specProfile, []byte(seed))

	if _, err := InstallTarget(Target{Path: legacyProfile, Shell: ShellPOSIX}); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallTargetFor(Target{Path: specProfile, Shell: ShellPOSIX},
		CLISpec{Name: "claude", Begin: BeginMarker, End: EndMarker}); err != nil {
		t.Fatal(err)
	}

	a, _ := safepath.ReadFile(filepath.Dir(legacyProfile), legacyProfile)
	b, _ := safepath.ReadFile(filepath.Dir(specProfile), specProfile)
	if string(a) != string(b) {
		t.Errorf("claude spec install drifted from legacy:\n legacy:\n%s\n spec:\n%s", a, b)
	}
}
