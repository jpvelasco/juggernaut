package provider

import (
	"strings"
	"testing"
)

func catalogModel(id, source string) CatalogModel {
	return CatalogModel{ID: id, Source: source, Status: "ACTIVE", Availability: "AVAILABLE"}
}

func TestProviders_ClassifyDiscoveredModels(t *testing.T) {
	tests := []struct {
		cli   string
		model CatalogModel
		want  bool
	}{
		{"claude", catalogModel("anthropic.claude-opus-4-8", "foundation"), true},
		{"claude", catalogModel("global.anthropic.claude-opus-4-8", "profile"), true},
		{"claude", catalogModel("amazon.nova-pro", "foundation"), false},
		{"codex", catalogModel("openai.gpt-5.6", "mantle"), true},
		{"codex", catalogModel("openai.gpt-oss-120b", "mantle"), false},
		{"opencode", catalogModel("moonshotai.kimi-k2.5", "mantle"), true},
		{"opencode", catalogModel("zai.glm-5", "mantle"), true},
		{"opencode", catalogModel("qwen.qwen3-coder-next", "mantle"), true},
		{"opencode", catalogModel("openai.gpt-5.6", "mantle"), false},
		{"grok", catalogModel("xai.grok-4.4", "mantle"), true},
		{"grok", catalogModel("moonshotai.kimi-k2.5", "mantle"), false},
	}
	for _, tt := range tests {
		t.Run(tt.cli+"/"+tt.model.ID, func(t *testing.T) {
			p, _ := Get(tt.cli)
			got := SupportsCatalogModel(p, tt.model)
			if got.Supported != tt.want {
				t.Fatalf("SupportsCatalogModel(%+v) = %+v, want supported=%v", tt.model, got, tt.want)
			}
		})
	}
}

func TestProviders_RejectInactiveAndUnavailableCatalogModels(t *testing.T) {
	for _, cli := range []string{"claude", "codex", "opencode", "grok"} {
		p, _ := Get(cli)
		id, source := "moonshotai.kimi-k2.5", "mantle"
		switch cli {
		case "claude":
			id, source = "anthropic.claude-opus-4-8", "foundation"
		case "codex":
			id = "openai.gpt-5.5"
		case "grok":
			id = "xai.grok-4.3"
		}
		inactive := catalogModel(id, source)
		inactive.Status = "LEGACY"
		if SupportsCatalogModel(p, inactive).Supported {
			t.Errorf("%s accepted inactive model", cli)
		}
		unavailable := catalogModel(id, source)
		unavailable.Availability = "NOT_AVAILABLE"
		if SupportsCatalogModel(p, unavailable).Supported {
			t.Errorf("%s accepted unavailable model", cli)
		}
	}
}

type providerWithoutCatalog struct{ Provider }

func TestCatalogProviderFallbacks(t *testing.T) {
	base := newTestProvider("minimal", "", "json", "minimal", nil)
	model := catalogModel("example.model", "mantle")
	if support := base.SupportsModel(model); support.Supported || !strings.Contains(support.Reason, "mantle") {
		t.Fatalf("base support = %+v", support)
	}
	if sources := base.CatalogSources(); sources != nil {
		t.Fatalf("base sources = %v", sources)
	}

	provider := providerWithoutCatalog{Provider: base}
	if support := SupportsCatalogModel(provider, model); support.Supported || !strings.Contains(support.Reason, "does not expose") {
		t.Fatalf("fallback support = %+v", support)
	}
	if sources := CatalogSourcesFor(provider); sources != nil {
		t.Fatalf("fallback sources = %v", sources)
	}
}

func TestOpenCode_BuildConfigAddsCompatibleLiveCatalog(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.ModelCatalog = []CatalogModel{
		catalogModel("moonshotai.kimi-k2.5", "mantle"),
		catalogModel("zai.glm-5", "mantle"),
		catalogModel("qwen.qwen3-coder-next", "mantle"),
		catalogModel("openai.gpt-5.5", "mantle"),
		{ID: "minimax.minimax-m2", Source: "mantle", Status: "INACTIVE", Availability: "AVAILABLE"},
	}
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := nestedMapChain(plan.Keys, "provider", mantleProviderID, "models")
	if !ok {
		t.Fatal("provider model inventory missing")
	}
	models := value.(map[string]any)
	for _, id := range []string{"openai.gpt-oss-120b", "moonshotai.kimi-k2.5", "zai.glm-5", "qwen.qwen3-coder-next"} {
		if _, ok := models[id]; !ok {
			t.Errorf("compatible/default model %q was not configured: %v", id, models)
		}
	}
	for _, id := range []string{"openai.gpt-5.5", "minimax.minimax-m2"} {
		if _, ok := models[id]; ok {
			t.Errorf("incompatible model %q was configured: %v", id, models)
		}
	}
}

func TestDynamicModelSelectionAndCatalogWarnings(t *testing.T) {
	tests := []struct {
		cli   string
		model string
		id    string
	}{
		{"codex", "gpt-5.6", "openai.gpt-5.6"},
		{"grok", "grok-4.4", "xai.grok-4.4"},
	}
	for _, tt := range tests {
		t.Run(tt.cli, func(t *testing.T) {
			p, _ := Get(tt.cli)
			opts := baseOpts()
			opts.Model = tt.model
			opts.ModelCatalog = []CatalogModel{catalogModel(tt.id, "mantle")}
			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig: %v", err)
			}
			if len(plan.Warnings) != 0 {
				t.Fatalf("available dynamic model warnings = %v", plan.Warnings)
			}

			opts.ModelCatalog = []CatalogModel{catalogModel("unrelated.model", "mantle")}
			plan, err = p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Warnings) == 0 || !strings.Contains(strings.Join(plan.Warnings, " "), "not returned as ACTIVE") {
				t.Fatalf("missing unavailable-catalog warning: %v", plan.Warnings)
			}
		})
	}
}
