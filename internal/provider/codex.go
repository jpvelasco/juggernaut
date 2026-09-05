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
// Routing note: Juggernaut v6 dropped Mantle. v5 wrote the CUSTOM
// "amazon-bedrock" provider (model_providers.amazon-bedrock) whose base_url
// pointed at bedrock-mantle; those configs are migrated on apply (see
// StripLegacyConfig / CleanLegacy). v6+ writes the built-in amazon-bedrock-runtime.
type codex struct {
	BaseProvider
}

// CodexBedrockRuntimeProviderID is the built-in Codex provider that routes to
// the bedrock-runtime Responses endpoint (SigV4 / API key). Codex >= 0.153.4
// ships it; Juggernaut requires that minimum (see cmd's codex version gate).
const CodexBedrockRuntimeProviderID = "amazon-bedrock-runtime"

// CodexLegacyProviderID is the custom provider id Juggernaut v5 (Mantle era)
// wrote. It still resolves to bedrock-mantle, which is dead in v6 — configs
// still pinned to it are migrated to CodexBedrockRuntimeProviderID on apply.
const CodexLegacyProviderID = "amazon-bedrock"

// ConfigPath is ~/.codex/config.toml (user) or ./.codex/config.toml (project).
func (c codex) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".codex", "config.toml"), nil
	}
	return safepath.JoinUnder(home, ".codex", "config.toml")
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
// Bedrock from Codex. Sourced from live foundation + inference-profile
// verification (2026-08-30).
//
// The GPT-5.6 family is INFERENCE_PROFILE-only on native Bedrock: the base
// foundation ID (openai.gpt-5.6-sol) is listed and ACTIVE but returns 400
// "on-demand throughput isn't supported" when invoked. Only the inference
// profile IDs (global.openai.gpt-5.6-sol, us.openai.gpt-5.6-sol) are callable.
// Juggernaut therefore writes the global. profile ID as the default.
type codexBedrockModel struct {
	ModelID string   // Bedrock inference-profile model ID, e.g. "global.openai.gpt-5.6-sol"
	Regions []string // regions where available (informational)
}

// codexModels maps a friendly key to its verified native facts.
var codexModels = map[string]codexBedrockModel{
	"sol": {
		ModelID: "global.openai.gpt-5.6-sol",
		Regions: []string{"us-east-1", "us-east-2", "us-west-2"},
	},
	"terra": {
		ModelID: "global.openai.gpt-5.6-terra",
		Regions: []string{"us-east-1", "us-east-2", "us-west-2"},
	},
	"luna": {
		ModelID: "global.openai.gpt-5.6-luna",
		Regions: []string{"us-east-1", "us-east-2", "us-west-2"},
	},
	// NOTE: gpt-oss is intentionally absent. Current Codex is Responses-API-only
	// (it rejects `wire_api = "chat"` at config load), but gpt-oss on Bedrock
	// serves only Chat Completions, so Codex cannot reach it.
	// gpt-oss remains available via OpenCode (which speaks Chat Completions).
}

// codexModel resolves a friendly key (sol/terra/luna) or a raw Bedrock model ID
// (global.openai.gpt-5.6-sol, openai.gpt-5.6-sol, or gpt-5.6-sol) to a
// codexBedrockModel. Unknown families return the zero value with ok=false.
func codexModel(key string) (codexBedrockModel, bool) {
	m, ok := codexModels[key]
	if ok {
		return m, true
	}
	// Accept the raw forms the user may pass:
	//   "gpt-5.6-sol"            → canonical global profile ID
	//   "openai.gpt-5.6-sol"     → same (base ID form, auto-upgraded to global.)
	//   "global.openai.gpt-5.6-sol" → already canonical
	// The GPT-5.6 family is INFERENCE_PROFILE-only, so any non-global form is
	// normalized to the global. profile ID.
	switch {
	case strings.HasPrefix(key, "global.openai.gpt-5.6-"):
		return codexBedrockModel{ModelID: key}, true
	case strings.HasPrefix(key, "openai.gpt-5.6-"):
		return codexBedrockModel{ModelID: "global." + key}, true
	case strings.HasPrefix(key, "gpt-5.6-"):
		return codexBedrockModel{ModelID: "global.openai." + key}, true
	}
	return m, ok
}

