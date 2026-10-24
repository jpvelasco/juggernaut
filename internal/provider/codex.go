package provider

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// codex is the OpenAI Codex CLI provider (config at ~/.codex/config.toml, TOML).
//
// Codex routes through its built-in amazon-bedrock-runtime provider, which
// speaks the bedrock-runtime Responses endpoint with SigV4 (no bearer token in
// IAM mode) and ships a native inference-profile model catalog (no "Model
// metadata not found" warnings or /model 404s). The config is minimal: model,
// model_provider, and an [aws] sub-table for region. Juggernaut's launch
// wrapper injects AWS_REGION (and AWS_BEARER_TOKEN_BEDROCK for API-key auth).
// Unlike Claude Code it has NO "use bedrock" env var — routing lives entirely
// in the config file.
//
// Opt-in mantle uses the built-in amazon-bedrock provider with a bare model
// ID. The auth endpoint record distinguishes it from v5's custom provider
// with the same ID; old overrides are migrated by CleanLegacy.
type codex struct {
	BaseProvider
}

// CodexBedrockRuntimeProviderID is the built-in Codex provider that routes to
// the bedrock-runtime Responses endpoint (SigV4 / API key). Codex >= 0.153.4
// ships it; Juggernaut requires that minimum (see cmd's codex version gate).
const CodexBedrockRuntimeProviderID = "amazon-bedrock-runtime"

// CodexLegacyProviderID is v5's custom provider ID and Codex's built-in mantle
// provider ID. A persisted endpoint=mantle (sidecar or legacy in-file metadata)
// marks a current opt-in config.
const CodexLegacyProviderID = "amazon-bedrock"

// ConfigPath is ~/.codex/config.toml (user) or ./.codex/config.toml (project).
func (c codex) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".codex", "config.toml"), nil
	}
	return safepath.JoinUnder(home, ".codex", "config.toml")
}

// Codex warns on unknown config keys (or rejects them with --strict-config),
// so Juggernaut's auth metadata lives beside config.toml, not inside it.
func (c codex) SidecarPath(home, scope string) (string, error) {
	return configSidecarPath(c, home, scope)
}

// NativeManagedKeys are the top-level config.toml keys Juggernaut owns for Codex.
// OwnsConfig recognizes a Codex config Juggernaut wrote by its Bedrock provider
// selection (model_provider == "amazon-bedrock-runtime" or the legacy v5
// "amazon-bedrock"). A plain user config that merely has a top-level `model`
// is NOT ours — critical so a first-time `apply --cli=codex` over an existing
// Codex config still prompts for auth instead of defaulting to iam.
func (c codex) OwnsConfig(data map[string]any) bool {
	switch data["model_provider"] {
	case CodexBedrockRuntimeProviderID, CodexLegacyProviderID:
		return true
	}
	return false
}

// OwnsManagedValue keeps a user-set root "model" alive through uninstall.
// Codex's model value has no provider prefix to inspect, so ownership is
// record-based: the root model is Juggernaut's while it still equals the
// value the last apply wrote (recorded in the auth sidecar). With no record
// — an install from before records shipped, or a preserved default — any
// value in the ID space Juggernaut writes for Codex (a codexModel-resolvable
// Bedrock inference-profile ID) is removed: the provider entry uninstall
// deletes is what made it routable, so it is dead weight either way, and
// every pre-record plain apply wrote exactly such a value. A foreign value
// (the user's own OpenAI model) survives. Every other managed key is owned
// regardless of value.
func (c codex) OwnsManagedValue(home, scope, key string, value any) bool {
	if key != "model" {
		return true
	}
	model, ok := value.(string)
	if !ok {
		return true
	}
	written := sidecarWrittenModel(c, home, scope)
	if written != "" {
		return model == written
	}
	// No record: remove the pre-record era's own ID space so uninstall
	// leaves no orphaned model behind.
	_, resolves := codexModel(model)
	return resolves
}

func (c codex) NativeManagedKeys() []string {
	return []string{
		"model",
		"model_provider",
		"model_providers",
	}
}

// DeepMergeKeys: model_providers is a nested [model_providers.<id>] table where a
// user may have their own providers; merge only our amazon-bedrock entry.
func (c codex) DeepMergeKeys() []string { return []string{"model_providers"} }

