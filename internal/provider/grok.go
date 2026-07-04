package provider

import (
	"fmt"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// grok is the OFFICIAL xAI Grok CLI provider (grok / grok.exe, config TOML at
// ~/.grok/config.toml). Juggernaut routes to Grok 4.3 on Bedrock Mantle by
// writing a [model.bedrock-grok] block (base_url → Mantle /openai/v1) selected
// via [models].default, PLUS an [auth] block whose auth_provider_command runs
// `juggernaut auth-token` to supply the keychain bearer token.
//
// Why the [auth] block (not env_key): Grok's per-model credential order is
// api_key → env_key → XAI_API_KEY, but a custom [model.*] block does NOT satisfy
// Grok's SESSION auth — with only env_key set, Grok still runs its interactive
// browser/OIDC login and prompts the user. The documented mechanism that
// REPLACES login is [auth] auth_provider_command: Grok runs it, reads a token
// from stdout, stores it in ~/.grok/auth.json, and auto-refreshes. Verified
// against the official xAI Grok CLI docs (README "External Auth Provider").
type grok struct{}

// grokModelName is the [model.<name>] key + [models].default value we write.
const grokModelName = "bedrock-grok"

// grokAuthCommand is the auth_provider_command Grok runs (via sh -c) to fetch
// the bearer token. It maps to cmd/auth_token.go, which prints the keychain
// token as {"access_token":...,"expires_in":...}.
const grokAuthCommand = "juggernaut auth-token"

// grokRegions are the regions where xai.grok-4.3 is verified available on Mantle
// (live-checked 2026-07-03). apply warns—does not fail—outside this set.
var grokRegions = []string{"us-east-1", "us-east-2", "us-west-2"}

func (grok) Name() string { return "grok" }

func (grok) BinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"grok.exe", "grok.cmd", "grok.bat"}
	}
	return []string{"grok"}
}

func (grok) ConfigFormatName() string { return "toml" }

// ConfigPath is ALWAYS ~/.grok/config.toml. Grok's project-scoped
// .grok/config.toml only accepts mcp_servers/plugins/permission — model config
// is user-scoped — so both scopes resolve to the user config.
func (grok) ConfigPath(home, _ string) (string, error) {
	return safepath.JoinUnder(home, ".grok", "config.toml")
}

// NativeManagedKeys are the top-level config.toml keys Juggernaut owns: the
// [model] and [models] tables (nested), plus the [auth] table.
func (grok) NativeManagedKeys() []string {
	return []string{"model", "models", "auth"}
}

// DeepMergeKeys: [model.<name>], [models], and [auth] are nested tables that may
// hold a user's own entries (model profiles, default selection, or their own
// OIDC/auth-provider config); merge only our own sub-keys, preserving theirs.
func (grok) DeepMergeKeys() []string { return []string{"model", "models", "auth"} }

// OwnedSubKeys: uninstall removes ONLY our sub-keys — the bedrock-grok model
// profile, the models.default pointer, and the two [auth] keys we set —
// preserving a user's other model profiles and any auth settings of their own.
// Note: apply sets models.default = bedrock-grok (that IS the point) and
// uninstall deletes that pointer rather than restoring the prior value (not
// persisted); Grok then falls back to its built-in default.
func (grok) OwnedSubKeys() map[string][]string {
	return map[string][]string{
		"model":  {grokModelName},
		"models": {"default"},
		"auth":   {"auth_provider_command", "auth_provider_label"},
	}
}

// OwnsConfig recognizes a config Juggernaut wrote by our named model block
// [model.bedrock-grok] — NOT a user's own model profiles.
func (grok) OwnsConfig(data map[string]any) bool {
	models, ok := data["model"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = models[grokModelName]
	return ok
}

func (grok) ActivationMarkers() (begin, end string) {
	return "# BEGIN: Juggernaut Grok Activation", "# END: Juggernaut Grok Activation"
}

func (grok) LaunchSpec() LaunchSpec {
	// Grok gets its token from the [auth] auth_provider_command (which reads the
	// keychain directly), so the launch wrapper does not strictly need to inject
	// the token. NeedsToken stays true: the shared token is still injected as an
	// env var (harmless), and apply's Mantle-only IAM guard keys off NeedsToken.
	return LaunchSpec{
		TokenEnvVar: bedrockAuthEnvName,
		NeedsToken:  true,
	}
}

func (grok) Supports(Capability) bool { return false }

// BuildConfig writes a [model.bedrock-grok] block routing to Grok 4.3 on Mantle,
// [models].default, and an [auth] block whose auth_provider_command supplies the
// keychain bearer token so Grok skips its interactive sign-in. Grok 4.3 is the
// only xAI model on Mantle, so there's no model table/passthrough.
//
// Deliberately NO env_key on the model block: with env_key set, Grok's session
// still falls through to interactive login. The [auth] block is the documented
// way to replace login entirely.
func (grok) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	baseURL := fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1", opts.Region)
	keys := map[string]any{
		"model": map[string]any{
			grokModelName: map[string]any{
				"model":       "xai.grok-4.3",
				"base_url":    baseURL,
				"name":        "Grok 4.3 (Amazon Bedrock Mantle)",
				"api_backend": "responses",
				// grok-4.3 on Mantle serves both Chat + Responses on /openai/v1
				// (verified 2026-07-03); Responses is the richer surface (web
				// search etc.), so we route through it.
				"context_window": 1000000,
			},
		},
		"models": map[string]any{
			"default": grokModelName,
		},
		"auth": map[string]any{
			"auth_provider_command": grokAuthCommand,
			"auth_provider_label":   "Bedrock",
		},
	}

	var warnings []string
	if !regionAllowed(opts.Region, grokRegions) {
		warnings = append(warnings, fmt.Sprintf(
			"model xai.grok-4.3 is not confirmed available in %s (known: %v) — apply will still write config, but requests may fail",
			opts.Region, grokRegions))
	}

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: grok{}.NativeManagedKeys(),
		Warnings:    warnings,
	}, nil
}
