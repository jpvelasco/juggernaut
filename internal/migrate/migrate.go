package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type State struct {
	HasV3Block bool
	V3Version  string
	AuthMode   string
	TooOld     bool
	AlreadyV4  bool
}

func Detect(homeDir string) (*State, error) {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading settings.json: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return &State{}, nil
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
	partsA := strings.SplitN(a, ".", 3)
	partsB := strings.SplitN(b, ".", 3)
	for i := 0; i < 3; i++ {
		var pa, pb int
		fmt.Sscanf(partsA[i], "%d", &pa)
		fmt.Sscanf(partsB[i], "%d", &pb)
		if pa != pb {
			if pa > pb {
				return 1
			}
			return -1
		}
	}
	return 0
}

func stripMarkerBlock(path, beginMarker, endMarker string) (bool, error) {
	data, err := os.ReadFile(path)
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
	return true, os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
