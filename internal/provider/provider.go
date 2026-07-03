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

import "fmt"

// Provider describes one coding CLI that Juggernaut can configure for Bedrock.
type Provider interface {
	// Name is the canonical --cli identifier ("claude", "codex", ...).
	Name() string

	// BinaryNames lists the real executable names to resolve on PATH when the
	// launch wrapper execs the underlying CLI, most-preferred first.
	BinaryNames() []string

	// ConfigFormatName reports the on-disk config encoding ("json", "toml").
	ConfigFormatName() string

	// NativeManagedKeys lists the top-level config keys Juggernaut fully owns
	// (replaced on apply, removed on uninstall).
	NativeManagedKeys() []string

	// ActivationMarkers returns the begin/end comment markers delimiting this
	// CLI's managed shell-activation block.
	ActivationMarkers() (begin, end string)

	// BedrockEnvVar is the env var (key,value) the launcher sets to route this
	// CLI through Bedrock.
	BedrockEnvVar() (key, value string)
}

// registry holds every known provider by name.
var registry = map[string]Provider{}

func register(p Provider) { registry[p.Name()] = p }

func init() {
	register(claude{})
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
