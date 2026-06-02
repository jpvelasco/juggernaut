package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags. Dev builds use the fallback.
var Version = "4.0.0"

var rootCmd = &cobra.Command{
	Use:   "juggernaut",
	Short: "Configure Claude Code to use Amazon Bedrock",
	Long:  `Juggernaut installs and configures Claude Code to route through Amazon Bedrock instead of Anthropic's direct API.`,
}

func Execute() {
	if isLauncherMode() {
		runLauncher()
		os.Exit(0)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ExecuteArgs is used in tests to run commands programmatically.
func ExecuteArgs(args []string) error {
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func isLauncherMode() bool {
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".exe")
	if base == "claude" {
		return true
	}
	for _, arg := range os.Args[1:] {
		if arg == "--launcher" {
			return true
		}
	}
	return false
}

func runLauncher() {
	var args []string
	for _, a := range os.Args[1:] {
		if a != "--launcher" {
			args = append(args, a)
		}
	}
	if err := launcher.RunAsLauncher(args); err != nil {
		fmt.Fprintln(os.Stderr, "juggernaut launcher:", err)
		os.Exit(1)
	}
}
