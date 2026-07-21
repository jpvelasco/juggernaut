package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// fakeBedrockClient is a hand-written stand-in for the real SDK client, used
// by every test in this package. No test ever calls real AWS.
type fakeBedrockClient struct {
	foundationModelsOut       *bedrock.ListFoundationModelsOutput
	foundationModelsErr       error
	foundationAvailabilityOut map[string]*bedrock.GetFoundationModelAvailabilityOutput
	foundationAvailabilityErr error
	inferenceProfilesOut      *bedrock.ListInferenceProfilesOutput
	inferenceProfilePages     map[string]*bedrock.ListInferenceProfilesOutput
	inferenceProfilesErr      error
}

func (f *fakeBedrockClient) ListFoundationModels(_ context.Context, _ *bedrock.ListFoundationModelsInput, _ ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error) {
	return f.foundationModelsOut, f.foundationModelsErr
}

func (f *fakeBedrockClient) GetFoundationModelAvailability(_ context.Context, in *bedrock.GetFoundationModelAvailabilityInput, _ ...func(*bedrock.Options)) (*bedrock.GetFoundationModelAvailabilityOutput, error) {
	if f.foundationAvailabilityErr != nil {
		return nil, f.foundationAvailabilityErr
	}
	if in == nil || in.ModelId == nil {
		return nil, nil
	}
	return f.foundationAvailabilityOut[*in.ModelId], nil
}

func (f *fakeBedrockClient) ListInferenceProfiles(_ context.Context, in *bedrock.ListInferenceProfilesInput, _ ...func(*bedrock.Options)) (*bedrock.ListInferenceProfilesOutput, error) {
	if f.inferenceProfilePages != nil {
		token := ""
		if in != nil && in.NextToken != nil {
			token = *in.NextToken
		}
		return f.inferenceProfilePages[token], f.inferenceProfilesErr
	}
	return f.inferenceProfilesOut, f.inferenceProfilesErr
}

func strPtr(s string) *string { return &s }

func TestPublicBedrockDiscovery_UsesConfiguredRegion(t *testing.T) {
	originalLoadConfig := loadDefaultAWSConfig
	originalMakeClient := makeBedrockClient
	t.Cleanup(func() {
		loadDefaultAWSConfig = originalLoadConfig
		makeBedrockClient = originalMakeClient
	})

	const region = "us-test-1"
	loadDefaultAWSConfig = func(_ context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
		options := config.LoadOptions{}
		for _, optFn := range optFns {
			if err := optFn(&options); err != nil {
				return aws.Config{}, err
			}
		}
		return aws.Config{Region: options.Region}, nil
	}
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{ModelSummaries: []types.FoundationModelSummary{{
			ModelId:        strPtr("anthropic.claude-opus-4-8"),
			ModelLifecycle: &types.FoundationModelLifecycle{Status: types.FoundationModelLifecycleStatusActive},
		}}},
		foundationAvailabilityOut: map[string]*bedrock.GetFoundationModelAvailabilityOutput{
			"anthropic.claude-opus-4-8": {
				AgreementAvailability:   &types.AgreementAvailability{Status: types.AgreementStatusAvailable},
				AuthorizationStatus:     types.AuthorizationStatusAuthorized,
				EntitlementAvailability: types.EntitlementAvailabilityAvailable,
				RegionAvailability:      types.RegionAvailabilityAvailable,
			},
		},
		inferenceProfilesOut: &bedrock.ListInferenceProfilesOutput{InferenceProfileSummaries: []types.InferenceProfileSummary{{
			InferenceProfileId: strPtr("global.anthropic.claude-opus-4-8"),
			Status:             types.InferenceProfileStatusActive,
		}}},
	}
	makeBedrockClient = func(cfg aws.Config) bedrockClient {
		if cfg.Region != region {
			t.Fatalf("AWS region = %q, want %q", cfg.Region, region)
		}
		return client
	}

	tests := []struct {
		name string
		list func(context.Context, string) ([]DiscoveredModel, error)
	}{
		{name: "anthropic", list: ListAnthropicModels},
		{name: "foundation", list: ListFoundationModels},
		{name: "profiles", list: ListInferenceProfiles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := tt.list(context.Background(), region)
			if err != nil {
				t.Fatalf("public discovery call: %v", err)
			}
			if len(models) != 1 {
				t.Fatalf("models = %+v, want one result", models)
			}
		})
	}
}

func TestPublicBedrockDiscovery_PropagatesConfigErrors(t *testing.T) {
	originalLoadConfig := loadDefaultAWSConfig
	t.Cleanup(func() { loadDefaultAWSConfig = originalLoadConfig })

	configErr := errors.New("configuration unavailable")
	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, configErr
	}

	tests := []struct {
		name string
		list func(context.Context, string) ([]DiscoveredModel, error)
	}{
		{name: "anthropic", list: ListAnthropicModels},
		{name: "foundation", list: ListFoundationModels},
		{name: "profiles", list: ListInferenceProfiles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.list(context.Background(), "us-test-1")
			if !errors.Is(err, configErr) {
				t.Fatalf("error = %v, want wrapped %v", err, configErr)
			}
		})
	}
}

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
	if got[0].ID != "anthropic.claude-opus-4-8" || got[0].Status != "ACTIVE" || got[0].Provider != "Anthropic" || got[0].Source != SourceFoundation {
		t.Errorf("unexpected first model: %+v", got[0])
	}
	if got[1].ID != "anthropic.claude-sonnet-4-20250514-v1:0" || got[1].Status != "LEGACY" {
		t.Errorf("unexpected second model: %+v", got[1])
	}
}

