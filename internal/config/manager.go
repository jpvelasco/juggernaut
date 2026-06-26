// Package config handles atomic read/merge/write of settings.json with backup rotation.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gofrs/flock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

const backupRetain = 5

// Manager handles atomic read/merge/write of a settings.json file.
type Manager struct {
	path string
	base string
}

// NewManager creates a Manager for the settings.json at the given path.
func NewManager(path string) *Manager {
	clean := filepath.Clean(path)
	return &Manager{path: clean, base: filepath.Dir(clean)}
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
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
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

	encoded, err := json.MarshalIndent(data, "", "  ")
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
		if k == "permissions" {
			mergePermissions(existing, v)
			continue
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
			return fmt.Errorf("unsupported type %T for native key %q (expected string, bool, []string, []any, or map[string]any)", v, k)
		}
	}
	return m.Write(existing)
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

// RemoveJuggernautBlock strips Juggernaut-managed keys from settings.json.
// For "permissions", only the defaultMode sub-key is removed so user-defined
// allow/deny rules are preserved.
func (m *Manager) RemoveJuggernautBlock() error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	delete(existing, "juggernaut")
	for _, k := range nativeManagedKeys {
		delete(existing, k)
	}
	// Only remove the Juggernaut-managed sub-key from permissions.
	mergePermissions(existing, nil)
	return m.Write(existing)
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
