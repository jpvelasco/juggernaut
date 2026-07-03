package config

import (
	"path/filepath"
	"testing"
)

// TestRemoveManagedKeys removes a provider's managed keys + the juggernaut block
// while preserving user keys and (for permissions) only stripping defaultMode.
func TestRemoveManagedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(path)
	if err := m.Write(map[string]any{
		"juggernaut":     map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"model":          "openai.gpt-5.5",
		"model_provider": "bedrock-mantle",
		"userKept":       "yes",
	}); err != nil {
		t.Fatal(err)
	}

	if err := m.RemoveManagedKeys([]string{"model", "model_provider", "model_providers"}); err != nil {
		t.Fatalf("RemoveManagedKeys: %v", err)
	}
	got, _ := m.Read()

	if _, ok := got["juggernaut"]; ok {
		t.Error("juggernaut block should be removed")
	}
	if _, ok := got["model"]; ok {
		t.Error("managed key 'model' should be removed")
	}
	if _, ok := got["model_provider"]; ok {
		t.Error("managed key 'model_provider' should be removed")
	}
	if got["userKept"] != "yes" {
		t.Error("user key must be preserved")
	}
}

// TestRemoveManagedKeys_MatchesLegacyForClaude: removing Claude's managed key set
// yields the same result as the legacy RemoveJuggernautBlock.
func TestRemoveManagedKeys_MatchesLegacyForClaude(t *testing.T) {
	seed := map[string]any{
		"juggernaut":            map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"env":                   map[string]any{"AWS_REGION": "us-west-2"},
		"model":                 "opusplan",
		"effortLevel":           "high",
		"skipWebFetchPreflight": true,
		"permissions":           map[string]any{"allow": []any{"Bash"}, "defaultMode": "plan"},
		"userKept":              "yes",
	}

	legacyPath := filepath.Join(t.TempDir(), "a.json")
	lm := NewManager(legacyPath)
	_ = lm.Write(cloneMap(seed))
	_ = lm.RemoveJuggernautBlock()
	legacyGot, _ := lm.Read()

	newPath := filepath.Join(t.TempDir(), "b.json")
	nm := NewManager(newPath)
	_ = nm.Write(cloneMap(seed))
	_ = nm.RemoveManagedKeys(nativeManagedKeys)
	newGot, _ := nm.Read()

	if len(legacyGot) != len(newGot) {
		t.Fatalf("key count differs: legacy=%v new=%v", legacyGot, newGot)
	}
	for k := range legacyGot {
		if _, ok := newGot[k]; !ok {
			t.Errorf("legacy kept %q, new dropped it", k)
		}
	}
}

// TestRemoveManagedKeys_PermissionsInKeyList exercises the "permissions" branch
// of the key loop: only defaultMode is stripped, user rules survive.
func TestRemoveManagedKeys_PermissionsInKeyList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(path)
	_ = m.Write(map[string]any{
		"juggernaut":  map[string]any{},
		"permissions": map[string]any{"allow": []any{"Bash"}, "defaultMode": "plan"},
	})
	if err := m.RemoveManagedKeys([]string{"permissions"}); err != nil {
		t.Fatalf("RemoveManagedKeys: %v", err)
	}
	got, _ := m.Read()
	perms, _ := got["permissions"].(map[string]any)
	if perms == nil {
		t.Fatal("permissions dropped entirely; user allow rule lost")
	}
	if _, ok := perms["defaultMode"]; ok {
		t.Error("defaultMode should be stripped")
	}
	if allow, _ := perms["allow"].([]any); len(allow) != 1 {
		t.Errorf("user allow rule must survive, got %v", perms["allow"])
	}
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
