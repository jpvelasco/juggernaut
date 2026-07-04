// Package config handles atomic read/merge/write of settings.json with backup rotation.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gofrs/flock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

const backupRetain = 5

// Manager handles atomic read/merge/write of a settings file.
type Manager struct {
	path   string
	base   string
	format ConfigFormat
}

// NewManager creates a Manager for the JSON settings file at the given path.
func NewManager(path string) *Manager {
	return NewManagerWithFormat(path, jsonFormat{})
}

// NewManagerWithFormat creates a Manager that reads/writes using the given
// on-disk format (JSON for Claude Code/OpenCode/Grok, TOML for Codex). A nil
// format is a programming error: Read/Write would otherwise nil-panic deep in
// the I/O path, so fail loudly at the boundary instead.
func NewManagerWithFormat(path string, format ConfigFormat) *Manager {
	if format == nil {
		panic("config.NewManagerWithFormat: format must not be nil")
	}
	clean := filepath.Clean(path)
	return &Manager{path: clean, base: filepath.Dir(clean), format: format}
}

func (m *Manager) Read() (map[string]any, error) {
	data, err := safepath.ReadFile(m.base, m.path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", m.path, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	data = stripUTF8BOM(data)
	result, err := m.format.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", m.path, err)
	}
	return result, nil
}

func (m *Manager) Write(data map[string]any) error {
	if err := safepath.MkdirAll(filepath.Dir(m.path)); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}

	lockPath := m.path + ".lock"
	fl := flock.New(lockPath)
	locked, err := fl.TryLock()
	if err != nil {
		return fmt.Errorf("acquiring settings.json lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("settings.json is locked by another process; if this persists, remove %s and retry", lockPath)
	}
	defer func() { _ = fl.Unlock() }()

	if _, err := os.Stat(m.path); err == nil {
		if err := m.rotateBackup(); err != nil {
			return err
		}
	}

	encoded, err := m.format.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding settings.json: %w", err)
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return fmt.Errorf("writing temp settings file: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		if rmErr := os.Remove(tmp); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("committing settings.json: %w (cleanup of temp file also failed: %v)", err, rmErr)
		}
		return fmt.Errorf("committing settings.json: %w", err)
	}
	return nil
}

// nativeManagedKeys lists every top-level settings.json key Juggernaut fully owns
// (replaced on apply, removed on uninstall). The "permissions" key is handled
// specially — only the "defaultMode" sub-key is managed so user-defined
// allow/deny rules are preserved.
var nativeManagedKeys = []string{
	"env",
	"model",
	"modelOverrides",
	"fallbackModel",
	"effortLevel",
	"alwaysThinkingEnabled",
	"skipWebFetchPreflight",
}

// MergeJuggernautBlock merges the juggernaut block and native top-level keys into
// existing settings. nativeKeys carries all non-env top-level values Juggernaut
// manages. Keys with zero/nil values are deleted from the file.
//
// The "permissions" key is deep-merged: only permissions.defaultMode is set or
// removed; other user-defined permission rules (allow, deny, ask) are preserved.
func (m *Manager) MergeJuggernautBlock(block map[string]any, nativeEnv map[string]string, nativeKeys map[string]any) error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	existing["juggernaut"] = block
	if len(nativeEnv) > 0 {
		existing["env"] = nativeEnv
	}
	for k, v := range nativeKeys {
		if err := applyManagedKey(existing, k, v); err != nil {
			return err
		}
	}
	return m.Write(existing)
}

// applyManagedKey sets or deletes one managed top-level key using the shared
// set-or-delete-by-zero-value semantics. "permissions" is deep-merged (only
// defaultMode is managed). This is the single source of truth for how a managed
// key is applied, shared by MergeJuggernautBlock and MergeConfigPlan.
func applyManagedKey(existing map[string]any, k string, v any) error {
	if k == "permissions" {
		mergePermissions(existing, v)
		return nil
	}
	switch val := v.(type) {
	case string:
		if val != "" {
			existing[k] = val
		} else {
			delete(existing, k)
		}
	case bool:
		if val {
			existing[k] = val
		} else {
			delete(existing, k)
		}
	case map[string]any:
		if len(val) > 0 {
			existing[k] = val
		} else {
			delete(existing, k)
		}
	case map[string]string:
		if len(val) > 0 {
			existing[k] = val
		} else {
			delete(existing, k)
		}
	case []string:
		if len(val) > 0 {
			existing[k] = val
		} else {
			delete(existing, k)
		}
	case []any:
		if len(val) > 0 {
			existing[k] = val
		} else {
			delete(existing, k)
		}
	case nil:
		delete(existing, k)
	default:
		return fmt.Errorf("unsupported type %T for native key %q (expected string, bool, []string, []any, map[string]string, or map[string]any)", v, k)
	}
	return nil
}

