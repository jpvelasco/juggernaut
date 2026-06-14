// Package migrate handles detection and execution of v3-to-v4 migration.
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/v4/internal/safepath"
)

// State describes what was found during migration detection.
type State struct {
	HasV3Block bool
	V3Version  string
	AuthMode   string
	Storage    string // v3 credential storage backend: keychain, profile, dpapi
	TooOld     bool   // version < 3.2.3 — must upgrade v3 first
	AlreadyV4  bool   // schemaVersion == 2 — migration already complete
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
		if storage, ok := auth["storage"].(string); ok {
			state.Storage = storage
		}
	}

	return state, nil
}

// ReadLegacyToken reads the v3 bearer token based on the storage backend recorded
// in the v3 block. Returns ("", nil) when the token is not found.
//
//   - keychain / "": handled by the caller via keychain.Default().Get()
//   - profile: reads the plaintext token file at
//     ${XDG_CONFIG_HOME:-$HOME/.config}/juggernaut/bearer-token
//   - dpapi: the binary DPAPI file cannot be decrypted in pure Go;
//     returns ("", nil) and sets dpapi=true so the caller can warn
func ReadLegacyToken(homeDir, storage string) (token string, dpapi bool, err error) {
	switch storage {
	case "dpapi":
		return "", true, nil
	case "profile":
		token, err = readProfileToken(homeDir)
		return token, false, err
	default:
		// keychain or empty — caller handles via keychain.Default().Get()
		return "", false, nil
	}
}

// CleanupLegacyFiles removes stale v3 credential files after a successful migration.
// Errors are logged but do not fail the migration.
func CleanupLegacyFiles(homeDir string) []string {
	var removed []string

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}

	candidates := []string{
		filepath.Join(homeDir, ".juggernaut", "bearer-token.dpapi.bin"),
		filepath.Join(configHome, "juggernaut", "bearer-token"),
	}

	for _, p := range candidates {
		if err := os.Remove(p); err == nil {
			removed = append(removed, p)
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: could not remove legacy credential file %s: %v\n", p, err)
		}
	}
	return removed
}

func readProfileToken(homeDir string) (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	}
	tokenPath := filepath.Join(configHome, "juggernaut", "bearer-token")

	data, err := os.ReadFile(tokenPath) // nosemgrep: go_filesystem_rule-fileread
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading profile token: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", nil
	}
	return token, nil
}

// StripLauncherBlocks removes legacy shell launcher blocks from shell profiles in homeDir.
func StripLauncherBlocks(homeDir string) []string {
	relProfiles := [][]string{
		{".bashrc"},
		{".bash_profile"},
		{".zshrc"},
		{".profile"},
		{".config", "fish", "config.fish"},
	}

	var stripped []string
	for _, rel := range relProfiles {
		p, err := safepath.JoinUnder(homeDir, rel...)
		if err != nil {
			continue
		}
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
