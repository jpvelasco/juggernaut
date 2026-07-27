package provider

import (
	"fmt"
	"strings"
	"testing"
)

// TestResolveMantleRegion covers the shared region-resolution policy used by
// Mantle providers: a model is only reachable in the regions where it's verified
// available. When the user did NOT explicitly choose a region and the resolved
// (defaulted) region can't serve the model, auto-switch to the model's first
// known-good region. When the user DID choose the region, honor it but warn.
func TestResolveMantleRegion(t *testing.T) {
	known := []string{"us-east-1", "us-east-2"}

	t.Run("defaulted region not serving model auto-switches", func(t *testing.T) {
		region, warn, switched := resolveMantleRegion("us-west-2", false, known)
		if region != "us-east-1" {
			t.Errorf("region = %q, want us-east-1 (first known)", region)
		}
		if !switched {
			t.Error("switched should be true")
		}
		if warn == "" {
			t.Error("expected a heads-up message on auto-switch")
		}
	})

	t.Run("defaulted region already serving model is kept", func(t *testing.T) {
		region, warn, switched := resolveMantleRegion("us-east-2", false, known)
		if region != "us-east-2" {
			t.Errorf("region = %q, want us-east-2 (already valid)", region)
		}
		if switched || warn != "" {
			t.Errorf("no switch/warn expected, got switched=%v warn=%q", switched, warn)
		}
	})

	t.Run("explicit region not serving model is OVERRIDDEN (Iron Fist)", func(t *testing.T) {
		region, warn, switched := resolveMantleRegion("us-west-2", true, known)
		if region != "us-east-1" {
			t.Errorf("region = %q, want us-east-1 (Iron Fist overrides even an explicit region)", region)
		}
		if !switched {
			t.Error("switched should be true — Juggernaut never writes a non-serving region")
		}
		if warn == "" {
			t.Error("expected a message noting the override")
		}
	})

	t.Run("explicit region serving model is silent", func(t *testing.T) {
		region, warn, switched := resolveMantleRegion("us-east-1", true, known)
		if region != "us-east-1" || switched || warn != "" {
			t.Errorf("valid explicit region should be silent, got region=%q warn=%q switched=%v", region, warn, switched)
		}
	})

	t.Run("empty known set is a no-op (no region data to enforce)", func(t *testing.T) {
		region, warn, switched := resolveMantleRegion("eu-west-1", false, nil)
		if region != "eu-west-1" || switched || warn != "" {
			t.Errorf("no known regions => keep as-is, got region=%q warn=%q switched=%v", region, warn, switched)
		}
	})
}

// TestAssembleMantleWarnings_TableDriven verifies the shared warning assembly
// helper used by Codex and Grok BuildConfig.
func TestAssembleMantleWarnings_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		regionMsg string
		modelID   string
		region    string
		sep       string
		wantCount int
		wantFirst string // first warning prefix (if any)
	}{
		{
			name:      "no warnings",
			regionMsg: "",
			modelID:   "xai.grok-4-3",
			region:    "us-east-1",
			sep:       " ",
			wantCount: 0,
		},
		{
			name:      "grok region warning",
			regionMsg: "not available in default region us-west-2; using us-east-1 instead (available: [us-east-1])",
			modelID:   "xai.grok-4-3",
			region:    "us-east-1",
			sep:       " ",
			wantCount: 1,
			wantFirst: "xai.grok-4-3 not available",
		},
		{
			name:      "codex region warning",
			regionMsg: "not available in default region us-west-2; using us-east-1 instead (available: [us-east-1])",
			modelID:   "openai.gpt-5.5",
			region:    "us-east-1",
			sep:       ": ",
			wantCount: 1,
			wantFirst: "openai.gpt-5.5: not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := assembleMantleWarnings(tt.regionMsg, tt.modelID, tt.region, "refresh", tt.sep, nil, nil, nil)
			if len(warnings) != tt.wantCount {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tt.wantCount, warnings)
			}
			if tt.wantFirst != "" && (len(warnings) == 0 || !strings.HasPrefix(warnings[0], tt.wantFirst)) {
				t.Errorf("first warning should start with %q, got %q", tt.wantFirst, warnings[0])
			}
		})
	}
}

