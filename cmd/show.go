package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
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
	scopes := resolvedScopes(showFlags.scope)
	prov, err := provider.Get("claude")
	if err != nil {
		return err
	}

	results := map[string]any{}
	for _, scope := range scopes {
		path, perr := prov.ConfigPath(home, scope)
		if perr != nil {
			warnf("could not determine %s scope path: %v", scope, perr)
			continue
		}
		mgr := config.NewManager(path)
		data, err := mgr.Read()
		if err != nil {
			warnf("could not read %s settings: %v", scope, err)
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
