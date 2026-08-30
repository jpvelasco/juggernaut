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

// warnf prints a formatted warning to stderr. All non-error warnings in the
// cmd package use this instead of bare fmt.Fprintf(os.Stderr, "Warning: …").
func warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
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

// toMap delegates to the single shared implementation in internal/provider.
func toMap(v any) (map[string]any, error) {
	return provider.ToMap(v)
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
	var po provider.Options
	po.FromSchemaOptions(o)
	return po
}

// validateAuthFlag rejects unknown --auth values up front. Every downstream
// consumer compares the mode by exact string, so a typo would otherwise fall
// into the IAM path, silently drop --bedrock-key, and still write a config
// reporting success.
func validateAuthFlag() error {
	if applyFlags.auth == "" ||
		applyFlags.auth == authmode.IAM ||
		authmode.IsBedrockAPIKey(applyFlags.auth) {
		return nil
	}
	return fmt.Errorf("unknown --auth mode %q; valid modes are %q and %q",
		applyFlags.auth, authmode.IAM, authmode.BedrockAPIKey)
}

func resolveOpusplanConflict() error {
	if applyFlags.noOpusplan && applyFlags.opusplan {
		return fmt.Errorf("--no-opusplan cannot be combined with --opusplan")
	}
	return nil
}

func resolveApplyInputs(home string, bCfg *bedrock.Config, prov provider.Provider) (authMode, region string, opusplan bool, err error) {
	region = applyFlags.region
	if region == "" {
		region = bCfg.Defaults.Region
	}
	opusplan = applyFlags.opusplan

	existing, isReapply, err := detectReapplyConfig(prov, home, applyFlags.scope)
	if err != nil {
		return "", "", false, err
	}

	if isReapply {
		return resolveReApplyInputs(bCfg, prov, authMode, region, opusplan, existing)
	}

	authMode = resolveAuthMode(applyFlags.auth, prov, bCfg, nil)
	if authMode != "" {
		return authMode, region, opusplan, nil
	}
	return promptApplyInputs(bCfg, region, opusplan)
}

// detectReapplyConfig reads the provider's config file and checks whether
// Juggernaut already owns it. Returns the existing config map, a re-apply flag,
// and any error. An empty or missing config is not a re-apply.
func detectReapplyConfig(prov provider.Provider, home, scope string) (existing map[string]any, isReapply bool, err error) {
	existing, err = readProviderConfig(prov, home, scope)
	if err != nil {
		return nil, false, fmt.Errorf("checking existing configuration: %w", err)
	}
	if len(existing) == 0 {
		return nil, false, nil
	}
	return existing, prov.OwnsConfig(existing), nil
}

// resolveAuthMode determines the authentication mode through the resolution
// chain: flag value → provider default (BedrockAPIKey for Mantle-only CLIs) →
// existing juggernaut block → bedrock config default.
func resolveAuthMode(flagValue string, prov provider.Provider, bCfg *bedrock.Config, existing map[string]any) string {
	if flagValue != "" {
		return flagValue
	}
	if !prov.Supports(provider.CapNativeAuth) {
		return authmode.BedrockAPIKey
	}
	if jBlock, ok := existing["juggernaut"].(map[string]any); ok {
		if auth, ok := jBlock["auth"].(map[string]any); ok {
			if mode, ok := auth["mode"].(string); ok && mode != "" {
				return mode
			}
		}
	}
	if bCfg != nil {
		return bCfg.Defaults.AuthMode
	}
	return ""
}

// resolveReApplyInputs preserves settings from an existing Juggernaut-managed
// config when re-applying. Reads auth mode and permission mode from the
// juggernaut block and native permissions block, falling back to the global
// default for auth mode when not supplied as a flag.
func resolveReApplyInputs(bCfg *bedrock.Config, prov provider.Provider, authMode, region string, opusplan bool, existing map[string]any) (string, string, bool, error) {
	authMode = resolveAuthMode(authMode, prov, bCfg, existing)
	if jBlock, ok := existing["juggernaut"].(map[string]any); ok {
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
		applyFlags.mode = effectivePermissionMode(existing, "")
	}
	return authMode, region, opusplan, nil
}

