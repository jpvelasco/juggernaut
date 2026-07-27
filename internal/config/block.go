// Package config provides helpers for reading the juggernaut block from
// provider config files (JSON or TOML).
package config

// JuggernautBlock holds the parsed contents of a juggernaut config block.
type JuggernautBlock struct {
	// ManagedBy is "juggernaut" when this block was written by Juggernaut.
	ManagedBy string
	// AuthMode is the persisted auth mode (IAM or BedrockAPIKey).
	AuthMode string
	// Region is the persisted AWS region.
	Region string
	// PermissionMode is the persisted permissions.defaultMode.
	PermissionMode string
}

// ParseJuggernautBlock extracts the juggernaut block from a config map.
// Returns the parsed block and true if the block exists and is managed by
// Juggernaut (managedBy == "juggernaut"); otherwise returns a zero block and false.
func ParseJuggernautBlock(data map[string]any) (*JuggernautBlock, bool) {
	block, ok := data["juggernaut"].(map[string]any)
	if !ok {
		return nil, false
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		return nil, false
	}
	managedBy, _ := meta["managedBy"].(string)
	if managedBy != "juggernaut" {
		return nil, false
	}

	jb := &JuggernautBlock{
		ManagedBy: managedBy,
	}

	if auth, ok := block["auth"].(map[string]any); ok {
		jb.AuthMode, _ = auth["mode"].(string)
		jb.Region, _ = auth["region"].(string)
	}

	if meta, ok := block["meta"].(map[string]any); ok {
		jb.PermissionMode, _ = meta["permissionMode"].(string)
	}

	return jb, true
}
