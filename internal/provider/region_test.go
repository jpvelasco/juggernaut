package provider

import (
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
