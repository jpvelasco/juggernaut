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
//   - uninstall removes only models Juggernaut wrote and keeps the user's.

// TestApply_OpenCode_ReapplyKeepsPinnedModel: a plain re-apply over a config
// Juggernaut owns keeps the previously pinned default instead of resetting it
// to the convenience default.
func TestApply_OpenCode_ReapplyKeepsPinnedModel(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)

	applyCLI(t, "opencode", "--model=gpt-oss-20b")
	plain := applyCLI(t, "opencode")

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
	setupIsolatedKeychain(t)
	path := writeHomeFile(t, home, ".config/opencode", "opencode.json", "{}")

	applyCLI(t, "opencode", "--model=gpt-oss-20b")
	// The user re-points the default at their own local model.
	rewriteModel(t, path, "batwing-coder/qwen-coder")

	plain := applyCLI(t, "opencode")
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
	setupIsolatedKeychain(t)

	writeHomeFile(t, home, ".config/opencode", "opencode.json", `{"model": "their-own-provider/their-own-model"}`)
	applyCLI(t, "opencode") // preserved foreign default + managed provider entry

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

// TestUninstall_OpenCode_RemovesJuggernautWrittenModel: a default Juggernaut
// wrote is removed with the rest of the managed config.
func TestUninstall_OpenCode_RemovesJuggernautWrittenModel(t *testing.T) {
	home := setupApplyTest(t)
	setupIsolatedKeychain(t)

	applyCLI(t, "opencode") // writes the convenience default
	uninstallOpenCode(t, home)

	cfg := readOpenCodeConfig(t, home)
	if _, present := cfg["model"]; present {
		t.Errorf("uninstall must remove the Juggernaut-written default, got: %v", cfg["model"])
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
