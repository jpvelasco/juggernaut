package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestApply_Claude_ForeignConfig_Refuses: a hand-written settings.json (no
// Juggernaut marker) that already sets a managed key ("model") must cause
// apply to refuse rather than silently overwrite it.
func TestApply_Claude_ForeignConfig_Refuses(t *testing.T) {
	home := setupApplyTest(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	foreign := `{"model": "my-own-model-id"}`
	if err := safepath.WriteFile(claudeDir, settingsPath, []byte(foreign)); err != nil {
		t.Fatalf("write foreign settings.json: %v", err)
	}

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected apply to refuse a foreign config with a colliding key")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("error should name the colliding key, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}

	// The foreign file must be untouched — refusal happens before any write.
	got, err := safepath.ReadFile(claudeDir, settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	if string(got) != foreign {
		t.Errorf("foreign settings.json must be untouched on refusal, got: %s", got)
	}
}

// TestApply_Claude_ForeignConfig_Force_Overwrites: --force bypasses the
// refusal and proceeds, still rotating a backup of the foreign file.
func TestApply_Claude_ForeignConfig_Force_Overwrites(t *testing.T) {
	home := setupApplyTest(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := safepath.WriteFile(claudeDir, settingsPath, []byte(`{"model": "my-own-model-id"}`)); err != nil {
		t.Fatalf("write foreign settings.json: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight", "--force",
	}); err != nil {
		t.Fatalf("apply --force should succeed: %v", err)
	}

	data := readSettingsJSON(t, home)
	if !strings.Contains(string(data), "juggernaut") {
		t.Error("expected settings.json to be written after --force")
	}

	matches, err := filepath.Glob(settingsPath + ".backup.*")
	if err != nil {
		t.Fatalf("globbing backups: %v", err)
	}
	if len(matches) == 0 {
		t.Error("expected a backup of the foreign config to be created even with --force")
	}
}

// TestApply_Claude_ForeignConfig_NoCollidingKeys_ProceedsWithoutForce: a
// foreign file that has content but NONE of it collides with Juggernaut's
// managed keys must apply cleanly without --force.
func TestApply_Claude_ForeignConfig_NoCollidingKeys_ProceedsWithoutForce(t *testing.T) {
	home := setupApplyTest(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := safepath.WriteFile(claudeDir, settingsPath, []byte(`{"someUnrelatedKey": "keep-me"}`)); err != nil {
		t.Fatalf("write foreign settings.json: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply should proceed when no keys collide: %v", err)
	}

	data := readSettingsJSON(t, home)
	if !strings.Contains(string(data), "someUnrelatedKey") {
		t.Error("unrelated foreign key must survive")
	}
	if !strings.Contains(string(data), "juggernaut") {
		t.Error("expected settings.json to be written")
	}
}

// TestApply_Claude_ForeignConfig_EnvSiblingSurvives: a user's own env var
// sitting alongside what Juggernaut writes must never trigger a refusal.
func TestApply_Claude_ForeignConfig_EnvSiblingSurvives(t *testing.T) {
	home := setupApplyTest(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := safepath.WriteFile(claudeDir, settingsPath, []byte(`{"env": {"NODE_ENV": "production"}}`)); err != nil {
		t.Fatalf("write foreign settings.json: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply should proceed when only sibling env vars are present: %v", err)
	}
}

// TestApply_Claude_ForeignConfig_PermissionsRulesSurviveNoForceNeeded: a
// user's own allow/deny permission rules must not trigger a refusal even
// though Juggernaut also writes into "permissions".
func TestApply_Claude_ForeignConfig_PermissionsRulesSurviveNoForceNeeded(t *testing.T) {
	home := setupApplyTest(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := safepath.WriteFile(claudeDir, settingsPath, []byte(`{"permissions": {"allow": ["Bash(ls:*)"]}}`)); err != nil {
		t.Fatalf("write foreign settings.json: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply should proceed when only unrelated permission rules are present: %v", err)
	}

	data := readSettingsJSON(t, home)
	if !strings.Contains(string(data), "Bash(ls:*)") {
		t.Error("user's permission rule must survive")
	}
}

// TestApply_Grok_ForeignConfig_Refuses: Grok's owned models.default leaf
// already present under a foreign (unmarked) config triggers a refusal. Using
// model.bedrock-grok itself would double as Grok's OwnsConfig marker (making
// the fixture indistinguishable from a legitimate re-apply), so this collides
// on a different owned leaf (models.default) instead.
func TestApply_Grok_ForeignConfig_Refuses(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // apply --bedrock-key stores a real credential; skip if backend hangs (macOS CI)

	grokDir := filepath.Join(home, ".grok")
	if err := safepath.MkdirAll(grokDir); err != nil {
		t.Fatalf("mkdir .grok: %v", err)
	}
	configPath := filepath.Join(grokDir, "config.toml")
	foreign := "[models]\ndefault = \"their-own-default\"\n"
	if err := safepath.WriteFile(grokDir, configPath, []byte(foreign)); err != nil {
		t.Fatalf("write foreign grok config: %v", err)
	}

	err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(), "--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected apply to refuse a foreign grok config with a colliding models.default leaf")
	}
	if !strings.Contains(err.Error(), "models.default") {
		t.Errorf("error should name the colliding path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}

	got, err := safepath.ReadFile(grokDir, configPath)
	if err != nil {
		t.Fatalf("reading config.toml: %v", err)
	}
	if string(got) != foreign {
		t.Errorf("foreign grok config must be untouched on refusal, got: %s", got)
	}

	// Refusal must have zero side effects — the credential must not have been
	// written to the keychain before the collision check ran.
	if token, _ := keychain.Default().GetWithFallback(home); token != "" {
		t.Errorf("refused apply must not store a credential in the keychain, got: %q", token)
	}
}

// TestApply_Grok_ForeignConfig_SiblingProfileSurvivesNoForceNeeded: a user's
// own sibling model profile in Grok's [model] table must never collide.
func TestApply_Grok_ForeignConfig_SiblingProfileSurvivesNoForceNeeded(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // apply proceeds and stores a real credential; skip if backend hangs (macOS CI)

	grokDir := filepath.Join(home, ".grok")
	if err := safepath.MkdirAll(grokDir); err != nil {
		t.Fatalf("mkdir .grok: %v", err)
	}
	configPath := filepath.Join(grokDir, "config.toml")
	if err := safepath.WriteFile(grokDir, configPath, []byte("[model.my-own-profile]\nbase_url = \"http://192.168.0.1/v1\"\n")); err != nil {
		t.Fatalf("write grok config: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(), "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply should proceed when only a sibling model profile is present: %v", err)
	}

	got, err := safepath.ReadFile(grokDir, configPath)
	if err != nil {
		t.Fatalf("reading config.toml: %v", err)
	}
	if !strings.Contains(string(got), "my-own-profile") {
		t.Error("user's sibling model profile must survive")
	}
	if !strings.Contains(string(got), "bedrock-grok") {
		t.Error("expected Juggernaut's bedrock-grok block to be written")
	}
}

// TestApply_DryRun_ForeignConfig_TOML_ReportsCollisionsWithoutWriting:
// --dry-run must surface the same refusal information for the TOML providers
// (Codex's exact colliding dotted path, Grok's models.default) without
// writing anything or creating a backup. This is the same contract Claude's
// dry-run test locks in — provider-agnostic behavior that only Claude was
// pinning down. Dry-run never resolves a credential, so no keychain is
// required.
func TestApply_DryRun_ForeignConfig_TOML_ReportsCollisionsWithoutWriting(t *testing.T) {
	cases := []struct {
		name             string
		cli, dir, region string
		foreign          string
		wantCollision    string
	}{
		{
			name: "codex", cli: "codex", dir: ".codex", region: "us-east-1",
			foreign:       "model = \"their-model\"\n\n[model_providers.amazon-bedrock.aws]\nregion = \"eu-west-1\"\n",
			wantCollision: "model_providers.amazon-bedrock.aws.region",
		},
		{
			name: "grok", cli: "grok", dir: ".grok", region: "us-west-2",
			foreign:       "[models]\ndefault = \"their-own-default\"\n",
			wantCollision: "models.default",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := setupApplyTest(t)

			configDir := filepath.Join(home, c.dir)
			if err := safepath.MkdirAll(configDir); err != nil {
				t.Fatalf("mkdir %s: %v", c.dir, err)
			}
			configPath := filepath.Join(configDir, "config.toml")
			if err := safepath.WriteFile(configDir, configPath, []byte(c.foreign)); err != nil {
				t.Fatalf("write foreign %s config: %v", c.cli, err)
			}

			out := captureStdout(t, func() {
				err := ExecuteArgs([]string{
					"apply", "--cli=" + c.cli, "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
					"--region=" + c.region, "--dry-run", "--skip-preflight",
				})
				if err != nil {
					t.Fatalf("dry-run should not error, it should report: %v", err)
				}
			})
			if !strings.Contains(out, c.wantCollision) {
				t.Errorf("dry-run should report the colliding path, got:\n%s", out)
			}
			if !strings.Contains(out, "--force") {
				t.Errorf("dry-run should mention --force, got:\n%s", out)
			}

			got, err := safepath.ReadFile(configDir, configPath)
			if err != nil {
				t.Fatalf("reading config.toml: %v", err)
			}
			if string(got) != c.foreign {
				t.Error("dry-run must not modify the foreign file")
			}
			matches, _ := filepath.Glob(configPath + ".backup.*")
			if len(matches) != 0 {
				t.Error("dry-run must not create a backup")
			}
		})
	}
}

// TestApply_Codex_ForeignConfig_DottedLeafCollision_Refuses: Codex's owned
// leaf (model_providers.amazon-bedrock.aws.region) already present under a
// foreign config triggers a refusal, even though the config has no
// Juggernaut marker (model_provider != "amazon-bedrock" in this fixture would
// normally make it look unowned — here we simulate a user who independently
// set up their own amazon-bedrock region without Juggernaut's marker).
func TestApply_Codex_ForeignConfig_DottedLeafCollision_Refuses(t *testing.T) {
	home := setupApplyTest(t)

	codexDir := filepath.Join(home, ".codex")
	if err := safepath.MkdirAll(codexDir); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	configPath := filepath.Join(codexDir, "config.toml")
	foreign := "model = \"their-model\"\n\n[model_providers.amazon-bedrock.aws]\nregion = \"eu-west-1\"\n"
	if err := safepath.WriteFile(codexDir, configPath, []byte(foreign)); err != nil {
		t.Fatalf("write foreign codex config: %v", err)
	}

	err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(), "--region=us-east-1", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected apply to refuse a foreign codex config with a colliding region leaf")
	}
	if !strings.Contains(err.Error(), "model_providers.amazon-bedrock.aws.region") {
		t.Errorf("error should name the colliding dotted path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}

	// The foreign file must be untouched — refusal happens before any write.
	got, err := safepath.ReadFile(codexDir, configPath)
	if err != nil {
		t.Fatalf("reading config.toml: %v", err)
	}
	if string(got) != foreign {
		t.Errorf("foreign codex config must be untouched on refusal, got: %s", got)
	}
}

// TestApply_OpenCode_ForeignConfig_Refuses: OpenCode's whole-value "model" key
// already present under a foreign config triggers a refusal. Colliding on
// provider.bedrock-mantle itself would double as OpenCode's OwnsConfig marker
// (making the fixture indistinguishable from a legitimate re-apply), so this
// uses the sibling "model" key instead.
func TestApply_OpenCode_ForeignConfig_Refuses(t *testing.T) {
	home := setupApplyTest(t)

	opencodeDir := filepath.Join(home, ".config", "opencode")
	if err := safepath.MkdirAll(opencodeDir); err != nil {
		t.Fatalf("mkdir opencode config dir: %v", err)
	}
	configPath := filepath.Join(opencodeDir, "opencode.json")
	foreign := `{"model": "their-own-provider/their-own-model"}`
	if err := safepath.WriteFile(opencodeDir, configPath, []byte(foreign)); err != nil {
		t.Fatalf("write foreign opencode config: %v", err)
	}

	err := ExecuteArgs([]string{
		"apply", "--cli=opencode", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(), "--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected apply to refuse a foreign opencode config with a colliding model key")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("error should name the colliding path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}

	got, err := safepath.ReadFile(opencodeDir, configPath)
	if err != nil {
		t.Fatalf("reading opencode.json: %v", err)
	}
	if string(got) != foreign {
		t.Errorf("foreign opencode config must be untouched on refusal, got: %s", got)
	}
}

// TestApply_DryRun_ForeignConfig_JSON_ReportsCollisionsWithoutWriting:
// --dry-run must surface the same refusal information for the JSON providers
// (Claude's settings.json, OpenCode's opencode.json) without writing anything
// or creating a backup. This is the same contract the TOML providers' dry-run
// table locks in — provider-agnostic behavior that only Claude was pinning
// down. Dry-run never resolves a credential, so no keychain is required.
func TestApply_DryRun_ForeignConfig_JSON_ReportsCollisionsWithoutWriting(t *testing.T) {
	cases := []struct {
		name           string
		cli, dir, file string
		auth, region   string
		keyFlag        string
		foreign        string
		wantCollision  string
	}{
		{
			name: "claude", cli: "", dir: ".claude", file: "settings.json",
			region: "us-west-2", auth: authmode.IAM,
			foreign:       `{"model": "my-own-model-id"}`,
			wantCollision: "model",
		},
		{
			name: "opencode", cli: "opencode", dir: filepath.Join(".config", "opencode"), file: "opencode.json",
			region: "us-west-2", auth: authmode.BedrockAPIKey, keyFlag: bedrockKeyFlag(),
			foreign:       `{"model": "their-own-provider/their-own-model"}`,
			wantCollision: "model",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := setupApplyTest(t)

			configDir := filepath.Join(home, c.dir)
			if err := safepath.MkdirAll(configDir); err != nil {
				t.Fatalf("mkdir %s: %v", c.dir, err)
			}
			configPath := filepath.Join(configDir, c.file)
			if err := safepath.WriteFile(configDir, configPath, []byte(c.foreign)); err != nil {
				t.Fatalf("write foreign %s config: %v", c.name, err)
			}

			args := []string{"apply"}
			if c.cli != "" {
				args = append(args, "--cli="+c.cli)
			}
			args = append(args, "--auth="+c.auth, "--region="+c.region, "--dry-run", "--skip-preflight")
			if c.keyFlag != "" {
				args = append(args, c.keyFlag)
			}
			out := captureStdout(t, func() {
				err := ExecuteArgs(args)
				if err != nil {
					t.Fatalf("dry-run should not error, it should report: %v", err)
				}
			})
			if !strings.Contains(out, c.wantCollision) {
				t.Errorf("dry-run should report the colliding key, got:\n%s", out)
			}
			if !strings.Contains(out, "--force") {
				t.Errorf("dry-run should mention --force, got:\n%s", out)
			}

			got, err := safepath.ReadFile(configDir, configPath)
			if err != nil {
				t.Fatalf("reading %s: %v", c.file, err)
			}
			if string(got) != c.foreign {
				t.Error("dry-run must not modify the foreign file")
			}
			matches, _ := filepath.Glob(configPath + ".backup.*")
			if len(matches) != 0 {
				t.Error("dry-run must not create a backup")
			}
		})
	}
}

// TestApply_ReApply_OwnedConfig_NoNewFriction: re-applying over a config
// Juggernaut already wrote must proceed with zero new prompts/errors, even
// though the file necessarily already has Juggernaut's own managed keys
// populated (which would otherwise look like collisions).
func TestApply_ReApply_OwnedConfig_NoNewFriction(t *testing.T) {
	home := setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply over Juggernaut-owned config must not require --force: %v", err)
	}

	data := readSettingsJSON(t, home)
	if !strings.Contains(string(data), "us-east-1") {
		t.Error("re-apply should have updated the region")
	}
}

// TestApply_ReApply_OwnedConfig_NoNewFriction_NonClaudeProviders: the same
// zero-new-friction guarantee for Codex, Grok, and OpenCode — each has its
// own OwnsConfig() marker, and collision detection must not mistake a
// provider's own prior values (written on the first apply) for a foreign
// collision on re-apply.
func TestApply_ReApply_OwnedConfig_NoNewFriction_NonClaudeProviders(t *testing.T) {
	for _, cli := range []string{"codex", "grok", "opencode"} {
		t.Run(cli, func(t *testing.T) {
			_ = setupApplyTest(t)
			setupIsolatedKeychain(t) // stores a real credential; skip if backend hangs (macOS CI)

			if err := ExecuteArgs([]string{
				"apply", "--cli=" + cli, "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
				"--region=us-west-2", "--skip-preflight",
			}); err != nil {
				t.Fatalf("first apply: %v", err)
			}

			if err := ExecuteArgs([]string{
				"apply", "--cli=" + cli, "--region=us-east-1", "--skip-preflight",
			}); err != nil {
				t.Fatalf("re-apply over Juggernaut-owned %s config must not require --force: %v", cli, err)
			}
		})
	}
}

// TestApply_ForeignConfig_Force_AllProviders: --force overwrites and a
// backup is created for every provider, not just Claude — the merge/write
// path is provider-agnostic and this must hold uniformly.
func TestApply_ForeignConfig_Force_AllProviders(t *testing.T) {
	cases := []struct {
		cli, dir, file, foreign string
	}{
		{"codex", ".codex", "config.toml", "model = \"their-model\"\n"},
		{"grok", ".grok", "config.toml", "[models]\ndefault = \"their-default\"\n"},
		{"opencode", filepath.Join(".config", "opencode"), "opencode.json", `{"model": "their-provider/their-model"}`},
	}
	for _, c := range cases {
		t.Run(c.cli, func(t *testing.T) {
			home := setupApplyTest(t)
			setupIsolatedKeychain(t) // --force proceeds and stores a real credential; skip if backend hangs (macOS CI)

			dir := filepath.Join(home, c.dir)
			if err := safepath.MkdirAll(dir); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			configPath := filepath.Join(dir, c.file)
			if err := safepath.WriteFile(dir, configPath, []byte(c.foreign)); err != nil {
				t.Fatalf("write foreign config: %v", err)
			}

			if err := ExecuteArgs([]string{
				"apply", "--cli=" + c.cli, "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
				"--region=us-west-2", "--skip-preflight", "--force",
			}); err != nil {
				t.Fatalf("apply --force should succeed for %s: %v", c.cli, err)
			}

			matches, err := filepath.Glob(configPath + ".backup.*")
			if err != nil {
				t.Fatalf("globbing backups: %v", err)
			}
			if len(matches) == 0 {
				t.Errorf("expected a backup of the foreign %s config to be created with --force", c.cli)
			}
		})
	}
}

// TestApply_Claude_ForeignConfig_MalformedJSON_ErrorsNotSilentlyIgnored:
// a syntactically invalid existing config must surface the parse error, not
// be silently treated as "empty, no collision, safe to proceed" — the read
// failure inside detectForeignCollisions must propagate.
func TestApply_Claude_ForeignConfig_MalformedJSON_ErrorsNotSilentlyIgnored(t *testing.T) {
	home := setupApplyTest(t)

	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := safepath.WriteFile(claudeDir, settingsPath, []byte("{not valid json")); err != nil {
		t.Fatalf("write malformed settings.json: %v", err)
	}

	err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected apply to error on malformed existing config, not silently proceed")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error should surface the parse failure, got: %v", err)
	}
	if strings.Contains(err.Error(), "refusing to modify") {
		t.Error("a parse error must not be misreported as a collision refusal")
	}
}

// TestApply_Grok_LegacyJuggernautConfig_PlainApplyMigrates: a plain apply
// (no --force) over a v5 Grok config (base_url pointing at Mantle) must
// migrate to the v6 bedrock-runtime config with a backup — not refuse.
func TestApply_Grok_LegacyJuggernautConfig_PlainApplyMigrates(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t) // apply stores a real credential; skip if backend hangs (macOS CI)

	grokDir := filepath.Join(home, ".grok")
	if err := safepath.MkdirAll(grokDir); err != nil {
		t.Fatalf("mkdir .grok: %v", err)
	}
	configPath := filepath.Join(grokDir, "config.toml")
	legacy := `[models]
default = "bedrock-grok"

[model.bedrock-grok]
model = "xai.grok-4.3"
base_url = "https://bedrock-mantle.us-west-2.api.aws/openai/v1"
name = "Amazon Bedrock (Mantle)"

[auth]
auth_provider_command = "juggernaut auth-token"
`
	if err := safepath.WriteFile(grokDir, configPath, []byte(legacy)); err != nil {
		t.Fatalf("write legacy grok config: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
			"--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("plain apply over legacy grok config should migrate, not refuse: %v", err)
		}
	})
	if !strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("apply should announce the v5→v6 migration, got:\n%s", out)
	}
	if !strings.Contains(out, "models refresh") {
		t.Errorf("apply should mention models refresh, got:\n%s", out)
	}

	// Verify the written config has the v6 bedrock-runtime URL, not Mantle.
	content, err := safepath.ReadFile(grokDir, configPath)
	if err != nil {
		t.Fatalf("read config after apply: %v", err)
	}
	written := string(content)
	if !strings.Contains(written, "bedrock-runtime") {
		t.Errorf("migrated config should contain bedrock-runtime base_url, got:\n%s", written)
	}
	if strings.Contains(written, "bedrock-mantle") {
		t.Errorf("migrated config must NOT contain bedrock-mantle URL, got:\n%s", written)
	}
	if !strings.Contains(written, "xai.grok-4.6") {
		t.Errorf("migrated config should have v6 default model xai.grok-4.6, got:\n%s", written)
	}

	// A backup of the legacy config should have been created.
	matches, _ := filepath.Glob(configPath + ".backup.*")
	if len(matches) == 0 {
		t.Error("expected a backup of the legacy grok config to be created")
	}
}

// TestApply_Codex_LegacyJuggernautConfig_PlainApplyMigrates: a plain apply
// (no --force) over a v5 Codex config (amazon-bedrock provider with old
// GPT-5.5 model ID) must migrate to the v6 model ID with a backup.
func TestApply_Codex_LegacyJuggernautConfig_PlainApplyMigrates(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)

	codexDir := filepath.Join(home, ".codex")
	if err := safepath.MkdirAll(codexDir); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	configPath := filepath.Join(codexDir, "config.toml")
	legacy := `model = "openai.gpt-5.5"
model_provider = "amazon-bedrock"

[model_providers.amazon-bedrock.aws]
region = "us-east-1"
`
	if err := safepath.WriteFile(codexDir, configPath, []byte(legacy)); err != nil {
		t.Fatalf("write legacy codex config: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
			"--region=us-west-2", "--skip-preflight",
		}); err != nil {
			t.Fatalf("plain apply over legacy codex config should migrate, not refuse: %v", err)
		}
	})
	if !strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("apply should announce the v5→v6 migration for codex, got:\n%s", out)
	}
	if !strings.Contains(out, "models refresh") {
		t.Errorf("apply should mention models refresh, got:\n%s", out)
	}

	content, err := safepath.ReadFile(codexDir, configPath)
	if err != nil {
		t.Fatalf("read config after apply: %v", err)
	}
	written := string(content)
	if !strings.Contains(written, "global.openai.gpt-5.6-sol") {
		t.Errorf("migrated config should have v6 default model global.openai.gpt-5.6-sol, got:\n%s", written)
	}
	if strings.Contains(written, "openai.gpt-5.5") {
		t.Errorf("migrated config must NOT contain old v5 model openai.gpt-5.5, got:\n%s", written)
	}
	assertCodexRuntimeMigration(t, configPath, "us-west-2")

	matches, _ := filepath.Glob(configPath + ".backup.*")
	if len(matches) == 0 {
		t.Error("expected a backup of the legacy codex config to be created")
	}
}