// OwnedSubKeys: uninstall removes only the region leaf we wrote — under the
// current amazon-bedrock-runtime.aws, plus the legacy amazon-bedrock.aws we
// wrote in v5 so uninstall also cleans a half-migrated file. Users may
// configure their own profile or other settings in those sub-tables —
// dot-notation targets the leaf so siblings survive.
func (c codex) OwnedSubKeys() map[string][]string {
	return map[string][]string{"model_providers": {
		"amazon-bedrock-runtime.aws.region",
		"amazon-bedrock.aws.region",
	}}
}

// codexBedrockModel describes one OpenAI-family model reachable through native
// Bedrock from Codex. Sourced from the AWS model cards and the regional
// availability matrix (verified 2026-10-19).
//
// The GPT-6.1, GPT-6, and GPT-5.6 families are INFERENCE_PROFILE-only on
// native Bedrock: the base foundation ID (openai.gpt-6.1-sol) is listed and
// ACTIVE but returns 400 "on-demand throughput isn't supported" when invoked.
// Only the inference profile IDs (global.openai.gpt-6.1-sol,
// us.openai.gpt-6.1-sol) are callable. Juggernaut therefore writes the global.
// profile ID as the default.
type codexBedrockModel struct {
	ModelID string   // Bedrock inference-profile model ID, e.g. "global.openai.gpt-6.1-sol"
	Regions []string // regions where the global profile is available (drives the Iron Fist)
}

// codexGPT6Regions are the regions where the GPT-6 generation's global
// inference profiles are available. GPT-6.1 Sol, GPT-6 Sol, GPT-6 Luna, and
// GPT-6 Astra publish the same set (AWS regional availability matrix,
// verified 2026-10-19). Regions[0] is the Iron Fist fallback — keep us-east-1
// first so an unsupported region deterministically routes to the flagship
// region.
var codexGPT6Regions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	"ca-central-1", "ca-west-1",
	"eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "eu-south-2", "eu-west-1", "eu-west-2", "eu-west-3",
	"ap-east-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-south-2", "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5", "ap-southeast-6", "ap-southeast-7",
	"af-south-1", "il-central-1", "mx-central-1", "sa-east-1",
}

// codexGPT56Regions are the regions where the GPT-5.6 global profiles are
// available (AWS regional availability matrix, verified 2026-10-19). Terra is
// the only GPT-5.6 model on the roster — there is no GPT-6 Terra.
var codexGPT56Regions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	"ca-central-1",
	"eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "eu-south-2", "eu-west-1", "eu-west-2", "eu-west-3",
	"ap-east-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-south-2", "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5", "ap-southeast-7",
	"il-central-1", "me-central-1", "me-south-1", "sa-east-1",
}

// codexModels maps a friendly key to its verified native facts. Sol, Luna, and
// Astra track the current GPT-6 generation; Terra has no GPT-6 successor, so
// it stays on the last Terra generation (GPT-5.6).
var codexModels = map[string]codexBedrockModel{
	"sol": {
		ModelID: "global.openai.gpt-6.1-sol",
		Regions: codexGPT6Regions,
	},
	"luna": {
		ModelID: "global.openai.gpt-6-luna",
		Regions: codexGPT6Regions,
	},
	"astra": {
		ModelID: "global.openai.gpt-6-astra",
		Regions: codexGPT6Regions,
	},
	"terra": {
		ModelID: "global.openai.gpt-5.6-terra",
		Regions: codexGPT56Regions,
	},
	// NOTE: gpt-oss is intentionally absent. Current Codex is Responses-API-only
	// (it rejects `wire_api = "chat"` at config load), but gpt-oss on Bedrock
	// serves only Chat Completions, so Codex cannot reach it.
	// gpt-oss remains available via OpenCode (which speaks Chat Completions).
}

