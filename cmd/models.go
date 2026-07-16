package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
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

	if anyLegacy {
		return fmt.Errorf("one or more pinned tiers is LEGACY in the live catalog — see report above")
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
// LEGACY. anthropic and inferenceProfiles are both already-normalized
// discovery.DiscoveredModel slices — inferenceProfiles is currently unused in
// the report body (the four tiers are all bedrock-runtime model IDs, not
// inference profile IDs, in this comparison) but is accepted here so a future
// report enhancement doesn't need to change this function's signature again.
func buildModelsReport(cfg *bedrock.Config, anthropic, inferenceProfiles []discovery.DiscoveredModel) (string, bool) {
	statusByID := make(map[string]string, len(anthropic))
	for _, m := range anthropic {
		statusByID[m.ID] = m.Status
	}

	var sb strings.Builder
	anyLegacy := false

	for _, tier := range discovery.AllTiers {
		pinned := tierPin(cfg, tier)
		bareID := bareModelID(pinned)
		status, found := statusByID[bareID]
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

// bareModelID strips a cross-region inference profile prefix so a pinned
// "global.anthropic.claude-opus-4-8" matches the bare "anthropic.claude-opus-4-8"
// ListFoundationModels returns. Mirrors the prefix list schema.go's
// regionalInferencePrefixes already uses elsewhere in this codebase.
func bareModelID(modelID string) string {
	for _, prefix := range []string{"global.", "us.", "us-gov.", "eu.", "apac."} {
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
