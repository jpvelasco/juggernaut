// Package cmd implements the juggernaut CLI commands.
package cmd

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Configure Claude Code to use Amazon Bedrock",
	Args:  validateApplyArgs,
	RunE:  runApply,
}

var applyFlags struct {
	cli                    string
	auth                   string
	endpoint               string
	bedrockKey             string
	preserveKey            bool
	region                 string
	model                  string
	opusModel              string
	sonnetModel            string
	haikuModel             string
	fableModel             string
	fallbackModel          string
	availableModels        string
	enforceAvailableModels bool
	effort                 string
	opusplan               bool
	noOpusplan             bool
	no1m                   bool
	scope                  string
	dryRun                 bool
	mode                   string
	alwaysThinking         bool
	serviceTier            string
	force                  bool
}

func init() {
	f := applyCmd.Flags()
	f.StringVar(&applyFlags.cli, "cli", "claude", "coding CLI to configure: "+provider.SupportedNames())
	f.StringVar(&applyFlags.auth, "auth", "", "authentication mode: iam or "+authmode.BedrockAPIKey)
	f.StringVar(&applyFlags.endpoint, "endpoint", "", "Bedrock endpoint: runtime (default) or mantle (opt-in; Claude Code and Grok)")
	f.StringVar(&applyFlags.bedrockKey, "bedrock-key", "", "Bedrock API key")
	f.BoolVar(&applyFlags.preserveKey, "preserve-key", false, "reuse existing key from keychain/env")
	f.StringVar(&applyFlags.region, "region", "", "AWS region (default: us-west-2)")
	f.StringVar(&applyFlags.model, "model", "", "model ID or provider-specific convenience alias")
	f.StringVar(&applyFlags.opusModel, "opus-model", "", "override Opus model ID")
	f.StringVar(&applyFlags.sonnetModel, "sonnet-model", "", "override Sonnet model ID")
	f.StringVar(&applyFlags.haikuModel, "haiku-model", "", "override Haiku model ID")
	f.StringVar(&applyFlags.fableModel, "fable-model", "", "override Fable model ID")
	f.StringVar(&applyFlags.fallbackModel, "fallback-model", "", "comma-separated fallback model IDs")
	f.StringVar(&applyFlags.availableModels, "available-models", "", "comma-separated model allowlist (families, version prefixes, or full IDs) written to Claude Code's native availableModels")
	f.BoolVar(&applyFlags.enforceAvailableModels, "enforce-available-models", false, "extend --available-models to the Default model option (requires --available-models)")
	f.StringVar(&applyFlags.effort, "effort", "high", "effort level: low|medium|high|xhigh|max|auto")
	f.BoolVar(&applyFlags.opusplan, "opusplan", false, "route planning to Opus, execution to Sonnet")
	f.BoolVar(&applyFlags.noOpusplan, "no-opusplan", false, "disable opusplan")
	f.BoolVar(&applyFlags.no1m, "no-1m-context", false, "disable 1M token context")
	// Deprecated: --1m-context was always the default and is now a no-op. Kept for script compatibility.
	var deprecated1m bool
	f.BoolVar(&deprecated1m, "1m-context", true, "")
	_ = f.MarkHidden("1m-context")
	f.StringVar(&applyFlags.scope, "scope", "user", "settings scope: user or project")
	f.BoolVar(&applyFlags.dryRun, "dry-run", false, "preview without writing")
	// Deprecated: --skip-preflight never gated any check and is now a no-op.
	// Kept hidden for script compatibility.
	var deprecatedSkipPreflight bool
	f.BoolVar(&deprecatedSkipPreflight, "skip-preflight", false, "")
	_ = f.MarkHidden("skip-preflight")
	f.StringVar(&applyFlags.mode, "mode", "", "permission mode: default|acceptEdits|plan|auto|dontAsk|bypassPermissions")
	f.BoolVar(&applyFlags.alwaysThinking, "always-thinking", false, "enable extended thinking by default")
	f.StringVar(&applyFlags.serviceTier, "service-tier", "", "Bedrock service tier: default|flex|priority")
	f.BoolVar(&applyFlags.force, "force", false, "overwrite a config file Juggernaut doesn't manage, even if it already has colliding keys")

	rootCmd.AddCommand(applyCmd)
}

func validateApplyArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}

	name, value, hasValue := strings.Cut(args[0], "=")
	if hasValue && cmd.Flags().Lookup(name) != nil {
		// Redact the value for secret-bearing flags so a mistyped
		// `bedrock-key=<api key>` does not leak the credential to stderr/logs.
		if isSecretFlag(name) {
			return fmt.Errorf("unexpected argument %s=<redacted>; did you mean --%s?", name, name)
		}
		return fmt.Errorf("unexpected argument %q; did you mean --%s=%s?", args[0], name, value)
	}
	return cobra.NoArgs(cmd, args)
}

// isSecretFlag returns true for flags that carry credentials.
func isSecretFlag(name string) bool {
	return name == "bedrock-key"
}

