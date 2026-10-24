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
// ~/.config/opencode/opencode.json). OpenCode is model-agnostic, so it routes
// via the built-in amazon-bedrock provider by default (options.region + live-discovered
// models + whitelist). Unlike Codex it has no "use bedrock" env var — routing
// lives in the config; the bearer token / IAM SigV4 chain is used at runtime.
//
// OpenCode targets any native Bedrock model compatible with Chat Completions.
// Juggernaut injects the account's cached live inventory (foundation + profile)
// instead of maintaining a runtime roster. Opt-in mantle uses V2's native
// mantle Responses package and the shared verified OpenAI model/region roster.
type opencode struct {
	BaseProvider
}

// bedrockProviderID is the provider key written into opencode.json.
const bedrockProviderID = "amazon-bedrock"

const openCodeMantleProviderID = "amazon-bedrock-mantle"

// openCodeLegacyMantleProviderID is the pre-v6 (Mantle-era) provider key
// Juggernaut wrote into opencode.json. A root model still pointing at it is
// a v5 leftover: no v6 apply keeps that entry, so the model is always
// Juggernaut's to replace (apply) or remove (uninstall).
const openCodeLegacyMantleProviderID = "bedrock-mantle"

// ConfigPath is ~/.config/opencode/opencode.json (user) or ./opencode.json
// (project).
func (o opencode) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", "opencode.json"), nil
	}
	return safepath.JoinUnder(home, ".config", "opencode", "opencode.json")
}

// NativeManagedKeys are the top-level opencode.json keys Juggernaut owns.
// The juggernaut auth-mode block is NOT one of them: OpenCode validates
// opencode.json against a strict schema (Config.additionalProperties: false)
// and rejects unknown top-level keys, so Juggernaut keeps the block in a
// sidecar file next to the config (SidecarAuthSource).
func (o opencode) NativeManagedKeys() []string {
	return []string{"model", "provider", "providers"}
}

// SidecarPath is the .juggernaut.json file next to the opencode config for
// the given scope.
func (o opencode) SidecarPath(home, scope string) (string, error) {
	return configSidecarPath(o, home, scope)
}

// DeepMergeKeys: "provider" is a nested map where a user may have their own
// providers (anthropic, openai, …); merge only our amazon-bedrock entry.
func (o opencode) DeepMergeKeys() []string { return []string{"provider", "providers"} }

// OwnedSubKeys: uninstall removes only our runtime and mantle provider entries
// (the model leaf is removed whole).
func (o opencode) OwnedSubKeys() map[string][]string {
	return map[string][]string{"provider": {bedrockProviderID}, "providers": {openCodeMantleProviderID}}
}