// TestAssembleMantleWarnings_IdentityWithCodex verifies assembleMantleWarnings
// produces byte-identical output to the current Codex BuildConfig warning path.
func TestAssembleMantleWarnings_IdentityWithCodex(t *testing.T) {
	regionMsg := "not available in default region us-west-2; using us-east-1 instead (available: [us-east-1 us-east-2])"
	modelID := "openai.gpt-5.5"
	region := "us-east-1"
	suffix := "refresh the catalog or select a listed model"

	warnings := assembleMantleWarnings(regionMsg, modelID, region, suffix, ": ", nil, nil, nil)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	// Codex uses "modelID: regionMsg" format
	want := modelID + ": " + regionMsg
	if warnings[0] != want {
		t.Errorf("warning = %q, want %q", warnings[0], want)
	}
}

// TestAssembleMantleWarnings_IdentityWithGrok verifies assembleMantleWarnings
// produces byte-identical output to the current Grok BuildConfig warning path.
func TestAssembleMantleWarnings_IdentityWithGrok(t *testing.T) {
	regionMsg := "not available in default region us-west-2; using us-east-1 instead (available: [us-east-1])"
	modelID := "xai.grok-4-3"
	region := "us-east-1"

	warnings := assembleMantleWarnings(regionMsg, modelID, region, "refresh", " ", nil, nil, nil)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	// Grok uses "modelID regionMsg" format (space-separated)
	want := modelID + " " + regionMsg
	if warnings[0] != want {
		t.Errorf("warning = %q, want %q", warnings[0], want)
	}
}

// TestBuildWithRegionWarnings_RegionSwitch verifies the full shared path
// (region resolution → build fn → warning assembly) correctly switches the
// region and attaches the warning when the requested region doesn't serve the model.
func TestBuildWithRegionWarnings_RegionSwitch(t *testing.T) {
	knownRegions := []string{"us-east-1", "us-east-2"}
	opts := Options{
		Region:         "us-west-2",
		RegionExplicit: false,
	}

	var capturedRegion string
	plan, err := buildWithRegionWarnings(opts, "openai.gpt-5.5", knownRegions, ": ", nil, func(region string) (ConfigPlan, error) {
		capturedRegion = region
		return ConfigPlan{Keys: map[string]any{"region": region}}, nil
	})
	if err != nil {
		t.Fatalf("buildWithRegionWarnings: %v", err)
	}
	if capturedRegion != "us-east-1" {
		t.Errorf("captured region = %q, want us-east-1 (first known)", capturedRegion)
	}
	if plan.Keys["region"] != "us-east-1" {
		t.Errorf("plan region = %q, want us-east-1", plan.Keys["region"])
	}
	if len(plan.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(plan.Warnings), plan.Warnings)
	}
	if !strings.HasPrefix(plan.Warnings[0], "openai.gpt-5.5: ") {
		t.Errorf("warning should start with modelID + separator, got %q", plan.Warnings[0])
	}
}

// TestBuildWithRegionWarnings_NoSwitch verifies the shared path passes
// through the requested region when it already serves the model.
func TestBuildWithRegionWarnings_NoSwitch(t *testing.T) {
	knownRegions := []string{"us-east-1", "us-east-2"}
	opts := Options{
		Region:         "us-east-1",
		RegionExplicit: true,
	}

	var capturedRegion string
	plan, err := buildWithRegionWarnings(opts, "openai.gpt-5.5", knownRegions, ": ", nil, func(region string) (ConfigPlan, error) {
		capturedRegion = region
		return ConfigPlan{Keys: map[string]any{"region": region}, Warnings: []string{"provider-specific"}}, nil
	})
	if err != nil {
		t.Fatalf("buildWithRegionWarnings: %v", err)
	}
	if capturedRegion != "us-east-1" {
		t.Errorf("captured region = %q, want us-east-1", capturedRegion)
	}
	// Provider-specific warning preserved, no region warning added
	if len(plan.Warnings) != 1 || plan.Warnings[0] != "provider-specific" {
		t.Errorf("expected only provider-specific warning, got %v", plan.Warnings)
	}
}

