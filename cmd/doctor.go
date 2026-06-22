package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
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
		if detail, ok := opusplanProblem(readScopeData(home, scope)); ok {
			r.Check("top-level model ("+scope+")", doctor.Warn, detail)
		}
	}

	storage := configuredStorage(home, "user")
	if s := configuredStorage(home, "project"); s != "" {
		storage = s
	}
	credLabel := "credential (" + storageName(storage) + ")"
	backend, resolveErr := keychain.Resolve(storage, home)
	switch {
	case resolveErr != nil:
		r.Check(credLabel, doctor.Warn, "error resolving storage: "+resolveErr.Error())
	default:
		token, err := backend.Get()
		switch {
		case err != nil:
			r.Check(credLabel, doctor.Warn, "error reading: "+err.Error())
		case token == "":
			r.Check(credLabel, doctor.OK, "no bearer token (IAM auth)")
		default:
			r.Check(credLabel, doctor.OK, "bearer token found")
		}
	}

	activationPaths := activation.InstalledTargets(home)
	if len(activationPaths) > 0 {
		r.Check("claude activation", doctor.OK, strings.Join(activationPaths, ", "))
	} else {
		r.Check("claude activation", doctor.Warn, "not installed — run `juggernaut apply` and restart or source your shell")
	}
	if status, detail := claudeCommandStatus(); status != "" {
		r.Check("claude binary", status, detail)
	}
	if status, detail := legacyArtifactStatus(home); status != "" {
		r.Check("v4.2.6 artifacts", status, detail)
	}
	if detected, detail := activation.DetectV3Install(activation.DefaultBinDir(home)); detected {
		r.Check("v3 install", doctor.Warn, detail)
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

func claudeCommandStatus() (doctor.Status, string) {
	found, err := activation.ResolveClaudeBinary(os.Getenv("PATH"))
	if err != nil {
		return doctor.Warn, "real Claude Code binary not found on PATH"
	}
	return doctor.OK, found
}

func legacyArtifactStatus(home string) (doctor.Status, string) {
	actions := activation.DetectLegacyArtifacts(activation.DefaultBinDir(home))
	if len(actions) == 0 {
		return doctor.OK, "no broken v4.2.6 artifacts detected"
	}
	parts := make([]string, 0, len(actions))
	for _, action := range actions {
		parts = append(parts, action.Action+": "+action.Path)
	}
	return doctor.Warn, strings.Join(parts, "; ") + " — run `juggernaut apply` to recover"
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
