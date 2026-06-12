// Package migrate handles detection and execution of v3-to-v4 migration.
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/internal/safepath"
)

// State describes what was found during migration detection.
type State struct {
	HasV3Block bool
	V3Version  string
	AuthMode   string
	TooOld     bool  // version < 3.2.3 — must upgrade v3 first
	AlreadyV4  bool  // schemaVersion == 2 — migration already complete
}

// Detect inspects homeDir for a v3 Juggernaut block.
func Detect(homeDir string) (*State, error) {
	settingsPath, err := safepath.JoinUnder(homeDir, ".claude", "settings.json")
	if err != nil {
		return nil, fmt.Errorf("invalid settings path: %w", err)
	}
	data, err := safepath.ReadFile(homeDir, settingsPath)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading settings.json: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("settings.json is corrupted: %w", err)
	}

	jRaw, ok := settings["juggernaut"]
	if !ok {
		return &State{}, nil
	}
	jBlock, ok := jRaw.(map[string]any)
	if !ok {
		return &State{}, nil
	}
	metaRaw, ok := jBlock["meta"].(map[string]any)
	if !ok {
		return &State{}, nil
	}
	if metaRaw["managedBy"] != "juggernaut" {
		return &State{}, nil
	}

	state := &State{HasV3Block: true}

	if v, ok := metaRaw["version"].(string); ok {
		state.V3Version = v
		if !meetsMinVersion(v, "3.2.3") {
			state.TooOld = true
		}
	}

	if sv, ok := metaRaw["schemaVersion"].(float64); ok && sv >= 2 {
		state.AlreadyV4 = true
	}

	if auth, ok := jBlock["auth"].(map[string]any); ok {
		if mode, ok := auth["mode"].(string); ok {
			state.AuthMode = mode
		}
	}

	return state, nil
}

// StripLauncherBlocks removes legacy shell launcher blocks from shell profiles in homeDir.
func StripLauncherBlocks(homeDir string) []string {
	profiles := []string{
		filepath.Join(homeDir, ".bashrc"),
		filepath.Join(homeDir, ".bash_profile"),
		filepath.Join(homeDir, ".zshrc"),
		filepath.Join(homeDir, ".profile"),
		filepath.Join(homeDir, ".config", "fish", "config.fish"),
	}

	var stripped []string
	for _, p := range profiles {
		if ok, err := stripMarkerBlock(p, "# BEGIN: Juggernaut Launcher", "# END: Juggernaut Launcher"); err == nil && ok {
			stripped = append(stripped, p)
		}
	}
	return stripped
}

func meetsMinVersion(version, min string) bool {
	return compareSemver(version, min) >= 0
}

func compareSemver(a, b string) int {
	pa := parseVersionParts(a)
	pb := parseVersionParts(b)
	for i := range 3 {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseVersionParts(version string) [3]int {
	parts := strings.SplitN(version, ".", 3)
	var result [3]int
	for i := range 3 {
		if i >= len(parts) {
			break
		}
		_, _ = fmt.Sscanf(parts[i], "%d", &result[i])
	}
	return result
}

func stripMarkerBlock(path, beginMarker, endMarker string) (bool, error) {
	baseDir := filepath.Dir(path)
	data, err := safepath.ReadFile(baseDir, path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	inBlock := false
	found := false

	for _, line := range lines {
		if strings.Contains(line, beginMarker) {
			inBlock = true
			found = true
			continue
		}
		if strings.Contains(line, endMarker) {
			inBlock = false
			continue
		}
		if !inBlock {
			out = append(out, line)
		}
	}

	if !found {
		return false, nil
	}
	return true, safepath.WriteFile(baseDir, path, []byte(strings.Join(out, "\n")))
}
