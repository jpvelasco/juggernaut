package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
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
// OpenCode targets any Mantle model compatible with Chat Completions. Juggernaut
// injects the account's cached live inventory instead of maintaining a roster.
// Mantle currently omits protocol metadata, so the compatibility rule is broad
// with a narrow exclusion for the known Responses-only OpenAI GPT-5 family.
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
		TokenEnvVar: authmode.BedrockAuthEnvName,
		NeedsToken:  true,
	}
}

// opencodeModelAliases are convenience spellings, not an availability roster.
// The live account/region catalog remains authoritative.
var opencodeModelAliases = map[string]string{
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
// A convenience alias resolves to a Mantle model ID; any other value is passed
// through verbatim. Every compatible discovered model is added to the provider
// block so OpenCode's model picker can use the account's actual inventory.
func (o opencode) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = opencodeDefaultModel()
	}

	var warnings []string
	modelID, aliased := opencodeModelAliases[key]
	if !aliased {
		// Passthrough: treat the given value as a raw Mantle model ID. Honest
		// about the risk — we haven't verified its API/path.
		modelID = key
		warnings = append(warnings, fmt.Sprintf(
			"model %q is not a known convenience alias — writing it verbatim as a Mantle model. "+
				"It may use a different API/path than Chat Completions and fail at runtime; "+
				"verify against the model card if it doesn't work.", key))
	}

	models := map[string]any{modelID: map[string]any{"name": modelID}}
	for _, discovered := range opts.ModelCatalog {
		if support := o.SupportsModel(discovered); support.Supported {
			models[discovered.ID] = map[string]any{"name": discovered.ID}
		}
	}
	if w := catalogUnavailableWarning(opts.ModelCatalog, modelID, opts.Region, "it remains configured for explicit use", o, opts.RefreshedSources); w != "" {
		warnings = append(warnings, w)
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
					"apiKey":  "{env:" + authmode.BedrockAuthEnvName + "}",
				},
				"models": models,
			},
		},
	}

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: o.NativeManagedKeys(),
		Warnings:    warnings,
	}, nil
}

func (o opencode) SupportsModel(model CatalogModel) ModelSupport {
	if model.Source != "mantle" {
		return ModelSupport{Reason: "OpenCode's Bedrock provider routes through Mantle"}
	}
	if model.Status != "ACTIVE" {
		return ModelSupport{Reason: "model is not ACTIVE"}
	}
	if !model.IsAvailable() {
		return ModelSupport{Reason: "model is not available to this AWS account"}
	}
	if strings.HasPrefix(model.ID, "openai.gpt-5.") {
		return ModelSupport{Reason: "GPT-5 on Mantle requires the Responses API"}
	}
	return ModelSupport{Supported: true, Reason: "Mantle Chat-compatible candidate"}
}

func (o opencode) CatalogSources() []string { return []string{"mantle"} }