// TestBuildWithRegionWarnings_NoRegionData verifies the shared path skips
// region enforcement when there's no known region data (empty list).
func TestBuildWithRegionWarnings_NoRegionData(t *testing.T) {
	opts := Options{
		Region:         "eu-west-1",
		RegionExplicit: false,
	}

	var capturedRegion string
	plan, err := buildWithRegionWarnings(opts, "xai.grok-5", nil, " ", nil, func(region string) (ConfigPlan, error) {
		capturedRegion = region
		return ConfigPlan{Keys: map[string]any{"region": region}}, nil
	})
	if err != nil {
		t.Fatalf("buildWithRegionWarnings: %v", err)
	}
	if capturedRegion != "eu-west-1" {
		t.Errorf("captured region = %q, want eu-west-1 (no enforcement)", capturedRegion)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("expected no warnings when no region data, got %v", plan.Warnings)
	}
}

// TestBuildWithRegionWarnings_BuildError propagates errors from the build function.
func TestBuildWithRegionWarnings_BuildError(t *testing.T) {
	opts := Options{Region: "us-east-1"}
	_, err := buildWithRegionWarnings(opts, "model", nil, "", nil, func(region string) (ConfigPlan, error) {
		return ConfigPlan{}, fmt.Errorf("build failed")
	})
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Errorf("expected build error propagated, got %v", err)
	}
}

// TestBuildWithRegionWarnings_CodexIdentity verifies buildWithRegionWarnings
// produces the same region resolution and warning format as Codex BuildConfig
// — the shared helper is the ONLY path; no per-provider logic remains.
func TestBuildWithRegionWarnings_CodexIdentity(t *testing.T) {
	// gpt-5.5 known regions: us-east-1, us-east-2
	// Requested region: us-west-2 (default, not explicit) → auto-switch to us-east-1
	opts := Options{
		Region:         "us-west-2",
		RegionExplicit: false,
	}

	plan, err := buildWithRegionWarnings(opts, "openai.gpt-5.5", []string{"us-east-1", "us-east-2"}, ": ", nil, func(region string) (ConfigPlan, error) {
		return ConfigPlan{Keys: map[string]any{"region": region}}, nil
	})
	if err != nil {
		t.Fatalf("buildWithRegionWarnings: %v", err)
	}
	if plan.Keys["region"] != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", plan.Keys["region"])
	}
	// Verify exact warning format matches what Codex BuildConfig produces
	// ("modelID: message" with Codex's ": " separator)
	if len(plan.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(plan.Warnings))
	}
	w := plan.Warnings[0]
	if !strings.HasPrefix(w, "openai.gpt-5.5: ") {
		t.Errorf("Codex warning format should be 'modelID: msg', got %q", w)
	}
	if !strings.Contains(w, "default region us-west-2") {
		t.Errorf("warning should mention default region, got %q", w)
	}
	if !strings.Contains(w, "using us-east-1 instead") {
		t.Errorf("warning should mention target region, got %q", w)
	}
}

// TestBuildWithRegionWarnings_GrokIdentity verifies buildWithRegionWarnings
// produces the same region resolution and warning format as Grok BuildConfig.
func TestBuildWithRegionWarnings_GrokIdentity(t *testing.T) {
	// grok-4.3 known regions: us-east-1, us-east-2, us-west-2
	// Requested region: eu-west-1 (explicit) → override to us-east-1
	opts := Options{
		Region:         "eu-west-1",
		RegionExplicit: true,
	}

	plan, err := buildWithRegionWarnings(opts, "xai.grok-4-3", []string{"us-east-1", "us-east-2", "us-west-2"}, " ", nil, func(region string) (ConfigPlan, error) {
		return ConfigPlan{Keys: map[string]any{"region": region}}, nil
	})
	if err != nil {
		t.Fatalf("buildWithRegionWarnings: %v", err)
	}
	if plan.Keys["region"] != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", plan.Keys["region"])
	}
	// Verify exact warning format matches Grok's "modelID message" (space separator)
	if len(plan.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(plan.Warnings))
	}
	w := plan.Warnings[0]
	if !strings.HasPrefix(w, "xai.grok-4-3 ") {
		t.Errorf("Grok warning format should be 'modelID msg', got %q", w)
	}
	if !strings.Contains(w, "requested region eu-west-1") {
		t.Errorf("warning should mention requested region, got %q", w)
	}
}