// MergeConfigPlan merges a provider's ConfigPlan.Keys into the existing config
// using the same set-or-delete semantics as MergeJuggernautBlock. The
// "juggernaut" key (if present) is always set. This is the generic,
// provider-driven entry point that supersedes MergeJuggernautBlock's fixed shape.
func (m *Manager) MergeConfigPlan(keys map[string]any) error {
	return m.MergeConfigPlanDeep(keys, nil)
}

// MergeConfigPlanDeep is like MergeConfigPlan but deep-merges the named keys
// (nested tables where Juggernaut owns only its OWN sub-keys, e.g. Grok's
// [model.<name>], Codex's [model_providers.<id>], OpenCode's provider.<id>):
// their sub-keys are merged into the existing table rather than replacing it, so
// a user's sibling entries survive. All other keys keep whole-value
// set-or-delete semantics.
func (m *Manager) MergeConfigPlanDeep(keys map[string]any, deepKeys []string) error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	deep := make(map[string]bool, len(deepKeys))
	for _, k := range deepKeys {
		deep[k] = true
	}
	for k, v := range keys {
		if k == "juggernaut" {
			existing[k] = v // the managed block is always set verbatim
			continue
		}
		if deep[k] {
			if err := mergeNested(existing, k, v, m.path); err != nil {
				return err
			}
			continue
		}
		if err := applyManagedKey(existing, k, v); err != nil {
			return err
		}
	}
	return m.Write(existing)
}

// mergeNested merges the sub-keys of a nested-table value into existing[k],
// preserving the user's other sub-keys. Juggernaut's leaves overwrite matching
// ones. If the incoming value isn't a map, it falls back to whole replace.
//
// If existing[k] is already present but holds a NON-table value (a scalar or
// array), the config is corrupt/foreign for a key Juggernaut deep-merges (these
// are always tables in a valid config). Rather than silently discard the user's
// value, refuse and tell them what to fix — losing data quietly is worse than
// stopping.
func mergeNested(existing map[string]any, k string, v any, path string) error {
	incoming, ok := v.(map[string]any)
	if !ok {
		existing[k] = v
		return nil
	}
	dst := map[string]any{}
	if raw, present := existing[k]; present {
		m, isMap := raw.(map[string]any)
		if !isMap {
			return fmt.Errorf("cannot merge into %q in %s: expected a table but found %T — "+
				"remove or fix that key in the file, then re-run", k, path, raw)
		}
		if m != nil {
			dst = m
		}
	}
	for sk, sv := range incoming {
		dst[sk] = sv
	}
	existing[k] = dst
	return nil
}

// mergePermissions sets or removes only the permissions.defaultMode sub-key,
// preserving any other user-defined permission rules (allow, deny, ask, etc.).
func mergePermissions(existing map[string]any, v any) {
	perms, _ := existing["permissions"].(map[string]any)

	var defaultMode string
	if m, ok := v.(map[string]any); ok {
		defaultMode, _ = m["defaultMode"].(string)
	}

	if defaultMode == "" {
		if perms != nil {
			delete(perms, "defaultMode")
			if len(perms) == 0 {
				delete(existing, "permissions")
			}
		}
		return
	}

	if perms == nil {
		perms = map[string]any{}
	}
	perms["defaultMode"] = defaultMode
	existing["permissions"] = perms
}

// RemoveJuggernautBlock strips Claude's Juggernaut-managed keys from
// settings.json. For "permissions", only the defaultMode sub-key is removed so
// user-defined allow/deny rules are preserved.
func (m *Manager) RemoveJuggernautBlock() error {
	return m.RemoveManagedKeys(nativeManagedKeys)
}