func runApply(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	// Fail fast on a typo'd mode before reading or writing any state, so an
	// invalid value can never produce a "successful" broken config.
	if err := validateAuthFlag(); err != nil {
		return err
	}
	if err := validateEndpointFlag(); err != nil {
		return err
	}

	prov, err := provider.Get(applyFlags.cli)
	if err != nil {
		return err
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		return err
	}

	if err := resolveOpusplanConflict(); err != nil {
		return err
	}

	endpoint, err := resolveApplyEndpoint(home, prov)
	if err != nil {
		return err
	}

	authMode, region, opusplan, err := resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		return err
	}
	region = resolveEndpointRegion(endpoint, region, prov)

	var token string
	if !applyFlags.dryRun {
		token, err = resolveCredential(authMode, home)
		if err != nil {
			return err
		}
	}

	plan, err := buildApplyPlan(prov, home, region, bCfg, authMode, endpoint, opusplan)
	if err != nil {
		return err
	}

	return executeApply(home, prov, bCfg, authMode, token, plan)
}

// applyPlan holds the resolved configuration produced by buildApplyPlan, ready
// for dry-run display or commit.
type applyPlan struct {
	block    *schema.Block
	provOpts provider.Options
}

// buildApplyPlan resolves models and options, then builds the schema
// block and provider options. This is the planning phase — no side effects.
func buildApplyPlan(prov provider.Provider, home, region string, bCfg *bedrock.Config, authMode, endpoint string, opusplan bool) (*applyPlan, error) {
	models, err := resolveApplyModelsForEndpoint(prov, endpoint, home)
	if err != nil {
		return nil, err
	}

	opts := buildApplyOptions(authMode, endpoint, region, opusplan, models)
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		return nil, err
	}

	provOpts, err := buildProviderOptions(home, region, opts)
	if err != nil {
		return nil, err
	}

	return &applyPlan{block: block, provOpts: provOpts}, nil
}

// executeApply performs the final step: dry-run display or commit.
func executeApply(home string, prov provider.Provider, bCfg *bedrock.Config, authMode string, token string, plan *applyPlan) error {
	warnCodexVersion(prov)
	if applyFlags.dryRun {
		return printApplyDryRun(home, plan.block, prov, bCfg, plan.provOpts)
	}
	return commitApply(home, authMode, token, plan.block, prov, bCfg, plan.provOpts)
}

// parseCommaSeparatedModels splits a comma-separated model ID string, then
// delegates validation to schema.ValidateModelList. Returns nil for empty input.
func parseCommaSeparatedModels(raw string, flagName string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		models = append(models, strings.TrimSpace(part))
	}
	return schema.ValidateModelList(models, flagName)
}

// resolvedModels holds the model-related values extracted from apply flags.
type resolvedModels struct {
	opusModel              string
	sonnetModel            string
	haikuModel             string
	fableModel             string
	fallbackModels         []string
	availableModels        []string
	enforceAvailableModels bool
}

// resolveApplyModels extracts and validates all model-related flags.
func resolveApplyModels() (*resolvedModels, error) {
	models := &resolvedModels{}
	overlayExplicitModels(models)

	fallbackModels, err := parseCommaSeparatedModels(applyFlags.fallbackModel, "--fallback-model")
	if err != nil {
		return nil, err
	}
	availableModels, err := parseCommaSeparatedModels(applyFlags.availableModels, "--available-models")
	if err != nil {
		return nil, err
	}

	// Validate that --enforce-available-models requires --available-models to be set.
	if applyFlags.enforceAvailableModels && len(availableModels) == 0 {
		return nil, fmt.Errorf("%s", schema.ErrEnforceRequiresAvailable)
	}

	models.fallbackModels = fallbackModels
	models.availableModels = availableModels
	models.enforceAvailableModels = applyFlags.enforceAvailableModels
	return models, nil
}

// overlayExplicitModels applies explicit tier flags to m. --model wins over
// every per-tier flag, matching the documented flag precedence.
func overlayExplicitModels(m *resolvedModels) {
	if applyFlags.opusModel != "" {
		m.opusModel = applyFlags.opusModel
	}
	if applyFlags.sonnetModel != "" {
		m.sonnetModel = applyFlags.sonnetModel
	}
	if applyFlags.haikuModel != "" {
		m.haikuModel = applyFlags.haikuModel
	}
	if applyFlags.fableModel != "" {
		m.fableModel = applyFlags.fableModel
	}
	if applyFlags.model != "" {
		m.opusModel = applyFlags.model
		m.sonnetModel = applyFlags.model
		m.haikuModel = applyFlags.model
		m.fableModel = applyFlags.model
	}
}

