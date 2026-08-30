// Package discovery queries AWS Bedrock's live model catalog. It is the ONLY
// package in Juggernaut that imports aws-sdk-go-v2 — every other package
// consumes just DiscoveredModel, so a future SDK bump or swap touches only
// this file.
package discovery

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
)

// Source identifies the Bedrock catalog endpoint that returned a model.
type Source string

const (
	SourceFoundation Source = "foundation"
	SourceProfile    Source = "profile"
)

// DiscoveredModel is Juggernaut's own view of a Bedrock model or inference
// profile, decoupled from the AWS SDK's response shape.
type DiscoveredModel struct {
	ID           string `json:"id"`           // e.g. "anthropic.claude-opus-4-8" or "openai.gpt-oss-120b"
	Status       string `json:"status"`       // "ACTIVE", "LEGACY", or "UNKNOWN" if AWS omits lifecycle info
	Availability string `json:"availability"` // "AVAILABLE", "NOT_AVAILABLE", or "UNKNOWN"
	Provider     string `json:"provider"`     // e.g. "Anthropic"; empty when the endpoint omits it
	Source       Source `json:"source"`       // foundation or profile
}

// bedrockClient is the subset of the real AWS SDK client this package needs.
// Satisfied by *bedrock.Client in production and a hand-written fake in tests.
type bedrockClient interface {
	ListFoundationModels(ctx context.Context, params *bedrock.ListFoundationModelsInput, optFns ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error)
	GetFoundationModelAvailability(ctx context.Context, params *bedrock.GetFoundationModelAvailabilityInput, optFns ...func(*bedrock.Options)) (*bedrock.GetFoundationModelAvailabilityOutput, error)
	ListInferenceProfiles(ctx context.Context, params *bedrock.ListInferenceProfilesInput, optFns ...func(*bedrock.Options)) (*bedrock.ListInferenceProfilesOutput, error)
}

// AWS construction is kept behind package variables so tests can exercise the
// public discovery entry points without contacting AWS.
var loadDefaultAWSConfig = config.LoadDefaultConfig
var makeBedrockClient = func(cfg aws.Config) bedrockClient { return bedrock.NewFromConfig(cfg) }

// loadAWSConfig returns an AWS config for the given region using the default
// credential chain. Shared by newClient and CallerAccount.
func loadAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	cfg, err := loadDefaultAWSConfig(ctx, config.WithRegion(region))
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading AWS config: %w", err)
	}
	return cfg, nil
}

// newClient builds a real Bedrock client for region using the default AWS
// credential chain (IAM/SSO/env — whatever the caller already has configured).
func newClient(ctx context.Context, region string) (bedrockClient, error) {
	cfg, err := loadAWSConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	return makeBedrockClient(cfg), nil
}

// ListAnthropicModels queries Bedrock's live foundation-model catalog,
// filtered to Anthropic, using the AWS SDK's default credential chain.
func ListAnthropicModels(ctx context.Context, region string) ([]DiscoveredModel, error) {
	client, err := newClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return listAnthropicModelsWith(ctx, client)
}

// ListFoundationModels queries Bedrock's complete legacy/native foundation
// model catalog. Unlike ListAnthropicModels it intentionally does not filter by
// provider, so callers can inventory everything visible in the account/region.
func ListFoundationModels(ctx context.Context, region string) ([]DiscoveredModel, error) {
	client, err := newClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return listAvailableFoundationModelsWith(ctx, client)
}

func listAvailableFoundationModelsWith(ctx context.Context, client bedrockClient) ([]DiscoveredModel, error) {
	models, err := listFoundationModelsWith(ctx, client, nil)
	if err != nil {
		return nil, err
	}
	for i := range models {
		if models[i].ID == "" {
			continue
		}
		out, availabilityErr := client.GetFoundationModelAvailability(ctx, &bedrock.GetFoundationModelAvailabilityInput{
			ModelId: &models[i].ID,
		})
		if availabilityErr != nil {
			return nil, fmt.Errorf("checking availability of foundation model %s: %w", models[i].ID, availabilityErr)
		}
		models[i].Availability = foundationModelAvailability(out)
	}
	return models, nil
}

// ListInferenceProfiles queries Bedrock's live cross-region inference profile
// catalog (the global./us. prefixed IDs bedrock-config.json pins).
func ListInferenceProfiles(ctx context.Context, region string) ([]DiscoveredModel, error) {
	client, err := newClient(ctx, region)
	if err != nil {
		return nil, err
	}
	return listInferenceProfilesWith(ctx, client)
}

func listAnthropicModelsWith(ctx context.Context, client bedrockClient) ([]DiscoveredModel, error) {
	provider := "Anthropic"
	return listFoundationModelsWith(ctx, client, &provider)
}

func listFoundationModelsWith(ctx context.Context, client bedrockClient, provider *string) ([]DiscoveredModel, error) {
	out, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByProvider: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("listing foundation models: %w", err)
	}

	models := make([]DiscoveredModel, 0, len(out.ModelSummaries))
	for _, m := range out.ModelSummaries {
		status := "UNKNOWN"
		if m.ModelLifecycle != nil {
			status = string(m.ModelLifecycle.Status)
		}
		id := ""
		if m.ModelId != nil {
			id = *m.ModelId
		}
		providerName := ""
		if m.ProviderName != nil {
			providerName = *m.ProviderName
		}
		models = append(models, DiscoveredModel{
			ID:           id,
			Status:       status,
			Availability: "UNKNOWN",
			Provider:     providerName,
			Source:       SourceFoundation,
		})
	}
	return models, nil
}

func foundationModelAvailability(out *bedrock.GetFoundationModelAvailabilityOutput) string {
	if out == nil {
		return "UNKNOWN"
	}
	agreementAvailable := out.AgreementAvailability != nil && out.AgreementAvailability.Status == "AVAILABLE"
	if agreementAvailable && out.AuthorizationStatus == "AUTHORIZED" &&
		out.EntitlementAvailability == "AVAILABLE" && out.RegionAvailability == "AVAILABLE" {
		return "AVAILABLE"
	}
	return "NOT_AVAILABLE"
}

func listInferenceProfilesWith(ctx context.Context, client bedrockClient) ([]DiscoveredModel, error) {
	var profiles []DiscoveredModel
	var nextToken *string
	for {
		out, err := client.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{NextToken: nextToken})
		if err != nil {
			return nil, fmt.Errorf("listing inference profiles: %w", err)
		}

		for _, p := range out.InferenceProfileSummaries {
			id := ""
			if p.InferenceProfileId != nil {
				id = *p.InferenceProfileId
			}
			profiles = append(profiles, DiscoveredModel{
				ID: id, Status: string(p.Status), Availability: "AVAILABLE", Source: SourceProfile,
			})
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}
	return profiles, nil
}
