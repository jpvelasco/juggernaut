package provider

import "testing"

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
