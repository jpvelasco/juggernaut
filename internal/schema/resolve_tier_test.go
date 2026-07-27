package schema

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

func testResolveConfig() *bedrock.Config {
	return &bedrock.Config{
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-5",
			Sonnet: "global.anthropic.claude-sonnet-5",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			Fable:  "global.anthropic.claude-fable-5",
		},
	}
}

func TestResolveTierModels_OverrideOpus(t *testing.T) {
	cfg := testResolveConfig()
	opts := Options{OpusModel: "us.anthropic.claude-opus-4-8"}

	opus, sonnet, haiku, fable := resolveTierModels(cfg, opts)

	if opus != "us.anthropic.claude-opus-4-8" {
		t.Errorf("expected opus override, got %q", opus)
	}
	if sonnet != cfg.Models.Sonnet {
		t.Errorf("expected default sonnet, got %q", sonnet)
	}
	if haiku != cfg.Models.Haiku {
		t.Errorf("expected default haiku, got %q", haiku)
	}
	if fable != cfg.Models.Fable {
		t.Errorf("expected default fable, got %q", fable)
	}
}

func TestResolveTierModels_OverrideSonnet(t *testing.T) {
	cfg := testResolveConfig()
	opts := Options{SonnetModel: "us.anthropic.claude-sonnet-4-6"}

	opus, sonnet, _, _ := resolveTierModels(cfg, opts)

	if sonnet != "us.anthropic.claude-sonnet-4-6" {
		t.Errorf("expected sonnet override, got %q", sonnet)
	}
	if opus != cfg.Models.Opus {
		t.Errorf("expected default opus, got %q", opus)
	}
}

func TestResolveTierModels_OverrideHaiku(t *testing.T) {
	cfg := testResolveConfig()
	opts := Options{HaikuModel: "anthropic.claude-haiku-3-5-20241022"}

	_, sonnet, haiku, _ := resolveTierModels(cfg, opts)

	if haiku != "anthropic.claude-haiku-3-5-20241022" {
		t.Errorf("expected haiku override, got %q", haiku)
	}
	if sonnet != cfg.Models.Sonnet {
		t.Errorf("expected default sonnet, got %q", sonnet)
	}
}

func TestResolveTierModels_OverrideFable(t *testing.T) {
	cfg := testResolveConfig()
	opts := Options{FableModel: "us.anthropic.claude-fable-5"}

	_, _, _, fable := resolveTierModels(cfg, opts)

	if fable != "us.anthropic.claude-fable-5" {
		t.Errorf("expected fable override, got %q", fable)
	}
}

func TestResolveTierModels_AllEmpty(t *testing.T) {
	cfg := testResolveConfig()
	opts := Options{}

	opus, sonnet, haiku, fable := resolveTierModels(cfg, opts)

	if opus != cfg.Models.Opus {
		t.Errorf("expected default opus, got %q", opus)
	}
	if sonnet != cfg.Models.Sonnet {
		t.Errorf("expected default sonnet, got %q", sonnet)
	}
	if haiku != cfg.Models.Haiku {
		t.Errorf("expected default haiku, got %q", haiku)
	}
	if fable != cfg.Models.Fable {
		t.Errorf("expected default fable, got %q", fable)
	}
}

func TestResolveTierModels_AllOverridden(t *testing.T) {
	cfg := testResolveConfig()
	opts := Options{
		OpusModel:   "custom-opus",
		SonnetModel: "custom-sonnet",
		HaikuModel:  "custom-haiku",
		FableModel:  "custom-fable",
	}

	opus, sonnet, haiku, fable := resolveTierModels(cfg, opts)

	if opus != "custom-opus" {
		t.Errorf("expected custom opus, got %q", opus)
	}
	if sonnet != "custom-sonnet" {
		t.Errorf("expected custom sonnet, got %q", sonnet)
	}
	if haiku != "custom-haiku" {
		t.Errorf("expected custom haiku, got %q", haiku)
	}
	if fable != "custom-fable" {
		t.Errorf("expected custom fable, got %q", fable)
	}
}

func TestResolveTierModels_EmptyConfig(t *testing.T) {
	cfg := &bedrock.Config{Models: bedrock.ModelSet{}}
	opts := Options{}

	opus, sonnet, haiku, fable := resolveTierModels(cfg, opts)

	if opus != "" {
		t.Errorf("expected empty opus, got %q", opus)
	}
	if sonnet != "" {
		t.Errorf("expected empty sonnet, got %q", sonnet)
	}
	if haiku != "" {
		t.Errorf("expected empty haiku, got %q", haiku)
	}
	if fable != "" {
		t.Errorf("expected empty fable, got %q", fable)
	}
}
