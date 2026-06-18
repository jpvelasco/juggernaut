package cmd

import (
	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/spf13/cobra"
)

var launchCmd = &cobra.Command{
	Use:                "launch -- [claude args...]",
	Short:              "Launch Claude Code with Juggernaut Bedrock activation",
	Hidden:             true,
	DisableFlagParsing: true,
	RunE:               runLaunch,
}

func init() {
	rootCmd.AddCommand(launchCmd)
}

func runLaunch(_ *cobra.Command, args []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	return activation.Launch(home, args)
}
