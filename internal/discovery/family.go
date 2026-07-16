package discovery

import (
	"fmt"
	"strings"
)

// Tier identifies one of bedrock-config.json's four pinned model tiers.
type Tier string

const (
	TierOpus   Tier = "opus"
	TierSonnet Tier = "sonnet"
	TierHaiku  Tier = "haiku"
	TierFable  Tier = "fable"
)

// AllTiers lists every tier in a stable, deterministic order for report output.
var AllTiers = []Tier{TierOpus, TierSonnet, TierHaiku, TierFable}

// MatchTier reports which tier modelID belongs to via a plain substring match
// against the tier name — the same convention bedrock-config.json's own JSON
// keys use. No version-ordering table: a model matching none of the four
// tiers is reported by the caller as "unrecognized", not an error here.
func MatchTier(modelID string) (Tier, bool) {
	for _, tier := range AllTiers {
		if strings.Contains(modelID, string(tier)) {
			return tier, true
		}
	}
	return "", false
}

// FormatCandidate renders one discovered model as a single report line.
func FormatCandidate(m DiscoveredModel) string {
	return fmt.Sprintf("%s (%s)", m.ID, m.Status)
}
