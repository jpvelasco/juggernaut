package provider

import (
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

func (c claude) SupportsModel(model CatalogModel) ModelSupport {
	if s := checkModelPreconditions(model); s.Reason != "" {
		return s
	}
	if model.Source != "foundation" && model.Source != "profile" {
		return ModelSupport{Reason: "Claude uses native Bedrock models and inference profiles"}
	}
	if !strings.Contains(model.ID, "anthropic.claude-") {
		return ModelSupport{Reason: "Claude Code supports Anthropic Claude models"}
	}
	return ModelSupport{Supported: true, Reason: "native Claude model"}
}

func (c claude) CatalogSources() []string { return []string{"foundation", "profile"} }

// claude is the Claude Code provider. Every value here is transcribed from the
// pre-abstraction sources so behavior is byte-identical: NativeManagedKeys from
// internal/config/manager.go's nativeManagedKeys slice, the markers and binary
// names from internal/activation/activation.go, and the Bedrock env var
// (CLAUDE_CODE_USE_BEDROCK=1) from the literal set inside activation.Launch.
// The provider_test.go pins guard against drift.
type claude struct {
	BaseProvider
}

// ConfigPath is ~/.claude/settings.json (user) or ./.claude/settings.json
// (project) — identical to the pre-abstraction cmd/helpers.settingsPath.
func (c claude) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".claude", "settings.json"), nil
	}
	return safepath.JoinUnder(home, ".claude", "settings.json")
}

// OwnsConfig recognizes a Claude config Juggernaut wrote by its managed
// juggernaut block with managedBy == "juggernaut".
func (c claude) OwnsConfig(data map[string]any) bool {
	block, ok := data["juggernaut"].(map[string]any)
	if !ok {
		return false
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		return false
	}
	return meta["managedBy"] == "juggernaut"
}

// DeepMergeKeys: Claude's managed keys are all whole-value (Juggernaut fully
// owns env/modelOverrides/etc.), so none are deep-merged.
func (c claude) DeepMergeKeys() []string { return nil }

// OwnedSubKeys: none — Claude has no deep-merge keys.
func (c claude) OwnedSubKeys() map[string][]string { return nil }

func (c claude) NativeManagedKeys() []string {
	return []string{
		"env",
		"model",
		"modelOverrides",
		"fallbackModel",
		"availableModels",
		"enforceAvailableModels",
		"effortLevel",
		"alwaysThinkingEnabled",
		"skipWebFetchPreflight",
	}
}

// BuildConfig wraps the existing schema.Build so Claude's persisted config is
// byte-identical to the pre-abstraction cmd/apply path: it packs the juggernaut
// block plus every native top-level key exactly as commitApply assembled them.
// Claude logic is relocated here, not rewritten — the golden-output test guards
// against drift.
func (c claude) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	block, err := schema.Build(cfg, toSchemaOptions(opts))
	if err != nil {
		return ConfigPlan{}, err
	}
	native := block.NativeKeys()

	blockMap, err := toMapViaJSON(block)
	if err != nil {
		return ConfigPlan{}, err
	}

	modelOverrides := map[string]any{}
	for k, v := range native.ModelOverrides {
		modelOverrides[k] = v
	}

	keys := map[string]any{
		"juggernaut":             blockMap,
		"env":                    native.Env,
		"model":                  native.Model,
		"modelOverrides":         modelOverrides,
		"fallbackModel":          native.FallbackModel,
		"availableModels":        native.AvailableModels,
		"enforceAvailableModels": native.EnforceAvailableModels,
		"effortLevel":            native.EffortLevel,
		"alwaysThinkingEnabled":  native.AlwaysThinking,
		"skipWebFetchPreflight":  native.SkipWebFetchPreflight,
		"permissions":            native.Permissions,
	}

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: c.NativeManagedKeys(),
		Warnings:    block.Warnings,
	}, nil
}

func (c claude) LaunchSpec() LaunchSpec {
	return LaunchSpec{
		TokenEnvVar: authmode.BedrockAuthEnvName,
		StaticEnv:   map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"},
		// NeedsToken is false: Claude's token requirement is auth-mode-dependent
		// (IAM/SSO use SigV4 and need NO bearer token; only bedrock-api-key does).
		// The launcher decides via needsBearerToken(authModes). Forcing true here
		// would break every Claude+IAM launch with "API key not found".
		NeedsToken: false,
	}
}

// toSchemaOptions maps the CLI-neutral provider.Options onto Claude's
// schema.Options. This mapping is the ONLY place the provider package couples to
// schema, keeping the coupling isolated to claude.go.
func toSchemaOptions(o Options) schema.Options {
	return schema.Options{
		AuthMode:               o.AuthMode,
		Region:                 o.Region,
		Effort:                 o.Effort,
		Scope:                  o.Scope,
		Version:                o.Version,
		OpusModel:              o.OpusModel,
		SonnetModel:            o.SonnetModel,
		HaikuModel:             o.HaikuModel,
		FableModel:             o.FableModel,
		Opusplan:               o.Opusplan,
		FallbackModels:         o.FallbackModels,
		AvailableModels:        o.AvailableModels,
		EnforceAvailableModels: o.EnforceAvailableModels,
		Use1M:                  o.Use1M,
		UseMantle:              o.UseMantle,
		MantleURL:              o.MantleURL,
		AuthValidated:          o.AuthValidated,
		PermissionMode:         o.PermissionMode,
		AlwaysThinking:         o.AlwaysThinking,
		ServiceTier:            o.ServiceTier,
	}
}
