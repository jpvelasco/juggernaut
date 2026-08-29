package provider

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

func TestTask2_OpenCode_BuiltInProvider(t *testing.T) {
	p, _ := Get("opencode")
	opts := Options{
		Region: "us-west-2",
		Model:  "gpt-oss-120b",
		ModelCatalog: []CatalogModel{
			{ID: "openai.gpt-oss-120b-1:0", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"},
			{ID: "qwen.qwen3-coder-480b-a35b-v1:0", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"},
			{ID: "xai.grok-4.6", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"},
		},
		RefreshedSources: []string{"foundation", "profile"},
	}
	plan, err := p.BuildConfig(&bedrock.Config{}, opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	// Must use built-in amazon-bedrock, not bedrock-mantle
	if _, ok := plan.Keys["provider"].(map[string]any)["amazon-bedrock"]; !ok {
		t.Fatalf("expected provider.amazon-bedrock block, got keys %v", plan.Keys["provider"])
	}
	if _, ok := plan.Keys["provider"].(map[string]any)["bedrock-mantle"]; ok {
		t.Fatalf("must not contain bedrock-mantle after Task 2")
	}
	// options.region
	provMap := plan.Keys["provider"].(map[string]any)["amazon-bedrock"].(map[string]any)
	optMap, _ := provMap["options"].(map[string]any)
	if optMap["region"] != "us-west-2" {
		t.Errorf("region = %v, want us-west-2", optMap["region"])
	}
	if _, hasBaseURL := optMap["baseURL"]; hasBaseURL {
		t.Errorf("must not have baseURL for built-in provider")
	}
	if _, hasAPIKey := optMap["apiKey"]; hasAPIKey {
		t.Errorf("must not have apiKey for built-in provider")
	}
	if _, hasNPM := provMap["npm"]; hasNPM {
		t.Errorf("must not have npm for built-in provider")
	}
	// models and whitelist
	models, _ := provMap["models"].(map[string]any)
	if len(models) == 0 {
		t.Error("models map should be populated from catalog")
	}
	if wl, _ := provMap["whitelist"].([]string); len(wl) == 0 {
		// also accept []any
		if wl2, ok := provMap["whitelist"].([]any); !ok || len(wl2) == 0 {
			t.Error("whitelist should be populated")
		}
	}
	// top-level model
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/openai.gpt-oss-120b-1:0" {
		t.Errorf("model = %q, want amazon-bedrock/openai.gpt-oss-120b-1:0", m)
	}
}

func TestTask2_OpenCode_SupportsNativeRejectsMantle(t *testing.T) {
	p, _ := Get("opencode")
	if s := p.(interface {
		SupportsModel(CatalogModel) ModelSupport
	}).SupportsModel(CatalogModel{ID: "openai.gpt-oss-120b-1:0", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"}); !s.Supported {
		t.Errorf("should support foundation native model, got %v", s.Reason)
	}
	if s := p.(interface {
		SupportsModel(CatalogModel) ModelSupport
	}).SupportsModel(CatalogModel{ID: "openai.gpt-oss-120b", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"}); s.Supported {
		t.Errorf("should reject mantle source after Task 2")
	}
}

func TestTask2_OpenCode_AliasesNative(t *testing.T) {
	cases := map[string]string{
		"qwen3-coder":  "qwen.qwen3-coder-480b-a35b-v1:0",
		"grok-4.3":     "xai.grok-4.6",
		"gpt-oss-120b": "openai.gpt-oss-120b-1:0",
		"gpt-oss-20b":  "openai.gpt-oss-20b-1:0",
	}
	for alias, wantID := range cases {
		if got, ok := opencodeModelAliases[alias]; !ok || got != wantID {
			t.Errorf("alias %q = %q, want %q", alias, got, wantID)
		}
	}
}
