package provider

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// grok is the OFFICIAL xAI Grok CLI provider (grok / grok.exe, config TOML at
// ~/.grok/config.toml). Juggernaut routes to Grok on native Bedrock
// (bedrock-runtime /openai/v1) by default, or — opt-in — through the parallel
// bedrock-mantle endpoint. Both write a [model.bedrock-grok] block selected via
// [models].default. API-key apply also writes an [auth] block whose
// auth_provider_command runs `juggernaut auth-token`; IAM apply omits that
// command so Grok does not demand a keychain bearer token.
//
// Why the [auth] block (not env_key): Grok's per-model credential order is
// api_key → env_key → XAI_API_KEY, but a custom [model.*] block does NOT satisfy
// Grok's SESSION auth — with only env_key set, Grok still runs its interactive
// browser/OIDC login and prompts the user. The documented mechanism that
// REPLACES login is [auth] auth_provider_command: Grok runs it, reads a token
// from stdout, stores it in ~/.grok/auth.json, and auto-refreshes. Verified
// against the official xAI Grok CLI docs (README "External Auth Provider").
type grok struct {
	BaseProvider
}

// grokModelName is the [model.<name>] key + [models].default value we write.
const grokModelName = "bedrock-grok"

// grokAuthCommand is the auth_provider_command Grok runs (via sh -c) to fetch
// the bearer token. It maps to cmd/auth_token.go, which prints the keychain
// token as {"access_token":...,"expires_in":...}.
const grokAuthCommand = "juggernaut auth-token"

// grokBedrockModel describes one xAI model reachable through Bedrock. On
// bedrock-runtime every current Grok model is inference-profile-only (requests
// name a us. geo or global. cross-Region profile); on bedrock-mantle the same
// models are served as bare foundation IDs, in-region only.
//
// ContextWindow is the AWS-published window shared between input and output
// (model cards, checked 2026-10-19); the Grok client uses it for compaction
// thresholds, so an inflated value invites server-side errors.
// USRegions/GlobalRegions are the commercial source regions each profile type
// is callable from per the card. us-east-1 stays first in both — it is the
// Iron Fist fallback and the region Juggernaut's own defaults target.
// MantleRegions is the verified in-region availability on bedrock-mantle (nil
// = the model is not served there).
type grokBedrockModel struct {
	ContextWindow int
	USRegions     []string
	GlobalRegions []string
	MantleRegions []string
}

// grokModels maps a bare Bedrock model ID to its documented facts. Unknown
// xai.grok-* families remain selectable verbatim on runtime; they just get no
// context_window or region enforcement. Mantle rejects unknown models because
// every mantle config pins one region, and we only write a region verified to
// serve that model.
var grokModels = map[string]grokBedrockModel{
	"xai.grok-4.7": {
		ContextWindow: 524288,
		USRegions:     []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "ca-west-1"},
		GlobalRegions: []string{
			"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1",
			"eu-north-1", "eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-central-2", "eu-south-1", "eu-south-2",
			"ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-south-2", "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5", "ap-southeast-7", "ap-east-2",
			"me-central-1", "il-central-1", "sa-east-1",
		},
		// Grok 4.7 is runtime-only; the mantle endpoint has no entry for it.
	},
	"xai.grok-4.6": {
		ContextWindow: 500000,
		USRegions:     []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "ca-west-1"},
		GlobalRegions: []string{
			"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1",
			"eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "eu-south-2", "eu-west-1", "eu-west-2", "eu-west-3",
			"ap-east-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-south-2", "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5", "ap-southeast-7",
			"il-central-1", "me-central-1", "me-south-1", "sa-east-1",
		},
		MantleRegions: []string{"us-west-2", "us-gov-east-1"},
	},
	// Grok 4.3 is mantle-only (the runtime endpoint has no entry for it), so it
	// has no CRIS regions; the runtime path rejects it up front.
	"xai.grok-4.3": {
		ContextWindow: 1000000,
		MantleRegions: []string{"us-west-2", "us-east-1", "us-east-2", "us-gov-west-1"},
	},
}

