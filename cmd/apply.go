// Package cmd implements the juggernaut CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/jpvelasco/juggernaut/internal/migrate"
	"github.com/jpvelasco/juggernaut/internal/schema"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Configure Claude Code to use Amazon Bedrock",
	RunE:  runApply,
}

var applyFlags struct {
	auth          string
	bedrockKey    string
	preserveKey   bool
	region        string
	model         string
	opusModel     string
	sonnetModel   string
	haikuModel    string
	effort        string
	opusplan      bool
	noOpusplan    bool
	use1m         bool
	no1m          bool
	noMantle      bool
	mantleURL     string
	scope         string
	dryRun        bool
	skipPreflight bool
	storage       string
}

func init() {
	f := applyCmd.Flags()
	f.StringVar(&applyFlags.auth, "auth", "", "authentication mode: iam or bedrock-api-key")
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
	f.BoolVar(&applyFlags.use1m, "1m-context", true, "enable 1M token context")
	f.BoolVar(&applyFlags.no1m, "no-1m-context", false, "disable 1M token context")
	f.BoolVar(&applyFlags.noMantle, "no-mantle", false, "disable Mantle routing")
	f.StringVar(&applyFlags.mantleURL, "mantle-url", "", "custom Mantle base URL")
	f.StringVar(&applyFlags.scope, "scope", "user", "settings scope: user or project")
	f.BoolVar(&applyFlags.dryRun, "dry-run", false, "preview without writing")
	f.BoolVar(&applyFlags.skipPreflight, "skip-preflight", false, "skip dependency checks")
	f.StringVar(&applyFlags.storage, "storage", "keychain", "credential storage: keychain|dpapi|profile")

	rootCmd.AddCommand(applyCmd)
}

func runApply(_ *cobra.Command, _ []string) error {
	home := homeDir()

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
		AuthMode:      authMode,
		Region:        region,
		Effort:        applyFlags.effort,
		Scope:         applyFlags.scope,
		Version:       Version,
		OpusModel:     opusModel,
		SonnetModel:   sonnetModel,
		HaikuModel:    haikuModel,
		Opusplan:      opusplan,
		Use1M:         !applyFlags.no1m,
		UseMantle:     !applyFlags.noMantle,
		MantleURL:     applyFlags.mantleURL,
		Storage:       applyFlags.storage,
		AuthValidated: true,
	}

	block, err := schema.Build(bCfg, opts)
	if err != nil {
		return err
	}
	native := block.NativeKeys()

	if applyFlags.dryRun {
		fmt.Println("Dry run — no changes written.")
		fmt.Printf("Would write juggernaut block to %s\n", settingsPath(home, applyFlags.scope))
		return nil
	}

	if authMode == "bedrock-api-key" && token != "" {
		if err := keychain.Default().Set(token); err != nil {
			return fmt.Errorf("storing API key: %w", err)
		}
	}

	mgr := config.NewManager(settingsPath(home, applyFlags.scope))
	blockMap, err := toMap(block)
	if err != nil {
		return err
	}
	if err := mgr.MergeJuggernautBlock(blockMap, native.Env, native.Model); err != nil {
		return err
	}

	binDir := launcher.DefaultBinDir()
	if !launcher.IsInstalled(binDir) {
		if err := launcher.Install(binDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not install claude shim: %v\n", err)
		} else {
			fmt.Printf("  ✓ Installed claude shim → %s\n", binDir)
		}
	}

	fmt.Println("Configuration written successfully.")
	return nil
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

	mgr := config.NewManager(settingsPath(home, applyFlags.scope))
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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Authentication method").
				Options(
					huh.NewOption("IAM / SSO (recommended for organizations)", "iam"),
					huh.NewOption("Bedrock API key", "bedrock-api-key"),
				).
				Value(&authMode),
			huh.NewInput().
				Title("AWS region").
				Placeholder(bCfg.Defaults.Region).
				Value(&region),
			huh.NewConfirm().
				Title("Enable opusplan? (routes planning to Opus 4.8, execution to Sonnet 4.6)").
				Value(&opusplan),
		),
	)
	err = form.Run()
	return
}

func resolveCredential(authMode string) (string, error) {
	if authMode != "bedrock-api-key" {
		return "", nil
	}
	if applyFlags.bedrockKey != "" {
		return applyFlags.bedrockKey, nil
	}
	if applyFlags.preserveKey {
		token, err := keychain.Default().Get()
		if err != nil {
			return "", fmt.Errorf("reading existing key: %w", err)
		}
		if token != "" {
			return token, nil
		}
	}
	var token string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Bedrock API key").
				EchoMode(huh.EchoModePassword).
				Value(&token),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return token, nil
}

func runMigrationIfNeeded(home string, dryRun bool) error {
	state, err := migrate.Detect(home)
	if err != nil || !state.HasV3Block || state.AlreadyV4 {
		return err
	}
	if state.TooOld {
		return fmt.Errorf(
			"legacy version detected (pre-v3.2.3). Please upgrade to v3.2.3 first:\n"+
				"  curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash\n"+
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

	if state.AuthMode == "bedrock-api-key" {
		token, err := keychain.Default().Get()
		if err == nil && token != "" {
			if err := keychain.Default().Set(token); err != nil {
				fmt.Fprintln(os.Stderr, "  Warning: could not transfer bearer token:", err)
			} else {
				fmt.Println("  ✓ Bearer token transferred to go-keyring")
			}
		}
	}

	stripped := migrate.StripLauncherBlocks(home)
	for _, p := range stripped {
		fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
	}

	fmt.Println("Migration complete. No credentials were re-entered.")
	return nil
}
