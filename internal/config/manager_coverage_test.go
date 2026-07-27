package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// =============================================================================
// RemoveManagedKeys
// =============================================================================

// TestRemoveManagedKeys_ConfigWithBlock verifies the primary path: a config
// with a juggernaut block plus managed keys has them stripped, while user
// keys and user-defined permission rules survive.
func TestRemoveManagedKeys_ConfigWithBlock(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Seed with managed keys, a juggernaut block, and user content.
	if err := m.Write(map[string]any{
		"juggernaut":            map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"env":                   map[string]any{"CLAUDE_CODE_USE_BEDROCK": "1"},
		"model":                 "sonnet",
		"effortLevel":           "high",
		"alwaysThinkingEnabled": true,
		"skipWebFetchPreflight": true,
		"permissions": map[string]any{
			"defaultMode": "auto",
			"allow":       []any{"Bash(git *)"},
		},
		"userKey": "keep-me",
	}); err != nil {
		t.Fatalf("Write seed: %v", err)
	}

	if err := m.RemoveManagedKeys([]string{
		"env", "model", "effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight",
	}); err != nil {
		t.Fatalf("RemoveManagedKeys: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	for _, k := range []string{"juggernaut", "env", "model", "effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight"} {
		if _, ok := got[k]; ok {
			t.Errorf("managed key %q should be removed, still present", k)
		}
	}

	// permissions.defaultMode should be stripped but user allow rules survive.
	perms, ok := got["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions should remain when user rules exist")
	}
	if _, has := perms["defaultMode"]; has {
		t.Error("permissions.defaultMode should be stripped")
	}
	if perms["allow"] == nil {
		t.Error("user allow rules should survive")
	}

	// User key preserved.
	if got["userKey"] != "keep-me" {
		t.Errorf("userKey = %v, want keep-me", got["userKey"])
	}
}

// TestRemoveManagedKeys_NoConfigFile verifies the no-op path: removing from
// a config that doesn't exist yet. RemoveManagedKeys calls withConfig which
// calls Read; Read returns an empty map for missing files, so the delete calls
// are no-ops and Write creates an empty file.
func TestRemoveManagedKeys_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// File does not exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should not exist yet: %v", err)
	}

	// Removing from a non-existent config should not error.
	if err := m.RemoveManagedKeys([]string{"model", "env"}); err != nil {
		t.Fatalf("RemoveManagedKeys on missing file: %v", err)
	}

	// The file may have been created (Write creates the file). Read should
	// succeed and return an empty-ish map.
	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read after remove on missing file: %v", err)
	}
	if _, ok := got["juggernaut"]; ok {
		t.Error("juggernaut block should not appear")
	}
}

