// Package schema builds and validates the Juggernaut block written to settings.json.
package schema

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

// SchemaVersion is the current version of the Juggernaut settings.json block.
const SchemaVersion = 2

// Options holds all user-supplied apply parameters.
type Options struct {
	AuthMode               string
	Region                 string
	Effort                 string
	Scope                  string
	Version                string
	OpusModel              string
	SonnetModel            string
	HaikuModel             string
	FableModel             string
	Opusplan               bool
	FallbackModels         []string
	AvailableModels        []string
	EnforceAvailableModels bool
	Use1M                  bool
	AuthValidated          bool
	PermissionMode         string // "", "default", "acceptEdits", "plan", "auto", "dontAsk", "bypassPermissions"
	AlwaysThinking         bool
	ServiceTier            string // "", "default", "flex", "priority"
}

// Block is the .juggernaut block written to settings.json.
type Block struct {
	Auth   Auth              `json:"auth"`
	Models ModelOverrides    `json:"modelOverrides"`
	Env    map[string]string `json:"env"`
	Meta   Meta              `json:"meta"`
	// Warnings are actionable heads-ups for apply/doctor output only — never
	// persisted to settings.json.
	Warnings []string `json:"-"`
}

// Auth holds the authentication configuration within the Juggernaut block.
type Auth struct {
	Mode   string `json:"mode"`
	Region string `json:"region"`
}

// ModelOverrides holds the Bedrock model IDs for each tier.
type ModelOverrides struct {
	Opus     string `json:"opus"`
	Sonnet   string `json:"sonnet"`
	Haiku    string `json:"haiku"`
	Fable    string `json:"fable,omitempty"`
	Subagent string `json:"subagent"`
}

// Meta holds Juggernaut metadata stored in the block.
type Meta struct {
	SchemaVersion          int      `json:"schemaVersion"`
	Version                string   `json:"version"`
	ManagedBy              string   `json:"managedBy"`
	Scope                  string   `json:"scope"`
	AppliedAt              string   `json:"appliedAt"`
	Opusplan               bool     `json:"opusplan"`
	Use1M                  bool     `json:"use1mContext"`
	Effort                 string   `json:"effort"`
	FallbackModels         []string `json:"fallbackModels,omitempty"`
	AvailableModels        []string `json:"availableModels,omitempty"`
	EnforceAvailableModels bool     `json:"enforceAvailableModels,omitempty"`
	PermissionMode         string   `json:"permissionMode,omitempty"`
	AlwaysThinking         bool     `json:"alwaysThinkingEnabled,omitempty"`
	ServiceTier            string   `json:"serviceTier,omitempty"`
}

// NativeKeys are the top-level settings.json keys Claude Code reads directly.
type NativeKeys struct {
	Model                  string            `json:"model,omitempty"`
	ModelOverrides         map[string]string `json:"modelOverrides,omitempty"`
	FallbackModel          []string          `json:"fallbackModel,omitempty"`
	AvailableModels        []string          `json:"availableModels,omitempty"`
	EnforceAvailableModels bool              `json:"enforceAvailableModels,omitempty"`
	Env                    map[string]string `json:"env"`
	EffortLevel            string            `json:"effortLevel,omitempty"`
	AlwaysThinking         bool              `json:"alwaysThinkingEnabled,omitempty"`
	SkipWebFetchPreflight  bool              `json:"skipWebFetchPreflight,omitempty"`
	Permissions            map[string]any    `json:"permissions,omitempty"`
}

var validEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true, "auto": true,
}

var validPermissionModes = map[string]bool{
	"default": true, "acceptEdits": true, "plan": true,
	"auto": true, "dontAsk": true, "bypassPermissions": true,
}

var validServiceTiers = map[string]bool{
	"default": true, "flex": true, "priority": true,
}

// ErrEnforceRequiresAvailable is returned when --enforce-available-models is
// set without a non-empty --available-models list. Used by both schema.Build
// and cmd/apply flag validation.
const ErrEnforceRequiresAvailable = "--enforce-available-models requires --available-models to be set to a non-empty list"

