package provider

import (
	"fmt"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// grok is the OFFICIAL xAI Grok CLI provider (grok / grok.exe, config TOML at
// ~/.grok/config.toml). Grok is a BYOK-friendly harness: custom providers are
// [model.<name>] tables with base_url + env_key, selected via [models].default.
// Juggernaut routes to Grok 4.3 on Bedrock Mantle by writing such a block with
// base_url → Mantle /openai/v1 and env_key → the injected bearer token (xAI's
// own docs prefer env_key over inline api_key). Schema verified against a real
// install + docs.x.ai/build/settings — see the grok-cli-config-schema memory.
type grok struct{}

// grokModelName is the [model.<name>] key + [models].default value we write.
const grokModelName = "bedrock-grok"

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

// NativeManagedKeys are the top-level config.toml keys Juggernaut owns. Note the
// whole [model] and [models] tables are managed here (Grok nests custom models
// under [model.<name>]); on uninstall RemoveManagedKeys drops them wholesale,
// which is acceptable for a Juggernaut-owned config.
func (grok) NativeManagedKeys() []string {
	return []string{"model", "models"}
}

// DeepMergeKeys: both [model.<name>] and [models] are nested tables holding a
// user's own model profiles + settings; merge only our bedrock-grok block and
// the default leaf, preserving their profiles.
func (grok) DeepMergeKeys() []string { return []string{"model", "models"} }

// OwnedSubKeys: uninstall removes only our bedrock-grok model profile and the
// models.default pointer we set — preserving the user's other model profiles
// and settings.
func (grok) OwnedSubKeys() map[string][]string {
	return map[string][]string{
		"model":  {grokModelName},
		"models": {"default"},
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
	// Grok routes via config (env_key references the token env var); no static
	// enable flag, but the bearer token is required (Mantle).
	return LaunchSpec{
		TokenEnvVar: bedrockAuthEnvName,
		NeedsToken:  true,
	}
}

func (grok) Supports(Capability) bool { return false }

// BuildConfig writes a [model.bedrock-grok] block routing to Grok 4.3 on Mantle
// plus [models].default. Grok 4.3 is the only xAI model on Mantle, so there's no
// model table/passthrough — just the one verified target.
func (grok) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	baseURL := fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1", opts.Region)
	keys := map[string]any{
		"model": map[string]any{
			grokModelName: map[string]any{
				"model":          "xai.grok-4.3",
				"base_url":       baseURL,
				"name":           "Grok 4.3 (Amazon Bedrock Mantle)",
				"env_key":        bedrockAuthEnvName,
				"api_backend":    "chat_completions",
				"context_window": 1000000,
			},
		},
		"models": map[string]any{
			"default": grokModelName,
		},
	}
	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: grok{}.NativeManagedKeys(),
	}, nil
}
