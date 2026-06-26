// Package bedrock loads and validates bedrock-config.json.
package bedrock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Config is the typed representation of bedrock-config.json.
type Config struct {
	Version                string            `json:"version"`
	Models                 ModelSet          `json:"models"`
	Environment            map[string]string `json:"environment"`
	EnvironmentBedrockAuth map[string]string `json:"environment_bedrock_auth"`
	Regions                []string          `json:"regions"`
	Defaults               Defaults          `json:"defaults"`
}

// ModelSet holds the Bedrock inference profile IDs for each model tier.
type ModelSet struct {
	Default string `json:"default"`
	Fast    string `json:"fast"`
	Opus    string `json:"opus"`
	Sonnet  string `json:"sonnet"`
	Haiku   string `json:"haiku"`
	Fable   string `json:"fable"`
}

// Defaults holds the default values for region, auth mode, and model.
type Defaults struct {
	Region   string `json:"region"`
	AuthMode string `json:"auth_mode"`
	Model    string `json:"model"`
}

// Load reads and parses bedrock-config.json from the given path.
func Load(path string) (*Config, error) {
	clean := filepath.Clean(path)
	if filepath.Base(clean) != "bedrock-config.json" {
		return nil, fmt.Errorf("invalid bedrock config filename: %s", filepath.Base(clean))
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("reading bedrock-config.json: %w", err)
	}
	return LoadBytes(data)
}

// LoadBytes parses a Config from raw JSON bytes (e.g. from an embedded file).
func LoadBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing bedrock-config.json: %w", err)
	}
	return &cfg, nil
}

// IsSupportedRegion returns true if region is in the supported list.
func (c *Config) IsSupportedRegion(region string) bool {
	return slices.Contains(c.Regions, region)
}
