package provider

import (
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// claude is the Claude Code provider. Every value here is transcribed from the
// pre-abstraction sources so behavior is byte-identical: NativeManagedKeys from
// internal/config/manager.go's nativeManagedKeys slice, the markers and binary
// names from internal/activation/activation.go, and the Bedrock env var
// (CLAUDE_CODE_USE_BEDROCK=1) from the literal set inside activation.Launch.
// The provider_test.go pins guard against drift.
type claude struct{}

func (claude) Name() string { return "claude" }

func (claude) BinaryNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"claude.exe", "claude.cmd", "claude.bat"}
	}
	return []string{"claude"}
}

func (claude) ConfigFormatName() string { return "json" }

// ConfigPath is ~/.claude/settings.json (user) or ./.claude/settings.json
// (project) — identical to the pre-abstraction cmd/helpers.settingsPath.
func (claude) ConfigPath(home, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".claude", "settings.json"), nil
	}
	return safepath.JoinUnder(home, ".claude", "settings.json")
}

func (claude) NativeManagedKeys() []string {
	return []string{
		"env",
		"model",
		"modelOverrides",
		"fallbackModel",
		"effortLevel",
		"alwaysThinkingEnabled",
		"skipWebFetchPreflight",
	}
}

func (claude) ActivationMarkers() (begin, end string) {
	return "# BEGIN: Juggernaut Claude Activation", "# END: Juggernaut Claude Activation"
}

func (claude) BedrockEnvVar() (key, value string) {
	return "CLAUDE_CODE_USE_BEDROCK", "1"
}

// BuildConfig wraps the existing schema.Build so Claude's persisted config is
// byte-identical to the pre-abstraction cmd/apply path: it packs the juggernaut
// block plus every native top-level key exactly as commitApply assembled them.
// Claude logic is relocated here, not rewritten — the golden-output test guards
// against drift.
func (claude) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
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
		"juggernaut":            blockMap,
		"env":                   native.Env,
		"model":                 native.Model,
		"modelOverrides":        modelOverrides,
		"fallbackModel":         native.FallbackModel,
		"effortLevel":           native.EffortLevel,
		"alwaysThinkingEnabled": native.AlwaysThinking,
		"skipWebFetchPreflight": native.SkipWebFetchPreflight,
		"permissions":           native.Permissions,
	}

	return ConfigPlan{
		Keys:        keys,
		ManagedKeys: claude{}.NativeManagedKeys(),
	}, nil
}

func (claude) LaunchSpec() LaunchSpec {
	return LaunchSpec{
		TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
		StaticEnv:   map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"},
		NeedsToken:  true,
	}
}

func (claude) Supports(c Capability) bool {
	switch c {
	case CapAutoMode, Cap1MContext, CapOpusplan, CapThinking, CapServiceTiers, CapEffortLevels:
		return true
	default:
		return false
	}
}

// toSchemaOptions maps the CLI-neutral provider.Options onto Claude's
// schema.Options. This mapping is the ONLY place the provider package couples to
// schema, keeping the coupling isolated to claude.go.
func toSchemaOptions(o Options) schema.Options {
	return schema.Options{
		AuthMode:       o.AuthMode,
		Region:         o.Region,
		Effort:         o.Effort,
		Scope:          o.Scope,
		Version:        o.Version,
		OpusModel:      o.OpusModel,
		SonnetModel:    o.SonnetModel,
		HaikuModel:     o.HaikuModel,
		FableModel:     o.FableModel,
		Opusplan:       o.Opusplan,
		FallbackModels: o.FallbackModels,
		Use1M:          o.Use1M,
		UseMantle:      o.UseMantle,
		MantleURL:      o.MantleURL,
		AuthValidated:  o.AuthValidated,
		PermissionMode: o.PermissionMode,
		AlwaysThinking: o.AlwaysThinking,
		ServiceTier:    o.ServiceTier,
	}
}
