package config

// readNested navigates a chain of keys through nested map[string]any structures.
// Returns the raw leaf value and true if every key exists and intermediates are maps;
// (nil, false) otherwise. Used by tests to avoid repeated type-assertion cascades.
//
// Usage: readNested(got, "model_providers", "amazon-bedrock", "aws", "region")
func readNested(data map[string]any, chain ...string) (any, bool) {
	current := data
	for i, k := range chain {
		val, ok := current[k]
		if !ok {
			return nil, false
		}
		if i == len(chain)-1 {
			return val, true
		}
		m, ok := val.(map[string]any)
		if !ok {
			return nil, false
		}
		current = m
	}
	return nil, false
}
