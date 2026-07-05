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
// model-provider-info Rust struct). Unlike Claude Code it has NO "use bedrock"
// env var — routing lives entirely in the config file, so its LaunchSpec has an
// empty StaticEnv and relies on the config plus the injected bearer token.
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
// OwnsConfig recognizes a Codex config Juggernaut wrote by its Bedrock-Mantle
// routing (model_provider == "bedrock-mantle"). A plain user config that merely
// has a top-level `model` is NOT ours — critical so a first-time
// `apply --cli=codex` over an existing Codex config still prompts for auth
// instead of defaulting to iam (Mantle requires a bearer token).
func (codex) OwnsConfig(data map[string]any) bool {
	return data["model_provider"] == "bedrock-mantle"
}

func (codex) NativeManagedKeys() []string {
	return []string{
		"model",
		"model_provider",
		"model_providers",
	}
}

// DeepMergeKeys: model_providers is a nested [model_providers.<id>] table where a
// user may have their own providers; merge only our bedrock-mantle entry.
func (codex) DeepMergeKeys() []string { return []string{"model_providers"} }

// OwnedSubKeys: uninstall removes only our bedrock-mantle provider from the
// model_providers table (model / model_provider leaves are removed whole).
func (codex) OwnedSubKeys() map[string][]string {
	return map[string][]string{"model_providers": {"bedrock-mantle"}}
}

func (codex) ActivationMarkers() (begin, end string) {
	return "# BEGIN: Juggernaut Codex Activation", "# END: Juggernaut Codex Activation"
}

// codexMantleModel describes one OpenAI-family model reachable through Bedrock
// Mantle from Codex. Current Codex is Responses-API-only (it rejects
// `wire_api = "chat"` at config load), so every entry here uses /openai/v1 +
// Responses. gpt-5.5 hard-rejects chat/completions, live-verified 2026-07-03.
type codexMantleModel struct {
	ModelID  string   // Bedrock Mantle model ID, e.g. "openai.gpt-5.5"
	BasePath string   // endpoint path suffix (always "/openai/v1" for Codex)
	WireAPI  string   // Codex wire_api (always "responses")
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
		return ConfigPlan{}, fmt.Errorf("unknown Codex model %q (supported: gpt-5.5, gpt-5.4)", key)
	}

	// A model is only reachable in the regions where it's verified available; a
	// user's default region may not serve it. Auto-switch when the region was
	// defaulted, honor-and-warn when it was explicit.
	region, regionMsg, _ := resolveMantleRegion(opts.Region, opts.RegionExplicit, m.Regions)

	baseURL := fmt.Sprintf("https://bedrock-mantle.%s.api.aws%s", region, m.BasePath)
	provBlock := map[string]any{
		"name":     "Amazon Bedrock (Mantle)",
		"base_url": baseURL,
		"wire_api": m.WireAPI,
		"env_key":  bedrockAuthEnvName,
		// Skip the ChatGPT/OpenAI login screen. Codex prompts for OpenAI auth only
		// when the active provider's requires_openai_auth is true (verified in
		// openai/codex tui should_show_login_screen); our credential is the bearer
		// token in env_key, so declare no OpenAI auth is required.
		"requires_openai_auth": false,
	}

	keys := map[string]any{
		"model":          m.ModelID,
		"model_provider": "bedrock-mantle",
		"model_providers": map[string]any{
			"bedrock-mantle": provBlock,
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
