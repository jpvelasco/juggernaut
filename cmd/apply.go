// Package cmd implements the juggernaut CLI commands.
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/spf13/cobra"
)

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
	effort         string
	opusplan       bool
	noOpusplan     bool
	no1m           bool
	mantle         bool
	noMantle       bool
	mantleURL      string
	scope          string
	dryRun         bool
	skipPreflight  bool
	storage        string
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
	f.StringVar(&applyFlags.effort, "effort", "high", "effort level: low|medium|high|xhigh|max")
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
	f.BoolVar(&applyFlags.skipPreflight, "skip-preflight", false, "skip dependency checks")
	f.StringVar(&applyFlags.storage, "storage", "keychain", "credential storage: keychain|dpapi|profile")
	f.StringVar(&applyFlags.mode, "mode", "", "permission mode: default|acceptEdits|plan|auto|dontAsk|bypassPermissions")
	f.BoolVar(&applyFlags.alwaysThinking, "always-thinking", false, "enable extended thinking by default")
	f.StringVar(&applyFlags.serviceTier, "service-tier", "", "Bedrock service tier: default|flex|priority")

	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		return err
	}

	storageChanged := cmd.Flags().Changed("storage")
	authMode, region, opusplan, err := resolveApplyInputs(home, bCfg, storageChanged)
	if err != nil {
		return err
	}

	backend, err := keychain.Resolve(applyFlags.storage, home)
	if err != nil {
		return err
	}
	token, err := resolveCredential(home, authMode, backend)
	if err != nil {
		return err
	}
	useMantle, err := resolveMantle()
	if err != nil {
		return err
	}

	opusModel := applyFlags.opusModel
	sonnetModel := applyFlags.sonnetModel
	haikuModel := applyFlags.haikuModel
	if applyFlags.model != "" {
		opusModel = applyFlags.model
		sonnetModel = applyFlags.model
		haikuModel = applyFlags.model
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
		Opusplan:       opusplan,
		Use1M:          !applyFlags.no1m,
		UseMantle:      useMantle,
		MantleURL:      applyFlags.mantleURL,
		Storage:        applyFlags.storage,
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
	return commitApply(home, authMode, token, block, backend)
}

func resolveMantle() (bool, error) {
	if applyFlags.noMantle && (applyFlags.mantle || applyFlags.mantleURL != "") {
		return false, fmt.Errorf("--no-mantle cannot be combined with --mantle or --mantle-url")
	}
	return applyFlags.mantle || applyFlags.mantleURL != "", nil
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

func commitApply(home, authMode, token string, block *schema.Block, backend keychain.Backend) error {
	native := block.NativeKeys()

	if authmode.IsBedrockAPIKey(authMode) && token != "" {
		if err := backend.Set(token); err != nil {
			if errors.Is(err, keychain.ErrCredentialTooBig) {
				return fmt.Errorf("API key too large for %s storage (OS credential store caps blobs at ~2560 bytes); re-run with --storage=dpapi (Windows) or --storage=profile", storageName(applyFlags.storage))
			}
			return fmt.Errorf("storing API key: %w", err)
		}
		// Clear any credential left in a previously-configured backend so
		// switching --storage does not orphan a stale key.
		if err := keychain.ClearOthers(applyFlags.storage, home); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not clear previous credential storage: %v\n", err)
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
		"effortLevel":           native.EffortLevel,
		"alwaysThinkingEnabled": native.AlwaysThinking,
		"skipWebFetchPreflight": native.SkipWebFetchPreflight,
		"permissions":           native.Permissions,
	}
	if err := mgr.MergeJuggernautBlock(blockMap, native.Env, nativeKeys); err != nil {
		return err
	}

	reportLegacyRecovery(home)
	installActivation(home)
	fmt.Println("Configuration written successfully.")
	return nil
}

func reportLegacyRecovery(home string) {
	actions, err := activation.RecoverLegacyArtifacts(activation.DefaultBinDir(home))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not recover legacy launcher artifacts: %v\n", err)
		return
	}
	for _, action := range actions {
		fmt.Printf("  ✓ %s: %s\n", action.Action, action.Path)
	}
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

func resolveApplyInputs(home string, bCfg *bedrock.Config, storageChanged bool) (authMode, region string, opusplan bool, err error) {
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
				// Preserve the configured storage backend when --storage is not
				// supplied, so a bare re-apply doesn't reset it to the keychain
				// default (which would also wipe the prior backend via ClearOthers).
				if !storageChanged {
					if auth, ok := jBlock["auth"].(map[string]any); ok {
						if st, ok := auth["storage"].(string); ok && st != "" {
							applyFlags.storage = st
						}
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

// storageName returns a human-readable name for a storage mode for messages.
func storageName(mode string) string {
	if mode == "" {
		return "keychain"
	}
	return mode
}

func resolveCredential(home, authMode string, backend keychain.Backend) (string, error) {
	if !authmode.IsBedrockAPIKey(authMode) {
		return "", nil
	}
	if applyFlags.bedrockKey != "" {
		return applyFlags.bedrockKey, nil
	}
	store := storageName(applyFlags.storage)
	token, err := backend.Get()
	if err != nil {
		if applyFlags.preserveKey {
			return "", fmt.Errorf("reading existing key: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Warning: could not read %s (will prompt for key): %v\n", store, err)
	} else if token != "" {
		return token, nil
	}

	// Nothing in the configured backend — try importing a v3-era credential
	// (e.g. a Windows Credential Manager UTF-16 entry or a profile/DPAPI file
	// left by an older install) before prompting or failing.
	if source, merr := keychain.MigrateInto(backend, home); merr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not migrate legacy credential: %v\n", merr)
	} else if source != "" {
		if migrated, gerr := backend.Get(); gerr == nil && migrated != "" {
			fmt.Printf("  ✓ Migrated Bedrock API key from legacy %s storage\n", source)
			return migrated, nil
		}
	}

	if applyFlags.preserveKey {
		return "", fmt.Errorf("no existing key found in %s; re-run without --preserve-key to enter one", store)
	}
	var input string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Bedrock API key").
				EchoMode(huh.EchoModePassword).
				Value(&input),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return input, nil
}
