package provider

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// ToMap serializes any value to a generic map via JSON round-trip.
// Used by providers to turn typed structs into ConfigPlan.Keys content.
// cmd/helpers.go delegates to this function — there should be only one
// implementation across the project.
func ToMap(v any) (map[string]any, error) {
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

// juggernautAuthBlock is the shared non-Claude auth metadata. Grok keeps it
// in-file; Codex and OpenCode keep it in sidecars for launch to read.
func juggernautAuthBlock(opts Options, region string) (map[string]any, error) {
	endpoint := opts.Endpoint
	if endpoint == bedrock.EndpointRuntime {
		endpoint = ""
	}
	block := &schema.Block{
		Auth: schema.Auth{
			Mode:     opts.AuthMode,
			Region:   region,
			Profile:  opts.Profile,
			Endpoint: endpoint,
		},
		Meta: schema.Meta{
			SchemaVersion: schema.SchemaVersion,
			Version:       opts.Version,
			ManagedBy:     "juggernaut",
			Scope:         opts.Scope,
			AppliedAt:     fmt.Sprintf("%d", time.Now().Unix()),
		},
	}
	m, err := ToMap(block)
	if err != nil {
		return nil, fmt.Errorf("serialize juggernaut block: %w", err)
	}
	return m, nil
}

// Options are the apply-time inputs a Provider turns into a ConfigPlan. It is a
// neutral, CLI-agnostic struct: cmd/ populates it, and each provider maps it to
// its own config shape internally (claude.go passes SchemaOpts through to
// schema.Build; codex/grok/opencode use the flat fields below).
type Options struct {
	// SchemaOpts carries the original schema.Options for the Claude provider.
	// The Claude provider passes this directly to schema.Build, avoiding a
	// field-by-field round-trip. Non-Claude providers ignore this field.
	SchemaOpts schema.Options
	// Region is the resolved AWS region. Shared by all providers.
	Region string
	// Endpoint is "runtime" (the default) or "mantle". Only providers that
	// advertise CapMantleEndpoint receive mantle; runtime providers ignore it.
	Endpoint string
	// RegionExplicit is true when the user passed --region, false when Region was
	// filled from the global default. Region-routed providers use it to word the
	// auto-switch message; the switch itself (to a region that actually serves
	// the model) happens either way — Juggernaut never writes a config for a
	// region that can't serve the model.
	RegionExplicit bool
	// AuthMode is the auth mode (iam or bedrock-api-key). Used by non-Claude
	// providers. Claude reads it from SchemaOpts.
	AuthMode string
	// Profile is the named AWS profile for IAM auth ("" = ambient default
	// chain). Non-Claude providers embed it in their auth plumbing (Grok) or
	// the juggernaut block; providers that don't care ignore it.
	Profile string
	// Scope is user or project scope. Used by non-Claude providers.
	Scope string
	// Version is the Juggernaut version string. Used by non-Claude providers.
	Version string
	Model   string // friendly model key or override (provider-interpreted)
	// ModelCatalog is the cached or freshly discovered account/region inventory.
	// Providers decide which entries their client protocol can actually use.
	ModelCatalog []CatalogModel
	// RefreshedSources lists the source names (e.g. "foundation", "profile")
	// that were successfully refreshed for this cache entry. A source present here
	// but absent from ModelCatalog means the refresh returned zero entries —
	// which is distinct from "never refreshed" and still counts as proof that
	// the model is unavailable.
	RefreshedSources []string
	// Existing is the provider's current parsed config for the target scope,
	// nil when the file does not exist yet. Providers may consult it to
	// preserve user-owned values instead of overwriting them — OpenCode, for
	// example, keeps an existing root "model" default when --model is omitted,
	// because that leaf is the CLI's own native setting rather than Bedrock
	// routing.
	Existing map[string]any
}

// FromSchemaOptions stores schema.Options directly so the Claude provider can
// pass it to schema.Build without a field-by-field round-trip. It also populates
// the flat fields (Region, AuthMode, Scope, Version) that non-Claude providers
// read. The remaining fields (RegionExplicit, Model, ModelCatalog,
// RefreshedSources) are the caller's responsibility.
func (o *Options) FromSchemaOptions(s schema.Options) {
	o.SchemaOpts = s
	o.Region = s.Region
	o.Endpoint = s.Endpoint
	o.AuthMode = s.AuthMode
	o.Profile = s.Profile
	o.Scope = s.Scope
	o.Version = s.Version
}

// CatalogModel is the SDK-independent model fact passed from discovery into a
// provider. Source is "foundation" or "profile"; "mantle" appears only in
// legacy caches, which are dropped on load.
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
// to no cache, so a partial refresh never produces a false warning from a
// foreign source. When refreshedSources is non-nil, a source present
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

// catalogUnavailableWarning returns a warning message when the provider's
// catalog is present but the selected model was not returned as available.
// The suffix parameter allows providers to customize the action text.
// Returns an empty string when the catalog is absent or the model is available.
func catalogUnavailableWarning(models []CatalogModel, selectedID, region, suffix string, p CatalogProvider, refreshedSources []string) string {
	if p == nil {
		return ""
	}
	hasRelevant, selectedAvailable := catalogSelectionState(models, selectedID, p, refreshedSources)
	if !hasRelevant || selectedAvailable {
		return ""
	}
	return fmt.Sprintf(
		"model %q was not returned as ACTIVE and available in %s; %s",
		selectedID, region, suffix)
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
	// (IAM/SSO via SigV4 on the native bedrock-runtime path). Every v6 provider
	// supports this; resolveAuthMode falls back to a bearer token only for a
	// provider that does not.
	CapNativeAuth
	// CapMantleEndpoint: the CLI can opt in to the bedrock-mantle endpoint.
	// Claude Code has first-class dual-endpoint support (CLAUDE_CODE_USE_MANTLE);
	// Grok uses a mantle base_url + Responses wire API with API-key auth.
	// Codex and OpenCode use their native mantle Responses providers.
	CapMantleEndpoint
)

// ConfigPlan is what a Provider persists to its config file at apply time.
type ConfigPlan struct {
	// ResolvedRegion is the effective provider region after model routing. It
	// keeps separately persisted auth metadata aligned with the native config.
	ResolvedRegion string
	Keys           map[string]any // merged into the config file (provider's format)
	ManagedKeys    []string       // top-level keys Juggernaut owns → removed on uninstall
	Warnings       []string       // actionable heads-ups (region/model notes, auto-mode gating)
	// RuntimeEnv is the non-secret environment that may be retained separately
	// when a provider opts into runtime-state fallback.
	RuntimeEnv map[string]string
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

// LaunchSpec is what the shell wrapper injects at launch time. Providers whose
// vendor-owned config may be reset can retain non-secret runtime state; bearer
// tokens are always fetched at launch and never persisted here.
type LaunchSpec struct {
	// PrepareArgs adapts a managed launch to the resolved CLI binary. It must
	// preserve user arguments and leave unmanaged launches unchanged.
	PrepareArgs         func(binary string, args []string) []string
	TokenEnvVar         string            // env var the keychain bearer token is injected into
	StaticEnv           map[string]string // static enable-flags, e.g. CLAUDE_CODE_USE_BEDROCK=1
	NeedsToken          bool              // true when the launch wrapper must inject the bearer token
	PersistRuntimeState bool              // retain user-scope auth + RuntimeEnv as a drift fallback
	ExtraArgs           []string          // extra args appended to the real CLI invocation (future)
}
