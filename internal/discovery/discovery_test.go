package discovery

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// fakeBedrockClient is a hand-written stand-in for the real SDK client, used
// by every test in this package. No test ever calls real AWS.
type fakeBedrockClient struct {
	foundationModelsOut  *bedrock.ListFoundationModelsOutput
	foundationModelsErr  error
	inferenceProfilesOut *bedrock.ListInferenceProfilesOutput
	inferenceProfilesErr error
}

func (f *fakeBedrockClient) ListFoundationModels(_ context.Context, _ *bedrock.ListFoundationModelsInput, _ ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error) {
	return f.foundationModelsOut, f.foundationModelsErr
}

func (f *fakeBedrockClient) ListInferenceProfiles(_ context.Context, _ *bedrock.ListInferenceProfilesInput, _ ...func(*bedrock.Options)) (*bedrock.ListInferenceProfilesOutput, error) {
	return f.inferenceProfilesOut, f.inferenceProfilesErr
}

func strPtr(s string) *string { return &s }

func TestListAnthropicModelsWith_MapsActiveAndLegacy(t *testing.T) {
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
					ModelId:      strPtr("anthropic.claude-sonnet-4-20250514-v1:0"),
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
		t.Fatalf("expected 2 models, got %d: %+v", len(got), got)
	}
	if got[0].ID != "anthropic.claude-opus-4-8" || got[0].Status != "ACTIVE" || got[0].Provider != "Anthropic" {
		t.Errorf("unexpected first model: %+v", got[0])
	}
	if got[1].ID != "anthropic.claude-sonnet-4-20250514-v1:0" || got[1].Status != "LEGACY" {
		t.Errorf("unexpected second model: %+v", got[1])
	}
}

func TestListAnthropicModelsWith_EmptyCatalog(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{ModelSummaries: nil},
	}
	got, err := listAnthropicModelsWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listAnthropicModelsWith: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestListAnthropicModelsWith_MissingLifecycleDefaultsToUnknownNotEmpty(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []types.FoundationModelSummary{
				{ModelId: strPtr("anthropic.claude-weird"), ProviderName: strPtr("Anthropic"), ModelLifecycle: nil},
			},
		},
	}
	got, err := listAnthropicModelsWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listAnthropicModelsWith: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 model, got %+v", got)
	}
	if got[0].Status == "" {
		t.Error("expected a non-empty placeholder status when ModelLifecycle is nil, e.g. \"UNKNOWN\"")
	}
}

func TestListAnthropicModelsWith_PropagatesAPIError(t *testing.T) {
	client := &fakeBedrockClient{foundationModelsErr: errors.New("access denied")}
	_, err := listAnthropicModelsWith(context.Background(), client)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestListInferenceProfilesWith_MapsActiveProfiles(t *testing.T) {
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
	if len(got) != 1 {
		t.Fatalf("expected 1 profile, got %+v", got)
	}
	if got[0].ID != "global.anthropic.claude-fable-5" || got[0].Status != "ACTIVE" {
		t.Errorf("unexpected profile: %+v", got[0])
	}
}

func TestListInferenceProfilesWith_EmptyCatalog(t *testing.T) {
	client := &fakeBedrockClient{
		inferenceProfilesOut: &bedrock.ListInferenceProfilesOutput{InferenceProfileSummaries: nil},
	}
	got, err := listInferenceProfilesWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listInferenceProfilesWith: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestListInferenceProfilesWith_PropagatesAPIError(t *testing.T) {
	client := &fakeBedrockClient{inferenceProfilesErr: errors.New("throttled")}
	_, err := listInferenceProfilesWith(context.Background(), client)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}