// TestApply_DryRun_LegacyJuggernautConfig_PreviewsMigration: a plain dry-run
// over a legacy v5 Grok config must preview the migration (announce + the
// standard "would write" output) without writing or creating a backup.
func TestApply_DryRun_LegacyJuggernautConfig_PreviewsMigration(t *testing.T) {
	home := setupApplyTest(t)

	grokDir := filepath.Join(home, ".grok")
	if err := safepath.MkdirAll(grokDir); err != nil {
		t.Fatalf("mkdir .grok: %v", err)
	}
	configPath := filepath.Join(grokDir, "config.toml")
	legacy := `[models]
default = "bedrock-grok"

[model.bedrock-grok]
model = "xai.grok-4.3"
base_url = "https://bedrock-mantle.us-west-2.api.aws/openai/v1"
name = "Amazon Bedrock (Mantle)"

[auth]
auth_provider_command = "juggernaut auth-token"
`
	if err := safepath.WriteFile(grokDir, configPath, []byte(legacy)); err != nil {
		t.Fatalf("write legacy grok config: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
			"--region=us-west-2", "--dry-run", "--skip-preflight",
		}); err != nil {
			t.Fatalf("dry-run should not error: %v", err)
		}
	})
	if !strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("dry-run should preview the v5→v6 migration, got:\n%s", out)
	}
	if !strings.Contains(out, "models refresh") {
		t.Errorf("dry-run should mention models refresh, got:\n%s", out)
	}
	if !strings.Contains(out, "Would write juggernaut config") {
		t.Errorf("dry-run should still show what would be written, got:\n%s", out)
	}

	got, err := safepath.ReadFile(grokDir, configPath)
	if err != nil {
		t.Fatalf("reading config.toml: %v", err)
	}
	if string(got) != legacy {
		t.Error("dry-run must not modify the legacy file")
	}
	matches, _ := filepath.Glob(configPath + ".backup.*")
	if len(matches) != 0 {
		t.Error("dry-run must not create a backup")
	}
}

