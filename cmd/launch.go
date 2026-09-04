package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/spf13/cobra"
)

var launchCmd = &cobra.Command{
	Use:                "launch [cli] -- [args...]",
	Short:              "Launch a CLI (default: Claude Code) with Juggernaut Bedrock activation",
	Hidden:             true,
	DisableFlagParsing: true,
	// Passthrough exec: the child's output and exit status are the result, so
	// never print cobra's "Error:" line or usage dump around them.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runLaunch,
}

// launchCLICmd gives non-Claude activation blocks a version-skew-safe entry
// point. Binaries predating multi-CLI support do not have this command, so they
// fail instead of interpreting the CLI name as a Claude argument.
var launchCLICmd = &cobra.Command{
	Use:                "launch-cli <cli> -- [args...]",
	Short:              "Launch a named CLI with Juggernaut Bedrock activation",
	Hidden:             true,
	DisableFlagParsing: true,
	SilenceUsage:       true,
	SilenceErrors:      true,
	RunE:               runLaunchCLI,
}

func init() {
	rootCmd.AddCommand(launchCmd)
	rootCmd.AddCommand(launchCLICmd)
}

// runLaunch parses `launch [cli] -- args...`. An optional CLI name may precede
// the `--` separator (e.g. `launch codex -- ...`); with no name it defaults to
// Claude, preserving the historical `launch -- ...` form.
func runLaunch(_ *cobra.Command, args []string) error {
	cli := "claude"
	if len(args) > 0 && args[0] != "--" {
		cli = args[0]
		args = args[1:]
	}
	return launchNamedCLI(cli, args)
}

func runLaunchCLI(_ *cobra.Command, args []string) error {
	if len(args) == 0 || args[0] == "--" {
		return fmt.Errorf("launch-cli requires a CLI name")
	}
	cli := args[0]
	return launchNamedCLI(cli, args[1:])
}

func launchNamedCLI(cli string, args []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	prov, err := provider.Get(cli)
	if err != nil {
		return err
	}

	selfPaths := resolveSelfPaths()

	// Probe project then user so launch sees the same juggernaut.auth.mode apply
	// wrote (project-only API-key apply used to be invisible to the wrapper).
	cfgPaths := resolveLaunchConfigPaths(prov, home)
	cfgPath := ""
	if len(cfgPaths) > 0 {
		cfgPath = cfgPaths[0]
	}
	target := launchTargetFor(prov, cfgPath)
	target.ConfigPaths = cfgPaths

	return activation.LaunchWithOptions(activation.LaunchOptions{
		Home:        home,
		Args:        args,
		Path:        os.Getenv("PATH"),
		TokenGetter: func() (string, error) { return keychain.Default().GetWithFallback(home) },
		Runner:      activation.RunBinary,
		Target:      target,
		SelfPaths:   selfPaths,
	})
}

// launchTargetFor maps a provider's LaunchSpec + binary names onto the
// activation package's LaunchTarget (activation stays provider-free).
func launchTargetFor(p provider.Provider, cfgPath string) activation.LaunchTarget {
	spec := p.LaunchSpec()
	target := activation.LaunchTarget{
		BinaryNames: p.BinaryNames(),
		TokenEnvVar: spec.TokenEnvVar,
		StaticEnv:   spec.StaticEnv,
		NeedsToken:  spec.NeedsToken,
		ConfigPath:  cfgPath,
	}
	if spec.PersistRuntimeState {
		target.RuntimeStateName = p.Name()
	}
	return target
}

// resolveSelfPaths returns additional executable paths that resolveBinary should
// skip alongside os.Executable(). On Windows staged launches, the npm shim sets
// JUGGERNAUT_ORIGINAL_BIN to the installed binary path so that PATH candidates
// hardlinked to the installed binary are also skipped (os.Executable() returns
// the temp copy, not the installed binary).
func resolveSelfPaths() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	originalBin := os.Getenv("JUGGERNAUT_ORIGINAL_BIN")
	if originalBin == "" {
		return nil
	}
	// Normalize to absolute to avoid any ambiguity.
	if !filepath.IsAbs(originalBin) {
		return nil
	}
	return []string{originalBin}
}