// grokDefaultModelID is the Grok model Juggernaut pins by default: the global.
// inference-profile ID (the bare foundation ID xai.grok-4.7 400s on the
// bedrock-runtime endpoint, which only serves CRIS profile IDs).
const grokDefaultModelID = "global.xai.grok-4.7"

// grokMantleDefaultModelID is the Grok model Juggernaut pins when the mantle
// endpoint is selected: the bare foundation ID. Mantle serves no inference
// profiles, and the runtime default (4.7) is not served there.
const grokMantleDefaultModelID = "xai.grok-4.6"

// grokModelFacts resolves a model ID under any form (bare foundation ID,
// global./us. profile) to its documented facts.
func grokModelFacts(modelID string) (grokBedrockModel, bool) {
	facts, ok := grokModels[bedrock.StripRegionPrefix(modelID)]
	return facts, ok
}

// normalizeGrokModelID resolves the model the user passed into a CRIS
// inference-profile ID the bedrock-runtime endpoint actually accepts. The
// runtime 400s on bare foundation IDs, so every bare form is normalized to a
// global. profile ID; an explicit regional profile (global.*/us.*) is kept
// verbatim so a user's us. pin stays us.
func normalizeGrokModelID(modelID string) (string, bool) {
	if modelID == "" {
		return grokDefaultModelID, true
	}
	if bare := bedrock.StripRegionPrefix(modelID); bare != modelID {
		// Already a regional profile ID — keep it (global./us./…).
		if strings.HasPrefix(bare, "xai.grok-") {
			return modelID, true
		}
		return "", false
	}
	switch {
	case strings.HasPrefix(modelID, "xai.grok-"):
		return "global." + modelID, true
	case strings.HasPrefix(modelID, "grok-"):
		return "global.xai." + modelID, true
	}
	return "", false
}

// normalizeGrokMantleModelID resolves the model for the mantle endpoint. Mantle
// serves bare foundation IDs only (no cross-region inference) and each model in
// its own regions; anything without verified mantle data is rejected rather
// than written into a config that fails at request time.
func normalizeGrokMantleModelID(modelID string) (string, error) {
	if modelID == "" {
		modelID = grokMantleDefaultModelID
	}
	if bare := bedrock.StripRegionPrefix(modelID); bare != modelID {
		bare = bareModelID(bare)
		if facts, ok := grokModels[bare]; ok && len(facts.MantleRegions) > 0 {
			return "", fmt.Errorf(
				"%q is a cross-region inference profile; the mantle endpoint requires the bare ID %q",
				modelID, bare)
		}
		return "", fmt.Errorf(
			"%q is a cross-region inference profile; the mantle endpoint requires a bare xai.grok-* ID",
			modelID)
	}
	if strings.HasPrefix(modelID, "grok-") {
		modelID = "xai." + modelID
	}
	modelID = bareModelID(modelID)
	facts, ok := grokModels[modelID]
	if !ok {
		return "", fmt.Errorf(
			"unknown Grok mantle model %q; mantle supports xai.grok-4.6 (default) and xai.grok-4.3",
			modelID)
	}
	if len(facts.MantleRegions) == 0 {
		return "", fmt.Errorf(
			"%s is not served on the mantle endpoint; use --endpoint=runtime (or --model=%s)",
			modelID, grokMantleDefaultModelID)
	}
	return modelID, nil
}

// MantleRegions implements MantleRegionSource: the verified mantle regions for
// the selected model (default xai.grok-4.6). Invalid or non-mantle models
// return nil so callers fall back to the endpoint-wide list; BuildConfig
// rejects them with an actionable error.
func (g grok) MantleRegions(modelID string) []string {
	mantleID, err := normalizeGrokMantleModelID(modelID)
	if err != nil {
		return nil
	}
	return grokModels[mantleID].MantleRegions
}

// ConfigPath is ALWAYS ~/.grok/config.toml. Grok's project-scoped
// .grok/config.toml only accepts mcp_servers/plugins/permission — model config
// is user-scoped — so both scopes resolve to the user config.
func (g grok) ConfigPath(home, _ string) (string, error) {
	return safepath.JoinUnder(home, ".grok", "config.toml")
}

