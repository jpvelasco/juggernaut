package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verify Juggernaut configuration and credentials",
	RunE:  runDoctor,
}

var doctorFlags struct {
	scope   string
	jsonOut bool
	cli     string
}

func init() {
	doctorCmd.Flags().StringVar(&doctorFlags.scope, "scope", "", "check only user or project scope")
	doctorCmd.Flags().StringVar(&doctorFlags.cli, "cli", "claude", "coding CLI to diagnose: "+provider.SupportedNames())
	doctorCmd.Flags().BoolVar(&doctorFlags.jsonOut, "json", false, "output as JSON")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	r, err := collectDoctorReport(home, doctorFlags.cli, doctorFlags.scope)
	if err != nil {
		return err
	}
	if doctorFlags.jsonOut {
		out, err := r.JSON()
		if err != nil {
			return fmt.Errorf("formatting doctor report as JSON: %w", err)
		}
		fmt.Println(string(out))
	} else {
		fmt.Print(r.String())
	}
	if r.HasFailures() {
		return fmt.Errorf("doctor found failures — see above")
	}
	return nil
}

func collectDoctorReport(home, cli, scopeFilter string) (*doctor.Report, error) {
	prov, err := provider.Get(cli)
	if err != nil {
		return nil, err
	}
	cliName := prov.Name()
	isClaude := cliName == "claude"
	r := doctor.NewReport()

	bCfg, err := loadBedrockConfig()
	if err != nil {
		r.Check("bedrock-config.json", doctor.Fail, err.Error())
	} else {
		r.Check("bedrock-config.json", doctor.OK, "loaded (v"+bCfg.Version+")")
	}

	scopes := resolvedScopes(scopeFilter)
	// Grok is always user-scoped (ConfigPath ignores scope).
	if cliName == "grok" {
		if scopeFilter == "project" {
			return nil, fmt.Errorf("grok has no project-scope config — omit --scope or use --scope=user")
		}
		scopes = []string{"user"}
	}
	for _, scope := range scopes {
		required := scope != "project" || scopeFilter == "project"
		label := providerConfigLabel(prov, scope)
		if symlinkDetail, warn := checkConfigPathSymlink(prov, home, scope); warn {
			r.Check(label+" symlink", doctor.Warn, symlinkDetail)
		}
		if status, detail := checkProviderConfigScope(prov, home, scope, required); status != "" {
			r.Check(label, status, detail)
		}
		scopeData := readProviderScopeData(prov, home, scope)
		if isClaude {
			if detail, ok := opusplanProblem(scopeData); ok {
				r.Check("top-level model ("+scope+")", doctor.Warn, detail)
			}
			checkAutoModeReadiness(r, scope, scopeData)
			checkFableDataRetention(r, scope, scopeData)
		}
	}
	if scopeFilter != "project" {
		checkRuntimeFallback(r, prov, home)
	}

	token, err := keychain.Default().GetWithFallback(home)
	switch {
	case err != nil:
		r.Check("keychain", doctor.Warn, "error reading: "+err.Error())
	case token == "":
		r.Check("keychain", doctor.OK, "no bearer token (IAM auth)")
	default:
		r.Check("keychain", doctor.OK, "bearer token found")
		checkKeyExpiry(r, token)
	}

	// Connectivity reads Claude's juggernaut.auth block; other CLIs store auth differently.
	if isClaude {
		checkConnectivity(r, home, token, scopes)
	}

	begin, end := prov.ActivationMarkers()
	actLabel := cliName + " activation"
	// Use the shared resolver to check activation on the effective profiles.
	if runtime.GOOS == "windows" {
		psResult := activation.ResolvePowerShellProfiles()
		if isClaude {
			// Claude keeps the richer multi-profile warning path.
			healthy, path, warnings := activation.CheckPowerShellActivationWith(home, &psResult)
			if healthy {
				r.Check(actLabel, doctor.OK, "active in "+path)
			} else {
				r.Check(actLabel, doctor.Warn, "not active in discovered profiles — run `juggernaut apply` and restart or source your shell")
			}
			for _, w := range warnings {
				r.Check("activation warning", doctor.Warn, w)
			}
		} else {
			activationPaths := activation.InstalledTargetsForMarkers(home, &psResult, begin, end)
			if len(activationPaths) > 0 {
				r.Check(actLabel, doctor.OK, "active in "+activationPaths[0])
			} else {
				r.Check(actLabel, doctor.Warn, "not active in discovered profiles — run `juggernaut apply --cli="+cliName+"` and restart or source your shell")
			}
		}
		// PowerShell discovery warnings are Windows-host health, not CLI-specific.
		if len(psResult.EditionsDiscovered) > 0 {
			editions := strings.Join(psResult.EditionsDiscovered, ", ")
			r.Check("powershell discovery", doctor.OK, "editions: "+editions)
		}
		for _, w := range psResult.DiscoveryWarnings {
			r.Check("powershell discovery", doctor.Warn, w)
		}
	} else {
		var activationPaths []string
		if isClaude {
			activationPaths = activation.InstalledTargets(home)
		} else {
			activationPaths = activation.InstalledTargetsForMarkers(home, nil, begin, end)
		}
		if len(activationPaths) > 0 {
			r.Check(actLabel, doctor.OK, strings.Join(activationPaths, ", "))
		} else {
			hint := "`juggernaut apply`"
			if !isClaude {
				hint = "`juggernaut apply --cli=" + cliName + "`"
			}
			r.Check(actLabel, doctor.Warn, "not installed — run "+hint+" and restart or source your shell")
		}
	}
	if status, detail := cliBinaryStatus(prov); status != "" {
		r.Check(cliName+" binary", status, detail)
	}
	if status, detail := doctorCodexVersion(prov); status != "" {
		r.Check("codex CLI version", status, detail)
	}
	if isClaude {
		legacyArtifactStatus(home, r)
	}
	return r, nil
}

