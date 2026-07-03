package provider

import "fmt"

// Capability identifies an optional feature a Provider may support, so cmd/ can
// gate CLI-specific flags without hardcoding per-CLI knowledge.
type Capability int

const (
	// CapAutoMode: turn-based permission mode; on Bedrock requires an Opus 4.7/4.8
	// session model. Claude only.
	CapAutoMode Capability = iota
	// Cap1MContext: 1M-token context window (model-gated). Claude only.
	Cap1MContext
	// CapOpusplan: route planning to Opus, execution to Sonnet. Claude only.
	CapOpusplan
	// CapThinking: extended / always-on thinking. Claude only.
	CapThinking
	// CapServiceTiers: Bedrock service tier (default/flex/priority). Claude only.
	CapServiceTiers
	// CapEffortLevels: reasoning-effort levels. Claude + Codex (gpt-5.x).
	CapEffortLevels
)

// ConfigPlan is what a Provider persists to its config file at apply time.
type ConfigPlan struct {
	Keys        map[string]any // merged into the config file (provider's format)
	ManagedKeys []string       // top-level keys Juggernaut owns → removed on uninstall
	Warnings    []string       // actionable heads-ups (Mantle tradeoffs, auto-mode gating)
}

// Validate checks the plan is internally coherent before it is written, so a
// malformed provider fails loudly here rather than silently on disk. Every
// ManagedKeys entry must be non-empty and present in Keys.
func (p ConfigPlan) Validate() error {
	for _, k := range p.ManagedKeys {
		if k == "" {
			return fmt.Errorf("ConfigPlan has an empty managed key")
		}
		if _, ok := p.Keys[k]; !ok {
			return fmt.Errorf("ConfigPlan managed key %q is not present in Keys", k)
		}
	}
	return nil
}

// LaunchSpec is what the shell wrapper injects at launch time. Never persisted —
// the bearer token refreshes and must not land in a config file.
type LaunchSpec struct {
	TokenEnvVar string            // env var the keychain bearer token is injected into
	StaticEnv   map[string]string // static enable-flags, e.g. CLAUDE_CODE_USE_BEDROCK=1
	NeedsToken  bool              // Mantle: true; native-IAM path: may be false
	ExtraArgs   []string          // extra args appended to the real CLI invocation (future)
}