// codexDefaultModel is sol — the flagship for GPT-5.6.
func codexDefaultModel() string { return "sol" }

// BuildConfig writes Codex's config.toml using the built-in
// amazon-bedrock-runtime provider. This provider routes to the bedrock-runtime
// Responses endpoint and ships a model catalog with native Bedrock inference
// profile IDs (global.openai.gpt-5.6-sol etc.), eliminating the "Model metadata
// not found" warning. It handles SigV4 itself, so no base_url or env_key is
// written — only the [aws] region sub-table is needed.
//
// The GPT-5.6 family is INFERENCE_PROFILE-only on native Bedrock: the base
// foundation ID (openai.gpt-5.6-sol) returns 400 "on-demand throughput isn't
// supported" when invoked. Juggernaut writes the global. profile ID so the
// config is actually callable.
//
// The built-in provider uses the standard AWS credential chain:
// AWS_BEARER_TOKEN_BEDROCK env var (injected by Juggernaut's launch wrapper) or
// standard AWS SDK credentials (SSO, IAM roles, credential_process).
//
// Config shape:
//
//	model = "global.openai.gpt-5.6-sol"
//	model_provider = "amazon-bedrock-runtime"
//	[model_providers.amazon-bedrock-runtime.aws]
//	  region = "us-west-2"
func (c codex) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	key := opts.Model
	if key == "" {
		key = codexDefaultModel()
	}
	m, ok := codexModel(key)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Codex model %q (expected a discovered openai.gpt-5.6-* model: sol, terra, luna)", key)
	}

	return buildWithRegionWarnings(opts, m.ModelID, m.Regions, ": ", c, func(region string) (ConfigPlan, error) {
		keys := map[string]any{
			"model":          m.ModelID,
			"model_provider": CodexBedrockRuntimeProviderID,
			"model_providers": map[string]any{
				CodexBedrockRuntimeProviderID: map[string]any{
					"aws": map[string]any{
						"region": region,
					},
				},
			},
		}

		// Persist the juggernaut block so the launch wrapper can read the auth mode
		// at runtime (IAM → no token needed; API key → inject bearer token).
		blockMap, err := juggernautAuthBlock(opts, region)
		if err != nil {
			return ConfigPlan{}, err
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
		if m.Source == "mantle" {
			return ModelSupport{Reason: "Codex no longer uses Mantle (native only)"}
		}
		// The GPT-5.6 family may appear in the catalog as either the base
		// foundation ID (openai.gpt-5.6-*) or an inference-profile ID
		// (global.openai.gpt-5.6-* / us.openai.gpt-5.6-*). Accept both; only
		// the profile IDs are actually callable on-demand.
		if !strings.Contains(m.ID, "gpt-5.6") {
			return ModelSupport{Reason: "Codex's built-in Bedrock provider supports OpenAI GPT-5.6 models (sol, terra, luna)"}
		}
		return ModelSupport{Supported: true, Reason: "Codex Responses model"}
	})
}

// CleanLegacy deletes the v5 [model_providers.amazon-bedrock] table when
// migrating an existing Codex config to the built-in amazon-bedrock-runtime
// provider. Deep-merge preserves sibling tables, so without this strip the
// stale Mantle-era table would persist forever and codex would 404 on it (it
// still points at the removed Mantle endpoint). This is Codex-scoped: the
// "provider.amazon-bedrock" table OpenCode uses is a different, still-current
// native provider and is never touched.
func (c codex) CleanLegacy(existing map[string]any) {
	tbl, ok := existing["model_providers"].(map[string]any)
	if !ok {
		return
	}
	delete(tbl, CodexLegacyProviderID)
	if len(tbl) == 0 {
		delete(existing, "model_providers")
	}
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