func claudeCommandStatus() (doctor.Status, string) {
	found, err := activation.ResolveClaudeBinary(os.Getenv("PATH"))
	if err != nil {
		return doctor.Warn, "real Claude Code binary not found on PATH"
	}
	return doctor.OK, found
}

func cliBinaryStatus(prov provider.Provider) (doctor.Status, string) {
	names := prov.BinaryNames()
	if len(names) == 0 {
		return doctor.Warn, "no binary names registered for " + prov.Name()
	}
	if prov.Name() == "claude" {
		return claudeCommandStatus()
	}
	found, err := activation.ResolveBinary(os.Getenv("PATH"), names)
	if err != nil {
		return doctor.Warn, "real " + names[0] + " binary not found on PATH"
	}
	return doctor.OK, found
}

func providerConfigLabel(prov provider.Provider, scope string) string {
	path, err := prov.ConfigPath("", scope)
	if err != nil || path == "" {
		return prov.Name() + " config (" + scope + ")"
	}
	base := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		base = path[i+1:]
	}
	return base + " (" + scope + ")"
}

func legacyArtifactStatus(home string, r *doctor.Report) {
	binDir := activation.DefaultBinDir(home)
	artifacts := activation.DetectLegacyArtifacts(binDir)
	for _, a := range artifacts {
		r.Check("v4.2.6 artifact", doctor.Warn, fmt.Sprintf("%s: %s", a.Action, a.Path))
	}
}

// checkKeyExpiry warns when a short-term Bedrock API key is expired or close to
// expiring. Long-term keys carry no embedded expiry and are silently skipped.
// Short-term keys cannot be refreshed by Juggernaut (it holds no AWS creds), so
// the guidance is to regenerate and re-apply.
func checkKeyExpiry(r *doctor.Report, token string) {
	exp, ok := bedrock.ParseAPIKeyExpiry(token)
	if !ok {
		return // long-term or unrecognised — no expiry to report
	}
	now := time.Now().UTC()
	switch {
	case !now.Before(exp):
		r.Check("api key expiry", doctor.Warn,
			"short-term key expired at "+exp.Format(time.RFC3339)+
				" — regenerate it and run `juggernaut apply --auth="+authmode.BedrockAPIKey+"`")
	case exp.Sub(now) < time.Hour:
		r.Check("api key expiry", doctor.Warn,
			"short-term key expires soon ("+exp.Format(time.RFC3339)+
				") — regenerate it before it lapses")
	default:
		r.Check("api key expiry", doctor.OK,
			"short-term key valid until at most "+exp.Format(time.RFC3339)+
				" (may expire sooner if the generating AWS session ends first)")
	}
}

func checkConnectivity(r *doctor.Report, home, token string, scopes []string) {
	cfg, err := loadBedrockConfig()
	if err != nil {
		r.Check("bedrock connectivity", doctor.Warn, "cannot load bedrock-config.json: "+err.Error())
		return
	}

	// Use the same scope(s) as the settings checks.
	scope := scopes[0]
	prov := provider.MustGet("claude")
	path, err := prov.ConfigPath(home, scope)
	if err != nil {
		r.Check("bedrock connectivity", doctor.Warn, "cannot resolve settings path: "+err.Error())
		return
	}
	mgr := config.NewManager(path)
	data, err := mgr.Read()
	if err != nil {
		r.Check("bedrock connectivity", doctor.Warn, "cannot read settings.json: "+err.Error())
		return
	}
	block, ok := data["juggernaut"].(map[string]any)
	if !ok {
		r.Check("bedrock connectivity", doctor.Warn, "no juggernaut block in settings.json")
		return
	}
	auth, ok := block["auth"].(map[string]any)
	if !ok {
		r.Check("bedrock connectivity", doctor.Warn, "no auth config in juggernaut block")
		return
	}
	mode, ok := auth["mode"].(string)
	if !ok {
		r.Check("bedrock connectivity", doctor.Warn, "missing auth mode")
		return
	}
	region, ok := auth["region"].(string)
	if !ok {
		r.Check("bedrock connectivity", doctor.Warn, "missing auth region")
		return
	}

	modelID := cfg.Models.Haiku
	result := checkBedrockConnectivity(mode, region, modelID, token)
	status := doctor.OK
	if result.IsFailure() {
		status = doctor.Fail
	}

	elapsed := result.Elapsed.Round(time.Millisecond)
	detail := fmt.Sprintf("%s (%s, %s, %s)", result.Message, mode, region, elapsed)
	if result.StatusCode > 0 {
		detail = fmt.Sprintf("%s, HTTP %d", detail, result.StatusCode)
	}
	r.Check("bedrock connectivity", status, detail)
}