func TestListAvailableFoundationModelsWith_MapsAccountAvailability(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []types.FoundationModelSummary{
				{ModelId: strPtr("available"), ModelLifecycle: &types.FoundationModelLifecycle{Status: types.FoundationModelLifecycleStatusActive}},
				{ModelId: strPtr("blocked"), ModelLifecycle: &types.FoundationModelLifecycle{Status: types.FoundationModelLifecycleStatusActive}},
				{ModelId: strPtr("unknown"), ModelLifecycle: &types.FoundationModelLifecycle{Status: types.FoundationModelLifecycleStatusActive}},
				{ModelId: nil, ModelLifecycle: &types.FoundationModelLifecycle{Status: types.FoundationModelLifecycleStatusActive}},
			},
		},
		foundationAvailabilityOut: map[string]*bedrock.GetFoundationModelAvailabilityOutput{
			"available": {
				AgreementAvailability:   &types.AgreementAvailability{Status: types.AgreementStatusAvailable},
				AuthorizationStatus:     types.AuthorizationStatusAuthorized,
				EntitlementAvailability: types.EntitlementAvailabilityAvailable,
				RegionAvailability:      types.RegionAvailabilityAvailable,
			},
			"blocked": {
				AgreementAvailability:   &types.AgreementAvailability{Status: types.AgreementStatusAvailable},
				AuthorizationStatus:     types.AuthorizationStatusNotAuthorized,
				EntitlementAvailability: types.EntitlementAvailabilityAvailable,
				RegionAvailability:      types.RegionAvailabilityAvailable,
			},
			"unknown": nil,
		},
	}

	got, err := listAvailableFoundationModelsWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listAvailableFoundationModelsWith: %v", err)
	}
	if len(got) != 4 || got[0].Availability != "AVAILABLE" || got[1].Availability != "NOT_AVAILABLE" || got[2].Availability != "UNKNOWN" || got[3].Availability != "UNKNOWN" {
		t.Fatalf("unexpected availability mapping: %+v", got)
	}
}

func TestListAvailableFoundationModelsWith_PropagatesListError(t *testing.T) {
	listErr := errors.New("listing denied")
	client := &fakeBedrockClient{foundationModelsErr: listErr}
	if _, err := listAvailableFoundationModelsWith(context.Background(), client); !errors.Is(err, listErr) {
		t.Fatalf("error = %v, want wrapped %v", err, listErr)
	}
}

func TestListAvailableFoundationModelsWith_PropagatesAvailabilityError(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{ModelSummaries: []types.FoundationModelSummary{
			{ModelId: strPtr("model-id")},
		}},
		foundationAvailabilityErr: errors.New("access denied"),
	}

	_, err := listAvailableFoundationModelsWith(context.Background(), client)
	if err == nil || !strings.Contains(err.Error(), "model-id") {
		t.Fatalf("expected model-specific availability error, got %v", err)
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

func TestListAnthropicModelsWith_NilModelIdAndProviderNameDefaultToEmpty(t *testing.T) {
	client := &fakeBedrockClient{
		foundationModelsOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []types.FoundationModelSummary{
				{
					ModelId:      nil,
					ProviderName: nil,
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
	if len(got) != 1 {
		t.Fatalf("expected 1 model, got %+v", got)
	}
	if got[0].ID != "" {
		t.Errorf("expected empty ID for nil ModelId, got %q", got[0].ID)
	}
	if got[0].Provider != "" {
		t.Errorf("expected empty Provider for nil ProviderName, got %q", got[0].Provider)
	}
	if got[0].Status != "ACTIVE" {
		t.Errorf("expected Status=ACTIVE unaffected by other nil fields, got %q", got[0].Status)
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

func TestListInferenceProfilesWith_Paginates(t *testing.T) {
	client := &fakeBedrockClient{
		inferenceProfilePages: map[string]*bedrock.ListInferenceProfilesOutput{
			"": {
				InferenceProfileSummaries: []types.InferenceProfileSummary{{
					InferenceProfileId: strPtr("first"), Status: types.InferenceProfileStatusActive,
				}},
				NextToken: strPtr("second-page"),
			},
			"second-page": {
				InferenceProfileSummaries: []types.InferenceProfileSummary{{
					InferenceProfileId: strPtr("second"), Status: types.InferenceProfileStatusActive,
				}},
			},
		},
	}

	got, err := listInferenceProfilesWith(context.Background(), client)
	if err != nil {
		t.Fatalf("listInferenceProfilesWith: %v", err)
	}
	if len(got) != 2 || got[0].ID != "first" || got[1].ID != "second" {
		t.Fatalf("unexpected paginated profiles: %+v", got)
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

func TestListInferenceProfilesWith_NilInferenceProfileIdDefaultsToEmpty(t *testing.T) {
	client := &fakeBedrockClient{
		inferenceProfilesOut: &bedrock.ListInferenceProfilesOutput{
			InferenceProfileSummaries: []types.InferenceProfileSummary{
				{
					InferenceProfileId: nil,
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
	if got[0].ID != "" {
		t.Errorf("expected empty ID for nil InferenceProfileId, got %q", got[0].ID)
	}
	if got[0].Status != "ACTIVE" {
		t.Errorf("expected Status=ACTIVE unaffected, got %q", got[0].Status)
	}
}

func TestListInferenceProfilesWith_PropagatesAPIError(t *testing.T) {
	client := &fakeBedrockClient{inferenceProfilesErr: errors.New("throttled")}
	_, err := listInferenceProfilesWith(context.Background(), client)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}
