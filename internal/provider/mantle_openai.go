package provider

import (
	"fmt"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

const openCodeMantlePackage = "@opencode/ai/providers/amazon-bedrock/mantle/responses"

func openAIMantleURL(region string) string {
	return fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1", region)
}

// openAIMantleRegions records in-region Responses availability from the AWS
// model cards (2026-10-23). Runtime CRIS availability is deliberately separate.
// https://docs.aws.amazon.com/bedrock/latest/userguide/models-region-compatibility.html
var openAIMantleRegions = map[string][]string{
	"openai.gpt-6.1-sol":   {"us-east-1"},
	"openai.gpt-6-sol":     {"us-east-1"},
	"openai.gpt-6-luna":    {"us-east-1"},
	"openai.gpt-6-astra":   {"us-east-1", "us-west-2"},
	"openai.gpt-5.6-terra": {"us-east-1", "us-east-2", "us-west-2", "us-gov-west-1", "us-gov-east-1"},
	"openai.gpt-5.6-luna":  {"us-east-1", "us-east-2", "us-west-2", "us-gov-west-1", "us-gov-east-1"},
	"openai.gpt-5.5":       {"us-east-1", "us-east-2"},
	"openai.gpt-5.4":       {"us-east-1", "us-east-2", "us-west-2", "us-gov-west-1"},
}

func openAIMantleModel(key string) (string, error) {
	if key == "" {
		key = "astra"
	}
	if m, ok := codexModels[key]; ok {
		key = bedrock.StripRegionPrefix(m.ModelID)
	}
	if bedrock.StripRegionPrefix(key) != key {
		return "", fmt.Errorf("%q is a cross-region inference profile; mantle requires a bare openai.* model ID", key)
	}
	if strings.HasPrefix(key, "gpt-") {
		key = "openai." + key
	}
	if _, ok := openAIMantleRegions[key]; !ok {
		return "", fmt.Errorf("unsupported OpenAI mantle Responses model %q; use openai.gpt-6-astra, openai.gpt-5.4, or another documented mantle Responses model", key)
	}
	return key, nil
}

func openAIMantleModelRegions(key string) []string {
	id, err := openAIMantleModel(key)
	if err != nil {
		return nil
	}
	return openAIMantleRegions[id]
}

func (c codex) MantleRegions(key string) []string    { return openAIMantleModelRegions(key) }
func (o opencode) MantleRegions(key string) []string { return openAIMantleModelRegions(key) }

func (o opencode) buildMantleConfig(opts Options) (ConfigPlan, error) {
	id, err := openAIMantleModel(opts.Model)
	if err != nil {
		return ConfigPlan{}, err
	}
	region, message, _ := resolveModelRegion(opts.Region, opts.RegionExplicit, openAIMantleRegions[id])
	// Deep-merge retains previous model entries. Explicit disabled markers hide
	// roster models that a previous apply exposed in another region/account.
	models := make(map[string]any, len(openAIMantleRegions))
	for model := range openAIMantleRegions {
		models[model] = map[string]any{"disabled": true}
	}
	models[id] = mantleOpenCodeModel(id)
	for _, m := range opts.ModelCatalog {
		if m.Source != "foundation" || checkModelPreconditions(m).Reason != "" || !regionAllowed(region, openAIMantleRegions[m.ID]) {
			continue
		}
		models[m.ID] = mantleOpenCodeModel(m.ID)
	}
	plan := ConfigPlan{
		Keys: map[string]any{
			"model": openCodeMantleProviderID + "/" + id,
			"providers": map[string]any{openCodeMantleProviderID: map[string]any{
				"package": openCodeMantlePackage,
				"env":     []string{authmode.BedrockAuthEnvName},
				"settings": map[string]any{
					"region":  region,
					"baseURL": openAIMantleURL(region),
				},
				"models": models,
			}},
		},
		ManagedKeys: []string{"model", "providers"},
	}
	if message != "" {
		plan.Warnings = append(plan.Warnings, id+" "+message)
	}
	return plan, nil
}

func mantleOpenCodeModel(id string) map[string]any {
	return map[string]any{"name": id, "disabled": false, "capabilities": map[string]any{"tools": true}}
}

// CleanEndpoint removes the inactive OpenCode routing table after a switch.
// Only the provider entries Juggernaut owns are removed; other vendors survive.
func (o opencode) CleanEndpoint(existing map[string]any, endpoint string) {
	key, id := "providers", openCodeMantleProviderID
	if endpoint == bedrock.EndpointMantle {
		key, id = "provider", bedrockProviderID
	}
	table, ok := existing[key].(map[string]any)
	if !ok {
		return
	}
	delete(table, id)
	if len(table) == 0 {
		delete(existing, key)
	}
}
