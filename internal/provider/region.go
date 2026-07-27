package provider

import (
	"fmt"
	"slices"
)

// regionAllowed reports whether region is in the model's set of known-available
// regions. An empty set means "no verified region data" — callers treat that as
// unconstrained (see resolveMantleRegion).
func regionAllowed(region string, known []string) bool {
	return slices.Contains(known, region)
}

// resolveMantleRegion applies the shared Mantle region policy — the "Iron Fist":
// a model is only reachable where it's verified available (known), and a user's
// configured region cannot be assumed to serve it, so Juggernaut ALWAYS routes to
// a region that actually works rather than writing a config that can't
// authenticate. If the requested region serves the model it's kept; otherwise
// Juggernaut overrides to the model's first known-good region — whether the
// region was defaulted OR explicitly passed — and says what it did.
//
//   - known is empty         → no region data to enforce; keep requested, silent.
//   - requested is in known  → keep it, silent.
//   - requested NOT in known → override to known[0] (switched=true) with a
//     message; the message wording notes whether the region was the user's
//     explicit choice or the global default.
//
// (A future flag may let users force a non-serving region; for now the Iron Fist
// guarantees a working config.)
func resolveMantleRegion(requested string, explicit bool, known []string) (region, message string, switched bool) {
	if len(known) == 0 || regionAllowed(requested, known) {
		return requested, "", false
	}
	source := "default region"
	if explicit {
		source = "requested region"
	}
	return known[0], fmt.Sprintf(
		"not available in %s %s; using %s instead (available: %v)",
		source, requested, known[0], known), true
}

// assembleMantleWarnings builds the warning list shared by Mantle-based
// providers (Codex, Grok). It combines the region-switch message (if any)
// with the catalog unavailability warning (if applicable). The separator
// parameter controls how the model ID and region message are joined:
// Codex uses ": " while Grok uses " ".
func assembleMantleWarnings(regionMsg, modelID, region, catalogSuffix, sep string, p CatalogProvider, models []CatalogModel, refreshedSources []string) []string {
	var warnings []string
	if regionMsg != "" {
		warnings = append(warnings, modelID+sep+regionMsg)
	}
	if w := catalogUnavailableWarning(models, modelID, region, catalogSuffix, p, refreshedSources); w != "" {
		warnings = append(warnings, w)
	}
	return warnings
}

// buildWithRegionWarnings resolves the Mantle region (if the model has known
// regions), then calls the provided build function with the final region.
// It appends the region-switch message and catalog unavailability warning to
// the returned warnings. This unifies the region + warning pattern shared by
// Codex and Grok BuildConfig implementations.
//
//   - modelRegions is the list of verified regions for this model (empty = skip region check)
//   - sep controls how the model ID and region message are joined in warnings
//   - build receives the resolved region and returns the plan plus any provider-specific warnings
type buildFn func(region string) (ConfigPlan, error)

func buildWithRegionWarnings(opts Options, modelID string, modelRegions []string, sep string, p CatalogProvider, build buildFn) (ConfigPlan, error) {
	region := opts.Region
	regionMsg := ""
	if len(modelRegions) > 0 {
		region, regionMsg, _ = resolveMantleRegion(opts.Region, opts.RegionExplicit, modelRegions)
	}

	plan, err := build(region)
	if err != nil {
		return ConfigPlan{}, err
	}

	plan.Warnings = append(plan.Warnings, assembleMantleWarnings(
		regionMsg, modelID, region, "refresh the catalog or select a listed model", sep,
		p, opts.ModelCatalog, opts.RefreshedSources,
	)...)
	return plan, nil
}
