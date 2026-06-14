// Package schema builds and validates the Juggernaut block written to settings.json.
package schema

import (
	"fmt"
	"maps"
	"time"

	"github.com/jpvelasco/juggernaut/v4/internal/bedrock"
)

// SchemaVersion is the current version of the Juggernaut settings.json block.
const SchemaVersion = 2

// Options holds all user-supplied apply parameters.
type Options struct {
	AuthMode       string
	Region         string
	Effort         string
	Scope          string
	Version        string
	OpusModel      string
	SonnetModel    string
	HaikuModel     string
	Opusplan       bool
	Use1M          bool
	UseMantle      bool
	MantleURL      string
	Storage        string
	AuthValidated  bool
	PermissionMode string // "", "default", "acceptEdits", "plan", "auto", "dontAsk", "bypassPermissions"
	AlwaysThinking bool
	ServiceTier    string // "", "default", "flex", "priority"
}

// Block is the .juggernaut block written to settings.json.
type Block struct {
	Auth   Auth              `json:"auth"`
	Models ModelOverrides    `json:"modelOverrides"`
	Env    map[string]string `json:"env"`
	Meta   Meta              `json:"meta"`
}

// Auth holds the authentication configuration within the Juggernaut block.
type Auth struct {
	Mode    string `json:"mode"`
	Region  string `json:"region"`
	Storage string `json:"storage,omitempty"`
}

// ModelOverrides holds the Bedrock model IDs for each tier.
type ModelOverrides struct {
	Opus     string `json:"opus"`
	Sonnet   string `json:"sonnet"`
	Haiku    string `json:"haiku"`
	Subagent string `json:"subagent"`
}

// Meta holds Juggernaut metadata stored in the block.
type Meta struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Version        string `json:"version"`
	ManagedBy      string `json:"managedBy"`
	Scope          string `json:"scope"`
	AppliedAt      string `json:"appliedAt"`
	Opusplan       bool   `json:"opusplan"`
	Use1M          bool   `json:"use1mContext"`
	UseMantle      bool   `json:"useMantle"`
	MantleURL      string `json:"mantleBaseUrl,omitempty"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permissionMode,omitempty"`
	AlwaysThinking bool   `json:"alwaysThinkingEnabled,omitempty"`
	ServiceTier    string `json:"serviceTier,omitempty"`
}

// NativeKeys are the top-level settings.json keys Claude Code reads directly.
type NativeKeys struct {
	Model                string            `json:"model,omitempty"`
	ModelOverrides       map[string]string `json:"modelOverrides,omitempty"`
	Env                  map[string]string `json:"env"`
	EffortLevel          string            `json:"effortLevel,omitempty"`
	AlwaysThinking       bool              `json:"alwaysThinkingEnabled,omitempty"`
	SkipWebFetchPreflight bool             `json:"skipWebFetchPreflight,omitempty"`
	Permissions          map[string]any    `json:"permissions,omitempty"`
}

var validEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

var validPermissionModes = map[string]bool{
	"default": true, "acceptEdits": true, "plan": true,
	"auto": true, "dontAsk": true, "bypassPermissions": true,
}

var validServiceTiers = map[string]bool{
	"default": true, "flex": true, "priority": true,
}

// Build constructs and validates a Block from bedrock config and options.
func Build(cfg *bedrock.Config, opts Options) (*Block, error) {
	if !cfg.IsSupportedRegion(opts.Region) {
		return nil, fmt.Errorf("unsupported region %q — run `juggernaut doctor` for supported regions", opts.Region)
	}
	if !validEfforts[opts.Effort] {
		return nil, fmt.Errorf("invalid effort %q — must be one of: low, medium, high, xhigh, max", opts.Effort)
	}
	if opts.PermissionMode != "" && !validPermissionModes[opts.PermissionMode] {
		return nil, fmt.Errorf("invalid mode %q — must be one of: default, acceptEdits, plan, auto, dontAsk, bypassPermissions", opts.PermissionMode)
	}
	if opts.ServiceTier != "" && !validServiceTiers[opts.ServiceTier] {
		return nil, fmt.Errorf("invalid service-tier %q — must be one of: default, flex, priority", opts.ServiceTier)
	}

	opus := opts.OpusModel
	if opus == "" {
		opus = cfg.Models.Opus
	}
	sonnet := opts.SonnetModel
	if sonnet == "" {
		sonnet = cfg.Models.Sonnet
	}
	haiku := opts.HaikuModel
	if haiku == "" {
		haiku = cfg.Models.Haiku
	}

	env := make(map[string]string, len(cfg.Environment))
	maps.Copy(env, cfg.Environment)

	if opts.AuthValidated {
		maps.Copy(env, cfg.EnvironmentBedrockAuth)
	}

	env["AWS_REGION"] = opts.Region
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = opus
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = sonnet
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
	env["CLAUDE_CODE_SUBAGENT_MODEL"] = haiku
	env["CLAUDE_CODE_EFFORT_LEVEL"] = opts.Effort

	if opts.Opusplan {
		env["ANTHROPIC_MODEL"] = "opusplan"
	}
	if opts.UseMantle {
		env["CLAUDE_CODE_USE_MANTLE"] = "1"
		if opts.MantleURL != "" {
			env["ANTHROPIC_BEDROCK_MANTLE_BASE_URL"] = opts.MantleURL
		}
	}
	if opts.PermissionMode == "auto" {
		// Required on Bedrock/Vertex/Foundry — without it auto mode silently does nothing.
		env["CLAUDE_CODE_ENABLE_AUTO_MODE"] = "1"
	}
	if opts.ServiceTier != "" {
		env["ANTHROPIC_BEDROCK_SERVICE_TIER"] = opts.ServiceTier
	}

	storage := opts.Storage
	if storage == "" {
		storage = "keychain"
	}

	return &Block{
		Auth: Auth{
			Mode:    opts.AuthMode,
			Region:  opts.Region,
			Storage: storage,
		},
		Models: ModelOverrides{
			Opus:     opus,
			Sonnet:   sonnet,
			Haiku:    haiku,
			Subagent: haiku,
		},
		Env: env,
		Meta: Meta{
			SchemaVersion:  SchemaVersion,
			Version:        opts.Version,
			ManagedBy:      "juggernaut",
			Scope:          opts.Scope,
			AppliedAt:      time.Now().UTC().Format(time.RFC3339),
			Opusplan:       opts.Opusplan,
			Use1M:          opts.Use1M,
			UseMantle:      opts.UseMantle,
			MantleURL:      opts.MantleURL,
			Effort:         opts.Effort,
			PermissionMode: opts.PermissionMode,
			AlwaysThinking: opts.AlwaysThinking,
			ServiceTier:    opts.ServiceTier,
		},
	}, nil
}

// NativeKeys derives the top-level settings.json keys from the block.
func (b *Block) NativeKeys() NativeKeys {
	model := ""
	if b.Meta.Opusplan {
		model = "opusplan"
	}

	var permissions map[string]any
	if b.Meta.PermissionMode != "" {
		permissions = map[string]any{
			"defaultMode": b.Meta.PermissionMode,
		}
	}

	return NativeKeys{
		Model: model,
		ModelOverrides: map[string]string{
			"opus":   b.Models.Opus,
			"sonnet": b.Models.Sonnet,
			"haiku":  b.Models.Haiku,
		},
		Env:                   b.Env,
		EffortLevel:           b.Meta.Effort,
		AlwaysThinking:        b.Meta.AlwaysThinking,
		SkipWebFetchPreflight: true, // always set for Bedrock users
		Permissions:           permissions,
	}
}