// TestRemoveManagedKeys_NoManagedKeys verifies the no-op path: a config
// that exists but has no managed keys. All the delete calls are no-ops.
func TestRemoveManagedKeys_NoManagedKeys(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	if err := m.Write(map[string]any{"userKey": "keep-me", "otherKey": 42}); err != nil {
		t.Fatalf("Write seed: %v", err)
	}

	if err := m.RemoveManagedKeys([]string{"model", "env"}); err != nil {
		t.Fatalf("RemoveManagedKeys: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["userKey"] != "keep-me" {
		t.Errorf("userKey = %v, want keep-me", got["userKey"])
	}
	if got["otherKey"] != 42.0 {
		t.Errorf("otherKey = %v, want 42", got["otherKey"])
	}
}

// =============================================================================
// rotateBackup
// =============================================================================

// TestRotateBackup_NoExistingBackups verifies the first-backup path: no prior
// backups exist, so pruneBackups has nothing to remove after the copy.
func TestRotateBackup_NoExistingBackups(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Write the initial file (no backup created on first write).
	if err := m.Write(map[string]any{"v": 1}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Manually invoke rotateBackup to test it directly.
	if err := m.rotateBackup(); err != nil {
		t.Fatalf("rotateBackup: %v", err)
	}

	// Exactly one backup should exist.
	matches, _ := filepath.Glob(path + ".backup.*")
	if len(matches) != 1 {
		t.Errorf("expected 1 backup, got %d", len(matches))
	}

	// The backup should contain the previous file content.
	// Verify the backup exists under our temp dir, then read it.
	if !strings.HasPrefix(matches[0], dir) {
		t.Fatalf("backup path %s not under temp dir %s", matches[0], dir)
	}
	// nosemgrep: go_filesystem_rule-fileread -- test-only; glob result is within t.TempDir()
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("Read backup: %v", err)
	}
	if len(data) == 0 {
		t.Error("backup should not be empty")
	}
}

// TestRotateBackup_PrunesToFive verifies that repeated writes (each triggering
// rotateBackup) prune old backups to the configured limit of 5.
func TestRotateBackup_PrunesToFive(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Write 8 times. Each write after the first triggers rotateBackup.
	// This creates 7 backups, but pruneBackups keeps only 5.
	for i := range 8 {
		if err := m.Write(map[string]any{"v": i}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	matches, _ := filepath.Glob(path + ".backup.*")
	if len(matches) > 5 {
		t.Errorf("expected ≤5 backups, got %d: %v", len(matches), matches)
	}
}

// TestRotateBackup_MultipleExistingBackups verifies the path where backups
// already exist from prior runs. rotateBackup creates one more, then prunes.
func TestRotateBackup_MultipleExistingBackups(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}

	// Pre-create 4 existing backups.
	for i := range 4 {
		ts := filepath.Base(path) + ".backup.2024010" + string(rune('0'+i)) + "_000000"
		_ = os.WriteFile(filepath.Join(dir, ts), []byte("old backup"), 0o600)
	}

	m := NewManager(path)
	if err := m.Write(map[string]any{"v": 1}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Now call rotateBackup directly — it will copy the current file and
	// prune to keep only 5.
	if err := m.rotateBackup(); err != nil {
		t.Fatalf("rotateBackup: %v", err)
	}

	matches, _ := filepath.Glob(path + ".backup.*")
	if len(matches) > 5 {
		t.Errorf("expected ≤5 backups after adding to existing set, got %d", len(matches))
	}
}

// =============================================================================
// withConfig error path (rollback)
// =============================================================================

// TestWithConfig_WriteError_Propagates verifies that when withConfig's mutation
// succeeds but the subsequent Write fails, the error propagates. We simulate
// this by acquiring the lock externally so Write fails with a lock-contention
// error after the mutation has already run in-memory.
func TestWithConfig_WriteError_Propagates(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Seed with valid config.
	if err := m.Write(map[string]any{"model": "sonnet", "userKey": "keep-me"}); err != nil {
		t.Fatalf("Write seed: %v", err)
	}

	// Hold the lock externally so Write inside withConfig fails.
	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		t.Fatalf("failed to acquire test lock: %v", err)
	}

	// MergeConfigPlanDeep calls withConfig which reads, mutates, then tries
	// to Write. The Write will fail because we hold the lock.
	err = m.MergeConfigPlanDeep(map[string]any{"model": "opus"}, nil)
	if err == nil {
		_ = fl.Unlock()
		t.Fatal("expected lock contention error from Write inside withConfig, got nil")
	}

	// Release our lock.
	_ = fl.Unlock()

	// The file should be unchanged — Write failed, so the mutation in-memory
	// never persisted.
	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["model"] != "sonnet" {
		t.Errorf("model should be unchanged after failed Write, got %v", got["model"])
	}
	if got["userKey"] != "keep-me" {
		t.Errorf("userKey should be unchanged, got %v", got["userKey"])
	}
}

// =============================================================================
// Write error paths
// =============================================================================

// TestWrite_LockAcquisitionError verifies the path where flock.TryLock itself
// returns an error (not just contention). This is hard to trigger on most
// platforms, so we test the contention path which is the practical manifestation.
func TestWrite_LockContentionError(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}

	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		t.Fatalf("failed to acquire test lock: %v", err)
	}
	defer func() { _ = fl.Unlock() }()

	m := NewManager(path)
	err = m.Write(map[string]any{"k": "v"})
	if err == nil {
		t.Error("expected lock contention error")
	}
}

