package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Juggernaut configuration",
	RunE:  runUninstall,
}

var uninstallFlags struct {
	cli    string
	scope  string
	full   bool
	force  bool
	dryRun bool
}

func init() {
	f := uninstallCmd.Flags()
	f.StringVar(&uninstallFlags.cli, "cli", "claude", "CLI to remove: claude, codex")
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

	prov, err := provider.Get(uninstallFlags.cli)
	if err != nil {
		return err
	}

	warnIfAutoModeWillBeLost(home, prov)

	if aborted, err := confirmUninstallAborted(); aborted || err != nil {
		return err
	}

	uninstallSettingsBlocks(home, prov)
	removeRuntimeState(home, prov)
	// The Bedrock bearer token is SHARED across all CLIs, so removing it on a
	// per-CLI uninstall would break any other CLI still configured. Only remove
	// it when uninstalling Claude (the primary). `--full` broadens shell-block
	// removal, NOT shared-credential removal.
	if !uninstallFlags.dryRun && prov.Name() == "claude" {
		removeKeychainToken(home)
	}
	if uninstallFlags.full {
		uninstallActivationFull(home, prov)
	}
	if !uninstallFlags.dryRun {
		fmt.Println("Uninstall complete.")
	}
	return nil
}

func removeRuntimeState(home string, prov provider.Provider) {
	if uninstallFlags.scope == "project" || !prov.LaunchSpec().PersistRuntimeState {
		return
	}
	if uninstallFlags.dryRun {
		if _, found, err := activation.LoadRuntimeState(home, prov.Name()); err == nil && found {
			fmt.Printf("Would remove saved %s runtime fallback\n", prov.Name())
		}
		return
	}
	if err := activation.RemoveRuntimeState(home, prov.Name()); err != nil {
		warnf("could not remove runtime fallback: %v", err)
	}
}

// warnIfAutoModeWillBeLost prints a heads-up when the config about to be
// uninstalled has auto permission mode active. Uninstall removes the
// juggernaut block along with permissions.defaultMode/env — nothing is left
// for a future `apply` (without --mode=auto) to restore auto mode from, so a
// re-apply after reinstalling silently comes back in default mode. This only
// reads, never mutates, so it's safe to run unconditionally (including
// --dry-run and --force). Claude-only: auto mode is a CapAutoMode feature.
func warnIfAutoModeWillBeLost(home string, prov provider.Provider) {
	if !prov.Supports(provider.CapAutoMode) {
		return
	}
	for _, scope := range uninstallScopes() {
		mgr, err := newProviderManager(prov, home, scope)
		if err != nil {
			continue
		}
		data, err := mgr.Read()
		if err != nil {
			continue
		}
		if effectivePermissionMode(data, "") == "auto" {
			fmt.Println("⚠ Auto mode is currently enabled — uninstalling will disable it.")
			fmt.Println("  After reinstalling, run `juggernaut apply --mode=auto` to re-enable it;")
			fmt.Println("  a plain `juggernaut apply` will come back in default mode.")
			return
		}
	}
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
	return resolvedScopes(uninstallFlags.scope)
}

func uninstallSettingsBlocks(home string, prov provider.Provider) {
	for _, scope := range uninstallScopes() {
		uninstallSettingsBlock(home, scope, prov)
	}
}

func uninstallSettingsBlock(home, scope string, prov provider.Provider) {
	mgr, err := newProviderManager(prov, home, scope)
	if err != nil {
		warnf("%s config: %v", scope, err)
		return
	}
	has, err := mgr.HasManagedKeys(prov.NativeManagedKeys())
	if err != nil {
		warnf("could not check %s scope: %v", scope, err)
		return
	}
	if !has {
		return
	}
	if uninstallFlags.dryRun {
		fmt.Printf("Would remove juggernaut block from %s %s config\n", scope, prov.Name())
		return
	}
	if err := mgr.RemoveManagedKeysDeep(prov.NativeManagedKeys(), prov.OwnedSubKeys()); err != nil {
		warnf("could not remove %s block: %v", scope, err)
		return
	}
	fmt.Printf("  ✓ Removed juggernaut block from %s %s config\n", scope, prov.Name())
}

func removeKeychainToken(home string) {
	if err := keychain.Default().DeleteWithFallback(home); err != nil {
		warnf("could not remove keychain entry: %v", err)
		return
	}
	fmt.Println("  ✓ Removed bearer token from keychain")
}

func uninstallActivationFull(home string, prov provider.Provider) {
	begin, end := prov.ActivationMarkers()
	title := prov.DisplayName()
	if uninstallFlags.dryRun {
		fmt.Printf("Would remove Juggernaut %s activation blocks from shell profiles\n", title)
		if prov.Name() == "claude" {
			fmt.Printf("Would recover known v4.2.6 launcher artifacts in %s\n", activation.DefaultBinDir(home))
		}
		return
	}
	removed, err := activation.UninstallWith(home, activation.UninstallOptions{
		Spec: activation.CLISpec{Name: prov.Name(), Begin: begin, End: end},
	})
	if err != nil {
		warnf("could not remove shell activation: %v", err)
	} else if len(removed) > 0 {
		fmt.Printf("  ✓ Removed %s activation from %d shell profile(s)\n", title, len(removed))
	}

	// Legacy v4.2.6 launcher-artifact recovery is Claude-specific.
	if prov.Name() != "claude" {
		return
	}
	reportLegacyRecovery(home)
}
