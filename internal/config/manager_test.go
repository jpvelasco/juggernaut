package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
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

func TestReadEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatalf("writing empty file: %v", err)
	}
	data, err := config.NewManager(path).Read()
	if err != nil {
		t.Fatalf("Read() on empty file error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty map for empty file, got %v", data)
	}
}

func TestReadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("writing invalid file: %v", err)
	}
	_, err := config.NewManager(path).Read()
	if err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("parsing")) {
		t.Errorf("unexpected error: %v", err)
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

func TestReadWithUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"someKey":"someValue"}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing BOM-prefixed settings: %v", err)
	}

	got, err := config.NewManager(path).Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got["someKey"] != "someValue" {
		t.Errorf("expected someKey=someValue, got %v", got["someKey"])
	}
}

func TestMergeConfigPlan_RewritesUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"userPref":"keep-me"}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing BOM-prefixed settings: %v", err)
	}

	m := config.NewManager(path)
	if err := m.MergeConfigPlan(map[string]any{"juggernaut": map[string]any{"managedBy": "juggernaut"}}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
	}

	written, err := safepath.ReadFile(filepath.Dir(path), path)
	if err != nil {
		t.Fatalf("reading rewritten settings: %v", err)
	}
	if bytes.HasPrefix(written, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("rewritten settings should not retain UTF-8 BOM")
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read() after merge error: %v", err)
	}
	if got["userPref"] != "keep-me" {
		t.Error("user pref should be preserved after merge")
	}
	if _, ok := got["juggernaut"]; !ok {
		t.Error("juggernaut block should be present")
	}
}

func TestMergeConfigPlan_FullApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	existing := map[string]any{"userPref": "keep-me"}
	_ = m.Write(existing)

	plan := map[string]any{
		"juggernaut":  map[string]any{"managedBy": "juggernaut"},
		"env":         map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"},
		"effortLevel": "xhigh",
		"fallbackModel": []string{
			"global.anthropic.claude-opus-4-8",
			"global.anthropic.claude-sonnet-4-6",
		},
		"skipWebFetchPreflight": true,
	}

	if err := m.MergeConfigPlan(plan); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
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
	fallbacks, ok := got["fallbackModel"].([]any)
	if !ok || len(fallbacks) != 2 {
		t.Fatalf("expected fallbackModel array with two entries, got %#v", got["fallbackModel"])
	}
	if fallbacks[0] != "global.anthropic.claude-opus-4-8" || fallbacks[1] != "global.anthropic.claude-sonnet-4-6" {
		t.Errorf("unexpected fallbackModel chain: %#v", fallbacks)
	}
	if got["skipWebFetchPreflight"] != true {
		t.Error("expected skipWebFetchPreflight=true")
	}
}

func TestMergeConfigPlan_BoolFalseDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	_ = m.Write(map[string]any{"alwaysThinkingEnabled": true})

	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":            map[string]any{},
		"alwaysThinkingEnabled": false,
	}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
	}

	got, _ := m.Read()
	if _, ok := got["alwaysThinkingEnabled"]; ok {
		t.Error("alwaysThinkingEnabled=false should remove the key")
	}
}

func TestMergeConfigPlan_EmptySliceDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	_ = m.Write(map[string]any{"fallbackModel": []any{"global.anthropic.claude-opus-4-8"}})

	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":    map[string]any{},
		"fallbackModel": []string{},
	}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
	}

	got, _ := m.Read()
	if _, ok := got["fallbackModel"]; ok {
		t.Error("empty fallbackModel slice should remove the key")
	}
}

func TestMergeConfigPlan_Permissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":  map[string]any{},
		"permissions": map[string]any{"defaultMode": "auto"},
	}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
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

func TestMergeConfigPlan_NilPermissionsRemovesDefaultMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	// Only defaultMode — whole permissions key should be gone after nil.
	_ = m.Write(map[string]any{"permissions": map[string]any{"defaultMode": "auto"}})

	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":  map[string]any{},
		"permissions": nil,
	}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
	}

	got, _ := m.Read()
	if _, ok := got["permissions"]; ok {
		t.Error("permissions key should be deleted when nil is passed and no user rules exist")
	}
}

