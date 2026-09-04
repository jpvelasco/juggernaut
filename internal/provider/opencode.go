package provider

import (
	"fmt"
	"path/filepath"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// opencode is the OpenCode CLI provider (sst/anomalyco, config JSON at
// ~/.config/opencode/opencode.json). OpenCode is model-agnostic, so it routes
// via the built-in amazon-bedrock provider (options.region + live-discovered
// models + whitelist). Unlike Codex it has no "use bedrock" env var — routing
// lives in the config; the bearer token / IAM SigV4 chain is used at runtime.
//
// OpenCode targets any native Bedrock model compatible with Chat Completions.
// Juggernaut injects the account's cached live inventory (foundation + profile)
// instead of maintaining a roster.
type opencode struct {
	BaseProvider
}

// bedrockProviderID is the provider key written into opencode.json.
const bedrockProviderID = "amazon-bedrock"

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
	return []string{"model", "provider", "juggernaut"}
}

// DeepMergeKeys: "provider" is a nested map where a user may have their own
// providers (anthropic, openai, …); merge only our amazon-bedrock entry.
func (o opencode) DeepMergeKeys() []string { return []string{"provider"} }

// OwnedSubKeys: uninstall removes only our amazon-bedrock provider from the
// provider map (the model leaf is removed whole).
func (o opencode) OwnedSubKeys() map[string][]string {
	return map[string][]string{"provider": {bedrockProviderID}}
}

// OwnsConfig recognizes a config Juggernaut wrote by our amazon-bedrock provider
// under the "provider" map — NOT a plain user opencode.json that merely has a
// "provider" block for other vendors or a "model" key.
func (o opencode) OwnsConfig(data map[string]any) bool {
	prov, ok := data["provider"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = prov[bedrockProviderID]
	return ok
}

func (o opencode) LaunchSpec() LaunchSpec {
	// OpenCode routes via config; the built-in amazon-bedrock provider uses
	// the AWS credential chain (SigV4 or AWS_BEARER_TOKEN_BEDROCK), so no static
	// enable flag. Token injection is decided at launch from juggernaut.auth.
	return LaunchSpec{
		TokenEnvVar: authmode.BedrockAuthEnvName,
		NeedsToken:  false, // auth mode in juggernaut block decides at launch
	}
}

// opencodeModelAliases are convenience spellings, not an availability roster.
// The live account/region catalog remains authoritative. IDs are native
// Bedrock foundation IDs verified via `models list --source native` (2026-08-29).
var opencodeModelAliases = map[string]string{
	"gpt-oss-120b":  "openai.gpt-oss-120b-1:0",
	"gpt-oss-20b":   "openai.gpt-oss-20b-1:0",
	"glm-4.7":       "zai.glm-4.7",
	"glm-5":         "zai.glm-5",
	"kimi-k2.5":     "moonshotai.kimi-k2.5",
	"deepseek-v3.2": "deepseek.v3.2",
	"qwen3-coder":   "qwen.qwen3-coder-480b-a35b-v1:0",
	"grok-4.6":      "xai.grok-4.6",
	"grok-4.3":      "xai.grok-4.6", // deprecated: kept for backwards compat; prefer grok-4.6
}

func opencodeDefaultModel() string { return "gpt-oss-120b" }

// BuildConfig writes OpenCode's config: the built-in amazon-bedrock provider
// (options.region + live-discovered models + whitelist) plus a top-level
// default model "provider_id/model_id". A convenience alias resolves to a
// native Bedrock model ID; any other value is passed through verbatim. Every
// compatible discovered model is added to the provider block so OpenCode's
// model picker can use the account's actual inventory.
func (o opencode) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = opencodeDefaultModel()
	}

	var warnings []string
	modelID, aliased := opencodeModelAliases[key]
	if !aliased {
		// Passthrough: treat the given value as a raw native Bedrock model ID.
		modelID = key
		warnings = append(warnings, fmt.Sprintf(
			"model %q is not a known convenience alias — writing it verbatim as a native Bedrock model. "+
				"It may use a different API/path than Chat Completions and fail at runtime; "+
				"verify against the model card if it doesn't work.", key))
	}

	models := map[string]any{modelID: map[string]any{"id": modelID, "name": modelID}}
	var whitelist []string
	for _, discovered := range opts.ModelCatalog {
		if support := o.SupportsModel(discovered); support.Supported {
			models[discovered.ID] = map[string]any{"id": discovered.ID, "name": discovered.ID}
			whitelist = append(whitelist, discovered.ID)
		}
	}
	if w := catalogUnavailableWarning(opts.ModelCatalog, modelID, opts.Region, "it remains configured for explicit use", o, opts.RefreshedSources); w != "" {
		warnings = append(warnings, w)
	}

	keys := map[string]any{
		"model": bedrockProviderID + "/" + modelID,
		"provider": map[string]any{
			bedrockProviderID: map[string]any{
				"options": map[string]any{
					"region": opts.Region,
				},
				"models":    models,
				"whitelist": whitelist,
			},
		},
	}

	blockMap, err := juggernautAuthBlock(opts, opts.Region)
	if err != nil {
		return ConfigPlan{}, err
	}
	keys["juggernaut"] = blockMap

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: o.NativeManagedKeys(),
		Warnings:    warnings,
	}, nil
}

func (o opencode) SupportsModel(model CatalogModel) ModelSupport {
	return o.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		if m.Source == "mantle" {
			return ModelSupport{Reason: "OpenCode's Bedrock provider no longer uses Mantle (native only)"}
		}
		return ModelSupport{Supported: true, Reason: "native Bedrock Chat-compatible candidate"}
	})
}
