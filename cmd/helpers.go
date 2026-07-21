package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// embeddedConfigBytes holds bedrock-config.json bytes injected at startup from main.go.
var embeddedConfigBytes []byte

// SetEmbeddedConfig is called by main() to inject the embedded bedrock-config.json bytes.
func SetEmbeddedConfig(data []byte) {
	embeddedConfigBytes = data
}

// loadBedrockConfig loads bedrock config, preferring the embedded bytes.
// Falls back to filesystem for tests and development builds.
func loadBedrockConfig() (*bedrock.Config, error) {
	if len(embeddedConfigBytes) > 0 {
		return bedrock.LoadBytes(embeddedConfigBytes)
	}
	// Fallback for tests and dev builds that don't set embeddedConfigBytes.
	path := findBedrockConfigFile()
	return bedrock.Load(path)
}

func findBedrockConfigFile() string {
	self, _ := os.Executable()
	if self != "" {
		if candidate := filepath.Join(filepath.Dir(self), "bedrock-config.json"); fileExists(candidate) {
			return candidate
		}
	}
	if fileExists("bedrock-config.json") {
		return "bedrock-config.json"
	}
	if fileExists("../bedrock-config.json") {
		return "../bedrock-config.json"
	}
	return "bedrock-config.json"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func homeDir() (string, error) {
	return safepath.HomeDir()
}

func settingsPath(homeDir, scope string) (string, error) {
	if scope == "project" {
		return filepath.Join(".", ".claude", "settings.json"), nil
	}
	return safepath.JoinUnder(homeDir, ".claude", "settings.json")
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("serializing block: %w", err)
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}

// fromMap is the reverse of toMap: it JSON round-trips a generic map into the
// struct pointed to by v.
func fromMap(m map[string]any, v any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("serializing map: %w", err)
	}
	return json.Unmarshal(b, v)
}

// credentialEchoMode is the EchoMode used for the Bedrock API key prompt.
// Must be textinput.EchoNone, NOT textinput.EchoPassword — EchoPassword
// breaks on Windows (keystrokes are silently dropped by the TUI input loop).
const credentialEchoMode huh.EchoMode = huh.EchoMode(textinput.EchoNone)

// ---- Shared config helpers ----

// newProviderManager resolves the config path and format for a provider and
// returns a ready-to-use Manager. Both read and write paths use this so the
// ConfigPath → FormatByName → NewManagerWithFormat sequence lives in one place.
func newProviderManager(prov provider.Provider, home, scope string) (*config.Manager, error) {
	path, err := prov.ConfigPath(home, scope)
	if err != nil {
		return nil, err
	}
	format, err := config.FormatByName(prov.ConfigFormatName())
	if err != nil {
		return nil, err
	}
	return config.NewManagerWithFormat(path, format), nil
}

// readProviderConfig reads a provider's config file for the given scope.
// Returns (nil, nil) when the file does not exist.
func readProviderConfig(prov provider.Provider, home, scope string) (map[string]any, error) {
	mgr, err := newProviderManager(prov, home, scope)
	if err != nil {
		return nil, err
	}
	return mgr.Read()
}

// resolvedScopes returns the scopes to operate on, respecting an optional
// single-scope filter. When filter is empty, both user and project are returned.
func resolvedScopes(filter string) []string {
	if filter != "" {
		return []string{filter}
	}
	return []string{"user", "project"}
}

// ---- Apply phase helpers (extracted from apply.go for size reduction) ----

