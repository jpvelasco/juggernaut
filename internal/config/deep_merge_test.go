package config

import (
	"path/filepath"
	"testing"
)

// TestMergeConfigPlan_DeepMergeKeys_PreservesSiblings is the data-loss
// regression: writing a nested-table key (e.g. Grok's "model") must merge only
// Juggernaut's own sub-key, preserving the user's other entries — NOT replace
// the whole table. Recreates the real scenario that would have wiped a user's
// batwing-coder / batmobile model profiles.
func TestMergeConfigPlan_DeepMergeKeys_PreservesSiblings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})

	// User's pre-existing config with their own model profiles + default.
	if err := m.Write(map[string]any{
		"model": map[string]any{
			"batwing-coder":    map[string]any{"base_url": "http://192.168.0.131:8082/v1", "model": "qwen36"},
			"batmobile-gemma4": map[string]any{"base_url": "http://192.168.0.112:1234/v1"},
		},
		"models": map[string]any{"default": "batwing-coder"},
	}); err != nil {
		t.Fatal(err)
	}

	// Juggernaut writes its bedrock-grok block. Deep-merge keys: model, models.
	err := m.MergeConfigPlanDeep(map[string]any{
		"model": map[string]any{
			"bedrock-grok": map[string]any{"model": "xai.grok-4.3", "base_url": "https://mantle/openai/v1"},
		},
		"models": map[string]any{"default": "bedrock-grok"},
	}, []string{"model", "models"})
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}

	got, _ := m.Read()
	modelTbl, _ := got["model"].(map[string]any)
	if modelTbl == nil {
		t.Fatal("model table lost")
	}
	// Both the user's profiles AND ours must be present.
	for _, want := range []string{"batwing-coder", "batmobile-gemma4", "bedrock-grok"} {
		if _, ok := modelTbl[want]; !ok {
			t.Errorf("model.%s missing after merge — data loss!", want)
		}
	}
	// models.default: Juggernaut set it to bedrock-grok (a leaf override).
	modelsTbl, _ := got["models"].(map[string]any)
	if modelsTbl["default"] != "bedrock-grok" {
		t.Errorf("models.default = %v, want bedrock-grok", modelsTbl["default"])
	}
}

// TestRemoveManagedKeysDeep_PreservesSiblings is the uninstall-side data-loss
// regression: removing our bedrock-grok block must NOT delete the user's other
// model profiles or the whole table.
func TestRemoveManagedKeysDeep_PreservesSiblings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	if err := m.Write(map[string]any{
		"model": map[string]any{
			"batwing-coder": map[string]any{"base_url": "http://x/v1"},
			"bedrock-grok":  map[string]any{"model": "xai.grok-4.3"},
		},
		"models": map[string]any{"default": "bedrock-grok", "web_search": "batwing-coder"},
	}); err != nil {
		t.Fatal(err)
	}
	err := m.RemoveManagedKeysDeep([]string{"model", "models"}, map[string][]string{
		"model":  {"bedrock-grok"},
		"models": {"default"},
	})
	if err != nil {
		t.Fatalf("RemoveManagedKeysDeep: %v", err)
	}
	got, _ := m.Read()
	modelTbl, _ := got["model"].(map[string]any)
	if modelTbl == nil {
		t.Fatal("model table wrongly deleted — data loss!")
	}
	if _, ok := modelTbl["batwing-coder"]; !ok {
		t.Error("user's batwing-coder profile lost on uninstall — data loss!")
	}
	if _, ok := modelTbl["bedrock-grok"]; ok {
		t.Error("our bedrock-grok should be removed")
	}
	modelsTbl, _ := got["models"].(map[string]any)
	if modelsTbl == nil {
		t.Fatal("models table wrongly deleted")
	}
	if _, ok := modelsTbl["default"]; ok {
		t.Error("our models.default should be removed")
	}
	if modelsTbl["web_search"] != "batwing-coder" {
		t.Error("user's models.web_search setting lost — data loss!")
	}
}

// TestRemoveManagedKeysDeep_EmptyTableCleaned: if removing our sub-key leaves the
// nested table empty, the table itself is removed (no orphan empty table).
func TestRemoveManagedKeysDeep_EmptyTableCleaned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManagerWithFormat(path, tomlFormat{})
	_ = m.Write(map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "x"}},
	})
	_ = m.RemoveManagedKeysDeep([]string{"model"}, map[string][]string{"model": {"bedrock-grok"}})
	got, _ := m.Read()
	if _, ok := got["model"]; ok {
		t.Error("empty model table should be removed after our only sub-key is gone")
	}
}

// TestMergeConfigPlanDeep_NonDeepKeysStillReplace verifies keys NOT in the
// deep-merge set keep whole-value replace semantics (back-compat for Claude's
// env / modelOverrides etc.).
func TestMergeConfigPlanDeep_NonDeepKeysStillReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(path)
	if err := m.Write(map[string]any{
		"env": map[string]any{"OLD_KEY": "stale"},
	}); err != nil {
		t.Fatal(err)
	}
	// env is NOT a deep-merge key → replaced wholesale.
	err := m.MergeConfigPlanDeep(map[string]any{
		"env": map[string]any{"AWS_REGION": "us-west-2"},
	}, nil) // no deep keys
	if err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}
	got, _ := m.Read()
	env, _ := got["env"].(map[string]any)
	if _, stale := env["OLD_KEY"]; stale {
		t.Error("non-deep key should be replaced wholesale, stale OLD_KEY survived")
	}
	if env["AWS_REGION"] != "us-west-2" {
		t.Errorf("env not replaced correctly: %v", env)
	}
}
