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
	// show always redacts secret-bearing fields; the bundle path passes false.
	out, err := formatShow(collectShowResults(prov, home, showFlags.scope, true), showFlags.scope, showFlags.jsonOut)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

func collectShowResults(prov provider.Provider, home, scopeFilter string, redact bool) map[string]any {
	results := map[string]any{}
	seenPath := map[string]bool{}
	for _, scope := range resolvedScopes(scopeFilter) {
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
		results[scope] = showPayload(prov, home, scope, data, redact)
	}
	return results
}

func formatShow(results map[string]any, scopeFilter string, jsonOut bool) (string, error) {
	if jsonOut {
		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return "", fmt.Errorf("serializing configuration: %w", err)
		}
		return string(out) + "\n", nil
	}

	// Iterate the canonical scopes order, not the map — Go randomizes map
	// iteration and scripts diff this output.
	var b strings.Builder
	for _, scope := range resolvedScopes(scopeFilter) {
		block, found := results[scope]
		if !found {
			continue
		}
		fmt.Fprintf(&b, "=== %s scope ===\n", scope)
		if block == nil {
			b.WriteString("  (not configured)\n")
			continue
		}
		out, err := json.MarshalIndent(block, "", "  ")
		if err != nil {
			return "", fmt.Errorf("serializing %s configuration: %w", scope, err)
		}
		b.WriteString(string(out))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// showPayload returns the Juggernaut-managed slice of a provider config, or
// nil when Juggernaut does not own the file. When redact is true, secret-bearing
// fields are masked; the logs export path passes false so its --raw bundles and
// its redact.String pass are the sole secret control.
func showPayload(prov provider.Provider, home, scope string, data map[string]any, redact bool) any {
	if data == nil || !prov.OwnsConfig(data) {
		return nil
	}
	cfg := managedShowConfig(prov, home, scope, data)
	if !redact {
		return cfg
	}
	return redactSecrets(cfg)
}

// managedShowConfig copies the juggernaut block (when present) plus the
// provider's native managed keys so show is CLI-agnostic. Sidecar providers
// (OpenCode) keep the block in the sidecar file, so include it from there when
// it is not present in the vendor config.
func managedShowConfig(prov provider.Provider, home, scope string, data map[string]any) map[string]any {
	out := map[string]any{}
	if v, ok := data["juggernaut"]; ok {
		out["juggernaut"] = v
	} else if block := sidecarBlock(prov, home, scope); block != nil {
		out["juggernaut"] = block
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

// sidecarBlock reads the scope's sidecar auth-metadata file and returns its
// juggernaut block, or nil when the provider has no sidecar or the file is
// missing/unparseable.
func sidecarBlock(prov provider.Provider, home, scope string) map[string]any {
	if !provider.HasSidecar(prov) {
		return nil
	}
	path, err := sidecarPath(prov, home, scope)
	if err != nil {
		return nil
	}
	return provider.ReadSidecarFile(path)
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
