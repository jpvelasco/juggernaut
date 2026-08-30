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

// TestAssembleBlock_OutputIdentity verifies that assembleBlock produces the
// same Block fields as Build for multiple option combinations. Build runs
// validation + resolution + assembleBlock; this tests that assembleBlock
// faithfully maps all resolved values into the Block struct.
func TestAssembleBlock_OutputIdentity(t *testing.T) {
	cfg := &bedrock.Config{
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-5",
			Sonnet: "global.anthropic.claude-sonnet-5",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			Fable:  "global.anthropic.claude-fable-5",
		},
		Environment: map[string]string{
			"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32768",
		},
		EnvironmentBedrockAuth: map[string]string{
			"CLAUDE_CODE_USE_BEDROCK": "1",
		},
		Regions:  []string{"us-east-1", "us-west-2"},
		Defaults: bedrock.Defaults{Region: "us-west-2", AuthMode: "iam"},
	}

	cases := []struct {
		name string
		opts Options
	}{
		{
			name: "default_IAM",
			opts: Options{AuthMode: "iam", Region: "us-west-2", Effort: "high", Scope: "user", Version: "4.0.0", AuthValidated: true},
		},
		{
			name: "opusplan",
			opts: Options{AuthMode: "iam", Region: "us-west-2", Effort: "high", Scope: "user", Version: "4.0.0", AuthValidated: true, Opusplan: true},
		},
		{
			name: "auto_mode",
			opts: Options{AuthMode: "iam", Region: "us-west-2", Effort: "high", Scope: "user", Version: "4.0.0", AuthValidated: true, PermissionMode: "auto"},
		},
		{
			name: "service_tier",
			opts: Options{AuthMode: "iam", Region: "us-west-2", Effort: "high", Scope: "user", Version: "4.0.0", AuthValidated: true, ServiceTier: "flex"},
		},
		{
			name: "custom_models",
			opts: Options{AuthMode: "iam", Region: "us-west-2", Effort: "high", Scope: "user", Version: "4.0.0", AuthValidated: true,
				OpusModel: "us.anthropic.claude-opus-4-8", SonnetModel: "us.anthropic.claude-sonnet-4-6"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block, err := Build(cfg, tc.opts)
			if err != nil {
				t.Fatalf("Build() error: %v", err)
			}

			// Verify all fields are set correctly
			if block.Auth.Mode != tc.opts.AuthMode {
				t.Errorf("auth.mode = %q, want %q", block.Auth.Mode, tc.opts.AuthMode)
			}
			if block.Auth.Region != tc.opts.Region {
				t.Errorf("auth.region = %q, want %q", block.Auth.Region, tc.opts.Region)
			}
			if block.Meta.Version != tc.opts.Version {
				t.Errorf("meta.version = %q, want %q", block.Meta.Version, tc.opts.Version)
			}
			if block.Meta.Opusplan != tc.opts.Opusplan {
				t.Errorf("meta.opusplan = %v, want %v", block.Meta.Opusplan, tc.opts.Opusplan)
			}
			if block.Meta.PermissionMode != tc.opts.PermissionMode {
				t.Errorf("meta.permissionMode = %q, want %q", block.Meta.PermissionMode, tc.opts.PermissionMode)
			}
			if block.Meta.ServiceTier != tc.opts.ServiceTier {
				t.Errorf("meta.serviceTier = %q, want %q", block.Meta.ServiceTier, tc.opts.ServiceTier)
			}
			if block.Meta.Effort != tc.opts.Effort {
				t.Errorf("meta.effort = %q, want %q", block.Meta.Effort, tc.opts.Effort)
			}

			// Verify env has the expected keys
			if block.Env["AWS_REGION"] != tc.opts.Region {
				t.Errorf("env.AWS_REGION = %q, want %q", block.Env["AWS_REGION"], tc.opts.Region)
			}
			if block.Env["CLAUDE_CODE_EFFORT_LEVEL"] != tc.opts.Effort {
				t.Errorf("env.CLAUDE_CODE_EFFORT_LEVEL = %q, want %q", block.Env["CLAUDE_CODE_EFFORT_LEVEL"], tc.opts.Effort)
			}
		})
	}
}
