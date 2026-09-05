package provider

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// grok is the OFFICIAL xAI Grok CLI provider (grok / grok.exe, config TOML at
// ~/.grok/config.toml). Juggernaut routes to Grok 4.6 on native Bedrock
// (bedrock-runtime /openai/v1) by writing a [model.bedrock-grok] block
// (base_url → bedrock-runtime) selected via [models].default. API-key apply
// also writes an [auth] block whose auth_provider_command runs
// `juggernaut auth-token`; IAM apply omits that command so Grok does not
// demand a keychain bearer token.
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

// grokRegions are the regions where xai.grok-4.6 is verified available on native
// Bedrock (live-checked 2026-08-29, foundation in us-west-2). apply warns—does
// not fail—outside this set.
var grokRegions = []string{"us-east-1", "us-east-2", "us-west-2"}

// grokDefaultModelID is the Grok model Juggernaut pins by default: the global.
// inference-profile ID (the bare foundation ID xai.grok-4.6 400s on the
// bedrock-runtime endpoint, which only serves CRIS profile IDs).
const grokDefaultModelID = "global.xai.grok-4.6"

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

// grokModelIs46 reports whether modelID is the Grok 4.6 model under any form
// (bare, global.xai.grok-4.6, us.xai.grok-4.6) — the only model with a verified
// 1M context window and a verified serving-region set.
func grokModelIs46(modelID string) bool {
	return bedrock.StripRegionPrefix(modelID) == "xai.grok-4.6"
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

// BuildConfig writes a [model.bedrock-grok] block routing to Grok 4.6 on native
// Bedrock (bedrock-runtime /openai/v1), [models].default, a juggernaut.auth
// block for launch, and — only for Bedrock API key auth — an [auth] block
// whose auth_provider_command supplies the keychain bearer token so Grok skips
// its interactive sign-in.
//
// Deliberately NO env_key on the model block: with env_key set, Grok's session
// still falls through to interactive login. The [auth] block is the documented
// way to replace login entirely.
func (g grok) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	modelID, ok := normalizeGrokModelID(opts.Model)
	if !ok {
		return ConfigPlan{}, fmt.Errorf("unknown Grok model %q (expected a discovered xai.grok-* model)", opts.Model)
	}

	// Route to a region that actually serves grok-4.6 rather than writing a
	// config that can't reach it.
	var modelRegions []string
	if grokModelIs46(modelID) {
		modelRegions = grokRegions
	}

	return buildWithRegionWarnings(opts, modelID, modelRegions, " ", g, func(region string) (ConfigPlan, error) {
		baseURL := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/openai/v1", region)
		modelBlock := map[string]any{
			"model":    modelID,
			"base_url": baseURL,
			"name":     modelID + " (Amazon Bedrock)",
		}
		if grokModelIs46(modelID) {
			modelBlock["context_window"] = 1000000
		}
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
			// Empty strings delete owned [auth] leaves on re-apply so IAM
			// does not leave auth_provider_command pointing at auth-token.
			keys["auth"] = map[string]any{
				"auth_provider_command": "",
				"auth_provider_label":   "",
			}
		}

		return ConfigPlan{
			Keys:        keys,
			ManagedKeys: g.NativeManagedKeys(),
		}, nil
	})
}

func (g grok) SupportsModel(model CatalogModel) ModelSupport {
	return g.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		if m.Source == "mantle" {
			return ModelSupport{Reason: "Grok no longer uses Mantle (native only)"}
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