// TestApply_ForeignConfig_NotLegacy_NoMigrationHint: a foreign config that
// does NOT look like a Juggernaut v5 config must NOT trigger the migration
// hint — it still requires --force.
func TestApply_ForeignConfig_NotLegacy_NoMigrationHint(t *testing.T) {
	home := setupApplyTest(t)

	grokDir := filepath.Join(home, ".grok")
	if err := safepath.MkdirAll(grokDir); err != nil {
		t.Fatalf("mkdir .grok: %v", err)
	}
	configPath := filepath.Join(grokDir, "config.toml")
	foreign := `[models]
default = "their-own-model"
`
	if err := safepath.WriteFile(grokDir, configPath, []byte(foreign)); err != nil {
		t.Fatalf("write foreign grok config: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{
			"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
			"--region=us-west-2", "--dry-run", "--skip-preflight",
		}); err != nil {
			t.Fatalf("dry-run should not error: %v", err)
		}
	})
	if strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("non-legacy foreign config must NOT trigger migration hint, got:\n%s", out)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("non-legacy foreign config must still mention --force, got:\n%s", out)
	}
}

// TestApply_Grok_LegacyJuggernautConfig_Force_MigratesToV6: --force over a
// legacy v5 Grok config must succeed, write the v6 bedrock-runtime base_url,
// and drop the Mantle URL. This is the actual migration path — the user
// doesn't have to hand-edit their config.
func TestApply_Grok_LegacyJuggernautConfig_Force_MigratesToV6(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)

	grokDir := filepath.Join(home, ".grok")
	if err := safepath.MkdirAll(grokDir); err != nil {
		t.Fatalf("mkdir .grok: %v", err)
	}
	configPath := filepath.Join(grokDir, "config.toml")
	legacy := `[models]
default = "bedrock-grok"

[model.bedrock-grok]
model = "xai.grok-4.3"
base_url = "https://bedrock-mantle.us-west-2.api.aws/openai/v1"
name = "Amazon Bedrock (Mantle)"

[auth]
auth_provider_command = "juggernaut auth-token"
`
	if err := safepath.WriteFile(grokDir, configPath, []byte(legacy)); err != nil {
		t.Fatalf("write legacy grok config: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
		"--region=us-west-2", "--skip-preflight", "--force",
	}); err != nil {
		t.Fatalf("apply --force over legacy grok config should succeed: %v", err)
	}

	// Verify the written config has the v6 bedrock-runtime URL, not Mantle.
	content, err := safepath.ReadFile(grokDir, configPath)
	if err != nil {
		t.Fatalf("read config after apply: %v", err)
	}
	written := string(content)
	if !strings.Contains(written, "bedrock-runtime") {
		t.Errorf("migrated config should contain bedrock-runtime base_url, got:\n%s", written)
	}
	if strings.Contains(written, "bedrock-mantle") {
		t.Errorf("migrated config must NOT contain bedrock-mantle URL, got:\n%s", written)
	}
	if !strings.Contains(written, "xai.grok-4.6") {
		t.Errorf("migrated config should have v6 default model xai.grok-4.6, got:\n%s", written)
	}

	// A backup of the legacy config should have been created.
	matches, _ := filepath.Glob(configPath + ".backup.*")
	if len(matches) == 0 {
		t.Error("expected a backup of the legacy grok config to be created with --force")
	}
}

