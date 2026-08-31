package provider

import "testing"

func TestTask4_Codex_SolDefault(t *testing.T) {
	p, _ := Get("codex")
	opts := Options{Region: "us-west-2", Model: ""}
	plan, err := p.BuildConfig(nil, opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "global.openai.gpt-5.6-sol" {
		t.Errorf("default model = %q, want global.openai.gpt-5.6-sol", m)
	}
}

func TestTask4_Codex_SupportsNative(t *testing.T) {
	p, _ := Get("codex")
	if s := p.(interface {
		SupportsModel(CatalogModel) ModelSupport
	}).SupportsModel(CatalogModel{ID: "openai.gpt-5.6-sol", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"}); !s.Supported {
		t.Errorf("should support foundation gpt-5.6-sol, got %v", s.Reason)
	}
	if s := p.(interface {
		SupportsModel(CatalogModel) ModelSupport
	}).SupportsModel(CatalogModel{ID: "openai.gpt-5.5", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"}); s.Supported {
		t.Errorf("should reject mantle after Task 4")
	}
}

func TestTask4_Codex_Aliases(t *testing.T) {
	// The GPT-5.6 family is INFERENCE_PROFILE-only; codexModel normalizes all
	// alias and raw-ID forms to the global. profile ID.
	cases := map[string]string{
		"sol":                       "global.openai.gpt-5.6-sol",
		"terra":                     "global.openai.gpt-5.6-terra",
		"luna":                      "global.openai.gpt-5.6-luna",
		"gpt-5.6-sol":               "global.openai.gpt-5.6-sol",
		"openai.gpt-5.6-sol":        "global.openai.gpt-5.6-sol",
		"global.openai.gpt-5.6-sol": "global.openai.gpt-5.6-sol",
	}
	for alias, wantID := range cases {
		m, ok := codexModel(alias)
		if !ok || m.ModelID != wantID {
			t.Errorf("codexModel(%q) = %v, want %q", alias, m.ModelID, wantID)
		}
	}
	if _, ok := codexModel("gpt-5.5"); ok {
		t.Error("gpt-5.5 should no longer be known (retired)")
	}
}
