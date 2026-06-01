package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/jpvelasco/juggernaut/internal/migrate"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Juggernaut configuration",
	RunE:  runUninstall,
}

var uninstallFlags struct {
	scope  string
	full   bool
	force  bool
	dryRun bool
}

func init() {
	f := uninstallCmd.Flags()
	f.StringVar(&uninstallFlags.scope, "scope", "", "remove only user or project scope")
	f.BoolVar(&uninstallFlags.full, "full", false, "also remove claude shim")
	f.BoolVarP(&uninstallFlags.force, "force", "f", false, "skip confirmation prompt")
	f.BoolVar(&uninstallFlags.dryRun, "dry-run", false, "preview without removing")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	home := homeDir()

	if !uninstallFlags.force && !uninstallFlags.dryRun {
		fmt.Print("Remove Juggernaut configuration? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if !strings.EqualFold(strings.TrimSpace(scanner.Text()), "y") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	scopes := []string{"user", "project"}
	if uninstallFlags.scope != "" {
		scopes = []string{uninstallFlags.scope}
	}

	for _, scope := range scopes {
		mgr := config.NewManager(settingsPath(home, scope))
		has, _ := mgr.HasJuggernautBlock()
		if !has {
			continue
		}
		if uninstallFlags.dryRun {
			fmt.Printf("Would remove juggernaut block from %s settings.json\n", scope)
			continue
		}
		if err := mgr.RemoveJuggernautBlock(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove %s block: %v\n", scope, err)
		} else {
			fmt.Printf("  ✓ Removed juggernaut block from %s settings.json\n", scope)
		}
	}

	if !uninstallFlags.dryRun {
		if err := keychain.Default().Delete(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove keychain entry: %v\n", err)
		} else {
			fmt.Println("  ✓ Removed bearer token from keychain")
		}
	}

	if uninstallFlags.full {
		binDir := launcher.DefaultBinDir()
		if uninstallFlags.dryRun {
			fmt.Printf("Would remove claude shim from %s\n", binDir)
		} else {
			_ = launcher.Uninstall(binDir)
			fmt.Println("  ✓ Removed claude shim")
			stripped := migrate.StripLauncherBlocks(home)
			for _, p := range stripped {
				fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
			}
		}
	}

	if !uninstallFlags.dryRun {
		fmt.Println("Uninstall complete.")
	}
	return nil
}