// Build constructs and validates a Block from bedrock config and options.
func Build(cfg *bedrock.Config, opts Options) (*Block, error) {
	if !cfg.IsSupportedRegion(opts.Region) {
		return nil, fmt.Errorf("unsupported region %q — run `juggernaut doctor` for supported regions", opts.Region)
	}
	if !validEfforts[opts.Effort] {
		return nil, fmt.Errorf("invalid effort %q — must be one of: low, medium, high, xhigh, max, auto", opts.Effort)
	}
	if opts.PermissionMode != "" && !validPermissionModes[opts.PermissionMode] {
		return nil, fmt.Errorf("invalid mode %q — must be one of: default, acceptEdits, plan, auto, dontAsk, bypassPermissions", opts.PermissionMode)
	}
	if opts.ServiceTier != "" && !validServiceTiers[opts.ServiceTier] {
		return nil, fmt.Errorf("invalid service-tier %q — must be one of: default, flex, priority", opts.ServiceTier)
	}

	opus, sonnet, haiku, fable := resolveTierModels(cfg, opts)

	fallbackModels, err := ValidateModelList(opts.FallbackModels, "fallback model chain")
	if err != nil {
		return nil, err
	}
	availableModels, err := ValidateModelList(opts.AvailableModels, "available models list")
	if err != nil {
		return nil, err
	}
	if opts.EnforceAvailableModels && len(availableModels) == 0 {
		return nil, fmt.Errorf("%s", ErrEnforceRequiresAvailable)
	}

	env := buildEnv(cfg, opts, opus, sonnet, haiku, fable)

	var warnings []string
	if IsFable5Model(fable) {
		warnings = append(warnings, FableDataRetentionWarning)
	}

	return assembleBlock(opts, opus, sonnet, haiku, fable, env, fallbackModels, availableModels, warnings), nil
}

// assembleBlock constructs the Block struct from resolved values.
// All validation and resolution must have completed before calling this.
func assembleBlock(opts Options, opus, sonnet, haiku, fable string, env map[string]string,
	fallbackModels, availableModels []string, warnings []string) *Block {
	return &Block{
		Auth: Auth{
			Mode:   opts.AuthMode,
			Region: opts.Region,
		},
		Models: ModelOverrides{
			Opus:     opus,
			Sonnet:   sonnet,
			Haiku:    haiku,
			Fable:    fable,
			Subagent: haiku,
		},
		Env: env,
		Meta: Meta{
			SchemaVersion:          SchemaVersion,
			Version:                opts.Version,
			ManagedBy:              "juggernaut",
			Scope:                  opts.Scope,
			AppliedAt:              time.Now().UTC().Format(time.RFC3339),
			Opusplan:               opts.Opusplan,
			Use1M:                  opts.Use1M,
			Effort:                 opts.Effort,
			FallbackModels:         fallbackModels,
			AvailableModels:        availableModels,
			EnforceAvailableModels: opts.EnforceAvailableModels,
			PermissionMode:         opts.PermissionMode,
			AlwaysThinking:         opts.AlwaysThinking,
			ServiceTier:            opts.ServiceTier,
		},
		Warnings: warnings,
	}
}

// resolveTierModels fills in per-tier model IDs from the config defaults.
func resolveTierModels(cfg *bedrock.Config, opts Options) (opus, sonnet, haiku, fable string) {
	opus = opts.OpusModel
	if opus == "" {
		opus = cfg.Models.Opus
	}
	sonnet = opts.SonnetModel
	if sonnet == "" {
		sonnet = cfg.Models.Sonnet
	}
	haiku = opts.HaikuModel
	if haiku == "" {
		haiku = cfg.Models.Haiku
	}
	fable = opts.FableModel
	if fable == "" {
		fable = cfg.Models.Fable
	}
	return
}

// buildEnv constructs the environment map for the Juggernaut block.
func buildEnv(cfg *bedrock.Config, opts Options, opus, sonnet, haiku, fable string) map[string]string {
	env := make(map[string]string, len(cfg.Environment))
	maps.Copy(env, cfg.Environment)

	if opts.AuthValidated {
		maps.Copy(env, cfg.EnvironmentBedrockAuth)
	}

	env["AWS_REGION"] = opts.Region
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = claudeCodeContextModelID(opus, opts.Use1M)
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = claudeCodeContextModelID(sonnet, opts.Use1M)
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
	if fable != "" {
		env["ANTHROPIC_DEFAULT_FABLE_MODEL"] = claudeCodeContextModelID(fable, opts.Use1M)
		setDefaultEnv(env, "ANTHROPIC_DEFAULT_FABLE_MODEL_NAME", "Fable 5")
		setDefaultEnv(env, "ANTHROPIC_DEFAULT_FABLE_MODEL_DESCRIPTION", "Configured Claude Code Fable alias with adaptive thinking metadata and Opus fallback support")
		setDefaultEnv(env, "ANTHROPIC_DEFAULT_FABLE_MODEL_SUPPORTED_CAPABILITIES", "effort,max_effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking")
	}
	env["CLAUDE_CODE_SUBAGENT_MODEL"] = haiku
	env["CLAUDE_CODE_EFFORT_LEVEL"] = opts.Effort

	if opts.Opusplan {
		env["ANTHROPIC_MODEL"] = claudeCodeContextModelID("opusplan", opts.Use1M)
	}
	if !opts.Use1M {
		env["CLAUDE_CODE_DISABLE_1M_CONTEXT"] = "1"
	}
	// Enable auto mode when it's the requested mode AND at least one configured
	// model can use it (Opus/Sonnet/Fable; Haiku never qualifies). The var only
	// makes auto AVAILABLE in the Shift+Tab cycle — Claude Code gates it by the
	// live session model — so a Sonnet-tier default with an auto-capable Opus
	// override (the standard config) still gets it. Required on
	// Bedrock/Vertex/Foundry; without it auto silently never appears.
	if opts.PermissionMode == "auto" &&
		(IsAutoModeCapableModel(opus) || IsAutoModeCapableModel(sonnet) || IsAutoModeCapableModel(fable)) {
		env["CLAUDE_CODE_ENABLE_AUTO_MODE"] = "1"
	}
	if opts.ServiceTier != "" {
		env["ANTHROPIC_BEDROCK_SERVICE_TIER"] = opts.ServiceTier
	}

	return env
}

