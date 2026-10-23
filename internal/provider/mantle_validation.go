package provider

import (
	"fmt"
	"strings"
)

func validateOpenAIMantleModel(id, region string) error {
	model, err := openAIMantleModel(id)
	if err != nil {
		return err
	}
	if id != model {
		return fmt.Errorf("mantle config requires the canonical model ID %q", model)
	}
	if !regionAllowed(region, openAIMantleRegions[model]) {
		return fmt.Errorf("model %s is not served by mantle in %s", model, region)
	}
	return nil
}

func (c codex) ValidateMantleConfig(data map[string]any, region string) error {
	model, _ := data["model"].(string)
	if err := validateOpenAIMantleModel(model, region); err != nil {
		return err
	}
	providers, _ := data["model_providers"].(map[string]any)
	entry, _ := providers[CodexLegacyProviderID].(map[string]any)
	aws, _ := entry["aws"].(map[string]any)
	if data["model_provider"] != CodexLegacyProviderID || aws["region"] != region {
		return fmt.Errorf("mantle requires amazon-bedrock with aws.region=%s", region)
	}
	for _, key := range []string{"base_url", "env_key", "auth"} {
		if _, present := entry[key]; present {
			return fmt.Errorf("unexpected mantle provider override %s; re-apply to repair the native provider", key)
		}
	}
	return nil
}

func (o opencode) ValidateMantleConfig(data map[string]any, region string) error {
	model, _ := data["model"].(string)
	id, found := strings.CutPrefix(model, openCodeMantleProviderID+"/")
	if !found {
		return fmt.Errorf("mantle model must select the %s provider", openCodeMantleProviderID)
	}
	if err := validateOpenAIMantleModel(id, region); err != nil {
		return err
	}
	providers, _ := data["providers"].(map[string]any)
	entry, _ := providers[openCodeMantleProviderID].(map[string]any)
	settings, _ := entry["settings"].(map[string]any)
	if entry["package"] != openCodeMantlePackage || settings["region"] != region || settings["baseURL"] != openAIMantleURL(region) {
		return fmt.Errorf("mantle provider package, region or baseURL does not match the auth metadata")
	}
	models, _ := entry["models"].(map[string]any)
	if _, ok := models[id]; !ok {
		return fmt.Errorf("mantle model %s is missing from the provider's models", id)
	}
	return nil
}
