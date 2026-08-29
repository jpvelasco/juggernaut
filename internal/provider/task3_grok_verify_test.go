package provider

import "testing"

func TestTask3_Grok_NativeEndpoint(t *testing.T) {
	p, _ := Get("grok")
	opts := Options{Region: "us-west-2", Model: ""}
	plan, err := p.BuildConfig(nil, opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	m, _ := plan.Keys["model"].(map[string]any)["bedrock-grok"].(map[string]any)
	if m["model"] != "xai.grok-4.6" {
		t.Errorf("model = %v, want xai.grok-4.6", m["model"])
	}
	if base, _ := m["base_url"].(string); base != "https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1" {
		t.Errorf("base_url = %q, want bedrock-runtime", base)
	}
	if _, ok := m["api_backend"]; ok {
		t.Errorf("api_backend should be removed for native, got %v", m["api_backend"])
	}
}

func TestTask3_Grok_SupportsNative(t *testing.T) {
	p, _ := Get("grok")
	if s := p.(interface {
		SupportsModel(CatalogModel) ModelSupport
	}).SupportsModel(CatalogModel{ID: "xai.grok-4.6", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"}); !s.Supported {
		t.Errorf("should support foundation grok-4.6, got %v", s.Reason)
	}
	if s := p.(interface {
		SupportsModel(CatalogModel) ModelSupport
	}).SupportsModel(CatalogModel{ID: "xai.grok-4.3", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"}); s.Supported {
		t.Errorf("should reject mantle after Task 3")
	}
}