func checkBedrockConnectivity(mode, region, modelID, token string) *bedrock.ConnectivityResult {
	if authmode.IsBedrockAPIKey(mode) {
		if token == "" {
			return &bedrock.ConnectivityResult{
				OK:       false,
				AuthMode: mode,
				Region:   region,
				ModelID:  modelID,
				Message:  "bedrock API key not found in keychain",
			}
		}
		return bedrock.CheckAPIKeyConnectivity(token, region, modelID)
	}
	// IAM mode: check endpoint reachability (no SigV4 signing).
	return bedrock.CheckIAMConnectivity(region, modelID)
}

func opusplanProblem(data map[string]any) (string, bool) {
	if data == nil {
		return "", false
	}
	model, _ := data["model"].(string)
	if model != "opusplan" {
		return "", false
	}
	return "top-level model is set to \"opusplan\"; run `juggernaut apply` to rewrite settings.json", true
}

// checkAutoModeReadiness reports whether Claude Code's auto permission mode
// will actually work for scope's persisted config, without requiring a
// re-apply to find out. Silent unless permissionMode=="auto" is configured —
// mirrors warnAutoModeModel's existing apply-time behavior. Reuses
// schema.Block.AutoModeUsable()/AutoModeAvailable() directly so this check
// can never drift out of sync with real auto-mode capability logic.
func checkAutoModeReadiness(r *doctor.Report, scope string, data map[string]any) {
	if data == nil {
		return
	}
	juggernautMap, ok := data["juggernaut"].(map[string]any)
	if !ok {
		return
	}
	var block schema.Block
	if err := fromMap(juggernautMap, &block); err != nil {
		r.Check("auto-mode readiness ("+scope+")", doctor.Warn,
			"could not parse juggernaut block: "+err.Error()+"; re-run `juggernaut apply` to repair")
		return
	}
	// Claude Code's Shift+Tab writes the effective mode straight to the native
	// top-level permissions.defaultMode without touching juggernaut.meta —
	// reconcile it the same way apply's own re-apply path does, so AutoModeUsable/
	// AutoModeAvailable (which gate on Meta.PermissionMode internally) see the
	// mode that's actually active, not a stale juggernaut-block copy.
	if mode := effectivePermissionMode(data, block.Meta.PermissionMode); mode != "" {
		block.Meta.PermissionMode = mode
	}
	if block.Meta.PermissionMode != "auto" {
		return
	}

	label := "auto-mode readiness (" + scope + ")"
	if block.AutoModeUsable() {
		r.Check(label, doctor.OK, "OK — active Sonnet-tier default model is auto-capable")
		return
	}
	if block.AutoModeAvailable() {
		r.Check(label, doctor.Warn, autoModeAvailableDetail(block))
		return
	}
	r.Check(label, doctor.Warn, "WARN — no configured model supports auto mode; "+
		"configure Opus 4.7 or later, or Sonnet 5, and re-run `juggernaut apply --mode=auto`")
}

// effectivePermissionMode returns the native top-level permissions.defaultMode
// if present, falling back to the juggernaut block's own copy otherwise. The
// native value is authoritative: it's what Claude Code actually reads at
// runtime, and Shift+Tab writes there directly without updating the
// juggernaut block — mirrors the reconciliation in apply.go's re-apply path.
func effectivePermissionMode(data map[string]any, juggernautMode string) string {
	if perms, ok := data["permissions"].(map[string]any); ok {
		if dmode, ok := perms["defaultMode"].(string); ok && dmode != "" {
			return dmode
		}
	}
	return juggernautMode
}

