package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current Juggernaut configuration",
	RunE:  runShow,
}

var showFlags struct {
	cli     string
	scope   string
	jsonOut bool
}

func init() {
	showCmd.Flags().StringVar(&showFlags.cli, "cli", "claude", "coding CLI to display: "+provider.SupportedNames())
	showCmd.Flags().StringVar(&showFlags.scope, "scope", "", "show only user or project scope")
	showCmd.Flags().BoolVar(&showFlags.jsonOut, "json", false, "output as JSON")
	rootCmd.AddCommand(showCmd)
}

func runShow(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	prov, err := provider.Get(showFlags.cli)
	if err != nil {
		return err
	}
	scopes := resolvedScopes(showFlags.scope)

	results := map[string]any{}
	seenPath := map[string]bool{}
	for _, scope := range scopes {
		path, perr := prov.ConfigPath(home, scope)
		if perr != nil {
			warnf("could not determine %s scope path: %v", scope, perr)
			continue
		}
		if seenPath[path] {
			continue
		}
		seenPath[path] = true
		data, err := readProviderConfig(prov, home, scope)
		if err != nil {
			warnf("could not read %s settings: %v", scope, err)
			continue
		}
		results[scope] = showPayload(prov, data)
	}

	if showFlags.jsonOut {
		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("serializing configuration: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	// Iterate the canonical scopes order, not the map — Go randomizes map
	// iteration and scripts diff this output.
	for _, scope := range scopes {
		block, found := results[scope]
		if !found {
			continue
		}
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

// showPayload returns the Juggernaut-managed slice of a provider config, or
// nil when Juggernaut does not own the file. Secret-bearing fields are redacted.
func showPayload(prov provider.Provider, data map[string]any) any {
	if data == nil || !prov.OwnsConfig(data) {
		return nil
	}
	return redactSecrets(managedShowConfig(prov, data))
}

// managedShowConfig copies the juggernaut block (when present) plus the
// provider's native managed keys so show is CLI-agnostic.
func managedShowConfig(prov provider.Provider, data map[string]any) map[string]any {
	out := map[string]any{}
	if v, ok := data["juggernaut"]; ok {
		out["juggernaut"] = v
	}
	for _, k := range prov.NativeManagedKeys() {
		if k == "juggernaut" {
			continue
		}
		if v, ok := data[k]; ok {
			out[k] = v
		}
	}
	return out
}

func redactSecrets(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if secretKeyName(k) {
				if s, ok := val.(string); ok && s != "" {
					out[k] = "[redacted]"
					continue
				}
			}
			out[k] = redactSecrets(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = redactSecrets(item)
		}
		return out
	default:
		return v
	}
}

func secretKeyName(k string) bool {
	n := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
	return strings.Contains(n, "token") ||
		strings.Contains(n, "secret") ||
		strings.Contains(n, "password") ||
		strings.Contains(n, "api_key") ||
		n == "apikey" ||
		strings.Contains(n, "access_key") ||
		n == "authorization" ||
		n == "credential"
}
