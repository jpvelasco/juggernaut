package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// The root "model" in opencode.json is the CLI's own native default. These
// tests lock in the apply/uninstall contract around it:
//
//   - a plain apply (no --model) preserves whatever default is already there,
//     foreign or Juggernaut-written, instead of pinning the convenience
//     default (the pre-6.6 behavior flipped it on every re-apply);
//   - --model still claims the key;
//   - uninstall removes only the default apply actually wrote (recorded in
//     the auth sidecar) and keeps a user's own — including a native
//     amazon-bedrock default, which shares the runtime prefix with
//     Juggernaut's writes.
//
// Applies use --auth=iam so no test touches the keychain: the token store
// only runs for bedrock-api-key applies, and on macOS CI the real keychain
// backend blocks headless writes (the behavior then runs on all three legs
// instead of skipping).

// applyOpenCodeIAM runs a keychain-free OpenCode apply and returns its output.
func applyOpenCodeIAM(t *testing.T, extra ...string) string {
	t.Helper()
	args := append([]string{
		"apply", "--cli=opencode", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}, extra...)
	var applyErr error
	out := captureStdout(t, func() {
		applyErr = ExecuteArgs(args)
	})
	if applyErr != nil {
		t.Fatalf("apply --cli=opencode: %v", applyErr)
	}
	return out
}

// TestApply_OpenCode_ReapplyKeepsPinnedModel: a plain re-apply over a config
// Juggernaut owns keeps the previously pinned default instead of resetting it
// to the convenience default.
func TestApply_OpenCode_ReapplyKeepsPinnedModel(t *testing.T) {
	home := setupApplyTest(t)

	applyOpenCodeIAM(t, "--model=gpt-oss-20b")
	plain := applyOpenCodeIAM(t)

	if !strings.Contains(plain, "preserved") {
		t.Errorf("re-apply should report the preserved default, got:\n%s", plain)
	}
	cfg := readOpenCodeConfig(t, home)
	if model, _ := cfg["model"].(string); model != "amazon-bedrock/openai.gpt-oss-20b-1:0" {
		t.Errorf("plain re-apply must keep the pinned default, got: %v", cfg["model"])
	}
}

// TestApply_OpenCode_ReapplyKeepsUserEditedModel: when the user changes the
// default under a config Juggernaut owns, a plain re-apply respects their
// edit — the flip-the-default-on-apply regression this behavior replaces.
func TestApply_OpenCode_ReapplyKeepsUserEditedModel(t *testing.T) {
	home := setupApplyTest(t)
	path := writeHomeFile(t, home, ".config/opencode", "opencode.json", "{}")

	applyOpenCodeIAM(t, "--model=gpt-oss-20b")
	// The user re-points the default at their own local model.
	rewriteModel(t, path, "batwing-coder/qwen-coder")

	plain := applyOpenCodeIAM(t)
	if !strings.Contains(plain, "preserved") {
		t.Errorf("re-apply should report the preserved default, got:\n%s", plain)
	}
	cfg := readOpenCodeConfig(t, home)
	if model, _ := cfg["model"].(string); model != "batwing-coder/qwen-coder" {
		t.Errorf("plain re-apply must keep the user's edited default, got: %v", cfg["model"])
	}
}

// TestUninstall_OpenCode_PreservesUserDefaultModel: uninstall removes the
// managed provider entries and the sidecar, but leaves a user-owned root
// model in place.
func TestUninstall_OpenCode_PreservesUserDefaultModel(t *testing.T) {
	home := setupApplyTest(t)

	writeHomeFile(t, home, ".config/opencode", "opencode.json", `{"model": "their-own-provider/their-own-model"}`)
	applyOpenCodeIAM(t) // preserved foreign default + managed provider entry

	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if model, _ := cfg["model"].(string); model != "their-own-provider/their-own-model" {
		t.Errorf("uninstall must keep the user's default model, got: %v", cfg["model"])
	}
	if prov, ok := cfg["provider"].(map[string]any); ok {
		if _, present := prov["amazon-bedrock"]; present {
			t.Errorf("uninstall must remove the managed provider entry, got: %v", prov)
		}
	}
	if provider.SidecarExists(provider.MustGet("opencode"), home, "user") {
		t.Error("uninstall must remove the auth metadata sidecar")
	}
}

// TestUninstall_OpenCode_PreservesNativeBedrockDefault: amazon-bedrock is
// OpenCode's own built-in provider id, so a native Bedrock default shares the
// runtime prefix with Juggernaut's writes without being Juggernaut's. Apply
// preserves it (no written-model record); uninstall must too.
func TestUninstall_OpenCode_PreservesNativeBedrockDefault(t *testing.T) {
	home := setupApplyTest(t)

	writeHomeFile(t, home, ".config/opencode", "opencode.json", `{"model": "amazon-bedrock/anthropic.claude-sonnet-5-5"}`)
	applyOpenCodeIAM(t)

	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if model, _ := cfg["model"].(string); model != "amazon-bedrock/anthropic.claude-sonnet-5-5" {
		t.Errorf("native Bedrock default must survive apply and uninstall, got: %v", cfg["model"])
	}
	if prov, ok := cfg["provider"].(map[string]any); ok {
		if _, present := prov["amazon-bedrock"]; present {
			t.Errorf("uninstall must remove the managed provider entry, got: %v", prov)
		}
	}
}