// toProviderOptions maps the cmd-built schema.Options onto the CLI-neutral
// provider.Options consumed by Provider.BuildConfig.
func toProviderOptions(o schema.Options) provider.Options {
	return provider.Options{
		AuthMode:               o.AuthMode,
		Region:                 o.Region,
		Effort:                 o.Effort,
		Scope:                  o.Scope,
		Version:                o.Version,
		OpusModel:              o.OpusModel,
		SonnetModel:            o.SonnetModel,
		HaikuModel:             o.HaikuModel,
		FableModel:             o.FableModel,
		Opusplan:               o.Opusplan,
		FallbackModels:         o.FallbackModels,
		AvailableModels:        o.AvailableModels,
		EnforceAvailableModels: o.EnforceAvailableModels,
		Use1M:                  o.Use1M,
		UseMantle:              o.UseMantle,
		MantleURL:              o.MantleURL,
		AuthValidated:          o.AuthValidated,
		PermissionMode:         o.PermissionMode,
		AlwaysThinking:         o.AlwaysThinking,
		ServiceTier:            o.ServiceTier,
	}
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

func resolveApplyInputs(home string, bCfg *bedrock.Config, prov provider.Provider) (authMode, region string, opusplan bool, err error) {
	authMode = applyFlags.auth
	// Mantle-only CLIs (Codex, OpenCode, Grok) have exactly one valid auth mode:
	// the Bedrock API key. Pin it up front so neither the interactive prompt, a
	// re-apply of an existing config (which stores no auth mode), nor the global
	// default (iam) can steer them to a tokenless mode the launch can't satisfy.
	if authMode == "" && !prov.Supports(provider.CapNativeAuth) {
		authMode = authmode.BedrockAPIKey
	}
	region = applyFlags.region
	if region == "" {
		region = bCfg.Defaults.Region
	}
	opusplan = applyFlags.opusplan

	existing, herr := readProviderConfig(prov, home, applyFlags.scope)
	if herr != nil {
		err = fmt.Errorf("checking existing configuration: %w", herr)
		return
	}
	// Re-apply detection must recognize a config JUGGERNAUT wrote for THIS
	// provider (Bedrock already configured) — not merely any shared key. A plain
	// Codex config already has a top-level `model`; treating that as "configured"
	// would skip the auth prompt on a FIRST apply and default to iam, breaking
	// Mantle which requires a bearer token. OwnsConfig is the strict check.
	if prov.OwnsConfig(existing) {
		// Preserve auth mode and permission mode from the existing block when not supplied as flags.
		{
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

	// authMode is already pinned for Mantle-only CLIs (top of this function), so
	// this point is reached only by CLIs that support IAM (Claude) with no --auth.
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

func printApplyDryRun(home string, block *schema.Block, prov provider.Provider, bCfg *bedrock.Config, provOpts provider.Options) error {
	fmt.Println("Dry run — no changes written.")
	path, err := prov.ConfigPath(home, applyFlags.scope)
	if err != nil {
		return err
	}

	plan, err := prov.BuildConfig(bCfg, provOpts)
	if err != nil {
		return err
	}
	collisions, err := detectForeignCollisions(home, applyFlags.scope, prov, plan)
	if err != nil {
		return err
	}
	if len(collisions) > 0 && !applyFlags.force {
		fmt.Printf("Would refuse to apply: %s isn't managed by Juggernaut and already has:\n%s\n"+
			"Would require --force to overwrite anyway (a backup would still be made before writing)\n",
			path, formatCollisions(collisions))
		return nil
	}

	title := providerDisplayName(prov.Name())
	fmt.Printf("Would write juggernaut config to %s\n", path)
	fmt.Printf("Would install Juggernaut %s activation blocks in shell profiles\n", title)
	// Legacy v4.2.6 launcher-artifact recovery is Claude-specific.
	if prov.Name() == "claude" {
		fmt.Printf("Would recover known v4.2.6 launcher artifacts in %s\n", activation.DefaultBinDir(home))
	}
	if prov.Supports(provider.CapThinking) {
		warnMantleTradeoffs(block)
	}
	return nil
}

// detectForeignCollisions checks whether the provider's config file already holds
// content Juggernaut doesn't own and, if so, whether any of it collides with the
// leaves plan is about to write. A config Juggernaut already owns (prov.OwnsConfig)
// is a re-apply and is never checked — zero new friction for the supported re-apply
// path ("Juggernaut law": once Juggernaut owns a file, touching its own prior
// values is not a collision).
func detectForeignCollisions(home, scope string, prov provider.Provider, plan provider.ConfigPlan) ([]config.Collision, error) {
	existing, err := readProviderConfig(prov, home, scope)
	if err != nil {
		return nil, fmt.Errorf("checking existing configuration: %w", err)
	}
	if prov.OwnsConfig(existing) || len(existing) == 0 {
		return nil, nil
	}
	return config.DetectCollisions(existing, plan.Keys, prov.DeepMergeKeys(), prov.OwnedSubKeys()), nil
}

// formatCollisions renders one line per collision for the refusal error/dry-run
// report, e.g. `  env.AWS_REGION: "eu-west-1" (foreign value)`.
func formatCollisions(collisions []config.Collision) string {
	lines := make([]string, len(collisions))
	for i, c := range collisions {
		lines[i] = fmt.Sprintf("  %s: %#v (foreign value)", c.Path, c.Existing)
	}
	return strings.Join(lines, "\n")
}

func commitApply(home, authMode, token string, block *schema.Block, prov provider.Provider, bCfg *bedrock.Config, provOpts provider.Options) error {
	plan, err := prov.BuildConfig(bCfg, provOpts)
	if err != nil {
		return err
	}
	if err := plan.Validate(); err != nil {
		return fmt.Errorf("invalid config plan for %s: %w", prov.Name(), err)
	}

	path, err := prov.ConfigPath(home, applyFlags.scope)
	if err != nil {
		return err
	}

	// Collision detection must run before ANY side effect — including storing
	// a credential in the OS keychain — so a refused apply changes nothing.
	collisions, err := detectForeignCollisions(home, applyFlags.scope, prov, plan)
	if err != nil {
		return err
	}
	if len(collisions) > 0 && !applyFlags.force {
		return fmt.Errorf("refusing to modify %s — it isn't managed by Juggernaut and already has:\n%s\n"+
			"run with --force to overwrite anyway (a backup is still made before writing)",
			path, formatCollisions(collisions))
	}

	if authmode.IsBedrockAPIKey(authMode) && token != "" {
		if err := keychain.Default().SetWithFallback(token, home); err != nil {
			return fmt.Errorf("storing API key: %w", err)
		}
	}

	mgr, err := newProviderManager(prov, home, applyFlags.scope)
	if err != nil {
		return err
	}
	if err := mgr.MergeConfigPlanDeep(plan.Keys, prov.DeepMergeKeys()); err != nil {
		return err
	}

	installActivation(home, prov)
	reportLegacyRecovery(home)
	fmt.Println("Configuration written successfully.")
	for _, w := range plan.Warnings {
		fmt.Printf("⚠ %s\n", w)
	}
	if prov.Supports(provider.CapAutoMode) {
		warnAutoModeModel(block)
	}
	if prov.Supports(provider.CapThinking) {
		warnMantleTradeoffs(block)
	}
	return nil
}

// warnAutoModeModel handles the two --mode=auto outcomes. If at least one
// configured model can use auto mode (AutoModeAvailable), Juggernaut has enabled
// it — print how to actually reach it, since auto only appears in the Shift+Tab
// cycle when the ACTIVE session model is capable (Sonnet 5 / Opus 4.7 / 4.8), not
// when the Sonnet-tier default is active. If NO configured model is capable, warn
// that auto can't be enabled at all. Only relevant when --mode=auto was requested.
func warnAutoModeModel(block *schema.Block) {
	if block.Meta.PermissionMode != "auto" {
		return
	}
	fmt.Println()
	if block.AutoModeAvailable() {
		fmt.Println("ℹ Auto mode is enabled (CLAUDE_CODE_ENABLE_AUTO_MODE=1).")
		fmt.Println("  On Bedrock it appears in the Shift+Tab cycle only while your active session")
		fmt.Println("  model is Sonnet 5, Opus 4.7, or Opus 4.8 — not the Sonnet-tier default. Run")
		fmt.Println("  `claude --model opus` (or `/model opus` in a session) to use it. Requires")
		fmt.Println("  Claude Code v2.1.158+.")
		return
	}
	fmt.Println("⚠ Auto mode cannot be enabled: none of the configured models support it.")
	fmt.Println("  On Bedrock auto mode requires Sonnet 5, Opus 4.7, or Opus 4.8 (Claude Code")
	fmt.Println("  v2.1.158+). Configure one of those (e.g. keep the default Opus 4.8 alias) and")
	fmt.Println("  re-run with --mode=auto.")
}

// warnMantleTradeoffs alerts the user that routing through Mantle disables
// features that native bedrock-runtime provides.
func warnMantleTradeoffs(block *schema.Block) {
	if !block.Meta.UseMantle {
		return
	}
	fmt.Println()
	fmt.Println("⚠ Mantle routing is enabled. Compared with native Bedrock (bedrock-runtime):")
	fmt.Println("  • prompt caching is unavailable on Mantle — repeated context is re-read")
	fmt.Println("    every turn, which costs more and adds latency for large codebases.")
	fmt.Println("  • only current-generation Claude models are reachable (Sonnet 5, Opus 4.7/4.8,")
	fmt.Println("    Haiku 4.5, Fable 5); older models stay on bedrock-runtime.")
	fmt.Println("  Leave Mantle off unless you specifically need it (e.g. non-Anthropic models).")
}

func installActivation(home string, prov provider.Provider) {
	begin, end := prov.ActivationMarkers()
	paths, err := activation.InstallWith(home, activation.InstallOptions{
		Spec: activation.CLISpec{Name: prov.Name(), Begin: begin, End: end},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not install shell activation: %v\n", err)
		return
	}
	title := providerDisplayName(prov.Name())
	if len(paths) == 0 {
		fmt.Printf("  ✓ %s shell activation already up to date\n", title)
		return
	}
	fmt.Printf("  ✓ Updated %s activation in %d shell profile(s)\n", title, len(paths))
}

// providerDisplayName returns a human-facing CLI name for messages.
func providerDisplayName(name string) string {
	switch name {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	case "grok":
		return "Grok"
	default:
		return strings.Title(name) //nolint:staticcheck // ASCII CLI name fallback
	}
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