// NativeManagedKeys are the top-level config.toml keys Juggernaut owns: the
// [model] and [models] tables (nested), plus the [auth] table.
func (g grok) NativeManagedKeys() []string {
	return []string{"model", "models", "auth", "juggernaut"}
}

// DeepMergeKeys: [model.<name>], [models], and [auth] are nested tables that may
// hold a user's own entries (model profiles, default selection, or their own
// OIDC/auth-provider config); merge only our own sub-keys, preserving theirs.
func (g grok) DeepMergeKeys() []string { return []string{"model", "models", "auth"} }

// OwnedSubKeys: uninstall removes ONLY our sub-keys — the bedrock-grok model
// profile, the models.default pointer, and the two [auth] keys we set —
// preserving a user's other model profiles and any auth settings of their own.
// Note: apply sets models.default = bedrock-grok (that IS the point) and
// uninstall deletes that pointer rather than restoring the prior value (not
// persisted); Grok then falls back to its built-in default.
func (g grok) OwnedSubKeys() map[string][]string {
	return map[string][]string{
		"model":  {grokModelName},
		"models": {"default"},
		"auth":   {"auth_provider_command", "auth_provider_label"},
	}
}

// OwnsConfig recognizes a config Juggernaut wrote by our named model block
// [model.bedrock-grok] — NOT a user's own model profiles.
func (g grok) OwnsConfig(data map[string]any) bool {
	models, ok := data["model"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = models[grokModelName]
	return ok
}

func (g grok) LaunchSpec() LaunchSpec {
	// Token injection is decided at launch from the stored auth mode:
	// API key → inject AWS_BEARER_TOKEN_BEDROCK (and write auth_provider_command);
	// IAM → AWS credential chain, no keychain token.
	return LaunchSpec{
		TokenEnvVar: authmode.BedrockAuthEnvName,
		NeedsToken:  false, // auth mode in juggernaut block decides at launch
	}
}

// BuildConfig writes a [model.bedrock-grok] block routing the Grok CLI to
// Bedrock, [models].default, a juggernaut.auth block for launch, and — only for
// Bedrock API key auth — an [auth] block whose auth_provider_command supplies
// the keychain bearer token so Grok skips its interactive sign-in.
//
// Runtime (default) targets bedrock-runtime /openai/v1 with a CRIS profile ID;
// the opt-in mantle endpoint targets bedrock-mantle /openai/v1 with a bare
// foundation ID and the Responses wire API.
//
// Deliberately NO env_key on the model block: with env_key set, Grok's session
// still falls through to interactive login. The [auth] block is the documented
// way to replace login entirely.
func (g grok) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	if opts.Endpoint == bedrock.EndpointMantle {
		return g.buildMantleConfig(opts)
	}
	return g.buildRuntimeConfig(opts)
}

// buildRuntimeConfig targets bedrock-runtime. The endpoint serves only CRIS
// profile IDs, so every bare form is normalized to a global. profile ID.
func (g grok) buildRuntimeConfig(opts Options) (ConfigPlan, error) {
	modelID, ok := normalizeGrokModelID(opts.Model)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Grok model %q (expected a discovered xai.grok-* model)", opts.Model)
	}
	// Mantle-only families (Grok 4.3) have no runtime endpoint; refuse rather
	// than write a profile ID that fails at request time.
	if !RuntimeSupported(modelID, "foundation") {
		return ConfigPlan{}, fmt.Errorf(
			"model %q is available only on the bedrock-mantle endpoint; re-run with --endpoint=mantle",
			opts.Model)
	}

	// Route to a region that can actually invoke the selected profile rather
	// than writing a config that can't reach the model. Global and US profiles
	// have different source-region sets; an unknown xai.grok-* model has no
	// verified data and passes through unconstrained.
	facts, known := grokModelFacts(modelID)
	var modelRegions []string
	if known {
		modelRegions = facts.GlobalRegions
		if strings.HasPrefix(modelID, "us.") {
			modelRegions = facts.USRegions
		}
	}

	return buildWithRegionWarnings(opts, modelID, modelRegions, " ", g, func(region string) (ConfigPlan, error) {
		baseURL := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/openai/v1", region)
		modelBlock := map[string]any{
			"model":    modelID,
			"base_url": baseURL,
			"name":     modelID + " (Amazon Bedrock)",
			// Empty string is the merge-time delete sentinel: switching from
			// the mantle endpoint back to runtime must not leave the Responses
			// wire API behind (runtime speaks the default Chat Completions).
			"api_backend": "",
		}
		if known {
			// Use the AWS-published context window for the selected model; the
			// client derives compaction thresholds from it.
			modelBlock["context_window"] = facts.ContextWindow
		}
		return g.grokPlan(opts, region, modelBlock)
	})
}

