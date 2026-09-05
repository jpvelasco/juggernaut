package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Version is set at build time via -ldflags. Dev builds use the fallback.
var Version = "6.3.0"

var rootCmd = &cobra.Command{
	Use:   "juggernaut",
	Short: "Configure Claude Code to use Amazon Bedrock",
	Long:  `Juggernaut configures Claude Code to route through Amazon Bedrock instead of Anthropic's direct API.`,
}

// Execute is the main entry point for the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// Launched CLIs pass through this process: their exit status IS the
		// result. Propagate it verbatim instead of collapsing every failure
		// to 1 with "exit status N" noise — scripts key on those codes.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ExecuteArgs is used in tests to run commands programmatically.
// It resets all flag values to defaults before each invocation to prevent state leakage.
func ExecuteArgs(args []string) error {
	resetFlags()
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// resetFlags resets all subcommand flags to their defaults between test runs.
func resetFlags() {
	resetCommandFlags(rootCmd)
}

func resetCommandFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for _, sub := range cmd.Commands() {
		resetCommandFlags(sub)
	}
}
