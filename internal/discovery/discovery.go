// Package discovery queries AWS Bedrock's live model catalog. It is the ONLY
// package in Juggernaut that imports aws-sdk-go-v2 — every other package
// consumes just DiscoveredModel, so a future SDK bump or swap touches only
// this file.
package discovery

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
)

// DiscoveredModel is Juggernaut's own view of a Bedrock model or inference
// profile, decoupled from the AWS SDK's response shape.
type DiscoveredModel struct {
	ID       string // e.g. "anthropic.claude-opus-4-8" or "global.anthropic.claude-fable-5"
	Status   string // "ACTIVE", "LEGACY", or "UNKNOWN" if AWS omits lifecycle info
	Provider string // e.g. "Anthropic"; empty for inference profiles (not returned there)
}

// bedrockClient is the subset of the real AWS SDK client this package needs.
// Satisfied by *bedrock.Client in production and a hand-written fake in tests.
type bedrockClient interface {
	ListFoundationModels(ctx context.Context, params *bedrock.ListFoundationModelsInput, optFns ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error)
	ListInferenceProfiles(ctx context.Context, params *bedrock.ListInferenceProfilesInput, optFns ...func(*bedrock.Options)) (*bedrock.ListInferenceProfilesOutput, error)
}

// newClient builds a real Bedrock client for region using the default AWS
// credential chain (IAM/SSO/env — whatever the caller already has configured).
func newClient(ctx context.Context, region string) (*bedrock.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return bedrock.NewFromConfig(cfg), nil
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
	out, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByProvider: &provider,
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
		models = append(models, DiscoveredModel{ID: id, Status: status, Provider: providerName})
	}
	return models, nil
}

func listInferenceProfilesWith(ctx context.Context, client bedrockClient) ([]DiscoveredModel, error) {
	out, err := client.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{})
	if err != nil {
		return nil, fmt.Errorf("listing inference profiles: %w", err)
	}

	profiles := make([]DiscoveredModel, 0, len(out.InferenceProfileSummaries))
	for _, p := range out.InferenceProfileSummaries {
		id := ""
		if p.InferenceProfileId != nil {
			id = *p.InferenceProfileId
		}
		profiles = append(profiles, DiscoveredModel{ID: id, Status: string(p.Status)})
	}
	return profiles, nil
}