// StripRegionPrefix removes a cross-region inference profile prefix from a
// model ID, recovering the bare provider model identifier. This is a re-export
// of bedrock.StripRegionPrefix for backward compatibility — the authoritative
// implementation lives in internal/bedrock.
func StripRegionPrefix(modelID string) string {
	return bedrock.StripRegionPrefix(modelID)
}

// normalizeModelID strips the [1m] context suffix and regional inference
// prefixes from a model ID, recovering the bare provider model identifier.
func normalizeModelID(modelID string) string {
	return StripRegionPrefix(strings.TrimSuffix(modelID, "[1m]"))
}

// autoModeCapablePrefixes lists model ID fragments that support auto
// permission mode on Bedrock/Vertex/Foundry: Claude Sonnet 5, Opus 4.7 or later.
var autoModeCapablePrefixes = []string{
	"claude-opus-5",
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-sonnet-5",
}

// IsAutoModeCapableModel reports whether modelID is a model that can use auto
// permission mode on Bedrock/Vertex/Foundry: Claude Sonnet 5, Opus 4.7 or later
// (4.7, 4.8, 5). Sonnet 4.6, Haiku, older Opus, and Fable are excluded. Verified
// against https://code.claude.com/docs/en/permission-modes ("only Claude Sonnet
// 5, Opus 4.7 or later, and Fable 5"). Note claude-sonnet-5 is capable while
// claude-sonnet-4-6 is not — the version digits matter, so match the full token,
// not just "sonnet".
func IsAutoModeCapableModel(modelID string) bool {
	normalized := normalizeModelID(modelID)
	for _, prefix := range autoModeCapablePrefixes {
		if strings.Contains(normalized, prefix) {
			return true
		}
	}
	return false
}

// IsFable5Model reports whether modelID refers to Claude Fable 5, regardless
// of cross-region inference prefix or the Claude Code [1m] context suffix.
func IsFable5Model(modelID string) bool {
	return strings.Contains(normalizeModelID(modelID), "claude-fable-5")
}

// FableDataRetentionWarning is shown whenever Fable is configured. AWS's
// Bedrock model card and abuse-detection docs state that using Claude Fable 5
// requires opting in to provider_data_share (inputs/outputs retained up to 30
// days and shared with Anthropic for abuse detection/human review) — a
// requirement Anthropic imposes, not AWS. There is no AWS API to read an
// account's current data-retention setting, so Juggernaut cannot verify
// opt-in status; this warns instead of silently shipping calls that may be
// denied at runtime. No claim is made about what is or isn't collected beyond
// what AWS documents — Juggernaut doesn't know and won't guess.
const FableDataRetentionWarning = "Fable 5 requires opting in to provider_data_share data retention " +
	"(Anthropic's requirement, not AWS's) — see " +
	"https://docs.aws.amazon.com/bedrock/latest/userguide/abuse-detection.html. Without it, Fable calls " +
	"may be denied at runtime. Juggernaut cannot check your account's current opt-in status — no AWS " +
	"API exposes it."

// AutoModeUsable reports whether auto mode will be offered for this block's
// ACTIVE DEFAULT session model (the Sonnet-tier pin, the account default on
// Bedrock). Distinct from AutoModeAvailable: this is the stricter "auto works
// out of the box without switching models" check. opusplan does not qualify —
// its execution phase runs on Sonnet, and the default model is still Sonnet-tier.
func (b *Block) AutoModeUsable() bool {
	return b.Meta.PermissionMode == "auto" && IsAutoModeCapableModel(b.Models.Sonnet)
}

