package config

import (
	"bytes"

	"github.com/BurntSushi/toml"
)

// tomlFormat is the ConfigFormat for Codex CLI (~/.codex/config.toml). Nested
// map[string]any values marshal to TOML tables, so a provider map produces the
// [model_providers.<id>] tables Codex expects.
type tomlFormat struct{}

func (tomlFormat) Name() string { return "toml" }

func (tomlFormat) Unmarshal(data []byte) (map[string]any, error) {
	result := map[string]any{}
	if err := toml.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (tomlFormat) Marshal(data map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
