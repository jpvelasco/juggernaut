package config

import (
	"path/filepath"
	"testing"
)

// TestMergeConfigPlan_SetAndDelete verifies the generic plan merge preserves the
// set-or-delete-by-zero-value semantics of the legacy MergeJuggernautBlock.
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