// autoModeAvailableDetail names which configured tier IS auto-capable when
// the active Sonnet-tier default isn't, mirroring warnAutoModeModel's guidance.
// Only Opus is ever named here: IsAutoModeCapableModel's own contract excludes
// Fable entirely (see its doc comment), and this function is only reached when
// AutoModeAvailable() is already true with AutoModeUsable() false — i.e. Sonnet
// isn't capable, so if Opus isn't either, no tier is, and the default applies.
func autoModeAvailableDetail(block schema.Block) string {
	if schema.IsAutoModeCapableModel(block.Models.Opus) {
		return "WARN — switch to Opus to reach auto mode (`claude --model opus`)"
	}
	return "WARN — auto mode enabled but no configured model tier supports it; " +
		"re-run `juggernaut apply --mode=auto` to reconfigure with a capable model"
}

// checkFableDataRetention warns whenever Fable is configured, on every doctor
// run — not just at apply time. There is no AWS API to read the account's
// actual provider_data_share opt-in status (checked live against AWS docs, see
// schema.FableDataRetentionWarning), so this can never resolve to OK; it exists
// so the requirement stays visible after the apply-time note has scrolled away.
func checkFableDataRetention(r *doctor.Report, scope string, data map[string]any) {
	if data == nil {
		return
	}
	juggernautMap, ok := data["juggernaut"].(map[string]any)
	if !ok {
		return
	}
	overrides, ok := juggernautMap["modelOverrides"].(map[string]any)
	if !ok {
		return
	}
	fable, _ := overrides["fable"].(string)
	if !schema.IsFable5Model(fable) {
		return
	}
	r.Check("fable data retention ("+scope+")", doctor.Warn, schema.FableDataRetentionWarning)
}

func checkSettingsScope(home, scope string, required bool) (doctor.Status, string) {
	// Back-compat helper for Claude-only call sites / tests.
	prov, err := provider.Get("claude")
	if err != nil {
		return doctor.Fail, err.Error()
	}
	return checkProviderConfigScope(prov, home, scope, required)
}

func checkProviderConfigScope(prov provider.Provider, home, scope string, required bool) (doctor.Status, string) {
	data, err := readProviderConfig(prov, home, scope)
	if err != nil {
		return doctor.Fail, err.Error()
	}
	if prov.OwnsConfig(data) {
		return doctor.OK, "juggernaut-managed Bedrock config present"
	}
	if required {
		return doctor.Fail, "juggernaut-managed config not found — run `juggernaut apply --cli=" + prov.Name() + "`"
	}
	return doctor.OK, "not configured"
}

// checkConfigPathSymlink Lstats the provider's config path for the scope and
// warns when it is a symlink: every Juggernaut write passes through the link,
// and a process that rewrites the durable target (dotfiles sync, Claude Code
// itself, a VCS checkout) can strip the managed block again — a state doctor
// would otherwise report as merely "config not found" (#454). Symlinks are
// platform-gated: Windows symlink creation requires Developer Mode or an
// admin token, so this path is untestable there (CI runs it on Linux/macOS).
func checkConfigPathSymlink(prov provider.Provider, home, scope string) (string, bool) {
	path, err := prov.ConfigPath(home, scope)
	if err != nil {
		return "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	return "config path is a symlink — writes pass through the link and the durable target may not carry (or may strip) the managed block; use a real file or keep the block in the target", true
}

func checkRuntimeFallback(r *doctor.Report, prov provider.Provider, home string) {
	if !prov.LaunchSpec().PersistRuntimeState {
		return
	}
	state, found, err := activation.LoadRuntimeState(home, prov.Name())
	label := prov.Name() + " runtime fallback"
	if err != nil {
		r.Check(label, doctor.Warn, "invalid: "+err.Error()+"; re-run `juggernaut apply --cli="+prov.Name()+"`")
		return
	}
	if !found {
		return
	}

	data, configErr := readProviderConfig(prov, home, "user")
	switch {
	case configErr != nil:
		r.Check(label, doctor.Warn, "saved for "+state.AuthMode+" auth, but user config could not be read: "+configErr.Error())
	case !prov.OwnsConfig(data):
		// The block is gone from the config: a directly-launched CLI never
		// reads the fallback, so only `juggernaut launch` is still routed
		// through Bedrock (#454).
		r.Check(label, doctor.Warn, "available for "+state.AuthMode+
			" auth, but the managed user config is missing — a directly-launched "+prov.Name()+
			" has no Bedrock env; use `juggernaut launch` or re-run `juggernaut apply --cli="+prov.Name()+"` to restore it")
	default:
		r.Check(label, doctor.OK, "saved for "+state.AuthMode+" auth")
	}
}

func readScopeData(home, scope string) map[string]any {
	prov, err := provider.Get("claude")
	if err != nil {
		return nil
	}
	return readProviderScopeData(prov, home, scope)
}

func readProviderScopeData(prov provider.Provider, home, scope string) map[string]any {
	data, _ := readProviderConfig(prov, home, scope)
	return data
}
