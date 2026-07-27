package provider

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// codex is the OpenAI Codex CLI provider (config at ~/.codex/config.toml, TOML).
//
// Codex uses the built-in amazon-bedrock provider which ships a model catalog
// (eliminates "Model metadata not found" warnings and /model 404s). The config
// is minimal: model, model_provider, and an [aws] sub-table for region. Auth
// uses the standard AWS credential chain — Juggernaut's launch wrapper injects
// AWS_BEARER_TOKEN_BEDROCK. Unlike Claude Code it has NO "use bedrock" env var
// — routing lives entirely in the config file.
type codex struct {
	BaseProvider
}

// ConfigPath is ~/.codex/config.toml (user) or ./.codex/config.toml (project).
func (c codex) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".codex", "config.toml"), nil
	}
	return safepath.JoinUnder(home, ".codex", "config.toml")
}

// NativeManagedKeys are the top-level config.toml keys Juggernaut owns for Codex.
// OwnsConfig recognizes a Codex config Juggernaut wrote by its Bedrock provider
// selection (model_provider == "amazon-bedrock"). A plain user config that merely
// has a top-level `model` is NOT ours — critical so a first-time
// `apply --cli=codex` over an existing Codex config still prompts for auth
// instead of defaulting to iam (Mantle requires a bearer token).
func (c codex) OwnsConfig(data map[string]any) bool {
	return data["model_provider"] == "amazon-bedrock"
}

func (c codex) NativeManagedKeys() []string {
	return []string{
		"juggernaut",
		"model",
		"model_provider",
		"model_providers",
	}
}

// DeepMergeKeys: model_providers is a nested [model_providers.<id>] table where a
// user may have their own providers; merge only our amazon-bedrock entry.
func (c codex) DeepMergeKeys() []string { return []string{"model_providers"} }

// OwnedSubKeys: uninstall removes only the region leaf we wrote under
// model_providers.amazon-bedrock.aws. Users may configure their own profile or
// other settings in that sub-table — dot-notation targets the leaf so siblings
// survive.
func (c codex) OwnedSubKeys() map[string][]string {
	return map[string][]string{"model_providers": {"amazon-bedrock.aws.region"}}
}

// codexMantleModel describes one OpenAI-family model reachable through Bedrock
// Mantle from Codex. gpt-5.5 hard-rejects chat/completions (Responses-API-only),
// live-verified 2026-07-03.
type codexMantleModel struct {
	ModelID string   // Bedrock Mantle model ID, e.g. "openai.gpt-5.5"
	Regions []string // regions where live-servable (informational)
}

// codexModels maps a friendly key to its verified Mantle facts. Sourced from AWS
// model cards + live inference against bedrock-mantle (see mantle-model-matrix).
var codexModels = map[string]codexMantleModel{
	"gpt-5.5": {
		ModelID: "openai.gpt-5.5",
		Regions: []string{"us-east-1", "us-east-2"},
	},
	"gpt-5.4": {
		ModelID: "openai.gpt-5.4",
		Regions: []string{"us-east-1", "us-east-2", "us-west-2"},
	},
	// NOTE: gpt-oss is intentionally absent. Current Codex is Responses-API-only
	// (it rejects `wire_api = "chat"` at config load — openai/codex
	// CHAT_WIRE_API_REMOVED_ERROR), but gpt-oss on Mantle serves only Chat
	// Completions on /v1 and has no Responses endpoint, so Codex cannot reach it.
	// gpt-oss remains available via OpenCode (which speaks Chat Completions).
}

func codexModel(key string) (codexMantleModel, bool) {
	m, ok := codexModels[key]
	if ok {
		return m, true
	}
	if strings.HasPrefix(key, "gpt-5.") {
		return codexMantleModel{ModelID: "openai." + key}, true
	}
	if strings.HasPrefix(key, "openai.gpt-5.") {
		return codexMantleModel{ModelID: key}, true
	}
	return m, ok
}

// codexDefaultModel is GPT-5.5 — the flagship, mirroring the native Codex CLI.
func codexDefaultModel() string { return "gpt-5.5" }

// BuildConfig writes Codex's config.toml using the built-in amazon-bedrock
// provider. This provider ships a model catalog with the dotted Mantle wire IDs
// (openai.gpt-5.5 etc.), eliminating the "Model metadata not found" warning and
// the /model 404 that occurred with a custom bedrock-mantle provider.
//
// The built-in provider uses the standard AWS credential chain:
// AWS_BEARER_TOKEN_BEDROCK env var (injected by Juggernaut's launch wrapper) or
// standard AWS SDK credentials (SSO, IAM roles, credential_process).
//
// Config shape:
//
//	model = "openai.gpt-5.5"
//	model_provider = "amazon-bedrock"
//	[model_providers.amazon-bedrock.aws]
//	  region = "us-east-1"
func (c codex) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = codexDefaultModel()
	}
	m, ok := codexModel(key)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Codex model %q (expected a discovered openai.gpt-5.* model)", key)
	}

	return buildWithRegionWarnings(opts, m.ModelID, m.Regions, ": ", c, func(region string) (ConfigPlan, error) {
		keys := map[string]any{
			"model":          m.ModelID,
			"model_provider": "amazon-bedrock",
			"model_providers": map[string]any{
				"amazon-bedrock": map[string]any{
					"aws": map[string]any{
						"region": region,
					},
				},
			},
		}

		// Persist the juggernaut block so the launch wrapper can read the auth mode
		// at runtime (IAM → no token needed; API key → inject bearer token).
		juggernautBlock := &schema.Block{
			Auth: schema.Auth{
				Mode:   opts.AuthMode,
				Region: region,
			},
			Meta: schema.Meta{
				SchemaVersion: 2,
				Version:       opts.Version,
				ManagedBy:     "juggernaut",
				Scope:         opts.Scope,
				AppliedAt:     fmt.Sprintf("%d", time.Now().Unix()),
			},
		}
		blockMap, err := ToMap(juggernautBlock)
		if err != nil {
			return ConfigPlan{}, fmt.Errorf("serialize juggernaut block: %w", err)
		}
		keys["juggernaut"] = blockMap

		return ConfigPlan{
			Keys:        keys,
			ManagedKeys: c.NativeManagedKeys(),
		}, nil
	})
}

func (c codex) SupportsModel(model CatalogModel) ModelSupport {
	return c.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		if m.Source != "mantle" {
			return ModelSupport{Reason: "Codex's Amazon Bedrock provider uses Mantle"}
		}
		if !strings.HasPrefix(m.ID, "openai.gpt-5.") {
			return ModelSupport{Reason: "Codex's built-in Bedrock provider supports OpenAI GPT-5 models"}
		}
		return ModelSupport{Supported: true, Reason: "Codex Responses model"}
	})
}

func (c codex) LaunchSpec() LaunchSpec {
	// Codex has no "use bedrock" flag — routing lives in config.toml.
	// Token injection is decided at launch time from the stored auth mode:
	// API key → inject AWS_BEARER_TOKEN_BEDROCK; IAM → SDK credential chain.
	return LaunchSpec{
		TokenEnvVar: authmode.BedrockAuthEnvName,
		NeedsToken:  false, // auth mode in juggernaut block decides at launch
	}
}
