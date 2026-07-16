package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/spf13/cobra"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Maintainer tools for the embedded bedrock-config.json model catalog",
}

var modelsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check bedrock-config.json's pinned models against AWS Bedrock's live catalog",
	RunE:  runModelsCheck,
}

var modelsCheckFlags struct {
	region    string
	write     bool
	setOpus   string
	setSonnet string
	setHaiku  string
	setFable  string
}

func init() {
	f := modelsCheckCmd.Flags()
	f.StringVar(&modelsCheckFlags.region, "region", "us-west-2", "AWS region to query")
	f.BoolVar(&modelsCheckFlags.write, "write", false, "write --set-* values to bedrock-config.json after re-verifying they are ACTIVE")
	f.StringVar(&modelsCheckFlags.setOpus, "set-opus", "", "pin a new Opus model ID (requires --write)")
	f.StringVar(&modelsCheckFlags.setSonnet, "set-sonnet", "", "pin a new Sonnet model ID (requires --write)")
	f.StringVar(&modelsCheckFlags.setHaiku, "set-haiku", "", "pin a new Haiku model ID (requires --write)")
	f.StringVar(&modelsCheckFlags.setFable, "set-fable", "", "pin a new Fable model ID (requires --write)")

	modelsCmd.AddCommand(modelsCheckCmd)
	rootCmd.AddCommand(modelsCmd)
}

// listAnthropicModels and listInferenceProfiles are swapped in tests so
// runModelsCheck never makes a real AWS call in CI.
var listAnthropicModels = discovery.ListAnthropicModels
var listInferenceProfiles = discovery.ListInferenceProfiles

func runModelsCheck(_ *cobra.Command, _ []string) error {
	sets := resolveSetFlags()
	if modelsCheckFlags.write && len(sets) == 0 {
		return fmt.Errorf("--write requires at least one --set-<tier>=<id> flag")
	}

	cfg, err := loadBedrockConfig()
	if err != nil {
		return err
	}

	ctx := context.Background()
	anthropic, err := listAnthropicModels(ctx, modelsCheckFlags.region)
	if err != nil {
		return fmt.Errorf("querying Bedrock foundation models: %w", err)
	}
	profiles, err := listInferenceProfiles(ctx, modelsCheckFlags.region)
	if err != nil {
		return fmt.Errorf("querying Bedrock inference profiles: %w", err)
	}

	report, anyLegacy := buildModelsReport(cfg, anthropic, profiles)
	fmt.Print(report)

	if len(sets) > 0 {
		if err := applyModelWrites(cfg, sets, anthropic, profiles); err != nil {
			return fmt.Errorf("not writing bedrock-config.json: %w", err)
		}
		if modelsCheckFlags.write {
			if err := writeBedrockConfigFile(cfg); err != nil {
				return err
			}
			fmt.Println("\nbedrock-config.json updated.")
			// Recompute exit status from the post-write pins so a successful
			// replacement of a LEGACY pin exits 0 without a second run.
			_, anyLegacy = buildModelsReport(cfg, anthropic, profiles)
		} else {
			// --set-* without --write: print validation feedback for each tier
			fmt.Println()
			for _, tier := range discovery.AllTiers {
				id, ok := sets[tier]
				if !ok {
					continue
				}
				fmt.Printf("%s: %s is ACTIVE (pass --write to persist)\n", tier, id)
			}
		}
	}

	if anyLegacy {
		return fmt.Errorf("one or more pinned tiers is LEGACY in the live catalog - see report above")
	}
	return nil
}

// tierPin returns the currently pinned model ID for tier from cfg.
func tierPin(cfg *bedrock.Config, tier discovery.Tier) string {
	switch tier {
	case discovery.TierOpus:
		return cfg.Models.Opus
	case discovery.TierSonnet:
		return cfg.Models.Sonnet
	case discovery.TierHaiku:
		return cfg.Models.Haiku
	case discovery.TierFable:
		return cfg.Models.Fable
	default:
		return ""
	}
}

// buildModelsReport is a pure function (no I/O) so it's directly unit
// testable: given the config and the already-fetched discovery results, it
// renders the full human-readable report and reports whether any tier is
// LEGACY. Prefixed pins (global./us./…) are verified against the inference
// profile catalog; bare foundation-model IDs against ListFoundationModels.
func buildModelsReport(cfg *bedrock.Config, anthropic, inferenceProfiles []discovery.DiscoveredModel) (string, bool) {
	var sb strings.Builder
	anyLegacy := false

	for _, tier := range discovery.AllTiers {
		pinned := tierPin(cfg, tier)
		status, found := pinStatus(pinned, anthropic, inferenceProfiles)
		switch {
		case !found:
			fmt.Fprintf(&sb, "%-8s %-50s not found in live catalog\n", tier, pinned)
			anyLegacy = true
		case status == "LEGACY":
			fmt.Fprintf(&sb, "%-8s %-50s LEGACY\n", tier, pinned)
			anyLegacy = true
		default:
			fmt.Fprintf(&sb, "%-8s %-50s %s\n", tier, pinned, status)
		}

		if !found || status == "LEGACY" {
			candidates := activeCandidatesForTier(anthropic, tier)
			if len(candidates) == 0 {
				sb.WriteString("  no ACTIVE replacement candidates found\n")
			}
			for _, c := range candidates {
				fmt.Fprintf(&sb, "  candidate: %s\n", discovery.FormatCandidate(c))
			}
		}
	}

	unrecognized := unrecognizedModels(anthropic)
	if len(unrecognized) > 0 {
		sb.WriteString("\nunrecognized (no matching tier):\n")
		for _, m := range unrecognized {
			fmt.Fprintf(&sb, "  %s\n", discovery.FormatCandidate(m))
		}
	}

	return sb.String(), anyLegacy
}

