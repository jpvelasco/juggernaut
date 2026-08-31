package provider

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
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
		{"codex", catalogModel("openai.gpt-5.6-sol", "foundation"), true},
		{"codex", catalogModel("global.openai.gpt-5.6-sol", "foundation"), true},
		{"codex", catalogModel("us.openai.gpt-5.6-sol", "foundation"), true},
		{"codex", catalogModel("openai.gpt-oss-120b-1:0", "foundation"), false},
		{"opencode", catalogModel("moonshotai.kimi-k2.5", "foundation"), true},
		{"opencode", catalogModel("zai.glm-5", "foundation"), true},
		{"opencode", catalogModel("qwen.qwen3-coder-480b-a35b-v1:0", "foundation"), true},
		{"opencode", catalogModel("openai.gpt-oss-120b-1:0", "foundation"), true},
		{"grok", catalogModel("xai.grok-4.6", "foundation"), true},
		{"grok", catalogModel("moonshotai.kimi-k2.5", "foundation"), false},
		{"claude", catalogModel("anthropic.claude-opus-4-8", "mantle"), false},
		{"codex", catalogModel("openai.gpt-5.6-sol", "mantle"), false},
		{"opencode", catalogModel("moonshotai.kimi-k2.5", "mantle"), false},
		{"grok", catalogModel("xai.grok-4.6", "mantle"), false},
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

func TestProviders_CatalogSources(t *testing.T) {
	tests := []struct {
		cli  string
		want string
	}{
		{cli: "claude", want: "foundation,profile"},
		{cli: "codex", want: "foundation,profile"},
		{cli: "opencode", want: "foundation,profile"},
		{cli: "grok", want: "foundation,profile"},
	}
	for _, tt := range tests {
		t.Run(tt.cli, func(t *testing.T) {
			p, err := Get(tt.cli)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(CatalogSourcesFor(p), ","); got != tt.want {
				t.Fatalf("CatalogSourcesFor(%q) = %q, want %q", tt.cli, got, tt.want)
			}
		})
	}
}

func TestCodexModel_AcceptsRawDiscoveredID(t *testing.T) {
	// The GPT-5.6 family is INFERENCE_PROFILE-only; codexModel normalizes any
	// non-global form to the global. profile ID.
	cases := []struct {
		key, want string
	}{
		{"openai.gpt-5.6-sol", "global.openai.gpt-5.6-sol"},
		{"gpt-5.6-sol", "global.openai.gpt-5.6-sol"},
		{"global.openai.gpt-5.6-sol", "global.openai.gpt-5.6-sol"},
		{"sol", "global.openai.gpt-5.6-sol"},
	}
	for _, c := range cases {
		model, ok := codexModel(c.key)
		if !ok || model.ModelID != c.want {
			t.Fatalf("codexModel(%q) = %+v, %v; want ModelID %q", c.key, model, ok, c.want)
		}
	}
}