func TestMergeConfigPlan_UnknownTypeErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	err := m.MergeConfigPlan(map[string]any{
		"juggernaut":  map[string]any{},
		"effortLevel": 42,
	})
	if err == nil {
		t.Error("expected error for unsupported native key type int")
	}
}

func TestRemoveManagedKeys_FullRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	data := map[string]any{
		"userPref":              "keep-me",
		"juggernaut":            map[string]any{"managedBy": "juggernaut"},
		"env":                   map[string]any{"CLAUDE_CODE_USE_BEDROCK": "1"},
		"model":                 "opusplan",
		"modelOverrides":        map[string]any{},
		"fallbackModel":         []any{"global.anthropic.claude-opus-4-8"},
		"effortLevel":           "xhigh",
		"alwaysThinkingEnabled": true,
		"skipWebFetchPreflight": true,
		"permissions":           map[string]any{"defaultMode": "auto"},
	}
	_ = m.Write(data)

	if err := m.RemoveManagedKeys([]string{
		"env", "model", "modelOverrides", "fallbackModel",
		"effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight",
	}); err != nil {
		t.Fatalf("RemoveManagedKeys() error: %v", err)
	}

	got, _ := m.Read()
	// permissions had only defaultMode so the whole key should be gone.
	for _, k := range []string{"juggernaut", "env", "model", "modelOverrides", "fallbackModel", "effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight", "permissions"} {
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
	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":  map[string]any{},
		"permissions": map[string]any{"defaultMode": "auto"},
	}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
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

func TestRemoveManagedKeys_PreservesUserPermissionRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	_ = m.Write(map[string]any{
		"juggernaut": map[string]any{},
		"permissions": map[string]any{
			"defaultMode": "auto",
			"allow":       []any{"Bash(git *)"},
		},
	})

	if err := m.RemoveManagedKeys([]string{"permissions"}); err != nil {
		t.Fatalf("RemoveManagedKeys() error: %v", err)
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

func TestHasManagedKeys_DetectsBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	has, _ := m.HasManagedKeys([]string{"model"})
	if has {
		t.Error("should not have block on empty file")
	}

	_ = m.Write(map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"managedBy": "juggernaut"},
		},
	})

	has, err := m.HasManagedKeys([]string{"model"})
	if err != nil {
		t.Fatalf("HasManagedKeys() error: %v", err)
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

func TestMergeConfigPlan_PopulatedMapAndAnySlice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	plan := map[string]any{
		"juggernaut":     map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"permissions":    map[string]any{"defaultMode": "auto"},
		"modelOverrides": map[string]any{"sonnet": "model-x"},
		"fallbackModel":  []any{"a", "b"},
	}
	if err := m.MergeConfigPlan(plan); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
	}

	got, _ := m.Read()
	perms, ok := got["permissions"].(map[string]any)
	if !ok || perms["defaultMode"] != "auto" {
		t.Errorf("expected permissions.defaultMode=auto, got %#v", got["permissions"])
	}
	overrides, ok := got["modelOverrides"].(map[string]any)
	if !ok || overrides["sonnet"] != "model-x" {
		t.Errorf("expected modelOverrides populated, got %#v", got["modelOverrides"])
	}
	fb, ok := got["fallbackModel"].([]any)
	if !ok || len(fb) != 2 {
		t.Errorf("expected fallbackModel []any of len 2, got %#v", got["fallbackModel"])
	}
}

func TestMergeConfigPlan_EmptyMapDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)
	_ = m.Write(map[string]any{"modelOverrides": map[string]any{"sonnet": "old"}})

	if err := m.MergeConfigPlan(map[string]any{
		"juggernaut":     map[string]any{},
		"modelOverrides": map[string]any{},
	}); err != nil {
		t.Fatalf("MergeConfigPlan() error: %v", err)
	}
	got, _ := m.Read()
	if _, ok := got["modelOverrides"]; ok {
		t.Error("empty map should delete modelOverrides")
	}
}

func TestWrite_CreatesNestedDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "settings.json")
	m := config.NewManager(path)
	if err := m.Write(map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Write() into nested dir error: %v", err)
	}
	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("expected k=v, got %v", got["k"])
	}
}
