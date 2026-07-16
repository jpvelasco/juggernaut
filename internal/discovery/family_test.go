package discovery

import "testing"

func TestMatchTier(t *testing.T) {
	cases := []struct {
		id       string
		wantTier Tier
		wantOK   bool
	}{
		{"anthropic.claude-opus-4-8", TierOpus, true},
		{"anthropic.claude-sonnet-5", TierSonnet, true},
		{"anthropic.claude-haiku-4-5-20251001-v1:0", TierHaiku, true},
		{"anthropic.claude-fable-5", TierFable, true},
		{"global.anthropic.claude-opus-4-9", TierOpus, true},
		{"amazon.nova-pro-v1:0", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := MatchTier(c.id)
		if got != c.wantTier || ok != c.wantOK {
			t.Errorf("MatchTier(%q) = (%q, %v), want (%q, %v)", c.id, got, ok, c.wantTier, c.wantOK)
		}
	}
}

func TestFormatCandidate(t *testing.T) {
	m := DiscoveredModel{ID: "anthropic.claude-opus-4-9", Status: "ACTIVE", Provider: "Anthropic"}
	got := FormatCandidate(m)
	want := "anthropic.claude-opus-4-9 (ACTIVE)"
	if got != want {
		t.Errorf("FormatCandidate = %q, want %q", got, want)
	}
}

func TestAllTiers_ContainsExactlyFourInStableOrder(t *testing.T) {
	want := []Tier{TierOpus, TierSonnet, TierHaiku, TierFable}
	if len(AllTiers) != len(want) {
		t.Fatalf("AllTiers has %d entries, want %d", len(AllTiers), len(want))
	}
	for i, tier := range want {
		if AllTiers[i] != tier {
			t.Errorf("AllTiers[%d] = %q, want %q", i, AllTiers[i], tier)
		}
	}
}