// TestWrite_NonexistentParentDir verifies that Write creates the parent
// directory via safepath.MkdirAll before writing.
func TestWrite_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	parent, err := safepath.JoinUnder(dir, "new", "nested", "dir")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	path, err := safepath.JoinUnder(parent, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}

	m := NewManager(path)
	if err := m.Write(map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Write into nonexistent parent: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("k = %v, want v", got["k"])
	}
}

// =============================================================================
// MergeConfigPlan shallow merge
// =============================================================================

// TestMergeConfigPlan_ShallowMerge adds new keys to an empty config without
// using deep-merge semantics. All keys are managed via applyManagedKey.
func TestMergeConfigPlan_ShallowMerge_AddsKeys(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// No file exists yet; MergeConfigPlan with shallow merge.
	plan := map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"env":        map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"},
		"model":      "sonnet",
	}
	if err := m.MergeConfigPlan(plan); err != nil {
		t.Fatalf("MergeConfigPlan: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, ok := got["juggernaut"]; !ok {
		t.Error("juggernaut block should be set")
	}
	if got["model"] != "sonnet" {
		t.Errorf("model = %v, want sonnet", got["model"])
	}
}

// TestMergeConfigPlan_ShallowMerge_ModifiesExistingKeys verifies that
// MergeConfigPlan overwrites existing key values (shallow replace, not deep merge).
func TestMergeConfigPlan_ShallowMerge_ModifiesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Seed with existing values.
	if err := m.Write(map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"model":      "opus",
		"env":        map[string]any{"OLD_KEY": "stale"},
		"userKey":    "keep-me",
	}); err != nil {
		t.Fatalf("Write seed: %v", err)
	}

	// MergeConfigPlan with new values — shallow merge replaces non-deep keys.
	plan := map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"model":      "haiku",
		"env":        map[string]string{"NEW_KEY": "fresh"},
	}
	if err := m.MergeConfigPlan(plan); err != nil {
		t.Fatalf("MergeConfigPlan: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["model"] != "haiku" {
		t.Errorf("model = %v, want haiku", got["model"])
	}
	// env is replaced wholesale (not deep-merged) since it's not in deepKeys.
	env, ok := got["env"].(map[string]any)
	if !ok {
		t.Fatalf("env should be a map, got %T", got["env"])
	}
	if _, has := env["OLD_KEY"]; has {
		t.Error("OLD_KEY should be gone after shallow replace of env")
	}
	if env["NEW_KEY"] != "fresh" {
		t.Errorf("NEW_KEY = %v, want fresh", env["NEW_KEY"])
	}
	// User key preserved.
	if got["userKey"] != "keep-me" {
		t.Errorf("userKey = %v, want keep-me", got["userKey"])
	}
}

// TestMergeConfigPlan_ShallowMerge_DeletesKeys verifies the delete-by-empty
// semantics: an empty string/bool false/empty slice in the plan removes the key.
func TestMergeConfigPlan_ShallowMerge_DeletesKeys(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	m := NewManager(path)

	// Seed with values that will be deleted.
	if err := m.Write(map[string]any{
		"juggernaut":  map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"model":       "sonnet",
		"effortLevel": "high",
	}); err != nil {
		t.Fatalf("Write seed: %v", err)
	}

	// Plan deletes model (empty string) and effortLevel (empty string).
	plan := map[string]any{
		"juggernaut":  map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"model":       "",
		"effortLevel": "",
	}
	if err := m.MergeConfigPlan(plan); err != nil {
		t.Fatalf("MergeConfigPlan: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, ok := got["model"]; ok {
		t.Error("model should be deleted (empty string in plan)")
	}
	if _, ok := got["effortLevel"]; ok {
		t.Error("effortLevel should be deleted (empty string in plan)")
	}
	// juggernaut is always set, even with empty plan — it's special-cased.
	if _, ok := got["juggernaut"]; !ok {
		t.Error("juggernaut block should always be set")
	}
}