// codexModel resolves a friendly key (sol/luna/astra/terra) or a raw Bedrock
// model ID from the GPT-6.1, GPT-6, or GPT-5.6 family (global.openai.gpt-6.1-sol,
// openai.gpt-6-astra, or gpt-5.6-terra) to a codexBedrockModel. Unknown
// families return the zero value with ok=false.
func codexModel(key string) (codexBedrockModel, bool) {
	m, ok := codexModels[key]
	if ok {
		return m, true
	}
	// Accept the raw forms the user may pass, for every supported family:
	//   "gpt-6.1-sol"               → canonical global profile ID
	//   "openai.gpt-6.1-sol"        → same (base ID form, auto-upgraded to global.)
	//   "global.openai.gpt-6.1-sol" → already canonical
	// The GPT-6.1, GPT-6, and GPT-5.6 families are INFERENCE_PROFILE-only, so
	// any non-global form is normalized to the global. profile ID.
	for _, family := range []string{"gpt-6.1-", "gpt-6-", "gpt-5.6-"} {
		switch {
		case strings.HasPrefix(key, "global.openai."+family):
			return codexBedrockModel{ModelID: key}, true
		case strings.HasPrefix(key, "openai."+family):
			return codexBedrockModel{ModelID: "global." + key}, true
		case strings.HasPrefix(key, family):
			return codexBedrockModel{ModelID: "global.openai." + key}, true
		}
	}
	return m, ok
}

// codexDefaultModel is sol — the flagship, now on GPT-6.1.
func codexDefaultModel() string { return "sol" }

// BuildConfig writes Codex's config.toml using the built-in
// amazon-bedrock-runtime provider. This provider routes to the bedrock-runtime
// Responses endpoint and ships a model catalog with native Bedrock inference
// profile IDs (global.openai.gpt-6.1-sol etc.), eliminating the "Model metadata
// not found" warning. It handles SigV4 itself, so no base_url or env_key is
// written — only the [aws] region sub-table is needed.
//
// The GPT-6.1, GPT-6, and GPT-5.6 families are INFERENCE_PROFILE-only on
// native Bedrock: the base foundation ID (openai.gpt-6.1-sol) returns 400
// "on-demand throughput isn't supported" when invoked. Juggernaut writes the
// global. profile ID so the config is actually callable.
//
// The built-in provider uses the standard AWS credential chain:
// AWS_BEARER_TOKEN_BEDROCK env var (injected by Juggernaut's launch wrapper) or
// standard AWS SDK credentials (SSO, IAM roles, credential_process).
//
// Config shape:
//
//	model = "global.openai.gpt-6.1-sol"
//	model_provider = "amazon-bedrock-runtime"
//	[model_providers.amazon-bedrock-runtime.aws]
//	  region = "us-west-2"
func (c codex) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	preserved, preserve := codexPreservedModel(opts)
	if opts.Endpoint == bedrock.EndpointMantle {
		id, err := openAIMantleModel(opts.Model)
		if err != nil {
			return ConfigPlan{}, err
		}
		if preserve {
			// The preserved model is mantle-shaped by construction; resolve
			// it so the plan carries its region facts.
			id, err = openAIMantleModel(preserved)
			if err != nil {
				return ConfigPlan{}, err
			}
		}
		plan, err := c.codexPlan(opts, codexBedrockModel{ModelID: id, Regions: openAIMantleRegions[id]}, CodexLegacyProviderID)
		if err != nil {
			return ConfigPlan{}, err
		}
		if preserve {
			preserveExistingDefault(&plan, preserved)
		}
		return plan, nil
	}
	key := opts.Model
	if preserve {
		// Region facts come from the preserved model; the file keeps the
		// user's exact value because the model write is stripped below.
		key = preserved
	} else if key == "" {
		key = codexDefaultModel()
	}
	m, ok := codexModel(key)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Codex model %q (expected a discovered openai.gpt-6.1-*, openai.gpt-6-*, or openai.gpt-5.6-* model: sol, luna, astra, terra)", key)
	}

	plan, err := c.codexPlan(opts, m, CodexBedrockRuntimeProviderID)
	if err != nil {
		return ConfigPlan{}, err
	}
	if preserve {
		preserveExistingDefault(&plan, preserved)
	}
	return plan, nil
}

// codexPreservedModel returns the existing root "model" a plain apply must
// preserve, and whether one applies. Codex's model is bound to the
// model_provider Juggernaut pins, so — unlike OpenCode's provider-prefixed
// root — only a model Codex can actually serve through that provider is
// worth preserving: one codexModel resolves (the GPT-6.x/5.6 families
// Juggernaut itself writes, in any spelling). A foreign default (the user's
// own OpenAI model) would 404 through Bedrock routing, so it falls back to
// the working convenience default. Cross-endpoint shapes are stale: runtime
// serves only the global. inference profiles, mantle only the bare openai.*
// IDs — and a config still routed through the v5 custom provider is a
// legacy migration target whose model is Juggernaut's own stale write (the
// runtime path only: mantle reuses that provider id legitimately).
func codexPreservedModel(opts Options) (string, bool) {
	if opts.Model != "" {
		return "", false
	}
	existing, ok := opts.Existing["model"].(string)
	if !ok || existing == "" {
		return "", false
	}
	if _, resolves := codexModel(existing); !resolves {
		return "", false
	}
	if opts.Endpoint == bedrock.EndpointMantle {
		if strings.HasPrefix(existing, "global.") {
			return "", false
		}
	} else {
		if strings.HasPrefix(existing, "openai.") {
			return "", false
		}
		if existingProvider, _ := opts.Existing["model_provider"].(string); existingProvider == CodexLegacyProviderID {
			return "", false
		}
	}
	return existing, true
}

