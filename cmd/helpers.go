package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

func bedrockConfigPath() string {
	// Try executable directory first
	self, _ := os.Executable()
	if self != "" {
		candidate := filepath.Join(filepath.Dir(self), "bedrock-config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Try current working directory
	if _, err := os.Stat("bedrock-config.json"); err == nil {
		return "bedrock-config.json"
	}
	// Try one level up (for tests running in cmd directory)
	if _, err := os.Stat("../bedrock-config.json"); err == nil {
		return "../bedrock-config.json"
	}
	// Fall back to relative path
	return "bedrock-config.json"
}

func settingsPath(homeDir, scope string) string {
	if scope == "project" {
		return filepath.Join(".", ".claude", "settings.json")
	}
	return filepath.Join(homeDir, ".claude", "settings.json")
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("serializing block: %w", err)
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}
