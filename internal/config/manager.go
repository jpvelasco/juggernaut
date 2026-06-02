package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gofrs/flock"
)

const backupRetain = 5

type Manager struct {
	path string
}

func NewManager(path string) *Manager {
	return &Manager{path: path}
}

func (m *Manager) Read() (map[string]any, error) {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", m.path, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", m.path, err)
	}
	return result, nil
}

func (m *Manager) Write(data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
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
	defer fl.Unlock()

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
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
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

func (m *Manager) MergeJuggernautBlock(block map[string]any, nativeEnv map[string]string, model string) error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	existing["juggernaut"] = block
	if len(nativeEnv) > 0 {
		existing["env"] = nativeEnv
	}
	if model != "" {
		existing["model"] = model
	} else {
		delete(existing, "model")
	}
	return m.Write(existing)
}

func (m *Manager) RemoveJuggernautBlock() error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	delete(existing, "juggernaut")
	delete(existing, "env")
	delete(existing, "model")
	delete(existing, "modelOverrides")
	return m.Write(existing)
}

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
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