func TestProviders_RejectInactiveAndUnavailableCatalogModels(t *testing.T) {
	for _, cli := range []string{"claude", "codex", "opencode", "grok"} {
		p, _ := Get(cli)
		id, source := "moonshotai.kimi-k2.5", "foundation"
		switch cli {
		case "claude":
			id, source = "anthropic.claude-opus-4-8", "foundation"
		case "codex":
			id, source = "openai.gpt-5.6-sol", "foundation"
		case "grok":
			id, source = "xai.grok-4.6", "foundation"
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

func TestCheckModelPreconditions(t *testing.T) {
	// Active and available — passes through
	s := checkModelPreconditions(CatalogModel{ID: "x", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"})
	if s.Reason != "" {
		t.Fatalf("expected pass-through, got: %s", s.Reason)
	}

	// Inactive — rejected
	s = checkModelPreconditions(CatalogModel{ID: "x", Source: "mantle", Status: "LEGACY", Availability: "AVAILABLE"})
	if s.Reason != "model is not ACTIVE" {
		t.Fatalf("expected inactive rejection, got: %s", s.Reason)
	}

	// Unavailable — rejected
	s = checkModelPreconditions(CatalogModel{ID: "x", Source: "mantle", Status: "ACTIVE", Availability: "NOT_AVAILABLE"})
	if s.Reason != "model is not available to this AWS account" {
		t.Fatalf("expected unavailable rejection, got: %s", s.Reason)
	}
}

func TestBaseProvider_SupportsModel(t *testing.T) {
	base := newTestProvider("test", "", "json", "test", nil)

	// Active mantle model — falls through to "no compatibility rule"
	s := base.SupportsModel(catalogModel("x", "mantle"))
	if s.Supported {
		t.Fatalf("expected rejection, got supported")
	}
	if !strings.Contains(s.Reason, "mantle") {
		t.Fatalf("expected source in reason, got: %s", s.Reason)
	}

	// Inactive — rejected by pre-check
	s = base.SupportsModel(CatalogModel{ID: "x", Source: "mantle", Status: "LEGACY"})
	if s.Supported {
		t.Fatalf("expected rejection for inactive")
	}
	if s.Reason != "model is not ACTIVE" {
		t.Fatalf("expected inactive reason, got: %s", s.Reason)
	}

	// Unavailable — rejected by pre-check
	s = base.SupportsModel(CatalogModel{ID: "x", Source: "mantle", Status: "ACTIVE", Availability: "NOT_AVAILABLE"})
	if s.Supported {
		t.Fatalf("expected rejection for unavailable")
	}
	if s.Reason != "model is not available to this AWS account" {
		t.Fatalf("expected unavailable reason, got: %s", s.Reason)
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
		catalogModel("moonshotai.kimi-k2.5", "foundation"),
		catalogModel("zai.glm-5", "foundation"),
		catalogModel("qwen.qwen3-coder-480b-a35b-v1:0", "foundation"),
		catalogModel("openai.gpt-oss-120b-1:0", "foundation"),
		{ID: "minimax.minimax-m2", Source: "foundation", Status: "INACTIVE", Availability: "AVAILABLE"},
	}
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := testutil.NestedMapChain(plan.Keys, "provider", bedrockProviderID, "models")
	if !ok {
		t.Fatal("provider model inventory missing")
	}
	models := value.(map[string]any)
	for _, id := range []string{"openai.gpt-oss-120b-1:0", "moonshotai.kimi-k2.5", "zai.glm-5", "qwen.qwen3-coder-480b-a35b-v1:0"} {
		if _, ok := models[id]; !ok {
			t.Errorf("compatible/default model %q was not configured: %v", id, models)
		}
	}
	for _, id := range []string{"minimax.minimax-m2"} {
		if _, ok := models[id]; ok {
			t.Errorf("incompatible model %q was configured: %v", id, models)
		}
	}
	// whitelist must be populated for native
	if wl, ok := testutil.NestedMapChain(plan.Keys, "provider", bedrockProviderID, "whitelist"); !ok {
		t.Error("whitelist missing for native provider")
	} else {
		switch v := wl.(type) {
		case []string:
			if len(v) == 0 {
				t.Error("whitelist should not be empty")
			}
		case []any:
			if len(v) == 0 {
				t.Error("whitelist should not be empty")
			}
		default:
			t.Errorf("whitelist has unexpected type %T", wl)
		}
	}
}

func TestDynamicModelSelectionAndCatalogWarnings(t *testing.T) {
	tests := []struct {
		cli   string
		model string
		id    string
	}{
		{"codex", "sol", "global.openai.gpt-5.6-sol"},
		{"grok", "grok-4.6", "xai.grok-4.6"},
	}
	for _, tt := range tests {
		t.Run(tt.cli, func(t *testing.T) {
			p, _ := Get(tt.cli)
			opts := baseOpts()
			opts.Model = tt.model
			opts.ModelCatalog = []CatalogModel{catalogModel(tt.id, "foundation")}
			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig: %v", err)
			}
			if len(plan.Warnings) != 0 {
				t.Fatalf("available dynamic model warnings = %v", plan.Warnings)
			}

			opts.ModelCatalog = []CatalogModel{catalogModel("unrelated.model", "foundation")}
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
