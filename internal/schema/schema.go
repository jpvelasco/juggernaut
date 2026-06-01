package schema

import (
	"fmt"
	"time"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
)

const SchemaVersion = 2

type Options struct {
	AuthMode      string
	Region        string
	Effort        string
	Scope         string
	Version       string
	OpusModel     string
	SonnetModel   string
	HaikuModel    string
	Opusplan      bool
	Use1M         bool
	UseMantle     bool
	MantleURL     string
	Storage       string
	AuthValidated bool
}

type Block struct {
	Auth   Auth              `json:"auth"`
	Models ModelOverrides    `json:"modelOverrides"`
	Env    map[string]string `json:"env"`
	Meta   Meta              `json:"meta"`
}

type Auth struct {
	Mode    string `json:"mode"`
	Region  string `json:"region"`
	Storage string `json:"storage,omitempty"`
}

type ModelOverrides struct {
	Opus     string `json:"opus"`
	Sonnet   string `json:"sonnet"`
	Haiku    string `json:"haiku"`
	Subagent string `json:"subagent"`
}

type Meta struct {
	SchemaVersion int    `json:"schemaVersion"`
	Version       string `json:"version"`
	ManagedBy     string `json:"managedBy"`
	Scope         string `json:"scope"`
	AppliedAt     string `json:"appliedAt"`
	Opusplan      bool   `json:"opusplan"`
	Use1M         bool   `json:"use1mContext"`
	UseMantle     bool   `json:"useMantle"`
	MantleURL     string `json:"mantleBaseUrl,omitempty"`
	Effort        string `json:"effort"`
}

type NativeKeys struct {
	Model          string            `json:"model,omitempty"`
	ModelOverrides map[string]string `json:"modelOverrides,omitempty"`
	Env            map[string]string `json:"env"`
}

var validEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

func Build(cfg *bedrock.Config, opts Options) (*Block, error) {
	if !cfg.IsSupportedRegion(opts.Region) {
		return nil, fmt.Errorf("unsupported region %q — run `juggernaut doctor` for supported regions", opts.Region)
	}
	if !validEfforts[opts.Effort] {
		return nil, fmt.Errorf("invalid effort %q — must be one of: low, medium, high, xhigh, max", opts.Effort)
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
	for k, v := range cfg.Environment {
		env[k] = v
	}

	if opts.AuthValidated {
		for k, v := range cfg.EnvironmentBedrockAuth {
			env[k] = v
		}
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
			SchemaVersion: SchemaVersion,
			Version:       opts.Version,
			ManagedBy:     "juggernaut",
			Scope:         opts.Scope,
			AppliedAt:     time.Now().UTC().Format(time.RFC3339),
			Opusplan:      opts.Opusplan,
			Use1M:         opts.Use1M,
			UseMantle:     opts.UseMantle,
			MantleURL:     opts.MantleURL,
			Effort:        opts.Effort,
		},
	}, nil
}

func (b *Block) NativeKeys() NativeKeys {
	model := ""
	if b.Meta.Opusplan {
		model = "opusplan"
	}
	return NativeKeys{
		Model: model,
		ModelOverrides: map[string]string{
			"opus":   b.Models.Opus,
			"sonnet": b.Models.Sonnet,
			"haiku":  b.Models.Haiku,
		},
		Env: b.Env,
	}
}
