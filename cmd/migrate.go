package cmd

import (
	"fmt"
	"os"

	"github.com/jpvelasco/juggernaut/v4/internal/authmode"
	"github.com/jpvelasco/juggernaut/v4/internal/launcher"
	"github.com/jpvelasco/juggernaut/v4/internal/migrate"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate from Juggernaut v3 (shell) to v4 (Go)",
	RunE:  runMigrate,
}

var migrateDryRun bool

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "preview without making changes")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}

	state, err := migrate.Detect(home)
	if err != nil {
		return err
	}

	if !state.HasV3Block {
		fmt.Println("No legacy Juggernaut v3 configuration found.")
		return nil
	}

	if state.AlreadyV4 {
		fmt.Println("Already on Juggernaut v4. Nothing to migrate.")
		return nil
	}

	if state.TooOld {
		return fmt.Errorf(
			"legacy version detected (pre-v3.2.3). Please upgrade to v3.2.3 first:\n" +
				"  curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash\n" +
				"Then re-run: juggernaut migrate",
		)
	}

	fmt.Printf("Found Juggernaut v%s configuration (%s auth).\n", state.V3Version, state.AuthMode)

	if migrateDryRun {
		fmt.Println("\nWould migrate:")
		if authmode.IsBedrockAPIKey(state.AuthMode) {
			switch state.Storage {
			case "profile":
				fmt.Println("  • Transfer bearer token from profile file → keychain")
			case "dpapi":
				fmt.Println("  • Warning: DPAPI storage cannot be migrated automatically — re-entry required")
			default:
				fmt.Println("  • Transfer bearer token from keychain → go-keyring")
			}
		}
		fmt.Println("  • Upgrade settings.json schema v1 → v2")
		fmt.Printf("  • Install claude shim → %s\n", launcher.DefaultBinDir())
		fmt.Println("  • Strip legacy shell launcher blocks from shell profiles")
		fmt.Println("\nRun without --dry-run to apply.")
		return nil
	}

	fmt.Println("Migrating to Juggernaut v4...")

	tokenOK := true
	if authmode.IsBedrockAPIKey(state.AuthMode) {
		tokenOK = transferLegacyToken(home, state.Storage)
	}

	binDir := launcher.DefaultBinDir()
	if err := launcher.Install(binDir); err != nil {
		fmt.Fprintln(os.Stderr, "  Warning: could not install claude shim:", err)
	} else {
		fmt.Printf("  ✓ Installed claude shim → %s\n", binDir)
	}

	stripped := migrate.StripLauncherBlocks(home)
	for _, p := range stripped {
		fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
	}

	for _, p := range migrate.CleanupLegacyFiles(home) {
		fmt.Printf("  ✓ Removed legacy credential file: %s\n", p)
	}

	if !tokenOK {
		fmt.Println("\nMigration complete with warnings — re-enter your credentials:")
		fmt.Println("  juggernaut apply --auth=" + authmode.BedrockAPIKey)
	} else {
		fmt.Println("\nMigration complete.")
	}
	fmt.Println("Run `juggernaut apply` to refresh your configuration with v4 settings.")
	return nil
}
