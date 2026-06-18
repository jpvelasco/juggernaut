package config_test

import (
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/config"
)

func TestReadMissing(t *testing.T) {
	m := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	data, err := m.Read()
	if err != nil {
		t.Fatalf("Read() on missing file error: %v", err)
	}
	if len(data) != 0 {
		t.Error("expected empty map for missing file")
	}
}

func TestWriteAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	initial := map[string]any{"someKey": "someValue"}
	if err := m.Write(initial); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got["someKey"] != "someValue" {
		t.Errorf("expected someKey=someValue, got %v", got["someKey"])
	}
}

func TestMergeJuggernautBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	existing := map[string]any{"userPref": "keep-me"}
	_ = m.Write(existing)

	block := map[string]any{"managedBy": "juggernaut"}
	nativeEnv := map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}
	nativeKeys := map[string]any{"effortLevel": "xhigh", "skipWebFetchPreflight": true}

	if err := m.MergeJuggernautBlock(block, nativeEnv, nativeKeys); err != nil {
		t.Fatalf("MergeJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	if got["userPref"] != "keep-me" {
		t.Error("user pref should be preserved after merge")
	}
	if _, ok := got["juggernaut"]; !ok {
		t.Error("juggernaut block should be present")
	}
	if got["effortLevel"] != "xhigh" {
		t.Errorf("expected effortLevel=xhigh, got %v", got["effortLevel"])
	}
	if got["skipWebFetchPreflight"] != true {
		t.Error("expected skipWebFetchPreflight=true")
	}
}

func TestMergeJuggernautBlock_NativeKeys_BoolFalseDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	_ = m.Write(map[string]any{"alwaysThinkingEnabled": true})

	if err := m.MergeJuggernautBlock(
		map[string]any{},
		nil,
		map[string]any{"alwaysThinkingEnabled": false},
	); err != nil {
		t.Fatalf("MergeJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	if _, ok := got["alwaysThinkingEnabled"]; ok {
		t.Error("alwaysThinkingEnabled=false should remove the key")
	}
}

func TestMergeJuggernautBlock_Permissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	nativeKeys := map[string]any{
		"permissions": map[string]any{"defaultMode": "auto"},
	}
	if err := m.MergeJuggernautBlock(map[string]any{}, nil, nativeKeys); err != nil {
		t.Fatalf("MergeJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key should be present and a map")
	}
	if perms["defaultMode"] != "auto" {
		t.Errorf("expected defaultMode=auto, got %v", perms["defaultMode"])
	}
}

func TestMergeJuggernautBlock_NativeKeys_NilPermissionsRemovesDefaultMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	// Only defaultMode — whole permissions key should be gone after nil.
	_ = m.Write(map[string]any{"permissions": map[string]any{"defaultMode": "auto"}})

	if err := m.MergeJuggernautBlock(map[string]any{}, nil, map[string]any{"permissions": nil}); err != nil {
		t.Fatalf("MergeJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	if _, ok := got["permissions"]; ok {
		t.Error("permissions key should be deleted when nil is passed and no user rules exist")
	}
}

func TestMergeJuggernautBlock_NativeKeys_UnknownTypeErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	err := m.MergeJuggernautBlock(map[string]any{}, nil, map[string]any{"effortLevel": 42})
	if err == nil {
		t.Error("expected error for unsupported native key type int")
	}
}

func TestRemoveJuggernautBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	data := map[string]any{
		"userPref":              "keep-me",
		"juggernaut":            map[string]any{"managedBy": "juggernaut"},
		"env":                   map[string]any{"CLAUDE_CODE_USE_BEDROCK": "1"},
		"model":                 "opusplan",
		"modelOverrides":        map[string]any{},
		"effortLevel":           "xhigh",
		"alwaysThinkingEnabled": true,
		"skipWebFetchPreflight": true,
		"permissions":           map[string]any{"defaultMode": "auto"},
	}
	_ = m.Write(data)

	if err := m.RemoveJuggernautBlock(); err != nil {
		t.Fatalf("RemoveJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	// permissions had only defaultMode so the whole key should be gone.
	for _, k := range []string{"juggernaut", "env", "model", "modelOverrides", "effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight", "permissions"} {
		if _, ok := got[k]; ok {
			t.Errorf("key %q should be removed after uninstall", k)
		}
	}
	if got["userPref"] != "keep-me" {
		t.Error("user pref should be preserved after remove")
	}
}

func TestMergePermissions_PreservesUserRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	// Pre-existing user permissions with allow/deny rules.
	_ = m.Write(map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(git *)"},
			"deny":  []any{"Bash(rm *)"},
		},
	})

	// Apply sets defaultMode=auto — should NOT wipe allow/deny.
	nativeKeys := map[string]any{
		"permissions": map[string]any{"defaultMode": "auto"},
	}
	if err := m.MergeJuggernautBlock(map[string]any{}, nil, nativeKeys); err != nil {
		t.Fatalf("MergeJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key should be present")
	}
	if perms["defaultMode"] != "auto" {
		t.Errorf("expected defaultMode=auto, got %v", perms["defaultMode"])
	}
	if perms["allow"] == nil {
		t.Error("user allow rules should be preserved")
	}
	if perms["deny"] == nil {
		t.Error("user deny rules should be preserved")
	}
}

func TestRemoveJuggernautBlock_PreservesUserPermissionRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	_ = m.Write(map[string]any{
		"juggernaut": map[string]any{},
		"permissions": map[string]any{
			"defaultMode": "auto",
			"allow":       []any{"Bash(git *)"},
		},
	})

	if err := m.RemoveJuggernautBlock(); err != nil {
		t.Fatalf("RemoveJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key should remain when user rules exist")
	}
	if _, hasDM := perms["defaultMode"]; hasDM {
		t.Error("defaultMode should be removed on uninstall")
	}
	if perms["allow"] == nil {
		t.Error("user allow rules should survive uninstall")
	}
}

func TestHasJuggernautBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	has, _ := m.HasJuggernautBlock()
	if has {
		t.Error("should not have block on empty file")
	}

	_ = m.Write(map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"managedBy": "juggernaut"},
		},
	})

	has, err := m.HasJuggernautBlock()
	if err != nil {
		t.Fatalf("HasJuggernautBlock() error: %v", err)
	}
	if !has {
		t.Error("should have block after writing")
	}
}

func TestBackupRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := config.NewManager(path)

	data := map[string]any{"x": 1}
	for i := range 7 {
		data["x"] = i
		_ = m.Write(data)
	}

	pattern := filepath.Join(dir, "settings.json.backup.*")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 5 {
		t.Errorf("expected ≤5 backups, got %d", len(matches))
	}
}