// RemoveManagedKeys removes the juggernaut block plus the given top-level
// managed keys, preserving user content. "permissions" is handled specially
// (only defaultMode is stripped). This is the generic, provider-driven form of
// RemoveJuggernautBlock.
func (m *Manager) RemoveManagedKeys(keys []string) error {
	return m.RemoveManagedKeysDeep(keys, nil)
}

// RemoveManagedKeysDeep removes the juggernaut block plus the given managed
// top-level keys. For keys listed in ownedSubKeys (nested tables where a user
// may have their own sibling entries), ONLY Juggernaut's own sub-keys are
// removed — preserving the user's; if that empties the table, the table is
// dropped. All other keys are removed whole-value.
func (m *Manager) RemoveManagedKeysDeep(keys []string, ownedSubKeys map[string][]string) error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	delete(existing, "juggernaut")
	for _, k := range keys {
		if k == "permissions" {
			mergePermissions(existing, nil)
			continue
		}
		if subs, deep := ownedSubKeys[k]; deep {
			if err := removeOwnedSubKeys(existing, k, subs, m.path); err != nil {
				return err
			}
			continue
		}
		delete(existing, k)
	}
	// Ensure the Juggernaut-managed permissions sub-key is stripped even if
	// "permissions" was not in the key list (matches legacy behavior).
	mergePermissions(existing, nil)
	return m.Write(existing)
}

// removeOwnedSubKeys deletes only the named sub-keys from existing[k]'s nested
// table, preserving the user's other entries. Drops the table entirely if it
// becomes empty. If the key is absent there is nothing to remove (a clean
// no-op); but if it is PRESENT holding a non-table value, the config is
// corrupt/foreign for a key Juggernaut owns as a table — surface that instead
// of silently leaving our sub-keys unremoved.
func removeOwnedSubKeys(existing map[string]any, k string, subs []string, path string) error {
	raw, present := existing[k]
	if !present {
		return nil
	}
	tbl, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("cannot remove managed sub-keys from %q in %s: expected a table but found %T — "+
			"remove or fix that key in the file, then re-run", k, path, raw)
	}
	for _, sk := range subs {
		delete(tbl, sk)
	}
	if len(tbl) == 0 {
		delete(existing, k)
	} else {
		existing[k] = tbl
	}
	return nil
}

// HasManagedKeys reports whether the config contains the juggernaut block or any
// of the given managed top-level keys. Unlike HasJuggernautBlock it does not
// require the Claude-specific juggernaut.meta.managedBy marker, so it correctly
// detects CLIs (e.g. Codex TOML) whose config carries only native keys.
func (m *Manager) HasManagedKeys(keys []string) (bool, error) {
	data, err := m.Read()
	if err != nil {
		return false, err
	}
	if _, ok := data["juggernaut"]; ok {
		return true, nil
	}
	for _, k := range keys {
		if _, ok := data[k]; ok {
			return true, nil
		}
	}
	return false, nil
}

// HasJuggernautBlock returns true if settings.json contains a managed Juggernaut block.
func (m *Manager) HasJuggernautBlock() (bool, error) {
	data, err := m.Read()
	if err != nil {
		return false, err
	}
	block, ok := data["juggernaut"].(map[string]any)
	if !ok {
		return false, nil
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		return false, nil
	}
	return meta["managedBy"] == "juggernaut", nil
}

func (m *Manager) rotateBackup() error {
	stamp := time.Now().UTC().Format("20060102_150405")
	backup := m.path + ".backup." + stamp
	if err := copyFile(m.path, backup); err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}
	return pruneBackups(m.path, backupRetain)
}

func pruneBackups(base string, keep int) error {
	pattern := base + ".backup.*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for len(matches) > keep {
		if err := os.Remove(matches[0]); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing old backup %s: %w", matches[0], err)
		}
		matches = matches[1:]
	}
	return nil
}

func copyFile(src, dst string) error {
	base := filepath.Dir(filepath.Clean(src))
	data, err := safepath.ReadFile(base, src)
	if err != nil {
		return err
	}
	dstBase := filepath.Dir(filepath.Clean(dst))
	return safepath.WriteFile(dstBase, dst, data)
}

func stripUTF8BOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}
