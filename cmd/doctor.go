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
}

func init() {
	doctorCmd.Flags().StringVar(&doctorFlags.scope, "scope", "", "check only user or project scope")
	doctorCmd.Flags().BoolVar(&doctorFlags.jsonOut, "json", false, "output as JSON")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	r := doctor.NewReport()

	bCfg, err := loadBedrockConfig()
	if err != nil {
		r.Check("bedrock-config.json", doctor.Fail, err.Error())
	} else {
		r.Check("bedrock-config.json", doctor.OK, "loaded (v"+bCfg.Version+")")
	}

	scopes := []string{"user", "project"}
	if doctorFlags.scope != "" {
		scopes = []string{doctorFlags.scope}
	}
	for _, scope := range scopes {
		required := scope != "project" || doctorFlags.scope == "project"
		if status, detail := checkSettingsScope(home, scope, required); status != "" {
			r.Check("settings.json ("+scope+")", status, detail)
		}
		scopeData := readScopeData(home, scope)
		if detail, ok := opusplanProblem(scopeData); ok {
			r.Check("top-level model ("+scope+")", doctor.Warn, detail)
		}
		checkAutoModeReadiness(r, scope, scopeData)
		checkFableDataRetention(r, scope, scopeData)
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

	checkConnectivity(r, home, token, scopes)

	// Use the shared resolver to check activation on the effective profiles.
	if runtime.GOOS == "windows" {
		psResult := activation.ResolvePowerShellProfiles()
		healthy, path, warnings := activation.CheckPowerShellActivationWith(home, &psResult)
		if healthy {
			r.Check("claude activation", doctor.OK, "active in "+path)
		} else {
			r.Check("claude activation", doctor.Warn, "not active in discovered profiles — run `juggernaut apply` and restart or source your shell")
		}
		for _, w := range warnings {
			r.Check("activation warning", doctor.Warn, w)
		}
		// Report discovery status (reuse the already-resolved result).
		// Only emit OK when editions were actually discovered.
		if len(psResult.EditionsDiscovered) > 0 {
			editions := strings.Join(psResult.EditionsDiscovered, ", ")
			r.Check("powershell discovery", doctor.OK, "editions: "+editions)
		}
		for _, w := range psResult.DiscoveryWarnings {
			r.Check("powershell discovery", doctor.Warn, w)
		}
	} else {
		activationPaths := activation.InstalledTargets(home)
		if len(activationPaths) > 0 {
			r.Check("claude activation", doctor.OK, strings.Join(activationPaths, ", "))
		} else {
			r.Check("claude activation", doctor.Warn, "not installed — run `juggernaut apply` and restart or source your shell")
		}
	}
	if status, detail := claudeCommandStatus(); status != "" {
		r.Check("claude binary", status, detail)
	}
	legacyArtifactStatus(home, r)
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

func claudeCommandStatus() (doctor.Status, string) {
	found, err := activation.ResolveClaudeBinary(os.Getenv("PATH"))
	if err != nil {
		return doctor.Warn, "real Claude Code binary not found on PATH"
	}
	return doctor.OK, found
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
	path, err := settingsPath(home, scope)
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
		"configure Opus 4.7/4.8 or Sonnet 5 and re-run `juggernaut apply --mode=auto`")
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
	path, err := settingsPath(home, scope)
	if err != nil {
		return doctor.Fail, err.Error()
	}
	mgr := config.NewManager(path)
	has, err := mgr.HasJuggernautBlock()
	switch {
	case err != nil:
		return doctor.Fail, err.Error()
	case has:
		return doctor.OK, "juggernaut block present"
	case required:
		return doctor.Fail, "juggernaut block not found — run `juggernaut apply`"
	default:
		return doctor.OK, "not configured"
	}
}

func readScopeData(home, scope string) map[string]any {
	path, err := settingsPath(home, scope)
	if err != nil {
		return nil
	}
	mgr := config.NewManager(path)
	data, err := mgr.Read()
	if err != nil {
		return nil
	}
	return data
}
