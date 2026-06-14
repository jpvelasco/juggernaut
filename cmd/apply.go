// Package cmd implements the juggernaut CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/v4/internal/authmode"
	"github.com/jpvelasco/juggernaut/v4/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v4/internal/config"
	"github.com/jpvelasco/juggernaut/v4/internal/keychain"
	"github.com/jpvelasco/juggernaut/v4/internal/launcher"
	"github.com/jpvelasco/juggernaut/v4/internal/migrate"
	"github.com/jpvelasco/juggernaut/v4/internal/schema"
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
	f.StringVar(&applyFlags.effort, "effort", "xhigh", "effort level: low|medium|high|xhigh|max")
	f.BoolVar(&applyFlags.opusplan, "opusplan", false, "route planning to Opus, execution to Sonnet")
	f.BoolVar(&applyFlags.noOpusplan, "no-opusplan", false, "disable opusplan")
	f.BoolVar(&applyFlags.no1m, "no-1m-context", false, "disable 1M token context")
	// Deprecated: --1m-context was always the default and is now a no-op. Kept for script compatibility.
	var deprecated1m bool
	f.BoolVar(&deprecated1m, "1m-context", true, "")
	_ = f.MarkHidden("1m-context")
	f.BoolVar(&applyFlags.noMantle, "no-mantle", false, "disable Mantle routing")
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

func runApply(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		return err
	}

	if err := runMigrationIfNeeded(home, applyFlags.dryRun); err != nil {
		return err
	}

	authMode, region, opusplan, err := resolveApplyInputs(home, bCfg)
	if err != nil {
		return err
	}

	token, err := resolveCredential(authMode)
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
		UseMantle:      !applyFlags.noMantle,
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
	return commitApply(home, authMode, token, block)
}

func printApplyDryRun(home string) error {
	fmt.Println("Dry run — no changes written.")
	path, err := settingsPath(home, applyFlags.scope)
	if err != nil {
		return err
	}
	fmt.Printf("Would write juggernaut block to %s\n", path)
	return nil
}

func commitApply(home, authMode, token string, block *schema.Block) error {
	native := block.NativeKeys()

	if authmode.IsBedrockAPIKey(authMode) && token != "" {
		if err := keychain.Default().Set(token); err != nil {
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
	nativeKeys := map[string]any{
		"model":                 native.Model,
		"effortLevel":           native.EffortLevel,
		"alwaysThinkingEnabled": native.AlwaysThinking,
		"skipWebFetchPreflight": native.SkipWebFetchPreflight,
		"permissions":           native.Permissions,
	}
	if err := mgr.MergeJuggernautBlock(blockMap, native.Env, nativeKeys); err != nil {
		return err
	}

	installLauncherShimIfMissing()
	fmt.Println("Configuration written successfully.")
	return nil
}

func installLauncherShimIfMissing() {
	binDir := launcher.DefaultBinDir()
	if launcher.IsInstalled(binDir) {
		return
	}
	if err := launcher.Install(binDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not install claude shim: %v\n", err)
		return
	}
	fmt.Printf("  ✓ Installed claude shim → %s\n", binDir)
}

func resolveApplyInputs(home string, bCfg *bedrock.Config) (authMode, region string, opusplan bool, err error) {
	authMode = applyFlags.auth
	region = applyFlags.region
	if region == "" {
		region = bCfg.Defaults.Region
	}
	opusplan = applyFlags.opusplan

	if authMode != "" {
		return
	}

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
		// Preserve existing auth mode from the block rather than reverting to the global default.
		if existing, rerr := mgr.Read(); rerr == nil {
			if jBlock, ok := existing["juggernaut"].(map[string]any); ok {
				if auth, ok := jBlock["auth"].(map[string]any); ok {
					if mode, ok := auth["mode"].(string); ok && mode != "" {
						authMode = mode
						return
					}
				}
			}
		}
		authMode = bCfg.Defaults.AuthMode
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

func resolveCredential(authMode string) (string, error) {
	if !authmode.IsBedrockAPIKey(authMode) {
		return "", nil
	}
	if applyFlags.bedrockKey != "" {
		return applyFlags.bedrockKey, nil
	}
	token, err := keychain.Default().Get()
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

func runMigrationIfNeeded(home string, dryRun bool) error {
	state, err := migrate.Detect(home)
	if err != nil || !state.HasV3Block || state.AlreadyV4 {
		return err
	}
	if state.TooOld {
		return fmt.Errorf(
			"legacy version detected (pre-v3.2.3). Please upgrade to v3.2.3 first:\n" +
				"  curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash\n" +
				"Then re-run: juggernaut apply",
		)
	}

	fmt.Printf("Existing Juggernaut configuration detected (v%s, %s auth).\n", state.V3Version, state.AuthMode)

	if dryRun {
		fmt.Println("Dry run — migration preview only, no changes made.")
		fmt.Println("  Would: transfer bearer token to go-keyring")
		fmt.Println("  Would: remove legacy shell launcher blocks from shell profiles")
		return nil
	}

	fmt.Println("Migrating to Juggernaut v4...")

	keychainOK := true
	if authmode.IsBedrockAPIKey(state.AuthMode) {
		token, err := keychain.Default().Get()
		if err != nil {
			fmt.Fprintln(os.Stderr, "  Warning: could not read bearer token for migration:", err)
			keychainOK = false
		} else if token != "" {
			if err := keychain.Default().Set(token); err != nil {
				fmt.Fprintln(os.Stderr, "  Warning: could not transfer bearer token:", err)
				keychainOK = false
			} else {
				fmt.Println("  ✓ Bearer token transferred to go-keyring")
			}
		}
	}

	stripped := migrate.StripLauncherBlocks(home)
	for _, p := range stripped {
		fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
	}

	if !keychainOK {
		fmt.Println("Migration complete with warnings. Re-enter your credentials:")
		fmt.Println("  juggernaut apply --auth=" + authmode.BedrockAPIKey)
	} else {
		fmt.Println("Migration complete. No credentials were re-entered.")
	}
	return nil
}
