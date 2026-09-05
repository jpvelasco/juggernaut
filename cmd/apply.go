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
	f.StringVar(&applyFlags.cli, "cli", "claude", "coding CLI to configure: claude")
	f.StringVar(&applyFlags.auth, "auth", "", "authentication mode: iam or "+authmode.BedrockAPIKey)
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

	authMode, region, opusplan, err := resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		return err
	}

	var token string
	if !applyFlags.dryRun {
		token, err = resolveCredential(authMode, home)
		if err != nil {
			return err
		}
	}

	plan, err := buildApplyPlan(home, region, bCfg, authMode, opusplan)
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
func buildApplyPlan(home, region string, bCfg *bedrock.Config, authMode string, opusplan bool) (*applyPlan, error) {
	models, err := resolveApplyModels()
	if err != nil {
		return nil, err
	}

	opts := buildApplyOptions(authMode, region, opusplan, models)
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
	opusModel := applyFlags.opusModel
	sonnetModel := applyFlags.sonnetModel
	haikuModel := applyFlags.haikuModel
	fableModel := applyFlags.fableModel
	if applyFlags.model != "" {
		opusModel = applyFlags.model
		sonnetModel = applyFlags.model
		haikuModel = applyFlags.model
		fableModel = applyFlags.model
	}

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

	return &resolvedModels{
		opusModel:              opusModel,
		sonnetModel:            sonnetModel,
		haikuModel:             haikuModel,
		fableModel:             fableModel,
		fallbackModels:         fallbackModels,
		availableModels:        availableModels,
		enforceAvailableModels: applyFlags.enforceAvailableModels,
	}, nil
}

// buildApplyOptions constructs the schema.Options from resolved inputs.
func buildApplyOptions(authMode, region string, opusplan bool, models *resolvedModels) schema.Options {
	return schema.Options{
		AuthMode:               authMode,
		Region:                 region,
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
	// default). Providers auto-switch a model to a region that serves it
	// only when the region was defaulted; an explicit --region is honored.
	provOpts.RegionExplicit = applyFlags.region != ""
	catalog, sources, err := cachedProviderCatalog(home, region)
	if err != nil {
		return provOpts, fmt.Errorf("loading cached model catalog: %w", err)
	}
	provOpts.ModelCatalog = catalog
	provOpts.RefreshedSources = sources
	return provOpts, nil
}
