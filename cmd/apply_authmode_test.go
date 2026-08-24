package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// Given the only valid auth modes are `iam` and `bedrock-api-key`,
// When the user runs apply with a misspelled mode plus --bedrock-key,
// Then apply must fail with an error naming the valid modes instead of
// writing a config that can never authenticate while silently discarding
// the provided key.
func TestApply_RejectsUnknownAuthMode(t *testing.T) {
	home := setupApplyTestWithReset(t)

	err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--scope=user", "--region=us-west-2",
		"--skip-preflight", "--auth=bogus-mode", bedrockKeyFlag(),
	})

	if err == nil {
		t.Fatal("apply accepted unknown --auth=bogus-mode; want explicit rejection")
	}
	if !strings.Contains(err.Error(), authmode.IAM) ||
		!strings.Contains(err.Error(), authmode.BedrockAPIKey) {
		t.Errorf("error should name the valid modes, got: %v", err)
	}
	if _, readErr := safepath.ReadFile(home, filepath.Join(home, ".claude", "settings.json")); readErr == nil {
		t.Error("a rejected apply must not write settings.json")
	}
}

// Given both supported modes are accepted,
// When apply runs with each exact value,
// Then validation passes them through untouched.
func TestApply_AcceptsExactAuthModes(t *testing.T) {
	for _, mode := range []string{authmode.IAM, authmode.BedrockAPIKey} {
		t.Run(mode, func(t *testing.T) {
			home := setupApplyTestWithReset(t)
			if mode == authmode.BedrockAPIKey {
				// The api-key apply persists a credential; isolate the
				// keychain so CI never touches (or hangs on) a live backend.
				setupIsolatedKeychain(t)
			}
			args := []string{
				"apply", "--cli=claude", "--scope=user", "--region=us-west-2",
				"--skip-preflight", "--auth=" + mode,
			}
			if mode == authmode.BedrockAPIKey {
				args = append(args, bedrockKeyFlag())
			}
			if err := ExecuteArgs(args); err != nil {
				t.Fatalf("apply with --auth=%s should succeed, got: %v", mode, err)
			}
			if got := readJuggernautAuthMode(t, home); got != mode {
				t.Errorf("persisted auth.mode = %q, want %q", got, mode)
			}
		})
	}
}
