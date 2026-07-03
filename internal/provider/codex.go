package provider

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// codex is the OpenAI Codex CLI provider (config at ~/.codex/config.toml, TOML).
//
// Codex routes to Bedrock via a custom [model_providers.<id>] block with a
// base_url + env_key + wire_api (verified from openai/codex's
// model-provider-info Rust struct). Unlike Claude Code it has NO
// "use bedrock" env var — routing lives entirely in the config file — so
// BedrockEnvVar returns empty and the launcher relies on the config + the
// injected bearer token (env_key).
type codex struct{}

func (codex) Name() string { return "codex" }

func (codex) BinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"codex.exe", "codex.cmd", "codex.bat"}
	}
	return []string{"codex"}
}

func (codex) ConfigFormatName() string { return "toml" }

// ConfigPath is ~/.codex/config.toml (user) or ./.codex/config.toml (project).
func (codex) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".codex", "config.toml"), nil
	}
	return safepath.JoinUnder(home, ".codex", "config.toml")
}

// NativeManagedKeys are the top-level config.toml keys Juggernaut owns for Codex.
func (codex) NativeManagedKeys() []string {
	return []string{
		"model",
		"model_provider",
		"model_providers",
	}
}

func (codex) ActivationMarkers() (begin, end string) {
	return "# BEGIN: Juggernaut Codex Activation", "# END: Juggernaut Codex Activation"
}

// BedrockEnvVar is empty for Codex: it has no "use bedrock" toggle; the provider
// block in config.toml plus the injected env_key bearer token do the routing.
func (codex) BedrockEnvVar() (key, value string) { return "", "" }

// codexMantleModel describes one OpenAI-family model reachable through Bedrock
// Mantle from Codex. Base path AND wire_api are PER-MODEL — this is the core
// finding, live-verified 2026-07-03: gpt-5.x use /openai/v1 + Responses, gpt-oss
// use /v1 + Chat, and gpt-5.5 hard-rejects chat/completions.
type codexMantleModel struct {
	ModelID  string   // Bedrock Mantle model ID, e.g. "openai.gpt-5.5"
	BasePath string   // endpoint path suffix: "/openai/v1" or "/v1"
	WireAPI  string   // Codex wire_api: "responses" or "chat"
	Regions  []string // regions where live-servable (informational)
}

// codexModels maps a friendly key to its verified Mantle facts. Sourced from AWS
// model cards + live inference against bedrock-mantle (see mantle-model-matrix).
var codexModels = map[string]codexMantleModel{
	"gpt-5.5": {
		ModelID:  "openai.gpt-5.5",
		BasePath: "/openai/v1",
		WireAPI:  "responses",
		Regions:  []string{"us-east-1", "us-east-2"},
	},
	"gpt-5.4": {
		ModelID:  "openai.gpt-5.4",
		BasePath: "/openai/v1",
		WireAPI:  "responses",
		Regions:  []string{"us-east-1", "us-west-2"},
	},
	"gpt-oss-120b": {
		ModelID:  "openai.gpt-oss-120b",
		BasePath: "/v1",
		WireAPI:  "chat",
		Regions:  []string{"us-east-1", "us-east-2", "us-west-2"},
	},
	"gpt-oss-20b": {
		ModelID:  "openai.gpt-oss-20b",
		BasePath: "/v1",
		WireAPI:  "chat",
		Regions:  []string{"us-east-1", "us-east-2", "us-west-2"},
	},
}

func codexModel(key string) (codexMantleModel, bool) {
	m, ok := codexModels[key]
	return m, ok
}

// codexDefaultModel is GPT-5.5 — the flagship, mirroring the native Codex CLI.
func codexDefaultModel() string { return "gpt-5.5" }

// BuildConfig writes Codex's config.toml keys: a top-level model + model_provider
// plus a [model_providers.bedrock-mantle] block whose base_url, wire_api, and
// env_key are derived from the chosen model's verified Mantle facts. Base path
// and wire_api are PER-MODEL (live-verified): gpt-5.x → /openai/v1 + responses;
// gpt-oss → /v1 + chat.
func (codex) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = codexDefaultModel()
	}
	m, ok := codexModel(key)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Codex model %q (supported: gpt-5.5, gpt-5.4, gpt-oss-120b, gpt-oss-20b)", key)
	}

	baseURL := fmt.Sprintf("https://bedrock-mantle.%s.api.aws%s", opts.Region, m.BasePath)
	provBlock := map[string]any{
		"name":     "Amazon Bedrock (Mantle)",
		"base_url": baseURL,
		"wire_api": m.WireAPI,
		"env_key":  "AWS_BEARER_TOKEN_BEDROCK",
	}

	keys := map[string]any{
		"model":          m.ModelID,
		"model_provider": "bedrock-mantle",
		"model_providers": map[string]any{
			"bedrock-mantle": provBlock,
		},
	}

	var warnings []string
	if len(m.Regions) > 0 && !regionAllowed(opts.Region, m.Regions) {
		warnings = append(warnings, fmt.Sprintf(
			"model %s is not confirmed available in %s (known: %v) — apply will still write config, but requests may fail",
			m.ModelID, opts.Region, m.Regions))
	}

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: codex{}.NativeManagedKeys(),
		Warnings:    warnings,
	}, nil
}

func (codex) LaunchSpec() LaunchSpec {
	// Codex has no "use bedrock" flag — routing lives in config.toml. Only the
	// bearer token is injected at runtime (Mantle requires it).
	return LaunchSpec{
		TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
		NeedsToken:  true,
	}
}

func (codex) Supports(c Capability) bool {
	return c == CapEffortLevels
}

func regionAllowed(region string, allowed []string) bool {
	for _, r := range allowed {
		if r == region {
			return true
		}
	}
	return false
}
