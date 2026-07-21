package provider

import (
	"strings"
	"testing"
)

func TestToMap_MarshalError(t *testing.T) {
	// Channel values cannot be marshaled to JSON, exercising the marshal error path.
	ch := make(chan int)
	_, err := ToMap(ch)
	if err == nil || !strings.Contains(err.Error(), "serializing block") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestToMap_Success(t *testing.T) {
	type block struct {
		Model  string `json:"model"`
		Region string `json:"region"`
	}
	got, err := ToMap(block{Model: "sonnet", Region: "us-west-2"})
	if err != nil {
		t.Fatalf("ToMap: %v", err)
	}
	if got["model"] != "sonnet" || got["region"] != "us-west-2" {
		t.Fatalf("unexpected map: %+v", got)
	}
}

func TestConfigPlan_Validate_EmptyManagedKey(t *testing.T) {
	plan := ConfigPlan{
		Keys:        map[string]any{"env": map[string]string{}},
		ManagedKeys: []string{""},
	}
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), "empty managed key") {
		t.Fatalf("expected empty key error, got %v", err)
	}
}

func TestConfigPlan_Validate_MissingKey(t *testing.T) {
	plan := ConfigPlan{
		Keys:        map[string]any{"env": map[string]string{}},
		ManagedKeys: []string{"env", "missing"},
	}
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing key error, got %v", err)
	}
}

func TestConfigPlan_Validate_Ok(t *testing.T) {
	plan := ConfigPlan{
		Keys:        map[string]any{"env": nil, "model": nil},
		ManagedKeys: []string{"env", "model"},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
}

// TestCatalogSelectionState_NoRelevantSource verifies that when the cache
// contains only a source the provider doesn't use, it's treated as no cache.
func TestCatalogSelectionState_NoRelevantSource(t *testing.T) {
	models := []CatalogModel{
		{ID: "anthropic.claude-opus-4-8", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	p, _ := Get("grok")
	hasRelevant, selectedAvailable := catalogSelectionState(models, "xai.grok-4.3", p.(CatalogProvider), nil)
	if hasRelevant {
		t.Error("grok should not find relevant source in foundation-only cache")
	}
	if selectedAvailable {
		t.Error("selected model should not be available when no relevant source")
	}
}

// TestCatalogSelectionState_RelevantButModelNotSupported verifies that
// hasRelevantSource is true but selectedAvailable is false when the model
// exists in the right source but is not supported by the provider.
func TestCatalogSelectionState_RelevantButModelNotSupported(t *testing.T) {
	models := []CatalogModel{
		{ID: "moonshotai.kimi-k2.5", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	p, _ := Get("grok")
	hasRelevant, selectedAvailable := catalogSelectionState(models, "xai.grok-4.3", p.(CatalogProvider), nil)
	if !hasRelevant {
		t.Error("grok should find relevant source in mantle cache")
	}
	if selectedAvailable {
		t.Error("selected grok model should not be available when not in catalog")
	}
}

// TestCatalogSelectionState_FullyAvailable verifies the happy path where the
// selected model is found in the relevant source and is supported.
func TestCatalogSelectionState_FullyAvailable(t *testing.T) {
	models := []CatalogModel{
		{ID: "xai.grok-4.3", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	p, _ := Get("grok")
	hasRelevant, selectedAvailable := catalogSelectionState(models, "xai.grok-4.3", p.(CatalogProvider), nil)
	if !hasRelevant {
		t.Error("expected hasRelevantSource=true")
	}
	if !selectedAvailable {
		t.Error("expected selectedAvailable=true")
	}
}

// TestCatalogSelectionState_EmptyCatalogWithRefreshedSource verifies that
// when a source was refreshed but returned zero entries, hasRelevantSource
// is still true (proving the model is unavailable rather than unrefreshed).
func TestCatalogSelectionState_EmptyCatalogWithRefreshedSource(t *testing.T) {
	// Empty model list — the refresh returned nothing
	var models []CatalogModel
	p, _ := Get("grok")
	hasRelevant, selectedAvailable := catalogSelectionState(models, "xai.grok-4.3", p.(CatalogProvider), []string{"mantle"})
	if !hasRelevant {
		t.Error("expected hasRelevantSource=true when mantle source was refreshed but empty")
	}
	if selectedAvailable {
		t.Error("selected model should not be available when catalog is empty")
	}
}
