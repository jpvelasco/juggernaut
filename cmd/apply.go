// Package cmd implements the juggernaut CLI commands.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/spf13/cobra"
)

// credentialEchoMode is the EchoMode used for the Bedrock API key prompt.
// Must be textinput.EchoNone, NOT textinput.EchoPassword — EchoPassword
// breaks on Windows (keystrokes are silently dropped by the TUI input loop).
const credentialEchoMode huh.EchoMode = huh.EchoMode(textinput.EchoNone)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Configure Claude Code to use Amazon Bedrock",
	RunE:  runApply,
}

var applyFlags struct {
	auth           string
	bedrockKey     string
	preserveKey    bool
	region         string
	model          string
	opusModel      string
	sonnetModel    string
	haikuModel     string
	fableModel     string
	fallbackModel  string
	effort         string
	opusplan       bool
	noOpusplan     bool
	no1m           bool
	mantle         bool
	noMantle       bool
	mantleURL      string
	scope          string
	dryRun         bool
	mode           string
	alwaysThinking bool
	serviceTier    string
}

func init() {
	f := applyCmd.Flags()
	f.StringVar(&applyFlags.auth, "auth", "", "authentication mode: iam or "+authmode.BedrockAPIKey)
	f.StringVar(&applyFlags.bedrockKey, "bedrock-key", "", "Bedrock API key")
	f.BoolVar(&applyFlags.preserveKey, "preserve-key", false, "reuse existing key from keychain/env")
	f.StringVar(&applyFlags.region, "region", "", "AWS region (default: us-west-2)")
	f.StringVar(&applyFlags.model, "model", "", "override all model IDs")
	f.StringVar(&applyFlags.opusModel, "opus-model", "", "override Opus model ID")
	f.StringVar(&applyFlags.sonnetModel, "sonnet-model", "", "override Sonnet model ID")
	f.StringVar(&applyFlags.haikuModel, "haiku-model", "", "override Haiku model ID")
	f.StringVar(&applyFlags.fableModel, "fable-model", "", "override Fable model ID")
	f.StringVar(&applyFlags.fallbackModel, "fallback-model", "", "comma-separated fallback model IDs")
	f.StringVar(&applyFlags.effort, "effort", "high", "effort level: low|medium|high|xhigh|max|auto")
	f.BoolVar(&applyFlags.opusplan, "opusplan", false, "route planning to Opus, execution to Sonnet")
	f.BoolVar(&applyFlags.noOpusplan, "no-opusplan", false, "disable opusplan")
	f.BoolVar(&applyFlags.no1m, "no-1m-context", false, "disable 1M token context")
	// Deprecated: --1m-context was always the default and is now a no-op. Kept for script compatibility.
	var deprecated1m bool
	f.BoolVar(&deprecated1m, "1m-context", true, "")
	_ = f.MarkHidden("1m-context")
	f.BoolVar(&applyFlags.mantle, "mantle", false, "enable Mantle routing")
	f.BoolVar(&applyFlags.noMantle, "no-mantle", false, "disable Mantle routing (accepted for compatibility; Mantle is disabled by default)")
	f.StringVar(&applyFlags.mantleURL, "mantle-url", "", "custom Mantle base URL")
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

	rootCmd.AddCommand(applyCmd)
}

func runApply(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
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

	authMode, region, opusplan, err := resolveApplyInputs(home, bCfg)
	if err != nil {
		return err
	}

	// Skip credential resolution in dry-run mode: it can prompt interactively
	// for a Bedrock API key, and a dry-run must have no side effects. The token
	// is only consumed by commitApply, which dry-run never reaches.
	var token string
	if !applyFlags.dryRun {
		token, err = resolveCredential(authMode, home)
		if err != nil {
			return err
		}
	}
	useMantle, err := resolveMantle()
	if err != nil {
		return err
	}

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

	fallbackModels, err := parseFallbackModels(applyFlags.fallbackModel)
	if err != nil {
		return err
	}

	opts := schema.Options{
		AuthMode:       authMode,
		Region:         region,
		Effort:         applyFlags.effort,
		Scope:          applyFlags.scope,
		Version:        Version,
		OpusModel:      opusModel,
		SonnetModel:    sonnetModel,
		HaikuModel:     haikuModel,
		FableModel:     fableModel,
		FallbackModels: fallbackModels,
		Opusplan:       opusplan,
		Use1M:          !applyFlags.no1m,
		UseMantle:      useMantle,
		MantleURL:      applyFlags.mantleURL,
		AuthValidated:  true,
		PermissionMode: applyFlags.mode,
		AlwaysThinking: applyFlags.alwaysThinking,
		ServiceTier:    applyFlags.serviceTier,
	}

	block, err := schema.Build(bCfg, opts)
	if err != nil {
		return err
	}

	if applyFlags.dryRun {
		return printApplyDryRun(home)
	}
	return commitApply(home, authMode, token, block)
}

