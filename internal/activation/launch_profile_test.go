package activation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// launchProfileFixture writes a managed config (JSON or TOML by suffix) whose
// juggernaut block declares an auth mode plus a named profile, launches, and
// returns the env the recorded runner received.
func launchProfileFixture(t *testing.T, cfgPath, content string) []string {
	t.Helper()
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	realDir := t.TempDir()
	binary := "codex"
	if runtime.GOOS == "windows" {
		binary = "codex.exe"
	}
	writeExecutableFile(t, realDir, filepath.Join(realDir, binary), "real cli")

	var gotEnv []string
	var err = LaunchWithOptions(LaunchOptions{
		Home:        testutil.NewTestHome(t),
		Args:        []string{"--version"},
		Path:        realDir,
		TokenGetter: func() (string, error) { return "", nil },
		Runner: func(_ string, _ []string, env []string) error {
			gotEnv = env
			return nil
		},
		Target: LaunchTarget{
			BinaryNames: []string{binary},
			TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK", // #nosec G101 -- env var name, not a credential
			NeedsToken:  false,
			ConfigPath:  cfgPath,
		},
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	return gotEnv
}

func launchProfileConfig(profile string) string {
	if profile == "" {
		return `[juggernaut]
  [juggernaut.auth]
    mode = "iam"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
	}
	return `[juggernaut]
  [juggernaut.auth]
    mode = "iam"
    profile = "` + profile + `"
  [juggernaut.meta]
    managedBy = "juggernaut"
`
}

// TestLaunchWithOptions_ExportsProfileEnv: when the juggernaut block carries a
// named profile and the parent environment does not define AWS_PROFILE, the
// launched CLI must get AWS_PROFILE=<profile> so its credential chain uses the
// pinned profile instead of the missing default chain.
func TestLaunchWithOptions_ExportsProfileEnv(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	env := launchProfileFixture(t, cfgPath, launchProfileConfig("developer"))
	if got := envValue(env, "AWS_PROFILE"); got != "developer" {
		t.Errorf("AWS_PROFILE = %q, want developer exported from the block", got)
	}
}

// TestLaunchWithOptions_ProfileRespectsParentEnv: a parent shell that already
// exports AWS_PROFILE owns the decision — the launcher must not override a
// value the user chose for the session.
func TestLaunchWithOptions_ProfileRespectsParentEnv(t *testing.T) {
	t.Setenv("AWS_PROFILE", "parent-choice")

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	env := launchProfileFixture(t, cfgPath, launchProfileConfig("developer"))
	if got := envValue(env, "AWS_PROFILE"); got != "parent-choice" {
		t.Errorf("AWS_PROFILE = %q, want the parent's choice preserved", got)
	}
}

// TestLaunchWithOptions_NoProfileNoExport: a block without a profile must not
// change AWS_PROFILE — the ambient chain is the contract. The parent env keeps
// whatever it had (here pinned to empty: no profile injected from anywhere).
func TestLaunchWithOptions_NoProfileNoExport(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	env := launchProfileFixture(t, cfgPath, launchProfileConfig(""))
	if got := envValue(env, "AWS_PROFILE"); got != "" {
		t.Errorf("AWS_PROFILE = %q, want untouched-empty without a stored profile, env=%v", got, env)
	}
}

// TestLaunchWithOptions_JSONConfigProfile: sidecar-style JSON configs (OpenCode
// sidecar, Claude settings.json) carry the profile too and must be read.
func TestLaunchWithOptions_JSONConfigProfile(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")

	cfgPath := filepath.Join(t.TempDir(), ".juggernaut.json")
	env := launchProfileFixture(t, cfgPath, `{
  "juggernaut": {
    "auth": {"mode": "iam", "region": "us-west-2", "profile": "developer"},
    "meta": {"managedBy": "juggernaut"}
  }
}
`)
	if got := envValue(env, "AWS_PROFILE"); got != "developer" {
		t.Errorf("AWS_PROFILE = %q, want developer exported from the JSON block", got)
	}
}
