package provider

import (
	"fmt"
	"path/filepath"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// opencode is the OpenCode CLI provider (sst/anomalyco, config JSON at
// ~/.config/opencode/opencode.json). OpenCode is model-agnostic, so it routes to
// Bedrock via a custom OpenAI-compatible provider block
// (npm "@ai-sdk/openai-compatible", options.baseURL → Mantle, apiKey via
// {env:VAR}). Unlike Codex it has no "use bedrock" env var — routing lives in the
// config; the bearer token is injected via the apiKey env interpolation.
//
// Because OpenCode targets ANY compatible model, it uses a curated tier of
// live-/table-verified Mantle models plus a --model passthrough for any other
// Mantle id (with an "unverified" warning). The curated coding models all speak
// Chat Completions on /v1 (verified in models-api-compatibility.html), so the
// provider block is uniform; the OpenAI Responses-only gpt-5.x family is
// intentionally NOT in the curated set (it needs a different npm package).
type opencode struct {
	BaseProvider
}

// mantleProviderID is the provider key written into opencode.json.
const mantleProviderID = "bedrock-mantle"

// ConfigPath is ~/.config/opencode/opencode.json (user) or ./opencode.json
// (project).
func (o opencode) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", "opencode.json"), nil
	}
	return safepath.JoinUnder(home, ".config", "opencode", "opencode.json")
}

// NativeManagedKeys are the top-level opencode.json keys Juggernaut owns.
func (o opencode) NativeManagedKeys() []string {
	return []string{"model", "provider"}
}

// DeepMergeKeys: "provider" is a nested map where a user may have their own
// providers (anthropic, openai, …); merge only our bedrock-mantle entry.
func (o opencode) DeepMergeKeys() []string { return []string{"provider"} }

// OwnedSubKeys: uninstall removes only our bedrock-mantle provider from the
// provider map (the model leaf is removed whole).
func (o opencode) OwnedSubKeys() map[string][]string {
	return map[string][]string{"provider": {mantleProviderID}}
}

// OwnsConfig recognizes a config Juggernaut wrote by our bedrock-mantle provider
// under the "provider" map — NOT a plain user opencode.json that merely has a
// "provider" block for other vendors or a "model" key.
func (o opencode) OwnsConfig(data map[string]any) bool {
	prov, ok := data["provider"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = prov[mantleProviderID]
	return ok
}

func (o opencode) LaunchSpec() LaunchSpec {
	// OpenCode routes via config; the bearer token is injected through the
	// apiKey {env:...} interpolation, so no static enable flag — but the token
	// is still required (Mantle).
	return LaunchSpec{
		TokenEnvVar: bedrockAuthEnvName,
		NeedsToken:  true,
	}
}

// opencodeCuratedModels maps a friendly key to a verified Mantle model ID. All
// are Chat Completions on /v1 (see models-api-compatibility.html + mantle-model-
// matrix). Popular coding models across vendors, reflecting OpenCode's
// model-agnostic design.
var opencodeCuratedModels = map[string]string{
	"gpt-oss-120b":  "openai.gpt-oss-120b",
	"gpt-oss-20b":   "openai.gpt-oss-20b",
	"glm-4.7":       "zai.glm-4.7",
	"glm-5":         "zai.glm-5",
	"kimi-k2.5":     "moonshotai.kimi-k2.5",
	"deepseek-v3.2": "deepseek.v3.2",
	"qwen3-coder":   "qwen.qwen3-coder-480b-a35b-instruct",
	"grok-4.3":      "xai.grok-4.3",
}

func opencodeDefaultModel() string { return "gpt-oss-120b" }

// BuildConfig writes OpenCode's config: a custom OpenAI-compatible provider block
// pointing at Mantle /v1, plus a top-level default model "provider_id/model_id".
// A curated key resolves to its verified Mantle model ID; any other value is
// passed through verbatim (BYO) with an "unverified" warning.
func (o opencode) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = opencodeDefaultModel()
	}

	var warnings []string
	modelID, curated := opencodeCuratedModels[key]
	if !curated {
		// Passthrough: treat the given value as a raw Mantle model ID. Honest
		// about the risk — we haven't verified its API/path.
		modelID = key
		warnings = append(warnings, fmt.Sprintf(
			"model %q is not in the curated set — writing it verbatim as a Mantle model. "+
				"It may use a different API/path than Chat Completions and fail at runtime; "+
				"verify against the model card if it doesn't work.", key))
	}

	baseURL := fmt.Sprintf("https://bedrock-mantle.%s.api.aws/v1", opts.Region)
	keys := map[string]any{
		"model": mantleProviderID + "/" + modelID,
		"provider": map[string]any{
			mantleProviderID: map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Amazon Bedrock (Mantle)",
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  "{env:" + bedrockAuthEnvName + "}",
				},
				"models": map[string]any{
					modelID: map[string]any{"name": modelID},
				},
			},
		},
	}

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: o.NativeManagedKeys(),
		Warnings:    warnings,
	}, nil
}
