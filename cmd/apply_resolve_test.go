package cmd

import (
	"slices"
	"testing"
)

func TestResolveApplyModels_Default(t *testing.T) {
	defer resetFlags()
	resetFlags()

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	if models.opusModel != "" {
		t.Errorf("expected empty opus model, got %q", models.opusModel)
	}
	if models.sonnetModel != "" {
		t.Errorf("expected empty sonnet model, got %q", models.sonnetModel)
	}
	if models.haikuModel != "" {
		t.Errorf("expected empty haiku model, got %q", models.haikuModel)
	}
	if models.fableModel != "" {
		t.Errorf("expected empty fable model, got %q", models.fableModel)
	}
	if models.fallbackModels != nil {
		t.Errorf("expected nil fallback models, got %v", models.fallbackModels)
	}
	if models.availableModels != nil {
		t.Errorf("expected nil available models, got %v", models.availableModels)
	}
	if models.enforceAvailableModels {
		t.Error("expected enforceAvailableModels=false")
	}
}

func TestResolveApplyModels_ModelOverride(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.model = "global.anthropic.claude-opus-4-8"

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	want := "global.anthropic.claude-opus-4-8"
	if models.opusModel != want {
		t.Errorf("expected opusModel=%q, got %q", want, models.opusModel)
	}
	if models.sonnetModel != want {
		t.Errorf("expected sonnetModel=%q, got %q", want, models.sonnetModel)
	}
	if models.haikuModel != want {
		t.Errorf("expected haikuModel=%q, got %q", want, models.haikuModel)
	}
	if models.fableModel != want {
		t.Errorf("expected fableModel=%q, got %q", want, models.fableModel)
	}
}

func TestResolveApplyModels_FallbackModel(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.fallbackModel = " global.anthropic.claude-opus-4-8 ,global.anthropic.claude-sonnet-4-6 "

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	want := []string{"global.anthropic.claude-opus-4-8", "global.anthropic.claude-sonnet-4-6"}
	if !slices.Equal(models.fallbackModels, want) {
		t.Errorf("expected fallbackModels=%v, got %v", want, models.fallbackModels)
	}
}

func TestResolveApplyModels_AvailableModels(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.availableModels = " sonnet ,claude-opus-4-8,haiku "

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	want := []string{"sonnet", "claude-opus-4-8", "haiku"}
	if !slices.Equal(models.availableModels, want) {
		t.Errorf("expected availableModels=%v, got %v", want, models.availableModels)
	}
}

func TestResolveApplyModels_EnforceWithoutAvailable(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.enforceAvailableModels = true

	_, err := resolveApplyModels()
	if err == nil {
		t.Fatal("expected error when --enforce-available-models without --available-models")
	}
}

func TestResolveApplyModels_FallbackEmpty(t *testing.T) {
	defer resetFlags()
	resetFlags()

	// Empty fallback-model → nil (not an error)
	applyFlags.fallbackModel = ""

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	if models.fallbackModels != nil {
		t.Errorf("expected nil fallback models for empty flag, got %v", models.fallbackModels)
	}
}

func TestResolveApplyModels_PerTierOverride(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.opusModel = "us.anthropic.claude-opus-4-8"

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	if models.opusModel != "us.anthropic.claude-opus-4-8" {
		t.Errorf("expected opus model override, got %q", models.opusModel)
	}
	if models.sonnetModel != "" {
		t.Errorf("expected empty sonnet model, got %q", models.sonnetModel)
	}
}

func TestResolveApplyModels_ModelOverride_ShadowsPerTier(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.model = "global.anthropic.claude-opus-5"
	applyFlags.opusModel = "should-be-ignored"

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	// --model should override --opus-model
	if models.opusModel != "global.anthropic.claude-opus-5" {
		t.Errorf("expected --model to override opus, got %q", models.opusModel)
	}
}

func TestResolveApplyModels_EnforceWithAvailable(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.availableModels = "sonnet, haiku"
	applyFlags.enforceAvailableModels = true

	models, err := resolveApplyModels()
	if err != nil {
		t.Fatalf("resolveApplyModels() error: %v", err)
	}
	if !models.enforceAvailableModels {
		t.Error("expected enforceAvailableModels=true")
	}
	want := []string{"sonnet", "haiku"}
	if !slices.Equal(models.availableModels, want) {
		t.Errorf("expected availableModels=%v, got %v", want, models.availableModels)
	}
}

func TestResolveApplyModels_FallbackEmptyEntry(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.fallbackModel = "global.anthropic.claude-opus-4-8, ,global.anthropic.claude-sonnet-4-6"

	_, err := resolveApplyModels()
	if err == nil {
		t.Fatal("expected error for empty fallback model entry")
	}
}

func TestResolveApplyModels_AvailableEmptyEntry(t *testing.T) {
	defer resetFlags()
	resetFlags()

	applyFlags.availableModels = "sonnet, ,haiku"

	_, err := resolveApplyModels()
	if err == nil {
		t.Fatal("expected error for empty available model entry")
	}
}