// buildMantleConfig targets the opt-in bedrock-mantle endpoint with the bare
// foundation model ID. Mantle is API-key only: the Grok CLI sends the bearer
// token from [auth] and cannot sign SigV4 requests.
func (g grok) buildMantleConfig(opts Options) (ConfigPlan, error) {
	if !authmode.IsBedrockAPIKey(opts.AuthMode) {
		return ConfigPlan{}, fmt.Errorf(
			"the Grok CLI reaches the mantle endpoint with a Bedrock API key, not SigV4; re-run with --auth=%s",
			authmode.BedrockAPIKey)
	}
	modelID, err := normalizeGrokMantleModelID(opts.Model)
	if err != nil {
		return ConfigPlan{}, err
	}
	facts := grokModels[modelID]

	// Mantle has no cross-region inference and each model is served in its own
	// regions, so the Iron Fist keeps a serving region or moves to the model's
	// first verified one.
	region, regionMsg, _ := resolveModelRegion(opts.Region, opts.RegionExplicit, facts.MantleRegions)

	baseURL := fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1", region)
	modelBlock := map[string]any{
		"model":          modelID,
		"base_url":       baseURL,
		"name":           modelID + " (Amazon Bedrock Mantle)",
		"context_window": facts.ContextWindow,
		"api_backend":    "responses",
	}
	plan, err := g.grokPlan(opts, region, modelBlock)
	if err != nil {
		return ConfigPlan{}, err
	}
	if regionMsg != "" {
		plan.Warnings = append(plan.Warnings, modelID+" "+regionMsg)
	}
	return plan, nil
}

// grokPlan assembles the config shape shared by both endpoints: the named model
// block selected via [models].default, the juggernaut.auth record launch reads,
// and the [auth] block — the API-key provider command, or empty leaves that
// delete it on re-apply so IAM does not leave auth-token wired up.
func (g grok) grokPlan(opts Options, region string, modelBlock map[string]any) (ConfigPlan, error) {
	keys := map[string]any{
		"model": map[string]any{
			grokModelName: modelBlock,
		},
		"models": map[string]any{
			"default": grokModelName,
		},
	}
	blockMap, err := juggernautAuthBlock(opts, region)
	if err != nil {
		return ConfigPlan{}, err
	}
	keys["juggernaut"] = blockMap
	if authmode.IsBedrockAPIKey(opts.AuthMode) {
		keys["auth"] = map[string]any{
			"auth_provider_command": grokAuthCommand,
			"auth_provider_label":   "Bedrock",
		}
	} else {
		keys["auth"] = map[string]any{
			"auth_provider_command": "",
			"auth_provider_label":   "",
		}
	}
	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: g.NativeManagedKeys(),
	}, nil
}

func (g grok) SupportsModel(model CatalogModel) ModelSupport {
	return g.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		if !RuntimeSupported(m.ID, m.Source) {
			return ModelSupport{Reason: mantleOnlyReason}
		}
		// Catalog entries carry CRIS profile IDs (global.xai.grok-4.6,
		// us.xai.grok-4.6); strip the regional prefix so the bare xai.grok-
		// family check matches them, not just the bare foundation form.
		if !strings.HasPrefix(bedrock.StripRegionPrefix(m.ID), "xai.grok-") {
			return ModelSupport{Reason: "the Grok client supports xAI Grok models"}
		}
		return ModelSupport{Supported: true, Reason: "xAI native model"}
	})
}