// pinStatus resolves a pinned model ID against the right live catalog.
// Prefixed inference-profile IDs (global./us./eu./…) must match
// ListInferenceProfiles; bare foundation-model IDs match ListFoundationModels
// (after stripping a prefix if present on the catalog entry itself).
func pinStatus(pinned string, anthropic, profiles []discovery.DiscoveredModel) (status string, found bool) {
	if pinned == "" {
		return "", false
	}
	if hasRegionalPrefix(pinned) {
		for _, p := range profiles {
			if p.ID == pinned {
				return p.Status, true
			}
		}
		return "", false
	}
	bare := bareModelID(pinned)
	for _, m := range anthropic {
		if m.ID == bare || bareModelID(m.ID) == bare {
			return m.Status, true
		}
	}
	return "", false
}

func hasRegionalPrefix(modelID string) bool {
	for _, prefix := range schema.RegionalInferencePrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

// bareModelID strips a cross-region inference profile prefix so a pinned
// "global.anthropic.claude-opus-4-8" matches the bare "anthropic.claude-opus-4-8"
// ListFoundationModels returns. Uses schema.RegionalInferencePrefixes to maintain
// a single source of truth across the codebase.
func bareModelID(modelID string) string {
	for _, prefix := range schema.RegionalInferencePrefixes {
		if rest, ok := strings.CutPrefix(modelID, prefix); ok {
			return rest
		}
	}
	return modelID
}

// activeCandidatesForTier returns every ACTIVE model matching tier, sorted
// alphabetically by ID for deterministic output.
func activeCandidatesForTier(models []discovery.DiscoveredModel, tier discovery.Tier) []discovery.DiscoveredModel {
	var candidates []discovery.DiscoveredModel
	for _, m := range models {
		if m.Status != "ACTIVE" {
			continue
		}
		if matched, ok := discovery.MatchTier(m.ID); ok && matched == tier {
			candidates = append(candidates, m)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	return candidates
}

// unrecognizedModels returns every discovered model matching none of the four
// known tiers, sorted alphabetically.
func unrecognizedModels(models []discovery.DiscoveredModel) []discovery.DiscoveredModel {
	var out []discovery.DiscoveredModel
	for _, m := range models {
		if _, ok := discovery.MatchTier(m.ID); !ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// resolveSetFlags collects every non-empty --set-<tier> flag into a
// tier→modelID map.
func resolveSetFlags() map[discovery.Tier]string {
	sets := map[discovery.Tier]string{}
	if modelsCheckFlags.setOpus != "" {
		sets[discovery.TierOpus] = modelsCheckFlags.setOpus
	}
	if modelsCheckFlags.setSonnet != "" {
		sets[discovery.TierSonnet] = modelsCheckFlags.setSonnet
	}
	if modelsCheckFlags.setHaiku != "" {
		sets[discovery.TierHaiku] = modelsCheckFlags.setHaiku
	}
	if modelsCheckFlags.setFable != "" {
		sets[discovery.TierFable] = modelsCheckFlags.setFable
	}
	return sets
}

// applyModelWrites verifies every entry in sets is ACTIVE in the appropriate
// live catalog, then mutates cfg.Models. All-or-nothing: if any entry fails
// verification, cfg is left completely unchanged and an error is returned.
func applyModelWrites(cfg *bedrock.Config, sets map[discovery.Tier]string, anthropic, profiles []discovery.DiscoveredModel) error {
	for _, tier := range discovery.AllTiers {
		id, ok := sets[tier]
		if !ok {
			continue
		}
		status, found := pinStatus(id, anthropic, profiles)
		if !found {
			return fmt.Errorf("%s: %q not found in live catalog", tier, id)
		}
		if status != "ACTIVE" {
			return fmt.Errorf("%s: %q is %s, not ACTIVE", tier, id, status)
		}
	}

	for tier, id := range sets {
		switch tier {
		case discovery.TierOpus:
			cfg.Models.Opus = id
		case discovery.TierSonnet:
			cfg.Models.Sonnet = id
		case discovery.TierHaiku:
			cfg.Models.Haiku = id
		case discovery.TierFable:
			cfg.Models.Fable = id
		}
	}
	return nil
}

// writeBedrockConfigFile patches only the model pin fields in the on-disk
// bedrock-config.json, preserving unknown top-level keys (description, notes,
// defaults, …) that the typed bedrock.Config struct does not carry.
func writeBedrockConfigFile(cfg *bedrock.Config) error {
	path := findBedrockConfigFile()
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s for write: %w", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parsing %s for write: %w", path, err)
	}
	models, _ := root["models"].(map[string]any)
	if models == nil {
		models = map[string]any{}
	}
	models["opus"] = cfg.Models.Opus
	models["sonnet"] = cfg.Models.Sonnet
	models["haiku"] = cfg.Models.Haiku
	models["fable"] = cfg.Models.Fable
	root["models"] = models

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding bedrock-config.json: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
