package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeDir_PrefersHOME(t *testing.T) {
	t.Setenv("HOME", "/tmp/home-pref")
	t.Setenv("USERPROFILE", "/tmp/userprofile")
	got, err := homeDir()
	if err != nil {
		t.Fatalf("homeDir() error: %v", err)
	}
	if got != "/tmp/home-pref" {
		t.Errorf("homeDir() = %q, want HOME value", got)
	}
}

func TestHomeDir_FallsBackToUserProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "/tmp/userprofile")
	got, err := homeDir()
	if err != nil {
		t.Fatalf("homeDir() error: %v", err)
	}
	if got != "/tmp/userprofile" {
		t.Errorf("homeDir() = %q, want USERPROFILE value", got)
	}
}

// TestHomeDir_FallsBackToUserHomeDir covers the branch where neither HOME nor
// USERPROFILE is set: homeDir falls through to os.UserHomeDir(). On both
// platforms os.UserHomeDir() consults the same now-empty variable ($HOME /
// %USERPROFILE%), so it returns an error — exercising the error branch. Either
// a non-empty path or a descriptive error is acceptable; a silent empty string
// is not.
func TestHomeDir_FallsBackToUserHomeDir(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	got, err := homeDir()
	if err != nil {
		if !strings.Contains(err.Error(), "could not determine home directory") {
			t.Errorf("unexpected error message: %v", err)
		}
		return
	}
	if got == "" {
		t.Error("homeDir() returned empty path and no error")
	}
}

func TestSetEmbeddedConfig_LoadsBytes(t *testing.T) {
	// Save and restore the package-level embedded bytes so other tests that
	// rely on the filesystem fallback are unaffected.
	orig := embeddedConfigBytes
	t.Cleanup(func() { embeddedConfigBytes = orig })

	cfgJSON := `{
		"version": "9.9.9",
		"defaults": {"region": "us-west-2", "authMode": "iam"},
		"models": {"opus": "o", "sonnet": "s", "haiku": "h", "fable": ""},
		"runtime": {}
	}`
	SetEmbeddedConfig([]byte(cfgJSON))

	cfg, err := loadBedrockConfig()
	if err != nil {
		t.Fatalf("loadBedrockConfig() with embedded bytes error: %v", err)
	}
	if cfg.Version != "9.9.9" {
		t.Errorf("loaded version = %q, want 9.9.9", cfg.Version)
	}
}

func TestToMap_RoundTrips(t *testing.T) {
	in := struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}{Name: "x", N: 3}

	m, err := toMap(in)
	if err != nil {
		t.Fatalf("toMap() error: %v", err)
	}
	if m["name"] != "x" {
		t.Errorf("toMap()[name] = %v, want x", m["name"])
	}
	// JSON numbers decode to float64.
	if m["n"].(float64) != 3 {
		t.Errorf("toMap()[n] = %v, want 3", m["n"])
	}
}

func TestFromMap_RoundTrips(t *testing.T) {
	m := map[string]any{"name": "x", "n": float64(3)}

	var out struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := fromMap(m, &out); err != nil {
		t.Fatalf("fromMap() error: %v", err)
	}
	if out.Name != "x" {
		t.Errorf("fromMap() Name = %q, want x", out.Name)
	}
	if out.N != 3 {
		t.Errorf("fromMap() N = %d, want 3", out.N)
	}
}

// TestFromMap_MarshalErrorPropagates covers the json.Marshal error branch:
// a map value that json.Marshal cannot encode (e.g. a channel) must surface
// as a wrapped error, not a panic or a silent zero-value struct.
func TestFromMap_MarshalErrorPropagates(t *testing.T) {
	m := map[string]any{"bad": make(chan int)}

	var out struct{}
	err := fromMap(m, &out)
	if err == nil {
		t.Fatal("expected fromMap() to error on an unmarshalable map value")
	}
	if !strings.Contains(err.Error(), "serializing map") {
		t.Errorf("expected error to mention serializing map, got: %v", err)
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "present")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !fileExists(existing) {
		t.Error("fileExists should report true for an existing file")
	}
	if fileExists(filepath.Join(dir, "absent")) {
		t.Error("fileExists should report false for a missing file")
	}
}

func TestWarnf_WritesToStderr(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old; r.Close(); w.Close() }()

	warnf("hello %s", "world")
	w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := string(data)
	if want := "Warning: hello world\n"; got != want {
		t.Errorf("warnf output = %q, want %q", got, want)
	}
}
