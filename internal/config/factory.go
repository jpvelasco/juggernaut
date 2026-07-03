package config

import "fmt"

// FormatByName resolves a format name ("json", "toml") to a ConfigFormat. This
// is the bridge that lets internal/provider stay decoupled from config: a
// Provider reports its format as a NAME string, and the composition root (cmd/)
// calls FormatByName to obtain the implementation — avoiding an import cycle
// (config must not import provider). An empty name defaults to JSON so existing
// callers are unaffected.
func FormatByName(name string) (ConfigFormat, error) {
	switch name {
	case "", "json":
		return jsonFormat{}, nil
	case "toml":
		return tomlFormat{}, nil
	default:
		return nil, fmt.Errorf("unknown config format %q (supported: json, toml)", name)
	}
}