func (c codex) codexPlan(opts Options, m codexBedrockModel, providerID string) (ConfigPlan, error) {
	var catalog CatalogProvider = c
	if opts.Endpoint == bedrock.EndpointMantle {
		// Runtime discovery cannot prove mantle availability.
		catalog = nil
	}
	return buildWithRegionWarnings(opts, m.ModelID, m.Regions, ": ", catalog, func(region string) (ConfigPlan, error) {
		keys := map[string]any{
			"model":          m.ModelID,
			"model_provider": providerID,
			"model_providers": map[string]any{
				providerID: map[string]any{
					"aws": map[string]any{
						"region": region,
					},
				},
			},
		}

		return ConfigPlan{
			Keys:           keys,
			ManagedKeys:    c.NativeManagedKeys(),
			ResolvedRegion: region,
		}, nil
	})
}

func (c codex) SupportsModel(model CatalogModel) ModelSupport {
	return c.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		if m.Source == "mantle" {
			return ModelSupport{Reason: legacyMantleCacheReason}
		}
		if !RuntimeSupported(m.ID, m.Source) {
			return ModelSupport{Reason: mantleOnlyReason}
		}
		// The GPT-6.1, GPT-6, and GPT-5.6 families may appear in the catalog as
		// either the base foundation ID (openai.gpt-6.1-*) or an inference-profile
		// ID (global.openai.gpt-6.1-* / us.openai.gpt-6.1-*). Accept all of them;
		// only the profile IDs are actually callable on-demand.
		if !strings.Contains(m.ID, "gpt-6") && !strings.Contains(m.ID, "gpt-5.6") {
			return ModelSupport{Reason: "Codex's built-in Bedrock provider supports OpenAI GPT-6.x and GPT-5.6 models (sol, luna, astra, terra)"}
		}
		return ModelSupport{Supported: true, Reason: "Codex Responses model"}
	})
}

// CleanLegacy deletes the v5 [model_providers.amazon-bedrock] table when
// migrating an existing Codex config to the built-in amazon-bedrock-runtime
// provider. Deep-merge preserves sibling tables, so without this strip the
// stale Mantle-era table would persist forever and codex would 404 on it (it
// still points at the Mantle endpoint Juggernaut v6 no longer routes through).
// This is Codex-scoped: the
// "provider.amazon-bedrock" table OpenCode uses is a different, still-current
// native provider and is never touched.
func (c codex) CleanLegacy(existing map[string]any, endpoint string) {
	tbl, ok := existing["model_providers"].(map[string]any)
	if !ok {
		return
	}
	// Cleanup runs after merge. The target endpoint comes from apply, not a
	// metadata key in the vendor file. Mantle reuses this ID: keep its aws table.
	if endpoint == bedrock.EndpointMantle {
		entry, _ := tbl[CodexLegacyProviderID].(map[string]any)
		for _, key := range []string{"name", "base_url", "env_key", "wire_api", "requires_openai_auth", "http_headers", "env_http_headers", "auth"} {
			delete(entry, key)
		}
		return
	}
	delete(tbl, CodexLegacyProviderID)
	if len(tbl) == 0 {
		delete(existing, "model_providers")
	}
}

func (c codex) LaunchSpec() LaunchSpec {
	// Codex has no "use bedrock" flag — routing lives in config.toml.
	// Token injection is decided at launch time from the sidecar auth mode:
	// API key → inject AWS_BEARER_TOKEN_BEDROCK; IAM → SDK credential chain.
	return LaunchSpec{
		PrepareArgs: prepareCodexLaunch,
		TokenEnvVar: authmode.BedrockAuthEnvName,
		NeedsToken:  false, // auth mode in juggernaut block decides at launch
	}
}
