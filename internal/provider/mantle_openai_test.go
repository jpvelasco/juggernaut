package provider

import (
	"reflect"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestOpenAIMantlePlans(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, mode := range []string{"iam", "bedrock-api-key"} {
			t.Run(cli+"/"+mode, func(t *testing.T) {
				p := MustGet(cli)
				plan, err := p.BuildConfig(nil, Options{Endpoint: "mantle", AuthMode: mode, Region: "us-west-2", Model: "openai.gpt-5.4"})
				if err != nil {
					t.Fatal(err)
				}
				if err := plan.Validate(); err != nil {
					t.Fatal(err)
				}
				if !p.Supports(CapMantleEndpoint) {
					t.Fatal("mantle capability missing")
				}
				if !p.OwnsConfig(plan.Keys) {
					t.Fatal("generated config is not recognized")
				}
				if cli == "codex" {
					if plan.Keys["model"] != "openai.gpt-5.4" || plan.Keys["model_provider"] != "amazon-bedrock" {
						t.Fatalf("wrong routing: %v", plan.Keys)
					}
				} else {
					if plan.Keys["model"] != "amazon-bedrock-mantle/openai.gpt-5.4" {
						t.Fatalf("wrong model: %v", plan.Keys)
					}
					entry := plan.Keys["providers"].(map[string]any)["amazon-bedrock-mantle"].(map[string]any)
					if entry["package"] != "@opencode/ai/providers/amazon-bedrock/mantle/responses" {
						t.Fatalf("wrong package: %v", entry)
					}
					settings := entry["settings"].(map[string]any)
					if settings["baseURL"] != "https://bedrock-mantle.us-west-2.api.aws/openai/v1" {
						t.Fatalf("wrong URL: %v", settings)
					}
					if _, ok := plan.Keys["juggernaut"]; ok {
						t.Fatal("auth must stay in sidecar")
					}
				}
			})
		}
	}
}

func TestOpenAIMantleRejectsProfilesAndUnsupportedModels(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, id := range []string{"global.openai.gpt-6-astra", "us.openai.gpt-5.4", "openai.gpt-oss-120b", "anthropic.claude-sonnet-5", "openai.gpt-unknown"} {
			t.Run(cli+"/"+id, func(t *testing.T) {
				if _, err := MustGet(cli).BuildConfig(nil, Options{Endpoint: "mantle", Region: "us-west-2", Model: id}); err == nil {
					t.Fatal("accepted incompatible mantle model")
				}
			})
		}
		if got := MantleRegionsFor(MustGet(cli), "sol"); !reflect.DeepEqual(got, []string{"us-east-1"}) {
			t.Fatalf("sol mantle regions = %v", got)
		}
	}
}

func TestOpenAIMantleRegionAndDefault(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		p := MustGet(cli)
		plan, err := p.BuildConfig(nil, Options{Endpoint: bedrock.EndpointMantle, Region: "eu-west-1", AuthMode: "iam"})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Warnings) == 0 {
			t.Fatal("missing region switch warning")
		}
		if cli == "codex" && plan.Keys["model"] != "openai.gpt-6-astra" {
			t.Fatal(plan.Keys)
		}
	}
}

func TestMantleValidationRejectsDrift(t *testing.T) {
	for _, cli := range []string{"codex", "opencode"} {
		for _, drift := range []string{"region", "model", "profile", "route", "missing model", "override"} {
			t.Run(cli+"/"+drift, func(t *testing.T) {
				p := MustGet(cli)
				plan, err := p.BuildConfig(nil, Options{Endpoint: "mantle", Region: "us-west-2", Model: "openai.gpt-5.4"})
				if err != nil {
					t.Fatal(err)
				}
				region := "us-west-2"
				if drift == "region" {
					region = "eu-west-1"
				}
				if cli == "codex" {
					entry := plan.Keys["model_providers"].(map[string]any)["amazon-bedrock"].(map[string]any)
					switch drift {
					case "model":
						plan.Keys["model"] = "gpt-5.4"
					case "profile":
						plan.Keys["model"] = "global.openai.gpt-5.4"
					case "route":
						plan.Keys["model_provider"] = "amazon-bedrock-runtime"
					case "missing model":
						delete(plan.Keys, "model")
					case "override":
						entry["base_url"] = "https://example.invalid"
					}
				} else {
					entry := plan.Keys["providers"].(map[string]any)["amazon-bedrock-mantle"].(map[string]any)
					switch drift {
					case "model":
						plan.Keys["model"] = "amazon-bedrock-mantle/gpt-5.4"
					case "profile":
						plan.Keys["model"] = "amazon-bedrock-mantle/global.openai.gpt-5.4"
					case "route":
						plan.Keys["model"] = "amazon-bedrock/openai.gpt-5.4"
					case "missing model":
						entry["models"] = map[string]any{}
					case "override":
						entry["settings"].(map[string]any)["baseURL"] = "https://example.invalid"
					}
				}
				if err := p.(MantleConfigChecker).ValidateMantleConfig(plan.Keys, region); err == nil {
					t.Fatal("doctor accepted drifted mantle config")
				}
			})
		}
	}
}

