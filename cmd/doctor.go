package cmd

import (
	"fmt"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/jpvelasco/juggernaut/internal/doctor"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
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

func runDoctor(cmd *cobra.Command, args []string) error {
	home := homeDir()
	r := doctor.NewReport()

	bCfg, err := bedrock.Load(bedrockConfigPath())
	if err != nil {
		r.Check("bedrock-config.json", doctor.Fail, err.Error())
	} else {
		r.Check("bedrock-config.json", doctor.OK, "loaded (v"+bCfg.Version+")")
	}

	mgr := config.NewManager(settingsPath(home, "user"))
	has, err := mgr.HasJuggernautBlock()
	if err != nil {
		r.Check("settings.json", doctor.Fail, err.Error())
	} else if !has {
		r.Check("settings.json", doctor.Fail, "juggernaut block not found — run `juggernaut apply`")
	} else {
		r.Check("settings.json", doctor.OK, "juggernaut block present")
	}

	token, err := keychain.Default().Get()
	if err != nil {
		r.Check("keychain", doctor.Warn, "error reading: "+err.Error())
	} else if token == "" {
		r.Check("keychain", doctor.OK, "no bearer token (IAM auth)")
	} else {
		r.Check("keychain", doctor.OK, "bearer token found")
	}

	binDir := launcher.DefaultBinDir()
	if launcher.IsInstalled(binDir) {
		r.Check("claude shim", doctor.OK, binDir)
	} else {
		r.Check("claude shim", doctor.Warn, "not installed — run `juggernaut apply` to install")
	}

	if doctorFlags.jsonOut {
		out, _ := r.JSON()
		fmt.Println(string(out))
	} else {
		fmt.Print(r.String())
	}

	if r.HasFailures() {
		return fmt.Errorf("doctor found failures — see above")
	}
	return nil
}
