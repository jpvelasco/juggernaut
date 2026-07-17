package discovery

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// TestListAnthropicModelsWith_MultipleStatuses verifies all lifecycle statuses
// are correctly mapped from the SDK enum to string.
func TestListAnthropicModelsWith_MultipleStatuses(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []types.FoundationModelSummary{
				{
					ModelId:      strPtr("anthropic.claude-opus-4-8"),
					ProviderName: strPtr("Anthropic"),
					ModelLifecycle: &types.FoundationModelLifecycle{
						Status: types.FoundationModelLifecycleStatusActive,
					},
				},
				{
					ModelId:      strPtr("anthropic.claude-sonnet-4-0"),
					ProviderName: strPtr("Anthropic"),
					ModelLifecycle: &types.FoundationModelLifecycle{
						Status: types.FoundationModelLifecycleStatusLegacy,
					},
				},
			},
		},
	}
	got, err := listAnthropicModelsWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listAnthropicModelsWith: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 models, got %d", len(got))
	}
	if got[0].Status != "ACTIVE" {
		t.Errorf("got[0].Status = %q, want ACTIVE", got[0].Status)
	}
	if got[1].Status != "LEGACY" {
		t.Errorf("got[1].Status = %q, want LEGACY", got[1].Status)
	}
}

// TestListInferenceProfilesWith_LegacyProfile verifies legacy inference
// profiles are correctly reported.
func TestListInferenceProfilesWith_LegacyProfile(t *testing.T) {
	client := &fakeBedrockClient{
		inferenceProfilesOut: &bedrock.ListInferenceProfilesOutput{
			InferenceProfileSummaries: []types.InferenceProfileSummary{
				{
					InferenceProfileId: strPtr("global.anthropic.claude-sonnet-4-0"),
					Status:             "LEGACY",
				},
			},
		},
	}
	got, err := listInferenceProfilesWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listInferenceProfilesWith: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(got))
	}
	if got[0].Status != "LEGACY" {
		t.Errorf("Status = %q, want LEGACY", got[0].Status)
	}
}

// TestListInferenceProfilesWith_MultipleProfiles verifies multiple profiles
// including mixed statuses.
func TestListInferenceProfilesWith_MultipleProfiles(t *testing.T) {
	client := &fakeBedrockClient{
		inferenceProfilesOut: &bedrock.ListInferenceProfilesOutput{
			InferenceProfileSummaries: []types.InferenceProfileSummary{
				{InferenceProfileId: strPtr("global.anthropic.claude-opus-4-8"), Status: types.InferenceProfileStatusActive},
				{InferenceProfileId: strPtr("global.anthropic.claude-sonnet-4-0"), Status: "LEGACY"},
				{InferenceProfileId: strPtr("global.anthropic.claude-haiku-4-5"), Status: types.InferenceProfileStatusActive},
			},
		},
	}
	got, err := listInferenceProfilesWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listInferenceProfilesWith: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(got))
	}
	if got[0].Status != "ACTIVE" || got[0].ID != "global.anthropic.claude-opus-4-8" {
		t.Errorf("profile[0] = %+v", got[0])
	}
	if got[1].Status != "LEGACY" {
		t.Errorf("profile[1].Status = %q, want LEGACY", got[1].Status)
	}
	if got[2].Status != "ACTIVE" {
		t.Errorf("profile[2].Status = %q, want ACTIVE", got[2].Status)
	}
}

// TestMatchTier_CaseSensitivity verifies MatchTier is case-sensitive.
func TestMatchTier_CaseSensitivity(t *testing.T) {
	// "OPUS" (uppercase) should not match.
	_, ok := MatchTier("anthropic.claude-OPUS-4-8")
	if ok {
		t.Error("expected case-sensitive match to fail for uppercase tier")
	}
}

// TestMatchTier_PartialSubstring verifies substring matching behavior.
func TestMatchTier_PartialSubstring(t *testing.T) {
	// A model ID containing "sonnet" as a substring should match.
	tier, ok := MatchTier("my-sonnet-model-v2")
	if !ok || tier != TierSonnet {
		t.Errorf("MatchTier(my-sonnet-model-v2) = (%q, %v), want (sonnet, true)", tier, ok)
	}
}

// TestFormatCandidate_LegacyStatus verifies formatting for legacy models.
func TestFormatCandidate_LegacyStatus(t *testing.T) {
	m := DiscoveredModel{ID: "anthropic.claude-sonnet-4-0", Status: "LEGACY"}
	got := FormatCandidate(m)
	want := "anthropic.claude-sonnet-4-0 (LEGACY)"
	if got != want {
		t.Errorf("FormatCandidate = %q, want %q", got, want)
	}
}

// TestFormatCandidate_UnknownStatus verifies formatting for unknown status.
func TestFormatCandidate_UnknownStatus(t *testing.T) {
	m := DiscoveredModel{ID: "anthropic.claude-weird", Status: "UNKNOWN"}
	got := FormatCandidate(m)
	want := "anthropic.claude-weird (UNKNOWN)"
	if got != want {
		t.Errorf("FormatCandidate = %q, want %q", got, want)
	}
}

// TestMatchTier_InferenceProfile verifies tier matching for global-prefixed
// inference profile IDs.
func TestMatchTier_InferenceProfile(t *testing.T) {
	cases := []struct {
		id       string
		wantTier Tier
		wantOK   bool
	}{
		{"global.anthropic.claude-opus-4-8", TierOpus, true},
		{"global.anthropic.claude-sonnet-4-20250514-v1:0", TierSonnet, true},
		{"global.anthropic.claude-haiku-4-5-20251001-v1:0", TierHaiku, true},
		{"global.anthropic.claude-fable-5", TierFable, true},
		{"us.anthropic.claude-opus-4-8", TierOpus, true},
	}
	for _, c := range cases {
		got, ok := MatchTier(c.id)
		if got != c.wantTier || ok != c.wantOK {
			t.Errorf("MatchTier(%q) = (%q, %v), want (%q, %v)", c.id, got, ok, c.wantTier, c.wantOK)
		}
	}
}

// TestListAnthropicModelsWith_ProviderNameVerification verifies provider name
// is correctly extracted from the SDK response.
func TestListAnthropicModelsWith_ProviderNameVerification(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []types.FoundationModelSummary{
				{
					ModelId:      strPtr("anthropic.claude-opus-4-8"),
					ProviderName: strPtr("Anthropic"),
					ModelLifecycle: &types.FoundationModelLifecycle{
						Status: types.FoundationModelLifecycleStatusActive,
					},
				},
			},
		},
	}
	got, err := listAnthropicModelsWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listAnthropicModelsWith: %v", err)
	}
	if got[0].Provider != "Anthropic" {
		t.Errorf("Provider = %q, want Anthropic", got[0].Provider)
	}
}

// TestListInferenceProfilesWith_ProviderEmptyForProfiles verifies inference
// profiles have empty Provider (not returned by the API).
func TestListInferenceProfilesWith_ProviderEmptyForProfiles(t *testing.T) {
	client := &fakeBedrockClient{
		inferenceProfilesOut: &bedrock.ListInferenceProfilesOutput{
			InferenceProfileSummaries: []types.InferenceProfileSummary{
				{
					InferenceProfileId: strPtr("global.anthropic.claude-fable-5"),
					Status:             types.InferenceProfileStatusActive,
				},
			},
		},
	}
	got, err := listInferenceProfilesWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listInferenceProfilesWith: %v", err)
	}
	if got[0].Provider != "" {
		t.Errorf("Provider = %q, want empty for inference profiles", got[0].Provider)
	}
}
