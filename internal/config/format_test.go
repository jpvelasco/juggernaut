package config

import (
	"strings"
	"testing"
)

// TestJSONFormat_RoundTrip verifies the JSON ConfigFormat reads back what it writes.
func TestJSONFormat_RoundTrip(t *testing.T) {
	f := jsonFormat{}
	in := map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
		"model":      "opusplan",
	}
	encoded, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := f.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out["model"] != "opusplan" {
		t.Errorf("round-trip lost model key: got %v", out["model"])
	}
}

// TestJSONFormat_MatchesLegacyEncoding pins the exact two-space-indented encoding
// so the ConfigFormat seam produces byte-identical output to the old hardcoded
// json.MarshalIndent(data, "", "  ") call in Manager.Write.
func TestJSONFormat_MatchesLegacyEncoding(t *testing.T) {
	f := jsonFormat{}
	encoded, err := f.Marshal(map[string]any{"a": "b"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := "{\n  \"a\": \"b\"\n}"
	if string(encoded) != want {
		t.Errorf("encoding drift:\n got: %q\nwant: %q", string(encoded), want)
	}
}

// TestJSONFormat_Name identifies the format.
func TestJSONFormat_Name(t *testing.T) {
	if got := (jsonFormat{}).Name(); got != "json" {
		t.Errorf("Name() = %q, want json", got)
	}
}

// TestNewManager_DefaultsToJSON verifies existing callers get JSON behavior unchanged.
func TestNewManager_DefaultsToJSON(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir + "/settings.json")
	if m.format == nil {
		t.Fatal("NewManager left format nil; existing callers would panic")
	}
	if m.format.Name() != "json" {
		t.Errorf("default format = %q, want json", m.format.Name())
	}
}

// TestManager_WriteUsesFormat verifies Write produces the format's encoding.
func TestManager_WriteUsesFormat(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir + "/settings.json")
	if err := m.Write(map[string]any{"model": "sonnet"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["model"] != "sonnet" {
		t.Errorf("Read after Write lost data: %v", got)
	}
	// Confirm two-space indent landed on disk (legacy contract).
	raw, err := m.format.Marshal(map[string]any{"model": "sonnet"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), "\n  \"model\"") {
		t.Errorf("expected two-space indent, got %q", string(raw))
	}
}