func TestOpenCodeMantleCatalogFiltersRegionAndProtocol(t *testing.T) {
	p := MustGet("opencode")
	plan, err := p.BuildConfig(nil, Options{Endpoint: "mantle", Region: "us-west-2", Model: "gpt-5.4", ModelCatalog: []CatalogModel{
		{ID: "openai.gpt-5.6-terra", Source: "foundation", Status: "ACTIVE"},
		{ID: "openai.gpt-5.5", Source: "foundation", Status: "ACTIVE"},
		{ID: "openai.gpt-6.1-sol", Source: "foundation", Status: "ACTIVE"},
		{ID: "openai.gpt-5.6-luna", Source: "foundation", Status: "ACTIVE", Availability: "NOT_AVAILABLE"},
		{ID: "global.openai.gpt-6-astra", Source: "profile", Status: "ACTIVE"},
		{ID: "openai.gpt-6-astra", Source: "foundation", Status: "LEGACY"},
		{ID: "openai.gpt-oss-120b-1:0", Source: "foundation", Status: "ACTIVE"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entry := plan.Keys["providers"].(map[string]any)["amazon-bedrock-mantle"].(map[string]any)
	models := entry["models"].(map[string]any)
	for id, raw := range models {
		disabled := raw.(map[string]any)["disabled"]
		wantDisabled := id != "openai.gpt-5.4" && id != "openai.gpt-5.6-terra"
		if disabled != wantDisabled {
			t.Fatalf("picker model %s disabled=%v, want %v", id, disabled, wantDisabled)
		}
	}
}

func TestOpenCodeEndpointCleanupPreservesForeignProviders(t *testing.T) {
	p := MustGet("opencode").(EndpointCleaner)
	for _, endpoint := range []string{"runtime", "mantle"} {
		inactive, id := "providers", "amazon-bedrock-mantle"
		if endpoint == "mantle" {
			inactive, id = "provider", "amazon-bedrock"
		}
		for _, foreign := range []bool{false, true} {
			data := map[string]any{inactive: map[string]any{id: map[string]any{"settings": map[string]any{"region": "us-west-2"}}}}
			if foreign {
				data[inactive].(map[string]any)["user-provider"] = "keep"
			}
			p.CleanEndpoint(data, endpoint)
			if foreign {
				table := data[inactive].(map[string]any)
				if len(table) != 1 || table["user-provider"] != "keep" {
					t.Fatal("cleanup touched foreign provider")
				}
			} else if _, ok := data[inactive]; ok {
				t.Fatal("empty parent survived cleanup")
			}
			p.CleanEndpoint(data, endpoint) // Repeat cleanup must be idempotent.
		}
	}
}

func TestMantleScopeAuthDoesNotInheritOtherScope(t *testing.T) {
	home := testutil.NewTestHome(t)
	t.Chdir(t.TempDir())
	p := MustGet("opencode")
	if err := WriteSidecar(p, home, Options{Scope: "project", Endpoint: "mantle", AuthMode: "iam"}); err != nil {
		t.Fatal(err)
	}
	if _, found := ReadScopeAuthBlock(p, home, "user", nil); found {
		t.Fatal("user inherited project mantle metadata")
	}
	block, found := ReadScopeAuthBlock(p, home, "project", nil)
	if !found || BlockEndpoint(block) != "mantle" {
		t.Fatal("project did not use sidecar")
	}
	legacy, err := juggernautAuthBlock(Options{Endpoint: "runtime", AuthMode: "iam"}, "us-west-2")
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"juggernaut": legacy}
	block, found = ReadScopeAuthBlock(p, home, "user", data)
	if !found || BlockEndpoint(block) != "" {
		t.Fatal("legacy fallback lost runtime endpoint")
	}
	block, found = ReadScopeAuthBlock(p, home, "project", data)
	if !found || BlockEndpoint(block) != "mantle" {
		t.Fatal("legacy block overrode authoritative sidecar")
	}
}

func TestCodexMantleLegacyCleanupKeepsNativeRegion(t *testing.T) {
	p := MustGet("codex")
	plan, err := p.BuildConfig(nil, Options{Endpoint: "mantle", Model: "openai.gpt-5.4", Region: "us-west-2", AuthMode: "iam"})
	if err != nil {
		t.Fatal(err)
	}
	entry := plan.Keys["model_providers"].(map[string]any)["amazon-bedrock"].(map[string]any)
	entry["base_url"], entry["env_key"], entry["wire_api"], entry["custom"] = "https://example.invalid", "OLD_KEY", "chat", "keep"
	StripLegacyConfig(p, plan.Keys, "mantle")
	if err := p.(MantleConfigChecker).ValidateMantleConfig(plan.Keys, "us-west-2"); err != nil {
		t.Fatal(err)
	}
	if entry["custom"] != "keep" {
		t.Fatal("unknown setting removed")
	}
	if _, exists := entry["wire_api"]; exists {
		t.Fatal("legacy wire API survived migration")
	}
	if got := p.(MantleRegionSource).MantleRegions("global.openai.gpt-5.4"); got != nil {
		t.Fatalf("CRIS model had mantle regions: %v", got)
	}
}
