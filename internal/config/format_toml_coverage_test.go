package config

import (
	"strings"
	"testing"
)

func TestTOMLFormat_Marshal_EmptyMap(t *testing.T) {
	f := tomlFormat{}
	data, err := f.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("Marshal empty map: %v", err)
	}
	// Empty map should produce empty or whitespace-only output.
	if strings.TrimSpace(string(data)) != "" {
		t.Errorf("expected empty output for empty map, got: %q", data)
	}
}

func TestTOMLFormat_Marshal_ScalarValues(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{
		"string":  "hello",
		"int":     42,
		"float":   3.14,
		"boolean": true,
	}
	data, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal scalars: %v", err)
	}
	out, err := f.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal back: %v", err)
	}
	if out["string"] != "hello" {
		t.Errorf("string = %v, want hello", out["string"])
	}
	// TOML decodes integers as int64.
	if iv, ok := out["int"].(int64); !ok || iv != 42 {
		t.Errorf("int = %v, want 42", out["int"])
	}
	if out["boolean"] != true {
		t.Errorf("boolean = %v, want true", out["boolean"])
	}
}

func TestTOMLFormat_Marshal_NestedMap(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{
		"outer": map[string]any{
			"inner": map[string]any{
				"key": "value",
			},
		},
	}
	data, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal nested: %v", err)
	}
	out, err := f.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal back: %v", err)
	}
	outer, ok := out["outer"].(map[string]any)
	if !ok {
		t.Fatalf("outer not a map: %T", out["outer"])
	}
	inner, ok := outer["inner"].(map[string]any)
	if !ok {
		t.Fatalf("inner not a map: %T", outer["inner"])
	}
	if inner["key"] != "value" {
		t.Errorf("key = %v, want value", inner["key"])
	}
}

func TestTOMLFormat_Marshal_SliceValues(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{
		"array": []any{"a", "b", "c"},
	}
	data, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal slice: %v", err)
	}
	out, err := f.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal back: %v", err)
	}
	arr, ok := out["array"].([]any)
	if !ok || len(arr) != 3 {
		t.Errorf("array = %v (len %d), want [a b c]", arr, len(arr))
	}
}

func TestTOMLFormat_Marshal_MixedNested(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{
		"top": "scalar",
		"table": map[string]any{
			"key1": "val1",
			"key2": map[string]any{
				"deep": true,
			},
		},
		"list": []any{1, 2, 3},
	}
	data, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal mixed: %v", err)
	}
	out, err := f.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal back: %v", err)
	}
	if out["top"] != "scalar" {
		t.Errorf("top = %v, want scalar", out["top"])
	}
}

func TestTOMLFormat_Marshal_BooleanFalse(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{"enabled": false}
	data, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal bool false: %v", err)
	}
	out, err := f.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal back: %v", err)
	}
	if out["enabled"] != false {
		t.Errorf("enabled = %v, want false", out["enabled"])
	}
}

func TestTOMLFormat_Marshal_NilValue(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{"nilKey": nil}
	data, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal nil: %v", err)
	}
	out, err := f.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal back: %v", err)
	}
	// nil should either be absent or nil.
	if v, ok := out["nilKey"]; ok && v != nil {
		t.Errorf("nilKey = %v, want nil or absent", v)
	}
}

func TestTOMLFormat_Marshal_OutputContainsTableHeaders(t *testing.T) {
	f := tomlFormat{}
	encoded, err := f.Marshal(map[string]any{
		"section": map[string]any{
			"sub": map[string]any{"key": "val"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(encoded)
	if !strings.Contains(got, "[section]") {
		t.Errorf("expected [section] header, got:\n%s", got)
	}
	if !strings.Contains(got, "[section.sub]") {
		t.Errorf("expected [section.sub] header, got:\n%s", got)
	}
}

func TestTOMLFormat_Unmarshal_EmptyData(t *testing.T) {
	f := tomlFormat{}
	out, err := f.Unmarshal([]byte{})
	if err != nil {
		t.Fatalf("Unmarshal empty: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map, got %v", out)
	}
}

func TestTOMLFormat_Unmarshal_WhitespaceOnly(t *testing.T) {
	f := tomlFormat{}
	out, err := f.Unmarshal([]byte("   \n  \t  "))
	if err != nil {
		t.Fatalf("Unmarshal whitespace: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty map for whitespace, got %v", out)
	}
}
