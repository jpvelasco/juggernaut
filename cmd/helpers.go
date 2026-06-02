package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
)

// embeddedConfigBytes holds bedrock-config.json bytes injected at startup from main.go.
var embeddedConfigBytes []byte

// SetEmbeddedConfig is called by main() to inject the embedded bedrock-config.json bytes.
func SetEmbeddedConfig(data []byte) {
	embeddedConfigBytes = data
}

// loadBedrockConfig loads bedrock config, preferring the embedded bytes.
// Falls back to filesystem for tests and development builds.
func loadBedrockConfig() (*bedrock.Config, error) {
	if len(embeddedConfigBytes) > 0 {
		return bedrock.LoadBytes(embeddedConfigBytes)
	}
	// Fallback for tests and dev builds that don't set embeddedConfigBytes.
	path := findBedrockConfigFile()
	return bedrock.Load(path)
}

func findBedrockConfigFile() string {
	self, _ := os.Executable()
	if self != "" {
		if candidate := filepath.Join(filepath.Dir(self), "bedrock-config.json"); fileExists(candidate) {
			return candidate
		}
	}
	if fileExists("bedrock-config.json") {
		return "bedrock-config.json"
	}
	if fileExists("../bedrock-config.json") {
		return "../bedrock-config.json"
	}
	return "bedrock-config.json"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
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