// AutoModeAvailable reports whether Juggernaut should enable auto mode (write
// CLAUDE_CODE_ENABLE_AUTO_MODE=1) for this block: PermissionMode=="auto" AND at
// least one configured model (Opus, Sonnet, or Fable — Haiku is never capable)
// can use auto mode. The env var only makes auto AVAILABLE in the Shift+Tab
// cycle; Claude Code still gates it by the live session model. So enabling it
// whenever a capable model is configured is correct — the user unlocks auto by
// running a capable model (e.g. `claude --model opus`), even when the default
// pin is Sonnet-tier.
func (b *Block) AutoModeAvailable() bool {
	if b.Meta.PermissionMode != "auto" {
		return false
	}
	return IsAutoModeCapableModel(b.Models.Opus) ||
		IsAutoModeCapableModel(b.Models.Sonnet) ||
		IsAutoModeCapableModel(b.Models.Fable)
}

// RegionalInferencePrefixes is a re-export of bedrock.RegionalInferencePrefixes
// for backward compatibility.
var RegionalInferencePrefixes = bedrock.RegionalInferencePrefixes

func claudeCodeContextModelID(model string, use1M bool) string {
	if !use1M || strings.HasSuffix(model, "[1m]") || !supportsClaudeCode1M(model) {
		return model
	}
	return model + "[1m]"
}

func supportsClaudeCode1M(model string) bool {
	normalized := normalizeModelID(model)
	if normalized == "opusplan" {
		return true
	}
	for _, fragment := range []string{
		"claude-fable-5",
		"claude-opus-5",
		"claude-opus-4-",
		"claude-sonnet-5",
		"claude-sonnet-4-6",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// NativeKeys derives the top-level settings.json keys from the block.
func (b *Block) NativeKeys() NativeKeys {
	model := ""
	if b.Meta.Opusplan {
		model = "opusplan"
		if b.Meta.Use1M {
			model = "opusplan[1m]"
		}
	}

	var permissions map[string]any
	if b.Meta.PermissionMode != "" {
		permissions = map[string]any{
			"defaultMode": b.Meta.PermissionMode,
		}
	}

	return NativeKeys{
		Model:                  model,
		ModelOverrides:         nativeModelOverrides(b.Models, b.Meta.Use1M),
		FallbackModel:          append([]string(nil), b.Meta.FallbackModels...),
		AvailableModels:        append([]string(nil), b.Meta.AvailableModels...),
		EnforceAvailableModels: b.Meta.EnforceAvailableModels,
		Env:                    b.Env,
		EffortLevel:            persistedEffortLevel(b.Meta.Effort),
		AlwaysThinking:         b.Meta.AlwaysThinking,
		SkipWebFetchPreflight:  true, // always set for Bedrock users
		Permissions:            permissions,
	}
}

func nativeModelOverrides(models ModelOverrides, use1M bool) map[string]string {
	overrides := map[string]string{
		"opus":                      models.Opus,
		"claude-opus-5":             models.Opus,
		"anthropic.claude-opus-5":   models.Opus,
		"sonnet":                    models.Sonnet,
		"claude-sonnet-5":           models.Sonnet,
		"anthropic.claude-sonnet-5": models.Sonnet,
		"haiku":                     models.Haiku,
		"claude-haiku-4-5":          models.Haiku,
		"claude-haiku-4-5-20251001": models.Haiku,
		"anthropic.claude-haiku-4-5-20251001-v1:0": models.Haiku,
	}
	if models.Fable != "" {
		overrides["fable"] = models.Fable
		overrides["claude-fable-5"] = models.Fable
		overrides["anthropic.claude-fable-5"] = models.Fable
	}
	if use1M {
		overrides["opus[1m]"] = models.Opus
		overrides["claude-opus-5[1m]"] = models.Opus
		overrides["anthropic.claude-opus-5[1m]"] = models.Opus
		overrides["sonnet[1m]"] = models.Sonnet
		overrides["claude-sonnet-5[1m]"] = models.Sonnet
		overrides["anthropic.claude-sonnet-5[1m]"] = models.Sonnet
		if models.Fable != "" {
			overrides["fable[1m]"] = models.Fable
			overrides["claude-fable-5[1m]"] = models.Fable
			overrides["anthropic.claude-fable-5[1m]"] = models.Fable
		}
	}
	return overrides
}

func persistedEffortLevel(effort string) string {
	if effort == "max" || effort == "auto" {
		return ""
	}
	return effort
}

// ValidateModelList trims and validates a list of model IDs, rejecting empty
// entries. Returns nil for empty input. Used by Build and cmd/apply — there
// should be only one implementation across the project.
func ValidateModelList(models []string, context string) ([]string, error) {
	if len(models) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		trimmed := strings.TrimSpace(model)
		if trimmed == "" {
			return nil, fmt.Errorf("%s contains an empty model ID", context)
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func setDefaultEnv(env map[string]string, key, value string) {
	if _, ok := env[key]; !ok {
		env[key] = value
	}
}
