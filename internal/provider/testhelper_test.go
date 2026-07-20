package provider

import "testing"

// --- nestedMapChain unit tests ---

func TestNestedMapChain_LeafFound(t *testing.T) {
	root := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "leaf",
			},
		},
	}
	v, ok := nestedMapChain(root, "a", "b", "c")
	if !ok || v != "leaf" {
		t.Errorf("nestedMapChain = %v, %v want leaf, true", v, ok)
	}
}

func TestNestedMapChain_MissingKey(t *testing.T) {
	root := map[string]any{"a": map[string]any{"b": "x"}}
	_, ok := nestedMapChain(root, "a", "b", "missing")
	if ok {
		t.Error("should not find missing key")
	}
}

func TestNestedMapChain_NonMapIntermediate(t *testing.T) {
	root := map[string]any{"a": map[string]any{"b": "not-a-map"}}
	_, ok := nestedMapChain(root, "a", "b", "c")
	if ok {
		t.Error("should not traverse through a non-map intermediate")
	}
}

func TestNestedMapChain_SingleKey(t *testing.T) {
	root := map[string]any{"top": 42}
	v, ok := nestedMapChain(root, "top")
	if !ok || v != 42 {
		t.Errorf("nestedMapChain = %v, %v want 42, true", v, ok)
	}
}

// --- Shared test helpers ---

// nestedMapChain navigates a chain of keys through nested map[string]any
// structures. The final key returns its raw value (not asserted as map).
// Returns (value, true) if every intermediate level is a map and the leaf
// exists; (nil, false) otherwise.
//
// Usage: nestedMapChain(plan.Keys, "model_providers", "amazon-bedrock", "aws", "region")
func nestedMapChain(root map[string]any, chain ...string) (any, bool) {
	current := root
	for i, k := range chain {
		val, ok := current[k]
		if !ok {
			return nil, false
		}
		// Last key — return the raw value regardless of type.
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