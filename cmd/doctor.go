package cmd

import (
	"fmt"

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
	home := homeDir()
	r := doctor.NewReport()

	bCfg, err := loadBedrockConfig()
	if err != nil {
		r.Check("bedrock-config.json", doctor.Fail, err.Error())
	} else {
		r.Check("bedrock-config.json", doctor.OK, "loaded (v"+bCfg.Version+")")
	}

	path, perr := settingsPath(home, "user")
	if perr != nil {
		r.Check("settings.json", doctor.Fail, perr.Error())
	} else {
		mgr := config.NewManager(path)
		has, err := mgr.HasJuggernautBlock()
		switch {
		case err != nil:
			r.Check("settings.json", doctor.Fail, err.Error())
		case !has:
			r.Check("settings.json", doctor.Fail, "juggernaut block not found — run `juggernaut apply`")
		default:
			r.Check("settings.json", doctor.OK, "juggernaut block present")
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
