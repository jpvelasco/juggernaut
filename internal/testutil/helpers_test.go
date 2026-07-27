package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMkdirAll(t *testing.T) {
	home := NewTestHome(t)
	dir := filepath.Join(home, "a", "b", "c")
	if err := MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestMkdirAll_Error(t *testing.T) {
	home := NewTestHome(t)
	file := filepath.Join(home, "is_a_file")
	_ = os.WriteFile(file, nil, 0o600)
	err := MkdirAll(file, 0o700)
	if err == nil {
		t.Fatal("expected error when path is a file")
	}
}

func TestParseJSON(t *testing.T) {
	settings, err := ParseJSON([]byte(`{"key": "value"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if settings["key"] != "value" {
		t.Errorf("unexpected value: %v", settings["key"])
	}
}

func TestParseJSON_Error(t *testing.T) {
	_, err := ParseJSON([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestOwnedJuggernautBlock(t *testing.T) {
	block := OwnedJuggernautBlock()
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		t.Fatal("expected meta map")
	}
	if meta["managedBy"] != "juggernaut" {
		t.Errorf("managedBy = %v, want juggernaut", meta["managedBy"])
	}
}
