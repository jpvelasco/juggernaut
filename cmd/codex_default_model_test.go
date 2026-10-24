package cmd

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/provider"
)

// The root "model" in config.toml is Codex's own native default. These tests
// lock in the same contract established for OpenCode in #519, adapted to
// Codex's provider-bound model: a plain apply preserves a default Codex can
// serve through the pinned provider (a codexModel-resolvable ID in the
// endpoint's shape), replaces unservable or stale values with the working
// default, and uninstall removes only the default apply actually wrote
// (recorded in the auth sidecar).
//
// Applies use --auth=iam so no test touches the keychain (the macOS CI
// keychain backend blocks headless writes).

// applyCodexIAM runs a keychain-free Codex apply and returns its output.
func applyCodexIAM(t *testing.T, extra ...string) string {
	t.Helper()
	args := append([]string{
		"apply", "--cli=codex", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}, extra...)
	var applyErr error
	out := captureStdout(t, func() {
		applyErr = ExecuteArgs(args)
	})
	if applyErr != nil {
		t.Fatalf("apply --cli=codex: %v", applyErr)
	}
	return out
}

// readCodexConfig parses the fake home's config.toml.
func readCodexConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	mgr, err := newProviderManager(provider.MustGet("codex"), home, "user")
	if err != nil {
		t.Fatalf("new provider manager: %v", err)
	}
	data, err := mgr.Read()
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	return data
}

// rewriteCodexModel re-points the root model in config.toml, simulating a
// user edit.
func rewriteCodexModel(t *testing.T, home, model string) {
	t.Helper()
	mgr, err := newProviderManager(provider.MustGet("codex"), home, "user")
	if err != nil {
		t.Fatalf("new provider manager: %v", err)
	}
	data, err := mgr.Read()
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	data["model"] = model
	if err := mgr.Write(data); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
}

func uninstallCodex(t *testing.T) {
	t.Helper()
	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=codex", "--force"}); err != nil {
			t.Fatalf("uninstall --cli=codex: %v", err)
		}
	})
	_ = out // failure assertions happen on the config, not the transcript
}

// TestApply_Codex_ReapplyKeepsPinnedModel: a plain re-apply keeps a
// previously pinned default instead of resetting it to sol.
func TestApply_Codex_ReapplyKeepsPinnedModel(t *testing.T) {
	home := setupApplyTest(t)

	applyCodexIAM(t, "--model=luna")
	plain := applyCodexIAM(t)

	if !strings.Contains(plain, "preserved") {
		t.Errorf("re-apply should report the preserved default, got:\n%s", plain)
	}
	cfg := readCodexConfig(t, home)
	if model, _ := cfg["model"].(string); model != "global.openai.gpt-6-luna" {
		t.Errorf("plain re-apply must keep the pinned default, got: %v", cfg["model"])
	}
}

// TestApply_Codex_ReapplyKeepsUserEditedModel: a user edit between applies
// survives a plain re-apply — the reset-to-sol-on-apply regression this
// behavior replaces.
func TestApply_Codex_ReapplyKeepsUserEditedModel(t *testing.T) {
	home := setupApplyTest(t)

	applyCodexIAM(t, "--model=sol")
	rewriteCodexModel(t, home, "global.openai.gpt-6-astra")

	plain := applyCodexIAM(t)
	if !strings.Contains(plain, "preserved") {
		t.Errorf("re-apply should report the preserved default, got:\n%s", plain)
	}
	cfg := readCodexConfig(t, home)
	if model, _ := cfg["model"].(string); model != "global.openai.gpt-6-astra" {
		t.Errorf("plain re-apply must keep the user's edited default, got: %v", cfg["model"])
	}
}