// TestApply_Codex_LegacyJuggernautConfig_Force_MigratesToV6: --force over a
// legacy v5 Codex config must succeed, write the v6 model ID (gpt-5.6-sol),
// and drop the old GPT-5.5 model.
func TestApply_Codex_LegacyJuggernautConfig_Force_MigratesToV6(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)

	codexDir := filepath.Join(home, ".codex")
	if err := safepath.MkdirAll(codexDir); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	configPath := filepath.Join(codexDir, "config.toml")
	legacy := `model = "openai.gpt-5.5"
model_provider = "bedrock-mantle"

[model_providers.bedrock-mantle]
name = "Amazon Bedrock (Mantle)"
base_url = "https://bedrock-mantle.us-west-2.api.aws/openai/v1"
wire_api = "responses"
`
	if err := safepath.WriteFile(codexDir, configPath, []byte(legacy)); err != nil {
		t.Fatalf("write legacy codex config: %v", err)
	}

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
		"--region=us-west-2", "--skip-preflight", "--force",
	}); err != nil {
		t.Fatalf("apply --force over legacy codex config should succeed: %v", err)
	}

	content, err := safepath.ReadFile(codexDir, configPath)
	if err != nil {
		t.Fatalf("read config after apply: %v", err)
	}
	written := string(content)
	if !strings.Contains(written, "global.openai.gpt-5.6-sol") {
		t.Errorf("migrated config should have v6 default model global.openai.gpt-5.6-sol, got:\n%s", written)
	}
	if strings.Contains(written, "openai.gpt-5.5") {
		t.Errorf("migrated config must NOT contain old v5 model openai.gpt-5.5, got:\n%s", written)
	}
	assertCodexRuntimeMigration(t, configPath, "us-west-2")

	matches, _ := filepath.Glob(configPath + ".backup.*")
	if len(matches) == 0 {
		t.Error("expected a backup of the legacy codex config to be created with --force")
	}
}