func resolveMantle() (bool, error) {
	if applyFlags.noMantle && (applyFlags.mantle || applyFlags.mantleURL != "") {
		return false, fmt.Errorf("--no-mantle cannot be combined with --mantle or --mantle-url")
	}
	return applyFlags.mantle || applyFlags.mantleURL != "", nil
}

func resolveOpusplanConflict() error {
	if applyFlags.noOpusplan && applyFlags.opusplan {
		return fmt.Errorf("--no-opusplan cannot be combined with --opusplan")
	}
	return nil
}

func printApplyDryRun(home string) error {
	fmt.Println("Dry run — no changes written.")
	path, err := settingsPath(home, applyFlags.scope)
	if err != nil {
		return err
	}
	fmt.Printf("Would write juggernaut block to %s\n", path)
	fmt.Println("Would install Juggernaut Claude activation blocks in shell profiles")
	fmt.Printf("Would recover known v4.2.6 launcher artifacts in %s\n", activation.DefaultBinDir(home))
	return nil
}

func commitApply(home, authMode, token string, block *schema.Block) error {
	native := block.NativeKeys()

	if authmode.IsBedrockAPIKey(authMode) && token != "" {
		if err := keychain.Default().SetWithFallback(token, home); err != nil {
			return fmt.Errorf("storing API key: %w", err)
		}
	}

	path, err := settingsPath(home, applyFlags.scope)
	if err != nil {
		return err
	}
	mgr := config.NewManager(path)
	blockMap, err := toMap(block)
	if err != nil {
		return err
	}
	modelOverrides := map[string]any{}
	for k, v := range native.ModelOverrides {
		modelOverrides[k] = v
	}
	nativeKeys := map[string]any{
		"model":                 native.Model,
		"modelOverrides":        modelOverrides,
		"fallbackModel":         native.FallbackModel,
		"effortLevel":           native.EffortLevel,
		"alwaysThinkingEnabled": native.AlwaysThinking,
		"skipWebFetchPreflight": native.SkipWebFetchPreflight,
		"permissions":           native.Permissions,
	}
	if err := mgr.MergeJuggernautBlock(blockMap, native.Env, nativeKeys); err != nil {
		return err
	}

	installActivation(home)
	reportLegacyRecovery(home)
	fmt.Println("Configuration written successfully.")
	warnAutoModeModel(block)
	return nil
}

func parseFallbackModels(raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model == "" {
			return nil, fmt.Errorf("--fallback-model contains an empty model ID")
		}
		models = append(models, model)
	}
	return models, nil
}

// warnAutoModeModel alerts the user when --mode=auto was requested but the
// active session model won't unlock auto mode on Bedrock. Claude Code only
// offers auto mode there when the model is Opus 4.7/4.8, so with Juggernaut's
// default Sonnet model the Shift+Tab cycle hides auto entirely.
func warnAutoModeModel(block *schema.Block) {
	if block.Meta.PermissionMode != "auto" || block.AutoModeUsable() {
		return
	}
	fmt.Println()
	fmt.Println("⚠ Auto mode on Bedrock requires Opus 4.7 or 4.8 (Claude Code v2.1.158+).")
	fmt.Printf("  Your default session model is %s, which Claude Code does\n", block.Models.Sonnet)
	fmt.Println("  not support for auto mode, so auto will not appear in the Shift+Tab cycle.")
	fmt.Println("  Run Claude Code on Opus to unlock it — launch with `claude --model opus`")
	fmt.Println("  or switch with `/model opus` inside a session (your opus alias is pinned to Opus 4.8).")
}

func installActivation(home string) {
	paths, err := activation.Install(home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not install shell activation: %v\n", err)
		return
	}
	if len(paths) == 0 {
		fmt.Println("  ✓ Shell activation already up to date")
		return
	}
	fmt.Printf("  ✓ Installed Claude activation in %d shell profile(s)\n", len(paths))
}