// TestApply_Codex_ForeignUnservableModel_Refuses: a foreign config whose
// default Codex cannot serve through the Bedrock runtime is NOT silently
// overwritten — the plan must write the working default, which collides with
// the user's value, so apply refuses without --force (the pre-existing
// collision contract; preservation only applies to servable models).
func TestApply_Codex_ForeignUnservableModel_Refuses(t *testing.T) {
	home := setupApplyTest(t)
	path := writeHomeFile(t, home, ".codex", "config.toml", `model = "gpt-5.2-codex"`+"\n")

	err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	})
	if err == nil {
		t.Fatal("expected apply to refuse a foreign codex config with a colliding model key")
	}
	if !strings.Contains(err.Error(), "model") || !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should name the colliding path and --force, got: %v", err)
	}

	// The foreign file must be untouched by the refusal.
	cfg := readCodexConfig(t, home)
	if model, _ := cfg["model"].(string); model != "gpt-5.2-codex" {
		t.Errorf("foreign config must be untouched on refusal, got: %v (path %s)", cfg["model"], path)
	}
}

// TestUninstall_Codex_RemovesJuggernautWrittenModel: a default Juggernaut
// wrote (recorded in the sidecar) is removed with the rest of the managed
// config.
func TestUninstall_Codex_RemovesJuggernautWrittenModel(t *testing.T) {
	home := setupApplyTest(t)

	applyCodexIAM(t)
	uninstallCodex(t)

	cfg := readCodexConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("uninstall must remove the Juggernaut-written default, got: %v", cfg["model"])
	}
}

// TestUninstall_Codex_PreservesUserEditedModel: apply wrote and recorded a
// default; the user re-pointed it; the stale record no longer matches, so
// uninstall keeps the user's edit.
func TestUninstall_Codex_PreservesUserEditedModel(t *testing.T) {
	home := setupApplyTest(t)

	applyCodexIAM(t, "--model=sol")
	rewriteCodexModel(t, home, "global.openai.gpt-6-astra")
	uninstallCodex(t)

	cfg := readCodexConfig(t, home)
	if model, _ := cfg["model"].(string); model != "global.openai.gpt-6-astra" {
		t.Errorf("user's post-apply edit must survive uninstall, got: %v", cfg["model"])
	}
}

// TestUninstall_Codex_RemovesPreRecordDefault: a v6.5.x-era install (records
// shipped with the preserve behavior) carries the seeded default with no
// writtenModel record in its sidecar. Uninstall must remove it — leaving it
// behind orphans the model now that its provider entry is gone (#523).
func TestUninstall_Codex_RemovesPreRecordDefault(t *testing.T) {
	home := setupApplyTest(t)
	writeHomeFile(t, home, ".codex", "config.toml", `model = "global.openai.gpt-6.1-sol"
model_provider = "amazon-bedrock-runtime"

[model_providers.amazon-bedrock-runtime.aws]
region = "us-west-2"
`)
	writeHomeFile(t, home, ".codex", ".juggernaut.json", `{"juggernaut": {"auth": {"mode": "iam", "region": "us-west-2"}, "meta": {"managedBy": "juggernaut", "scope": "user", "schemaVersion": 2}}}`)
	uninstallCodex(t)

	cfg := readCodexConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("pre-record seeded default must be removed on uninstall, got: %v", cfg["model"])
	}
}

// TestUninstall_Codex_PreservesForeignModelWithoutRecord: a foreign default
// (the user's own OpenAI model) under a pre-record install survives
// uninstall — only the ID space Juggernaut writes is cleaned up.
func TestUninstall_Codex_PreservesForeignModelWithoutRecord(t *testing.T) {
	home := setupApplyTest(t)
	writeHomeFile(t, home, ".codex", "config.toml", `model = "gpt-5.2-codex"
model_provider = "amazon-bedrock-runtime"

[model_providers.amazon-bedrock-runtime.aws]
region = "us-west-2"
`)
	writeHomeFile(t, home, ".codex", ".juggernaut.json", `{"juggernaut": {"auth": {"mode": "iam", "region": "us-west-2"}, "meta": {"managedBy": "juggernaut", "scope": "user", "schemaVersion": 2}}}`)
	uninstallCodex(t)

	cfg := readCodexConfig(t, home)
	if model, _ := cfg["model"].(string); model != "gpt-5.2-codex" {
		t.Errorf("user's foreign model must survive uninstall, got: %v", cfg["model"])
	}
}
