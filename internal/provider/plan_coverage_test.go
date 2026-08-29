package provider

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/schema"
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

func TestFromSchemaOptions_FillsFields(t *testing.T) {
	src := schema.Options{
		AuthMode:               "iam",
		Region:                 "us-west-2",
		Effort:                 "high",
		Scope:                  "user",
		Version:                "5.4.0",
		OpusModel:              "global.anthropic.claude-opus-4-8",
		SonnetModel:            "global.anthropic.claude-sonnet-5",
		HaikuModel:             "anthropic.claude-haiku-4-5",
		FableModel:             "global.anthropic.claude-fable-5",
		Opusplan:               true,
		FallbackModels:         []string{"sonnet", "haiku"},
		AvailableModels:        []string{"opus"},
		EnforceAvailableModels: true,
		Use1M:                  true,
		AuthValidated:          true,
		PermissionMode:         "auto",
		AlwaysThinking:         true,
		ServiceTier:            "flex",
	}
	var opts Options
	opts.FromSchemaOptions(src)

	// Verify SchemaOpts was copied (struct contains slices so compare fields individually).
	if opts.SchemaOpts.AuthMode != src.AuthMode || opts.SchemaOpts.Region != src.Region {
		t.Errorf("SchemaOpts not fully copied: got %+v, want %+v", opts.SchemaOpts, src)
	}
	if opts.SchemaOpts.OpusModel != src.OpusModel {
		t.Errorf("SchemaOpts.OpusModel = %q, want %q", opts.SchemaOpts.OpusModel, src.OpusModel)
	}
	if opts.SchemaOpts.PermissionMode != src.PermissionMode {
		t.Errorf("SchemaOpts.PermissionMode = %q, want %q", opts.SchemaOpts.PermissionMode, src.PermissionMode)
	}
	if !opts.SchemaOpts.Opusplan || !opts.SchemaOpts.Use1M || !opts.SchemaOpts.AuthValidated {
		t.Errorf("SchemaOpts booleans not set: opusplan=%v use1m=%v authValidated=%v",
			opts.SchemaOpts.Opusplan, opts.SchemaOpts.Use1M, opts.SchemaOpts.AuthValidated)
	}
	if len(opts.SchemaOpts.FallbackModels) != 2 || opts.SchemaOpts.FallbackModels[0] != "sonnet" {
		t.Errorf("SchemaOpts.FallbackModels = %v, want [sonnet haiku]", opts.SchemaOpts.FallbackModels)
	}
	if opts.Region != "us-west-2" {
		t.Errorf("Region = %q, want us-west-2", opts.Region)
	}
	if opts.AuthMode != "iam" {
		t.Errorf("AuthMode = %q, want iam", opts.AuthMode)
	}
	if opts.Scope != "user" {
		t.Errorf("Scope = %q, want user", opts.Scope)
	}
	if opts.Version != "5.4.0" {
		t.Errorf("Version = %q, want 5.4.0", opts.Version)
	}
}

func TestFromSchemaOptions_Empty(t *testing.T) {
	var opts Options
	opts.FromSchemaOptions(schema.Options{})
	if opts.SchemaOpts.AuthMode != "" || opts.Region != "" || opts.Scope != "" || opts.Version != "" {
		t.Errorf("expected zero values, got %+v", opts)
	}
}