// TestUninstall_OpenCode_RemovesJuggernautWrittenModel: a default Juggernaut
// wrote (recorded in the sidecar) is removed with the rest of the managed
// config — but only while it still equals the record.
func TestUninstall_OpenCode_RemovesJuggernautWrittenModel(t *testing.T) {
	home := setupApplyTest(t)

	applyOpenCodeIAM(t) // writes the convenience default and records it
	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("uninstall must remove the Juggernaut-written default, got: %v", cfg["model"])
	}
}

// TestUninstall_OpenCode_KeepsEditedDefaultAfterPinnedApply: apply wrote and
// recorded a default; the user then re-pointed it at their own model; the
// stale record no longer matches, so uninstall keeps the user's edit.
func TestUninstall_OpenCode_KeepsEditedDefaultAfterPinnedApply(t *testing.T) {
	home := setupApplyTest(t)
	path := writeHomeFile(t, home, ".config/opencode", "opencode.json", "{}")

	applyOpenCodeIAM(t, "--model=gpt-oss-20b") // written + recorded
	rewriteModel(t, path, "batwing-coder/qwen-coder")

	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if model, _ := cfg["model"].(string); model != "batwing-coder/qwen-coder" {
		t.Errorf("user's post-apply edit must survive uninstall, got: %v", cfg["model"])
	}
}

// TestUninstall_OpenCode_RemovesMantleEraModelWithoutRecord: the mantle-era
// provider ids are Juggernaut-exclusive, so a leftover root model pointing at
// them is owned even with no sidecar record.
func TestUninstall_OpenCode_RemovesMantleEraModelWithoutRecord(t *testing.T) {
	home := setupApplyTest(t)
	path := writeHomeFile(t, home, ".config/opencode", "opencode.json", "{}")

	applyOpenCodeIAM(t)
	rewriteModel(t, path, "bedrock-mantle/openai.gpt-oss-120b-1:0") // simulate the v5 leftover

	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("mantle-era leftover must be removed on uninstall, got: %v", cfg["model"])
	}
}

// TestUninstall_OpenCode_RemovesPreservedJuggernautDefault: a plain
// re-apply preserves a Juggernaut-written default and must carry the
// ownership record forward — otherwise uninstall would leave that default
// dangling after removing its provider entry (apply --model, plain apply,
// uninstall).
func TestUninstall_OpenCode_RemovesPreservedJuggernautDefault(t *testing.T) {
	home := setupApplyTest(t)

	applyOpenCodeIAM(t, "--model=gpt-oss-20b") // written + recorded
	applyOpenCodeIAM(t)                        // preserved; the record must carry forward

	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("uninstall must remove the preserved Juggernaut-written default, got: %v", cfg["model"])
	}
}

// TestUninstall_OpenCode_KeepsUserEditAfterReapply: the record carries
// forward only while the preserved default still matches what a prior apply
// wrote; a user edit between applies breaks the chain and uninstall keeps
// the edit.
func TestUninstall_OpenCode_KeepsUserEditAfterReapply(t *testing.T) {
	home := setupApplyTest(t)
	path := writeHomeFile(t, home, ".config/opencode", "opencode.json", "{}")

	applyOpenCodeIAM(t, "--model=gpt-oss-20b") // written + recorded
	rewriteModel(t, path, "batwing-coder/qwen-coder")
	applyOpenCodeIAM(t) // preserves the edit; the record must NOT carry

	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if model, _ := cfg["model"].(string); model != "batwing-coder/qwen-coder" {
		t.Errorf("user's edit must survive re-apply and uninstall, got: %v", cfg["model"])
	}
}

// TestUninstall_OpenCode_RemovesPreRecordSeedDefault: a v6.5.x-era install
// (records shipped with the preserve behavior) carries the seeded default
// with no writtenModel record. Uninstall must remove it — leaving it behind
// orphans the model now that Juggernaut's provider entry is gone (#523).
func TestUninstall_OpenCode_RemovesPreRecordSeedDefault(t *testing.T) {
	home := setupApplyTest(t)
	writeHomeFile(t, home, ".config/opencode", "opencode.json", `{
	"model": "amazon-bedrock/openai.gpt-oss-120b-1:0",
	"provider": {
		"amazon-bedrock": {
			"options": {"region": "us-west-2"},
			"models": {"openai.gpt-oss-120b-1:0": {"id": "openai.gpt-oss-120b-1:0", "name": "openai.gpt-oss-120b-1:0"}}
		}
	}
}`)
	writeHomeFile(t, home, ".config/opencode", ".juggernaut.json", `{"juggernaut": {"auth": {"mode": "iam", "region": "us-west-2"}, "meta": {"managedBy": "juggernaut", "scope": "user", "schemaVersion": 2}}}`)
	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("pre-record seeded default must be removed on uninstall, got: %v", cfg["model"])
	}
}

func readOpenCodeConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(readHomeFile(t, filepath.Dir(path), path)), &cfg); err != nil {
		t.Fatalf("opencode.json is not valid JSON: %v", err)
	}
	return cfg
}

func rewriteModel(t *testing.T, path, model string) {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(readHomeFile(t, filepath.Dir(path), path)), &cfg); err != nil {
		t.Fatalf("opencode.json is not valid JSON: %v", err)
	}
	cfg["model"] = model
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("re-serialize opencode.json: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(path), path, b); err != nil {
		t.Fatalf("rewrite opencode.json: %v", err)
	}
}

func uninstallOpenCode(t *testing.T, home string) {
	t.Helper()
	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=opencode", "--force"}); err != nil {
			t.Fatalf("uninstall --cli=opencode: %v", err)
		}
	})
	_ = out // failure assertions happen on the config, not the transcript
}
