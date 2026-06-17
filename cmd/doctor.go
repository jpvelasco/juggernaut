package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jpvelasco/juggernaut/v4/internal/config"
	"github.com/jpvelasco/juggernaut/v4/internal/doctor"
	"github.com/jpvelasco/juggernaut/v4/internal/keychain"
	"github.com/jpvelasco/juggernaut/v4/internal/launcher"
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

	token, err := keychain.Default().Get()
	switch {
	case err != nil:
		r.Check("keychain", doctor.Warn, "error reading: "+err.Error())
	case token == "":
		r.Check("keychain", doctor.OK, "no bearer token (IAM auth)")
	default:
		r.Check("keychain", doctor.OK, "bearer token found")
	}

	binDir := launcher.DefaultBinDir()
	if launcher.IsInstalled(binDir) {
		r.Check("claude shim", doctor.OK, binDir)
	} else {
		r.Check("claude shim", doctor.Warn, "not installed — run `juggernaut apply` to install")
	}
	if status, detail := claudeCommandStatus(binDir); status != "" {
		r.Check("claude command", status, detail)
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

func claudeCommandStatus(binDir string) (doctor.Status, string) {
	found, err := exec.LookPath("claude")
	if err != nil {
		return doctor.Warn, "not found on PATH — add " + binDir + " ahead of other Claude installs"
	}
	expected := launcher.ShimPath(binDir)
	if samePath(found, expected) {
		return doctor.OK, found
	}
	return doctor.Warn, "resolves to " + found + " instead of Juggernaut's shim at " + expected
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

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
