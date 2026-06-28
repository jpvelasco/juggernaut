package bedrock_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

func TestLoad(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	cfg, err := bedrock.Load(filepath.Join(repoRoot, "bedrock-config.json"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Version == "" {
		t.Error("expected Version to be set")
	}
	if cfg.Models.Sonnet == "" {
		t.Error("expected Models.Sonnet to be set")
	}
	if len(cfg.Regions) == 0 {
		t.Error("expected at least one region")
	}
	if cfg.Defaults.Region == "" {
		t.Error("expected Defaults.Region to be set")
	}
}

func TestLoad_RejectsWrongFilename(t *testing.T) {
	_, err := bedrock.Load(filepath.Join("..", "..", "go.mod"))
	if err == nil {
		t.Fatal("expected error for wrong filename")
	}
	if !strings.Contains(err.Error(), "invalid bedrock config filename") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_MissingFileErrors(t *testing.T) {
	_, err := bedrock.Load(filepath.Join(t.TempDir(), "bedrock-config.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "reading bedrock-config.json") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_ParsesValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bedrock-config.json")
	content := `{"version":"1.2.3","defaults":{"region":"us-west-2","authMode":"iam"},` +
		`"models":{"opus":"o","sonnet":"s","haiku":"h","fable":""},"regions":["us-west-2"]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg, err := bedrock.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", cfg.Version)
	}
}

func TestLoadBytes_InvalidJSONErrors(t *testing.T) {
	_, err := bedrock.LoadBytes([]byte("{not valid json"))
	if err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing bedrock-config.json") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsSupportedRegion(t *testing.T) {
	cfg := &bedrock.Config{Regions: []string{"us-east-1", "us-west-2"}}
	if !cfg.IsSupportedRegion("us-east-1") {
		t.Error("us-east-1 should be supported")
	}
	if cfg.IsSupportedRegion("eu-fake-1") {
		t.Error("eu-fake-1 should not be supported")
	}
}