func reportLegacyRecovery(home string) {
	binDir := activation.DefaultBinDir(home)
	actions, err := activation.RecoverLegacyArtifacts(binDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not recover legacy artifacts: %v\n", err)
		return
	}
	for _, a := range actions {
		fmt.Printf("  ✓ %s: %s\n", a.Action, a.Path)
	}
}

func resolveApplyInputs(home string, bCfg *bedrock.Config) (authMode, region string, opusplan bool, err error) {
	authMode = applyFlags.auth
	region = applyFlags.region
	if region == "" {
		region = bCfg.Defaults.Region
	}
	opusplan = applyFlags.opusplan

	path, herr := settingsPath(home, applyFlags.scope)
	if herr != nil {
		err = herr
		return
	}
	mgr := config.NewManager(path)
	has, herr := mgr.HasJuggernautBlock()
	if herr != nil {
		err = fmt.Errorf("checking existing configuration: %w", herr)
		return
	}
	if has {
		// Preserve auth mode and permission mode from the existing block when not supplied as flags.
		if existing, rerr := mgr.Read(); rerr == nil {
			if jBlock, ok := existing["juggernaut"].(map[string]any); ok {
				if authMode == "" {
					if auth, ok := jBlock["auth"].(map[string]any); ok {
						if mode, ok := auth["mode"].(string); ok && mode != "" {
							authMode = mode
						}
					}
				}
				if applyFlags.mode == "" {
					if meta, ok := jBlock["meta"].(map[string]any); ok {
						if pmode, ok := meta["permissionMode"].(string); ok && pmode != "" {
							applyFlags.mode = pmode
						}
					}
				}
			}
			// Also adopt a permission mode set outside Juggernaut (e.g. Claude
			// Code's Shift+Tab writes native permissions.defaultMode without
			// touching our meta block). Without this, a re-apply with no --mode
			// would wipe the user's externally-chosen mode.
			if applyFlags.mode == "" {
				if perms, ok := existing["permissions"].(map[string]any); ok {
					if dmode, ok := perms["defaultMode"].(string); ok && dmode != "" {
						applyFlags.mode = dmode
					}
				}
			}
		}
		if authMode == "" {
			authMode = bCfg.Defaults.AuthMode
		}
		return
	}

	if authMode != "" {
		return
	}

	permMode := applyFlags.mode
	flushConsoleInput()
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Authentication method").
				Options(
					huh.NewOption("IAM / SSO (recommended for organizations)", "iam"),
					huh.NewOption("Bedrock API key", authmode.BedrockAPIKey),
				).
				Value(&authMode),
			huh.NewInput().
				Title("AWS region").
				Placeholder(bCfg.Defaults.Region).
				Value(&region),
			huh.NewSelect[string]().
				Title("Permission mode").
				Options(
					huh.NewOption("default (prompt for each action)", "default"),
					huh.NewOption("acceptEdits (auto-approve file edits)", "acceptEdits"),
					huh.NewOption("auto (agentic safety classifier)", "auto"),
					huh.NewOption("plan (propose only, no execution)", "plan"),
				).
				Value(&permMode),
			huh.NewConfirm().
				Title("Enable opusplan? (routes planning to Opus 4.8, execution to Sonnet 4.6)").
				Value(&opusplan),
		),
	)
	if err = form.Run(); err != nil {
		return
	}
	if permMode != "" && applyFlags.mode == "" {
		applyFlags.mode = permMode
	}
	return
}

func resolveCredential(authMode string, home string) (string, error) {
	if !authmode.IsBedrockAPIKey(authMode) {
		return "", nil
	}
	if applyFlags.bedrockKey != "" {
		return applyFlags.bedrockKey, nil
	}
	token, err := keychain.Default().GetWithFallback(home)
	if err != nil {
		if applyFlags.preserveKey {
			return "", fmt.Errorf("reading existing key: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Warning: could not read keychain (will prompt for key): %v\n", err)
	} else if token != "" {
		return token, nil
	}
	if applyFlags.preserveKey {
		return "", fmt.Errorf("no existing key found in keychain; re-run without --preserve-key to enter one")
	}
	var input string
	flushConsoleInput()
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Bedrock API key").
				EchoMode(credentialEchoMode).
				DescriptionFunc(func() string {
					if input == "" {
						return "typing hidden — nothing echoed"
					}
					return "key entered"
				}, &input).
				Value(&input),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return input, nil
}
