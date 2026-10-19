package config

import (
	"path/filepath"
	"testing"
)

// TestMergeConfigPlan_SetAndDelete verifies the generic plan merge preserves the
// set-or-delete-by-zero-value semantics of the generic plan merge.
func TestMergeConfigPlan_SetAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(path)

	// Pre-existing user key must survive; managed keys set/deleted by value.
	if err := m.Write(map[string]any{"userKept": "yes", "effortLevel": "stale"}); err != nil {
		t.Fatal(err)
	}
	plan := map[string]any{
		"juggernaut":            map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"env":                   map[string]string{"AWS_REGION": "us-west-2"},
		"model":                 "",         // empty string → delete
		"effortLevel":           "high",     // set
		"alwaysThinkingEnabled": false,      // false bool → delete
		"skipWebFetchPreflight": true,       // set
		"fallbackModel":         []string{}, // empty slice → delete
	}
	if err := m.MergeConfigPlan(plan); err != nil {
		t.Fatalf("MergeConfigPlan: %v", err)
	}
	got, _ := m.Read()

	if got["userKept"] != "yes" {
		t.Error("user key must be preserved")
	}
	if _, ok := got["juggernaut"]; !ok {
		t.Error("juggernaut block must always be set")
	}
	if got["effortLevel"] != "high" {
		t.Errorf("effortLevel = %v, want high", got["effortLevel"])
	}
	if _, ok := got["model"]; ok {
		t.Error("empty model should be deleted")
	}
	if _, ok := got["alwaysThinkingEnabled"]; ok {
		t.Error("false bool should be deleted")
	}
	if _, ok := got["fallbackModel"]; ok {
		t.Error("empty slice should be deleted")
	}
	if got["skipWebFetchPreflight"] != true {
		t.Error("skipWebFetchPreflight should be set true")
	}
}

// TestMergeConfigPlanDeep_FreshTableDropsDeleteSentinels verifies the deep
// merge does not write empty-string delete sentinels into a table created from
// scratch (there is no existing table to merge into), at any nesting depth.
func TestMergeConfigPlanDeep_FreshTableDropsDeleteSentinels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	m := NewManager(path)

	if err := m.MergeConfigPlanDeep(map[string]any{
		"model": map[string]any{
			"bedrock-grok": map[string]any{
				"model":       "global.xai.grok-4.7",
				"api_backend": "", // sentinel: must not be written on a fresh table
				"nested": map[string]any{
					"kept": "yes",
					"dead": "",
				},
			},
		},
	}, []string{"model"}); err != nil {
		t.Fatalf("MergeConfigPlanDeep: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	block := got["model"].(map[string]any)["bedrock-grok"].(map[string]any)
	if _, ok := block["api_backend"]; ok {
		t.Error("empty api_backend sentinel must not be written to a fresh table")
	}
	if block["model"] != "global.xai.grok-4.7" {
		t.Errorf("model = %v, want global.xai.grok-4.7", block["model"])
	}
	nested := block["nested"].(map[string]any)
	if _, ok := nested["dead"]; ok {
		t.Error("nested empty-string sentinel must not be written")
	}
	if nested["kept"] != "yes" {
		t.Errorf("nested kept = %v, want yes", nested["kept"])
	}

	// An existing table still deletes the sentinel key and preserves siblings.
	block["api_backend"] = "responses"
	if err := m.Write(got); err != nil {
		t.Fatalf("Write seeded table: %v", err)
	}
	if err := m.MergeConfigPlanDeep(map[string]any{
		"model": map[string]any{
			"bedrock-grok": map[string]any{
				"api_backend": "",
				"model":       "global.xai.grok-4.7",
			},
		},
	}, []string{"model"}); err != nil {
		t.Fatalf("MergeConfigPlanDeep re-apply: %v", err)
	}
	got, err = m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	block = got["model"].(map[string]any)["bedrock-grok"].(map[string]any)
	if _, ok := block["api_backend"]; ok {
		t.Error("empty api_backend must delete an existing table entry")
	}
	nested = block["nested"].(map[string]any)
	if nested["kept"] != "yes" {
		t.Error("user sibling in existing table must survive")
	}
}

// TestMergeConfigPlan_PermissionsDeepMerge verifies only permissions.defaultMode
// is managed; user allow/deny rules survive.
func TestMergeConfigPlan_PermissionsDeepMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(path)
	if err := m.Write(map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash"}, "defaultMode": "old"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":  map[string]any{},
		"permissions": map[string]any{"defaultMode": "plan"},
	}); err != nil {
		t.Fatalf("MergeConfigPlan: %v", err)
	}
	got, _ := m.Read()
	perms, _ := got["permissions"].(map[string]any)
	if perms == nil {
		t.Fatal("permissions dropped")
	}
	if perms["defaultMode"] != "plan" {
		t.Errorf("defaultMode = %v, want plan", perms["defaultMode"])
	}
	allow, _ := perms["allow"].([]any)
	if len(allow) != 1 || allow[0] != "Bash" {
		t.Errorf("user allow rule must survive, got %v", perms["allow"])
	}
}
