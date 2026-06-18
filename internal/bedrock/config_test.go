package bedrock_test

import (
	"path/filepath"
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

func TestIsSupportedRegion(t *testing.T) {
	cfg := &bedrock.Config{Regions: []string{"us-east-1", "us-west-2"}}
	if !cfg.IsSupportedRegion("us-east-1") {
		t.Error("us-east-1 should be supported")
	}
	if cfg.IsSupportedRegion("eu-fake-1") {
		t.Error("eu-fake-1 should not be supported")
	}
}
