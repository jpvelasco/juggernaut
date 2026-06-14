package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jpvelasco/juggernaut/v4/internal/config"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current Juggernaut configuration",
	RunE:  runShow,
}

var showFlags struct {
	scope   string
	jsonOut bool
}

func init() {
	showCmd.Flags().StringVar(&showFlags.scope, "scope", "", "show only user or project scope")
	showCmd.Flags().BoolVar(&showFlags.jsonOut, "json", false, "output as JSON")
	rootCmd.AddCommand(showCmd)
}

func runShow(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	scopes := []string{"user", "project"}
	if showFlags.scope != "" {
		scopes = []string{showFlags.scope}
	}

	results := map[string]any{}
	for _, scope := range scopes {
		path, perr := settingsPath(home, scope)
		if perr != nil {
			continue
		}
		mgr := config.NewManager(path)
		data, err := mgr.Read()
		if err != nil {
			continue
		}
		results[scope] = data["juggernaut"]
	}

	if showFlags.jsonOut {
		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("serializing configuration: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	for scope, block := range results {
		fmt.Printf("=== %s scope ===\n", scope)
		if block == nil {
			fmt.Println("  (not configured)")
			continue
		}
		out, err := json.MarshalIndent(block, "", "  ")
		if err != nil {
			return fmt.Errorf("serializing %s configuration: %w", scope, err)
		}
		fmt.Println(string(out))
	}
	return nil
}