const legacyCodexMantleProvider = `model = "openai.gpt-5.5"
model_provider = "bedrock-mantle"

[model_providers.bedrock-mantle]
name = "Amazon Bedrock (Mantle)"
base_url = "https://bedrock-mantle.us-west-2.api.aws/openai/v1"
wire_api = "responses"
`

const legacyCodexHybridMantle = `model = "global.openai.gpt-5.6-sol"
model_provider = "amazon-bedrock"

[model_providers.amazon-bedrock.aws]
region = "us-west-2"

[model_providers.bedrock-mantle]
base_url = "https://bedrock-mantle.us-west-2.api.aws/openai/v1"
`

const legacyOpenCodeMantle = `{
  "model": "bedrock-mantle/openai.gpt-oss-120b-1:0",
  "provider": {
    "bedrock-mantle": {
      "options": {"region": "us-west-2"}
    }
  }
}
`

// assertCodexRuntimeMigration structurally verifies a migrated Codex config:
// the top-level model_provider is the built-in runtime id, the legacy v5
// [model_providers.amazon-bedrock] table is GONE, and the runtime table holds
// the region. A plain substring check is a trap here — "amazon-bedrock" is a
// prefix of "amazon-bedrock-runtime" and would pass either way.
func assertCodexRuntimeMigration(t *testing.T, cfgPath, wantRegion string) {
	t.Helper()
	tomlFmt, _ := config.FormatByName("toml")
	mgr := config.NewManagerWithFormat(cfgPath, tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse migrated config.toml: %v", err)
	}
	if mp := got["model_provider"]; mp != provider.CodexBedrockRuntimeProviderID {
		t.Errorf("model_provider = %v, want %s", mp, provider.CodexBedrockRuntimeProviderID)
	}
	tbl, _ := got["model_providers"].(map[string]any)
	if _, ok := tbl[provider.CodexLegacyProviderID]; ok {
		t.Errorf("legacy [model_providers.%s] table must be stripped on migrate, got: %v", provider.CodexLegacyProviderID, tbl[provider.CodexLegacyProviderID])
	}
	rt, ok := tbl[provider.CodexBedrockRuntimeProviderID].(map[string]any)
	if !ok {
		t.Fatalf("[model_providers.%s] table missing after migrate: %v", provider.CodexBedrockRuntimeProviderID, got["model_providers"])
	}
	aws, ok := rt["aws"].(map[string]any)
	if !ok || aws["region"] != wantRegion {
		t.Errorf("migrated config must set model_providers.%s.aws.region = %q, got: %v", provider.CodexBedrockRuntimeProviderID, wantRegion, rt["aws"])
	}
}

