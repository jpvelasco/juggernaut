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
// Codex uses the built-in amazon-bedrock provider which ships a model catalog
// (eliminates "Model metadata not found" warnings and /model 404s). The config
// is minimal: model, model_provider, and an [aws] sub-table for region. Auth
// uses the standard AWS credential chain — Juggernaut's launch wrapper injects
// AWS_BEARER_TOKEN_BEDROCK. Unlike Claude Code it has NO "use bedrock" env var
// — routing lives entirely in the config file.
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
// OwnsConfig recognizes a Codex config Juggernaut wrote by its Bedrock provider
// selection (model_provider == "amazon-bedrock"). A plain user config that merely
// has a top-level `model` is NOT ours — critical so a first-time
// `apply --cli=codex` over an existing Codex config still prompts for auth
// instead of defaulting to iam (Mantle requires a bearer token).
func (codex) OwnsConfig(data map[string]any) bool {
	return data["model_provider"] == "amazon-bedrock"
}

func (codex) NativeManagedKeys() []string {
	return []string{
		"model",
		"model_provider",
		"model_providers",
	}
}

// DeepMergeKeys: model_providers is a nested [model_providers.<id>] table where a
// user may have their own providers; merge only our amazon-bedrock entry.
func (codex) DeepMergeKeys() []string { return []string{"model_providers"} }

// OwnedSubKeys: uninstall removes only the aws sub-table we wrote under
// model_providers.amazon-bedrock. The amazon-bedrock provider is built-in to
// Codex — users may configure their own profile or other settings there — so we
// don't delete the entire provider entry. Dot-notation paths are supported by
// removeOwnedSubKeys for nested removal.
func (codex) OwnedSubKeys() map[string][]string {
	return map[string][]string{"model_providers": {"amazon-bedrock.aws"}}
}

func (codex) ActivationMarkers() (begin, end string) {
	return "# BEGIN: Juggernaut Codex Activation", "# END: Juggernaut Codex Activation"
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
		Regions: []string{"us-east-1", "us-west-2"},
	},
	// NOTE: gpt-oss is intentionally absent. Current Codex is Responses-API-only
	// (it rejects `wire_api = "chat"` at config load — openai/codex
	// CHAT_WIRE_API_REMOVED_ERROR), but gpt-oss on Mantle serves only Chat
	// Completions on /v1 and has no Responses endpoint, so Codex cannot reach it.
	// gpt-oss remains available via OpenCode (which speaks Chat Completions).
}

func codexModel(key string) (codexMantleModel, bool) {
	m, ok := codexModels[key]
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
func (codex) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = codexDefaultModel()
	}
	m, ok := codexModel(key)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Codex model %q (supported: gpt-5.5, gpt-5.4)", key)
	}

	// A model is only reachable in the regions where it's verified available; a
	// user's default region may not serve it. Auto-switch when the region was
	// defaulted, honor-and-warn when it was explicit.
	region, regionMsg, _ := resolveMantleRegion(opts.Region, opts.RegionExplicit, m.Regions)

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

	var warnings []string
	if regionMsg != "" {
		warnings = append(warnings, fmt.Sprintf("%s: %s", m.ModelID, regionMsg))
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
		TokenEnvVar: bedrockAuthEnvName,
		NeedsToken:  true,
	}
}

func (codex) Supports(c Capability) bool {
	return c == CapEffortLevels
}
