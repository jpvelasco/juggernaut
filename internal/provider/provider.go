// Package provider abstracts the CLI-specific surface Juggernaut configures for
// Bedrock. Claude Code is the first (and, before the multi-CLI PRs, only)
// implementation; Codex, OpenCode, and Grok are added behind this interface so
// schema/config/activation stop being hardcoded to Claude Code.
//
// PR 1 scope: this interface centralizes the STRUCTURAL Claude-specific surface a
// second CLI genuinely needs — identity, binary names, config location/format,
// native managed keys, activation markers, and the launch-time Bedrock env var.
// The env-var *emission* (schema.Build) is intentionally NOT moved here yet; it
// migrates to a BuildEnv method in the Codex PR, when a real second CLI forces
// the interface shape rather than a guessed one.
package provider

import (
	"fmt"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

// Provider describes one coding CLI that Juggernaut can configure for Bedrock.
type Provider interface {
	// Name is the canonical --cli identifier ("claude", "codex", ...).
	Name() string

	// BinaryNames lists the real executable names to resolve on PATH when the
	// launch wrapper execs the underlying CLI, most-preferred first.
	BinaryNames() []string

	// ConfigFormatName reports the on-disk config encoding ("json", "toml").
	ConfigFormatName() string

	// ConfigPath returns the file this CLI's config is written to, for the given
	// home dir and scope ("user" or "project"). Each provider owns its own path
	// (Claude: ~/.claude/settings.json; Codex: ~/.codex/config.toml).
	ConfigPath(home, scope string) (string, error)

	// NativeManagedKeys lists the top-level config keys Juggernaut fully owns
	// (replaced on apply, removed on uninstall).
	NativeManagedKeys() []string

	// DeepMergeKeys lists the subset of NativeManagedKeys that are NESTED tables
	// where Juggernaut owns only its OWN sub-key (e.g. Grok's [model.<name>],
	// Codex's [model_providers.<id>], OpenCode's provider.<id>). On apply these
	// are deep-merged so a user's sibling entries survive instead of the whole
	// table being replaced. Providers whose keys are all whole-value return nil.
	DeepMergeKeys() []string

	// OwnedSubKeys maps each deep-merge key to the specific sub-keys Juggernaut
	// writes into it, so uninstall removes ONLY those (preserving a user's
	// sibling entries) instead of deleting the whole nested table. Keys not
	// listed here are removed whole-value on uninstall. Also drives apply-time
	// foreign-config collision detection (config.DetectCollisions): the same
	// sub-keys define exactly which leaves must be empty before apply is
	// allowed to touch a config it doesn't already own.
	OwnedSubKeys() map[string][]string

	// OwnsConfig reports whether the given parsed config was written by
	// Juggernaut for THIS provider (i.e. Bedrock is already configured). It must
	// be stricter than "any managed key is present": a plain user config that
	// merely shares a key name (e.g. Codex's own top-level `model`) must NOT
	// count, or a first-time apply would wrongly skip the auth prompt. Used to
	// decide whether an apply is a re-apply.
	OwnsConfig(data map[string]any) bool

	// ActivationMarkers returns the begin/end comment markers delimiting this
	// CLI's managed shell-activation block.
	ActivationMarkers() (begin, end string)

	// BuildConfig turns apply-time Options (plus the embedded Bedrock config, a
	// genuine input to config-building) into the config the Provider persists.
	BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error)

	// LaunchSpec is what the shell wrapper injects at launch time.
	LaunchSpec() LaunchSpec

	// Supports reports whether the Provider handles a given optional capability,
	// so cmd/ can gate CLI-specific flags without per-CLI branches.
	Supports(Capability) bool
}

// registry holds every known provider by name.
var registry = map[string]Provider{}

func register(p Provider) { registry[p.Name()] = p }

func init() {
	register(claude{})
	register(codex{})
	register(opencode{})
	register(grok{})
}

// Get resolves a provider by name. An empty name defaults to "claude" so every
// existing caller (which passes no --cli) keeps getting Claude Code behavior.
func Get(name string) (Provider, error) {
	if name == "" {
		name = "claude"
	}
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown CLI %q — supported: %s", name, supportedNames())
	}
	return p, nil
}

func supportedNames() string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	// Deterministic order for the error message.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
