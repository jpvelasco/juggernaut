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
	if m["model"] != "global.xai.grok-4.7" {
		t.Errorf("model = %v, want global.xai.grok-4.7", m["model"])
	}
	if base, _ := m["base_url"].(string); base != "https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1" {
		t.Errorf("base_url = %q, want bedrock-runtime", base)
	}
	// api_backend is mantle-only; runtime plans carry the empty-string delete
	// sentinel so a mantle→runtime apply clears it from the deep-merged table.
	if got, ok := m["api_backend"]; !ok || got != "" {
		t.Errorf("api_backend = %v, want the empty deletion sentinel for native", got)
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