func writeHomeFile(t *testing.T, home, relDir, filename, contents string) string {
	t.Helper()
	dir := filepath.Join(home, filepath.FromSlash(relDir))
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, filename)
	if err := safepath.WriteFile(dir, path, []byte(contents)); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func applyCLI(t *testing.T, cli string, extra ...string) string {
	t.Helper()
	args := append([]string{
		"apply", "--cli=" + cli, "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
		"--region=us-west-2", "--skip-preflight",
	}, extra...)
	var applyErr error
	out := captureStdout(t, func() {
		applyErr = ExecuteArgs(args)
	})
	if applyErr != nil {
		t.Fatalf("apply --cli=%s: %v", cli, applyErr)
	}
	return out
}

func readHomeFile(t *testing.T, dir, path string) string {
	t.Helper()
	content, err := safepath.ReadFile(dir, path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func assertMigrated(t *testing.T, out, written string, wantContains, wantAbsent []string) {
	t.Helper()
	if !strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("apply should announce the v5→v6 migration, got:\n%s", out)
	}
	for _, s := range wantContains {
		if !strings.Contains(written, s) {
			t.Errorf("migrated config should contain %q, got:\n%s", s, written)
		}
	}
	for _, s := range wantAbsent {
		if strings.Contains(written, s) {
			t.Errorf("migrated config must NOT contain %q, got:\n%s", s, written)
		}
	}
}

// TestApply_Codex_MantleProvider_PlainApplyMigrates: a real v5 Codex config
// (model_provider = "bedrock-mantle") must migrate on plain apply — OwnsConfig
// is false for that id, so this used to refuse without --force (#435).
func TestApply_Codex_MantleProvider_PlainApplyMigrates(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	path := writeHomeFile(t, home, ".codex", "config.toml", legacyCodexMantleProvider)

	out := applyCLI(t, "codex")
	written := readHomeFile(t, filepath.Dir(path), path)
	assertMigrated(t, out, written,
		[]string{"global.openai.gpt-5.6-sol", "amazon-bedrock"},
		[]string{"bedrock-mantle", "openai.gpt-5.5"},
	)
	if matches, _ := filepath.Glob(path + ".backup.*"); len(matches) == 0 {
		t.Error("expected a backup of the leftover Mantle Codex config")
	}
}

// TestApply_Codex_HybridMantleLeftover_PlainApplyMigrates: v6 model pin still
// paired with a leftover [model_providers.bedrock-mantle] table must migrate
// and drop the Mantle block (deep-merge would otherwise keep the sibling).
func TestApply_Codex_HybridMantleLeftover_PlainApplyMigrates(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	path := writeHomeFile(t, home, ".codex", "config.toml", legacyCodexHybridMantle)

	out := applyCLI(t, "codex")
	written := readHomeFile(t, filepath.Dir(path), path)
	assertMigrated(t, out, written,
		[]string{"global.openai.gpt-5.6-sol", "amazon-bedrock"},
		[]string{"bedrock-mantle"},
	)
	// The hybrid fixture is the definitive legacy-table case: v6 model ID
	// still paired with the v5 custom amazon-bedrock table. After the migrate
	// the legacy table must be stripped (substring checks can't tell
	// "amazon-bedrock" apart from "amazon-bedrock-runtime").
	assertCodexRuntimeMigration(t, path, "us-west-2")
}

// TestApply_OpenCode_MantleProvider_PlainApplyMigrates: leftover
// provider.bedrock-mantle must migrate to amazon-bedrock on plain apply.
func TestApply_OpenCode_MantleProvider_PlainApplyMigrates(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	path := writeHomeFile(t, home, ".config/opencode", "opencode.json", legacyOpenCodeMantle)

	out := applyCLI(t, "opencode")
	written := readHomeFile(t, filepath.Dir(path), path)
	assertMigrated(t, out, written,
		[]string{"amazon-bedrock"},
		[]string{"bedrock-mantle"},
	)
}

// legacyOpenCodeSidecarEra is an opencode.json written by v6.2.0–v6.3.0: the
// auth-mode block is inside the vendor file (OpenCode's schema rejects it) and
// the pre-RC2 nil whitelist marshaled to null.
const legacyOpenCodeSidecarEra = `{
  "model": "amazon-bedrock/openai.gpt-oss-120b-1:0",
  "juggernaut": {
    "auth": {"mode": "iam", "region": "us-west-2"},
    "meta": {"managedBy": "juggernaut", "schemaVersion": 2}
  },
  "provider": {
    "amazon-bedrock": {
      "options": {"region": "us-west-2"},
      "models": {},
      "whitelist": null
    }
  }
}
`

// TestApply_OpenCode_LegacyAuthBlock_PlainApplyStrips: a plain apply over a
// v6.2.0–v6.3.0 opencode.json must strip the in-file juggernaut block (OpenCode
// schema: additionalProperties=false) and the stale null whitelist, and persist
// the auth mode to the sidecar file instead (RC1+RC2).
func TestApply_OpenCode_LegacyAuthBlock_PlainApplyStrips(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	dir := filepath.Join(home, ".config", "opencode")
	cfgPath := writeHomeFile(t, home, ".config/opencode", "opencode.json", legacyOpenCodeSidecarEra)

	out := applyCLI(t, "opencode")
	if !strings.Contains(out, "Migrating OpenCode") {
		t.Errorf("plain apply should announce the legacy sidecar migration, got:\n%s", out)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(readHomeFile(t, dir, cfgPath)), &cfg); err != nil {
		t.Fatalf("opencode.json is not valid JSON: %v", err)
	}
	if _, ok := cfg["juggernaut"]; ok {
		t.Errorf("legacy in-file juggernaut block must be stripped by plain apply, got:\n%s", readHomeFile(t, dir, cfgPath))
	}
	prov, _ := cfg["provider"].(map[string]any)
	ab, _ := prov["amazon-bedrock"].(map[string]any)
	if wl, present := ab["whitelist"]; present {
		if arr, ok := wl.([]any); !ok || len(arr) == 0 {
			t.Errorf("whitelist must be a non-empty array or absent after migration, got %#v", wl)
		}
	}
	if region, _ := ab["options"].(map[string]any)["region"].(string); region != "us-west-2" {
		t.Errorf("region = %v, want us-west-2", region)
	}
	// Auth mode moved to the user-scope sidecar, readable by launch.
	block := provider.ReadSidecarFile(filepath.Join(dir, ".juggernaut.json"))
	if block == nil {
		t.Fatalf("sidecar .juggernaut.json missing or unparseable after migration")
	}
	mode, _ := block["auth"].(map[string]any)["mode"].(string)
	if mode != authmode.BedrockAPIKey {
		t.Errorf("sidecar auth.mode = %q, want %s (the --auth flag apply used)", mode, authmode.BedrockAPIKey)
	}
}

// TestApply_Codex_MantleProvider_DryRunPreviewsMigration: dry-run over a
// leftover Mantle Codex config must announce the rewrite without writing.
func TestApply_Codex_MantleProvider_DryRunPreviewsMigration(t *testing.T) {
	home := setupApplyTest(t)
	path := writeHomeFile(t, home, ".codex", "config.toml", legacyCodexMantleProvider)

	out := applyCLI(t, "codex", "--dry-run")
	if !strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("dry-run should preview the v5→v6 migration, got:\n%s", out)
	}
	if !strings.Contains(out, "Would write juggernaut config") {
		t.Errorf("dry-run should still show what would be written, got:\n%s", out)
	}
	got := readHomeFile(t, filepath.Dir(path), path)
	if got != legacyCodexMantleProvider {
		t.Error("dry-run must not modify the leftover Mantle file")
	}
	if matches, _ := filepath.Glob(path + ".backup.*"); len(matches) != 0 {
		t.Error("dry-run must not create a backup")
	}
}