// OwnsConfig recognizes a config Juggernaut wrote by our amazon-bedrock provider
// under the "provider" map — NOT a plain user opencode.json that merely has a
// "provider" block for other vendors or a "model" key.
func (o opencode) OwnsConfig(data map[string]any) bool {
	if providers, ok := data["providers"].(map[string]any); ok {
		if _, found := providers[openCodeMantleProviderID]; found {
			return true
		}
	}
	prov, ok := data["provider"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = prov[bedrockProviderID]
	return ok
}

// OwnsManagedValue keeps a user's root "model" default alive through
// uninstall. The runtime prefix alone can't decide ownership — amazon-bedrock
// is OpenCode's own built-in provider id, so a native Bedrock default
// (something Juggernaut never wrote) shares it with Juggernaut's writes.
// A model is Juggernaut's when it still equals the value the last apply
// wrote (recorded in the auth sidecar), when it points at a
// Juggernaut-exclusive provider entry (the mantle id and its v5 leftover,
// which no native setup uses), or — with no record, i.e. an install from
// before records shipped or a preserved default — when it is exactly the
// seeded convenience default Juggernaut wrote for every pre-record plain
// apply; removing it restores the pre-Juggernaut state instead of leaving
// an orphaned default behind. Everything else, including a native
// amazon-bedrock default and a preserved or later-edited value, is the
// user's. Every other managed key is owned regardless of value.
func (o opencode) OwnsManagedValue(home, scope, key string, value any) bool {
	if key != "model" {
		return true
	}
	model, ok := value.(string)
	if !ok {
		return true
	}
	if strings.HasPrefix(model, openCodeMantleProviderID+"/") ||
		strings.HasPrefix(model, openCodeLegacyMantleProviderID+"/") {
		return true
	}
	written := sidecarWrittenModel(o, home, scope)
	if written != "" {
		return model == written
	}
	// No record: a pre-record install (records shipped with the preserve
	// behavior) or a preserved default. The one value that era's plain
	// applies provably wrote is the seeded convenience default.
	return model == bedrockProviderID+"/"+opencodeModelAliases[opencodeDefaultModel()]
}

func (o opencode) LaunchSpec() LaunchSpec {
	// OpenCode routes via config; the built-in amazon-bedrock provider uses
	// the AWS credential chain (SigV4 or AWS_BEARER_TOKEN_BEDROCK), so no static
	// enable flag. Token injection is decided at launch from juggernaut.auth,
	// read from the .juggernaut.json sidecar (legacy: the in-file block).
	return LaunchSpec{
		TokenEnvVar: authmode.BedrockAuthEnvName,
		NeedsToken:  false, // auth mode in juggernaut block decides at launch
	}
}

// opencodeModelAliases are convenience spellings, not an availability roster.
// The live account/region catalog remains authoritative. IDs are native Bedrock
// foundation IDs (2026-08-29) — except the grok-4.6/4.3 and glm-5.3 entries,
// which use the global. cross-Region inference profile: the bedrock-runtime
// endpoint 400s on the bare xai.grok-4.6 foundation ID (verified against the
// Grok 4.6 model card; the Grok provider applies the same normalization), and
// glm-5.3 is pinned to its global profile deliberately (cross-region serving).
var opencodeModelAliases = map[string]string{
	"gpt-oss-120b":  "openai.gpt-oss-120b-1:0",
	"gpt-oss-20b":   "openai.gpt-oss-20b-1:0",
	"glm-4.7":       "zai.glm-4.7",
	"glm-5":         "zai.glm-5",
	"glm-5.3":       "global.zai.glm-5.3",
	"kimi-k2.5":     "moonshotai.kimi-k2.5",
	"deepseek-v3.2": "deepseek.v3.2",
	"qwen3-coder":   "qwen.qwen3-coder-480b-a35b-v1:0",
	"grok-4.6":      "global.xai.grok-4.6",
	"grok-4.3":      "global.xai.grok-4.6", // deprecated: kept for backwards compat; prefer grok-4.6
}

func opencodeDefaultModel() string { return "gpt-oss-120b" }

// BuildConfig writes OpenCode's config: the built-in amazon-bedrock provider
// (options.region + live-discovered models + whitelist, omitted when empty)
// plus a top-level default model "provider_id/model_id". A convenience alias
// resolves to a native Bedrock model ID; any other value is passed through
// verbatim. Every compatible discovered model is added to the provider block
// so OpenCode's model picker can use the account's actual inventory. The
// auth-mode metadata is NOT part of this plan — OpenCode's strict schema
// rejects unknown top-level keys, so Juggernaut writes it to the sidecar
// file (see SidecarAuthSource).
func (o opencode) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	if opts.Endpoint == bedrock.EndpointMantle {
		return o.buildMantleConfig(opts)
	}
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
	// A plain apply preserves an existing default: there is no selected
	// model to vet against the catalog in that case — the table entry is
	// for explicit use only.
	preserved, preserveExisting := preservedDefaultModel(opts)
	if !preserveExisting {
		if w := catalogUnavailableWarning(opts.ModelCatalog, modelID, opts.Region, "it remains configured for explicit use", o, opts.RefreshedSources); w != "" {
			warnings = append(warnings, w)
		}
	}

	providerTable := map[string]any{
		"options": map[string]any{
			"region": opts.Region,
		},
		"models": models,
	}
	// OpenCode's schema requires whitelist to be an array; a nil slice
	// marshals to null and an empty array would hide every model from the
	// picker. Omit the key entirely when discovery found no supported models.
	if len(whitelist) > 0 {
		providerTable["whitelist"] = whitelist
	}

	plan := ConfigPlan{
		Keys: map[string]any{
			"model":    bedrockProviderID + "/" + modelID,
			"provider": map[string]any{bedrockProviderID: providerTable},
		},
		ManagedKeys: []string{"model", "provider"},
		Warnings:    warnings,
	}
	if preserveExisting {
		// OpenCode's root "model" is the CLI's own native session default,
		// not Bedrock routing: a plain apply keeps the user's value instead
		// of pinning Juggernaut's convenience default.
		preserveExistingDefault(&plan, preserved)
	}
	return plan, nil
}

// preserveExistingDefault strips the root model write from a plan whose apply
// preserves the CLI's existing default (see preservedDefaultModel), leaves the
// remaining managed keys, and records the preservation notice.
func preserveExistingDefault(plan *ConfigPlan, preserved string) {
	delete(plan.Keys, "model")
	kept := make([]string, 0, len(plan.ManagedKeys))
	for _, k := range plan.ManagedKeys {
		if k != "model" {
			kept = append(kept, k)
		}
	}
	plan.ManagedKeys = kept
	plan.Warnings = append(plan.Warnings, fmt.Sprintf(
		"existing default model %q preserved; pass --model to set the default", preserved))
}

// preservedDefaultModel returns the existing root default model an apply
// without --model must preserve, and whether one applies. OpenCode's root
// "model" is the native session default for every agent that does not select
// its own, so a plain apply keeps it instead of pinning Juggernaut's
// convenience default. A model pointing at the OpenCode provider entry this
// apply removes (the other endpoint's) is not preserved — it would dangle
// once the endpoint switch cleans the inactive entry.
func preservedDefaultModel(opts Options) (string, bool) {
	if opts.Model != "" {
		return "", false
	}
	existing, ok := opts.Existing["model"].(string)
	if !ok || existing == "" {
		return "", false
	}
	if isOpenCodeStaleModel(existing, opts.Endpoint) {
		return "", false
	}
	return existing, true
}

// isOpenCodeStaleModel reports whether a root model references an OpenCode
// provider entry no apply to the given endpoint keeps — the other endpoint's
// current id or the v5 Mantle-era leftover. Endpoint switches delete the
// inactive entry (CleanEndpoint) and migrations drop the v5 table, which
// would leave the model dangling; foreign and same-endpoint models are never
// stale.
func isOpenCodeStaleModel(model, endpoint string) bool {
	if strings.HasPrefix(model, openCodeLegacyMantleProviderID+"/") {
		return true
	}
	if endpoint == bedrock.EndpointMantle {
		return strings.HasPrefix(model, bedrockProviderID+"/")
	}
	return strings.HasPrefix(model, openCodeMantleProviderID+"/")
}

func (o opencode) SupportsModel(model CatalogModel) ModelSupport {
	return o.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		if m.Source == "mantle" {
			return ModelSupport{Reason: legacyMantleCacheReason}
		}
		if !RuntimeSupported(m.ID, m.Source) {
			return ModelSupport{Reason: mantleOnlyReason}
		}
		return ModelSupport{Supported: true, Reason: "native Bedrock Chat-compatible candidate"}
	})
}
