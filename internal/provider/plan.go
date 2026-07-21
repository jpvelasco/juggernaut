package provider

import (
	"encoding/json"
	"fmt"
)

// toMapViaJSON serializes a struct to a generic map via JSON round-trip (mirrors
// cmd.toMap), so a typed block becomes ConfigPlan.Keys content.
func toMapViaJSON(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("serializing block: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Options are the apply-time inputs a Provider turns into a ConfigPlan. It is a
// neutral, CLI-agnostic struct: cmd/ populates it, and each provider maps it to
// its own config shape internally (claude.go maps it to schema.Options; codex.go
// selects a per-model Mantle block). Kept separate from schema.Options so the
// provider package stays decoupled from Claude's schema — the mapping lives only
// in claude.go.
type Options struct {
	AuthMode string
	Region   string
	// RegionExplicit is true when the user passed --region, false when Region was
	// filled from the global default. Mantle providers use this to decide whether
	// they may auto-switch a model to a region that actually serves it (default)
	// or must honor the user's explicit choice (and only warn).
	RegionExplicit         bool
	Effort                 string
	Scope                  string
	Version                string
	Model                  string // friendly model key or override (provider-interpreted)
	OpusModel              string
	SonnetModel            string
	HaikuModel             string
	FableModel             string
	Opusplan               bool
	FallbackModels         []string
	AvailableModels        []string
	EnforceAvailableModels bool
	Use1M                  bool
	UseMantle              bool
	MantleURL              string
	AuthValidated          bool
	PermissionMode         string
	AlwaysThinking         bool
	ServiceTier            string
	Route                  string // "mantle" (default) or "native"
	// ModelCatalog is the cached or freshly discovered account/region inventory.
	// Providers decide which entries their client protocol can actually use.
	ModelCatalog []CatalogModel
	// RefreshedSources lists the source names (e.g. "native", "mantle") that
	// were successfully refreshed for this cache entry. A source present here
	// but absent from ModelCatalog means the refresh returned zero entries —
	// which is distinct from "never refreshed" and still counts as proof that
	// the model is unavailable.
	RefreshedSources []string
}

// CatalogModel is the SDK-independent model fact passed from discovery into a
// provider. Source is "foundation", "profile", or "mantle".
type CatalogModel struct {
	ID           string
	Status       string
	Availability string
	Provider     string
	Source       string
}

// IsAvailable reports whether AWS positively marked a model as available or
// the older cache entry predates availability metadata. Only an explicit
// NOT_AVAILABLE classification rejects a model.
func (m CatalogModel) IsAvailable() bool { return m.Availability != "NOT_AVAILABLE" }

// ModelSupport explains whether a provider may safely expose a catalog entry.
type ModelSupport struct {
	Supported bool
	Reason    string
}

// catalogSelectionState reports whether a cache contains the provider's
// relevant source and whether the selected model is usable according to that
// provider. A cache containing only another provider's sources is equivalent
// to no cache, so a partial native-only refresh never produces a false Mantle
// warning (and vice versa). When refreshedSources is non-nil, a source present
// there but absent from the model list still counts as "refreshed but empty" —
// meaning the model is provably unavailable rather than merely unrefreshed.
func catalogSelectionState(models []CatalogModel, selectedID string, p CatalogProvider, refreshedSources []string) (hasRelevantSource, selectedAvailable bool) {
	sources := make(map[string]bool)
	for _, source := range p.CatalogSources() {
		sources[source] = true
	}
	// A refreshed-but-empty source still counts as "relevant" — it proves the
	// model is unavailable rather than merely unrefreshed.
	for _, src := range refreshedSources {
		if sources[src] {
			hasRelevantSource = true
		}
	}
	for _, model := range models {
		if !sources[model.Source] {
			continue
		}
		hasRelevantSource = true
		if model.ID == selectedID && p.SupportsModel(model).Supported {
			selectedAvailable = true
		}
	}
	return hasRelevantSource, selectedAvailable
}

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
	// CapNativeAuth: the CLI can authenticate to Bedrock WITHOUT a bearer token
	// (IAM/SSO via SigV4 on the native bedrock-runtime path). Claude supports this;
	// the Mantle-only CLIs (Codex, OpenCode, Grok) do NOT — Mantle requires a
	// bearer token, so apply must reject --auth=iam for them.
	CapNativeAuth
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