// TestApply_Codex_AlreadyMigrated_ReapplyDoesNotReannounce: a config that was
// already migrated to the built-in amazon-bedrock-runtime provider is NOT
// legacy — re-applying over it must be a plain re-apply, with no v5→v6
// migration announcement.
func TestApply_Codex_AlreadyMigrated_ReapplyDoesNotReannounce(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	path := writeHomeFile(t, home, ".codex", "config.toml",
		`model = "global.openai.gpt-5.6-sol"
model_provider = "amazon-bedrock-runtime"

[model_providers.amazon-bedrock-runtime.aws]
region = "us-west-2"
`)

	out := applyCLI(t, "codex")
	if strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("re-apply over an already-migrated config must NOT re-announce the v5→v6 migration, got:\n%s", out)
	}
	assertCodexRuntimeMigration(t, path, "us-west-2")
	// The backup (taken before the write) captured the already-migrated
	// runtime config — proof the re-apply saw a current config, not a legacy
	// one.
	matches, _ := filepath.Glob(path + ".backup.*")
	if len(matches) == 0 {
		t.Fatal("expected a pre-write backup of the config")
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(data), "model_provider = \"amazon-bedrock-runtime\"") {
		t.Errorf("backup should preserve the already-migrated config, got:\n%s", data)
	}
}

// TestApply_Grok_AlreadyMigrated_ReapplyDoesNotReannounce: same contract for
// Grok — a config already on the v6 bedrock-runtime URL is not legacy.
func TestApply_Grok_AlreadyMigrated_ReapplyDoesNotReannounce(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)
	path := writeHomeFile(t, home, ".grok", "config.toml",
		`[models]
default = "bedrock-grok"

[model.bedrock-grok]
model = "global.xai.grok-4.6"
base_url = "https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1"
name = "Amazon Bedrock (Runtime)"
`)

	out := applyCLI(t, "grok")
	if strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("re-apply over an already-migrated config must NOT re-announce the v5→v6 migration, got:\n%s", out)
	}
	got := readHomeFile(t, filepath.Dir(path), path)
	if !strings.Contains(got, "bedrock-runtime") {
		t.Errorf("re-applied grok config must still route via bedrock-runtime, got:\n%s", got)
	}
}