// resolveApplyModelsForEndpoint layers mantle-specific model behavior on top of
// the normal flag resolution. Only Claude Code has mantle tier pins and
// anthropic.* model-ID rules; other mantle-capable CLIs (Grok) validate their
// own model IDs in BuildConfig. Mantle model IDs are account-allowlisted, so a
// Claude re-apply of a mantle config preserves the previous tier pins instead
// of resetting them to the runtime defaults. Explicit flags still win.
func resolveApplyModelsForEndpoint(prov provider.Provider, endpoint, home string) (*resolvedModels, error) {
	models, err := resolveApplyModels()
	if err != nil || endpoint != bedrock.EndpointMantle || prov.Name() != "claude" {
		return models, err
	}
	if err := preserveMantleModels(models, home); err != nil {
		return nil, err
	}
	if err := validateMantleModelFlags(); err != nil {
		return nil, err
	}
	return models, nil
}

// preserveMantleModels seeds m from the previous mantle config's
// modelOverrides, then re-applies explicit flags. It is a no-op unless the
// existing config is Juggernaut-managed and already opted in to mantle.
func preserveMantleModels(m *resolvedModels, home string) error {
	existing, isReapply, err := detectReapplyConfig(provider.MustGet("claude"), home, applyFlags.scope)
	if err != nil {
		return err
	}
	if !isReapply || existingEndpoint(existing) != bedrock.EndpointMantle {
		return nil
	}
	jBlock, ok := existing["juggernaut"].(map[string]any)
	if !ok {
		return nil
	}
	var block schema.Block
	if err := fromMap(jBlock, &block); err != nil {
		return fmt.Errorf("reading existing mantle model overrides: %w", err)
	}
	preserved := block.Models
	if preserved.Opus != "" {
		m.opusModel = preserved.Opus
	}
	if preserved.Sonnet != "" {
		m.sonnetModel = preserved.Sonnet
	}
	if preserved.Haiku != "" {
		m.haikuModel = preserved.Haiku
	}
	if preserved.Fable != "" {
		m.fableModel = preserved.Fable
	}
	overlayExplicitModels(m)
	return nil
}

// validateMantleModelFlags rejects runtime-format IDs when the user explicitly
// pins a model for the mantle endpoint. Mantle does not support CRIS prefixes
// or version suffixes; Claude Code routes by ID format.
func validateMantleModelFlags() error {
	if applyFlags.model != "" {
		return requireMantleModelID("--model", applyFlags.model)
	}
	for _, f := range []struct{ flag, value string }{
		{"--opus-model", applyFlags.opusModel},
		{"--sonnet-model", applyFlags.sonnetModel},
		{"--haiku-model", applyFlags.haikuModel},
		{"--fable-model", applyFlags.fableModel},
	} {
		if f.value == "" {
			continue
		}
		if err := requireMantleModelID(f.flag, f.value); err != nil {
			return err
		}
	}
	return nil
}

func requireMantleModelID(flag, value string) error {
	if bedrock.IsMantleModelID(value) {
		return nil
	}
	return fmt.Errorf("%s %q is not a mantle model ID; mantle requires a bare anthropic.* ID with no CRIS prefix or version suffix (for example anthropic.claude-sonnet-5)", flag, value)
}

// buildApplyOptions constructs the schema.Options from resolved inputs.
func buildApplyOptions(authMode, endpoint, region string, opusplan bool, models *resolvedModels) schema.Options {
	return schema.Options{
		AuthMode:               authMode,
		Region:                 region,
		Endpoint:               endpoint,
		Effort:                 applyFlags.effort,
		Scope:                  applyFlags.scope,
		Version:                Version,
		OpusModel:              models.opusModel,
		SonnetModel:            models.sonnetModel,
		HaikuModel:             models.haikuModel,
		FableModel:             models.fableModel,
		FallbackModels:         models.fallbackModels,
		AvailableModels:        models.availableModels,
		EnforceAvailableModels: models.enforceAvailableModels,
		Opusplan:               opusplan,
		Use1M:                  !applyFlags.no1m,
		AuthValidated:          true,
		PermissionMode:         applyFlags.mode,
		AlwaysThinking:         applyFlags.alwaysThinking,
		ServiceTier:            applyFlags.serviceTier,
	}
}

// buildProviderOptions constructs the provider.Options from the schema options
// that were already built for apply, plus the cached model catalog.
func buildProviderOptions(home, region string, scopts schema.Options) (provider.Options, error) {
	provOpts := toProviderOptions(scopts)
	// For non-Claude CLIs, --model is a provider model KEY (e.g. gpt-oss-120b),
	// not one of Claude's per-tier IDs. Thread the raw flag through so the
	// provider selects the right model instead of silently defaulting.
	provOpts.Model = applyFlags.model
	// Whether the region was explicitly chosen (vs filled from the global
	// default). Providers use it to word the region auto-switch message; a
	// non-serving region is overridden either way (the Iron Fist).
	provOpts.RegionExplicit = applyFlags.region != ""
	catalog, sources, err := cachedProviderCatalog(home, region)
	if err != nil {
		return provOpts, fmt.Errorf("loading cached model catalog: %w", err)
	}
	provOpts.ModelCatalog = catalog
	provOpts.RefreshedSources = sources
	return provOpts, nil
}
