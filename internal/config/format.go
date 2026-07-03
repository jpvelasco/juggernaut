package config

import "encoding/json"

// ConfigFormat abstracts the on-disk encoding of a settings file so the Manager
// can read/write JSON (Claude Code, OpenCode, Grok) or TOML (Codex) without
// caring which. Implementations must round-trip: Unmarshal(Marshal(x)) == x.
type ConfigFormat interface {
	// Name identifies the format ("json", "toml").
	Name() string
	// Unmarshal decodes bytes into a generic map.
	Unmarshal(data []byte) (map[string]any, error)
	// Marshal encodes a generic map to bytes.
	Marshal(data map[string]any) ([]byte, error)
}

// jsonFormat is the default ConfigFormat. Its Marshal reproduces the exact
// two-space-indented encoding the Manager used before the format seam existed,
// so existing settings.json output is byte-identical.
type jsonFormat struct{}

func (jsonFormat) Name() string { return "json" }

func (jsonFormat) Unmarshal(data []byte) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (jsonFormat) Marshal(data map[string]any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
