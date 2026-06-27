package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
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
	f.BoolVar(&uninstallFlags.full, "full", false, "also remove shell activation blocks")
	f.BoolVarP(&uninstallFlags.force, "force", "f", false, "skip confirmation prompt")
	f.BoolVar(&uninstallFlags.dryRun, "dry-run", false, "preview without removing")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	if aborted, err := confirmUninstallAborted(); aborted || err != nil {
		return err
	}

	uninstallSettingsBlocks(home)
	if !uninstallFlags.dryRun {
		removeKeychainToken()
	}
	if uninstallFlags.full {
		uninstallActivationFull(home)
	}
	if !uninstallFlags.dryRun {
		fmt.Println("Uninstall complete.")
	}
	return nil
}

func confirmUninstallAborted() (aborted bool, err error) {
	if uninstallFlags.force || uninstallFlags.dryRun {
		return false, nil
	}
	fmt.Print("Remove Juggernaut configuration? [y/N] ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("reading confirmation: %w", err)
		}
		fmt.Println("Aborted.")
		return true, nil
	}
	if !strings.EqualFold(strings.TrimSpace(scanner.Text()), "y") {
		fmt.Println("Aborted.")
		return true, nil
	}
	return false, nil
}

func uninstallScopes() []string {
	if uninstallFlags.scope != "" {
		return []string{uninstallFlags.scope}
	}
	return []string{"user", "project"}
}

func uninstallSettingsBlocks(home string) {
	for _, scope := range uninstallScopes() {
		uninstallSettingsBlock(home, scope)
	}
}

func uninstallSettingsBlock(home, scope string) {
	path, err := settingsPath(home, scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: invalid %s settings path: %v\n", scope, err)
		return
	}
	mgr := config.NewManager(path)
	has, err := mgr.HasJuggernautBlock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not check %s scope: %v\n", scope, err)
		return
	}
	if !has {
		return
	}
	if uninstallFlags.dryRun {
		fmt.Printf("Would remove juggernaut block from %s settings.json\n", scope)
		return
	}
	if err := mgr.RemoveJuggernautBlock(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove %s block: %v\n", scope, err)
		return
	}
	fmt.Printf("  ✓ Removed juggernaut block from %s settings.json\n", scope)
}

func removeKeychainToken() {
	if err := keychain.Default().Delete(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove keychain entry: %v\n", err)
		return
	}
	fmt.Println("  ✓ Removed bearer token from keychain")
}

func uninstallActivationFull(home string) {
	if uninstallFlags.dryRun {
		fmt.Println("Would remove Juggernaut Claude activation blocks from shell profiles")
		fmt.Printf("Would recover known v4.2.6 launcher artifacts in %s\n", activation.DefaultBinDir(home))
		return
	}
	removed, err := activation.Uninstall(home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove shell activation: %v\n", err)
	} else if len(removed) > 0 {
		fmt.Printf("  ✓ Removed Claude activation from %d shell profile(s)\n", len(removed))
	}

	binDir := activation.DefaultBinDir(home)
	actions, err := activation.RecoverLegacyArtifacts(binDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not recover legacy artifacts: %v\n", err)
		return
	}
	for _, a := range actions {
		fmt.Printf("  ✓ %s: %s\n", a.Action, a.Path)
	}
}