// promptApplyInputs launches the interactive first-time setup form.
// Returns auth mode, region, opusplan, and error.
func promptApplyInputs(bCfg *bedrock.Config, region string, opusplan bool) (authMode, resolvedRegion string, resolvedOpusplan bool, err error) {
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
				Value(&resolvedRegion),
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
				Title("Enable opusplan? (routes planning to Opus, execution to Sonnet)").
				Value(&resolvedOpusplan),
		),
	)
	if err = form.Run(); err != nil {
		return "", resolvedRegion, resolvedOpusplan, err
	}
	if permMode != "" && applyFlags.mode == "" {
		applyFlags.mode = permMode
	}
	return authMode, resolvedRegion, resolvedOpusplan, nil
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
		warnf("could not read keychain (will prompt for key): %v", err)
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

	title := prov.DisplayName()
	fmt.Printf("Would write juggernaut config to %s\n", path)
	fmt.Printf("Would install Juggernaut %s activation blocks in shell profiles\n", title)
	// Legacy v4.2.6 launcher-artifact recovery is Claude-specific.
	if prov.Name() == "claude" {
		fmt.Printf("Would recover known v4.2.6 launcher artifacts in %s\n", activation.DefaultBinDir(home))
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

	persistRuntimeState(home, prov, authMode, plan)
	installActivation(home, prov)
	reportLegacyRecovery(home)
	fmt.Println("Configuration written successfully.")
	for _, w := range plan.Warnings {
		fmt.Printf("⚠ %s\n", w)
	}
	if prov.Supports(provider.CapAutoMode) {
		warnAutoModeModel(block)
	}
	return nil
}

// persistRuntimeState retains only provider-generated, non-secret user-scope
// launch data. Project applies must not create a global fallback.
func persistRuntimeState(home string, prov provider.Provider, authMode string, plan provider.ConfigPlan) {
	spec := prov.LaunchSpec()
	if applyFlags.scope != "user" || !spec.PersistRuntimeState {
		return
	}
	env := make(map[string]string, len(plan.RuntimeEnv)+len(spec.StaticEnv))
	for k, v := range plan.RuntimeEnv {
		if !strings.EqualFold(k, spec.TokenEnvVar) {
			env[k] = v
		}
	}
	for k, v := range spec.StaticEnv {
		env[k] = v
	}
	if err := activation.SaveRuntimeState(home, prov.Name(), activation.RuntimeState{
		AuthMode: authMode,
		Env:      env,
	}); err != nil {
		// Never retain stale fallback state after an auth-mode change.
		_ = activation.RemoveRuntimeState(home, prov.Name())
		warnf("could not save runtime fallback: %v", err)
	}
}

// warnAutoModeModel handles the two --mode=auto outcomes. If at least one
// configured model can use auto mode (AutoModeAvailable), Juggernaut has enabled
// it — print how to actually reach it, since auto only appears in the Shift+Tab
// cycle when the ACTIVE session model is capable (Sonnet 5 / Opus 4.7 or later),
// not when the Sonnet-tier default is active. If NO configured model is capable,
// warn that auto can't be enabled at all. Only relevant when --mode=auto was
// requested.
func warnAutoModeModel(block *schema.Block) {
	if block.Meta.PermissionMode != "auto" {
		return
	}
	fmt.Println()
	if block.AutoModeAvailable() {
		fmt.Println("ℹ Auto mode is enabled (CLAUDE_CODE_ENABLE_AUTO_MODE=1).")
		fmt.Println("  On Bedrock it appears in the Shift+Tab cycle only while your active session")
		fmt.Println("  model is Sonnet 5, or Opus 4.7 or later — not the Sonnet-tier default. Run")
		fmt.Println("  `claude --model opus` (or `/model opus` in a session) to use it. Requires")
		fmt.Println("  Claude Code v2.1.158+.")
		return
	}
	fmt.Println("⚠ Auto mode cannot be enabled: none of the configured models support it.")
	fmt.Println("  On Bedrock auto mode requires Sonnet 5, or Opus 4.7 or later (Claude Code")
	fmt.Println("  v2.1.158+). Configure one of those (e.g. keep the default Opus alias) and")
	fmt.Println("  re-run with --mode=auto.")
}

func installActivation(home string, prov provider.Provider) {
	begin, end := prov.ActivationMarkers()
	paths, err := activation.InstallWith(home, activation.InstallOptions{
		Spec: activation.CLISpec{Name: prov.Name(), Begin: begin, End: end},
	})
	if err != nil {
		warnf("could not install shell activation: %v", err)
		return
	}
	title := prov.DisplayName()
	if len(paths) == 0 {
		fmt.Printf("  ✓ %s shell activation already up to date\n", title)
		return
	}
	fmt.Printf("  ✓ Updated %s activation in %d shell profile(s)\n", title, len(paths))
}

func reportLegacyRecovery(home string) {
	binDir := activation.DefaultBinDir(home)
	actions, err := activation.RecoverLegacyArtifacts(binDir)
	if err != nil {
		warnf("could not recover legacy artifacts: %v", err)
		return
	}
	for _, a := range actions {
		fmt.Printf("  ✓ %s: %s\n", a.Action, a.Path)
	}
}
