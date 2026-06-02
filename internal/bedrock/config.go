package bedrock

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadBytes parses a Config from raw JSON bytes (e.g. from an embedded file).
func LoadBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing bedrock-config.json: %w", err)
	}
	return &cfg, nil
}

type Config struct {
	Version                string            `json:"version"`
	Models                 ModelSet          `json:"models"`
	Environment            map[string]string `json:"environment"`
	EnvironmentBedrockAuth map[string]string `json:"environment_bedrock_auth"`
	Regions                []string          `json:"regions"`
	Defaults               Defaults          `json:"defaults"`
}

type ModelSet struct {
	Default string `json:"default"`
	Fast    string `json:"fast"`
	Opus    string `json:"opus"`
	Sonnet  string `json:"sonnet"`
	Haiku   string `json:"haiku"`
}

type Defaults struct {
	Region   string `json:"region"`
	AuthMode string `json:"auth_mode"`
	Model    string `json:"model"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading bedrock-config.json: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing bedrock-config.json: %w", err)
	}
	return &cfg, nil
}

func (c *Config) IsSupportedRegion(region string) bool {
	for _, r := range c.Regions {
		if r == region {
			return true
		}
	}
	return false
}
