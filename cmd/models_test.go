package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
)

func testBedrockConfigForModels() *bedrock.Config {
	return &bedrock.Config{
		Version: "9.9.9",
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-4-8",
			Sonnet: "global.anthropic.claude-sonnet-4-6",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			Fable:  "global.anthropic.claude-fable-5",
		},
	}
}

func TestBareModelID_StripsRegionalPrefixes(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"global.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"us.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"us-gov.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"eu.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"apac.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"", ""},
	}
	for _, c := range cases {
		if got := bareModelID(c.id); got != c.want {
			t.Errorf("bareModelID(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestBuildModelsReport_AllActiveNoLegacy(t *testing.T) {
	cfg := testBedrockConfigForModels()
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-8", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-fable-5", Status: "ACTIVE", Provider: "Anthropic"},
	}
	report, anyLegacy := buildModelsReport(cfg, anthropic, nil)
	if anyLegacy {
		t.Error("expected anyLegacy=false when all tiers are ACTIVE")
	}
	if !strings.Contains(report, "opus") || !strings.Contains(report, "ACTIVE") {
		t.Errorf("expected report to mention opus/ACTIVE, got:\n%s", report)
	}
}

func TestBuildModelsReport_LegacyTierFlagsAndListsCandidates(t *testing.T) {
	cfg := testBedrockConfigForModels()
	// Pinned opus ID doesn't match any entry below → treated as LEGACY/not-found.
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-8", Status: "LEGACY", Provider: "Anthropic"},
		{ID: "anthropic.claude-opus-4-9", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-fable-5", Status: "ACTIVE", Provider: "Anthropic"},
	}
	report, anyLegacy := buildModelsReport(cfg, anthropic, nil)
	if !anyLegacy {
		t.Error("expected anyLegacy=true when opus tier is LEGACY")
	}
	if !strings.Contains(report, "anthropic.claude-opus-4-9") {
		t.Errorf("expected LEGACY opus tier to list claude-opus-4-9 as an ACTIVE candidate, got:\n%s", report)
	}
}

func TestBuildModelsReport_UnrecognizedModelSurfaced(t *testing.T) {
	cfg := testBedrockConfigForModels()
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-8", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-fable-5", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-nova-1", Status: "ACTIVE", Provider: "Anthropic"},
	}
	report, _ := buildModelsReport(cfg, anthropic, nil)
	if !strings.Contains(report, "unrecognized") {
		t.Errorf("expected an unrecognized section, got:\n%s", report)
	}
	if !strings.Contains(report, "anthropic.claude-nova-1") {
		t.Errorf("expected claude-nova-1 listed under unrecognized, got:\n%s", report)
	}
}

func TestBuildModelsReport_DeterministicSortedCandidates(t *testing.T) {
	cfg := testBedrockConfigForModels()
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-8", Status: "LEGACY", Provider: "Anthropic"},
		{ID: "anthropic.claude-opus-4-9", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-opus-4-10", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE", Provider: "Anthropic"},
		{ID: "anthropic.claude-fable-5", Status: "ACTIVE", Provider: "Anthropic"},
	}
	report, _ := buildModelsReport(cfg, anthropic, nil)
	idx410 := strings.Index(report, "anthropic.claude-opus-4-10")
	idx49 := strings.Index(report, "anthropic.claude-opus-4-9")
	if idx410 == -1 || idx49 == -1 {
		t.Fatalf("expected both candidates listed, got:\n%s", report)
	}
	if idx410 > idx49 {
		t.Errorf("expected alphabetically sorted candidates (4-10 before 4-9 alphabetically), got:\n%s", report)
	}
}

func TestRunModelsCheck_WriteWithoutSetIsError(t *testing.T) {
	// Explicitly reset the modelsCheckFlags struct fields to clear any state from prior tests
	modelsCheckFlags.write = false
	modelsCheckFlags.setOpus = ""
	modelsCheckFlags.setSonnet = ""
	modelsCheckFlags.setHaiku = ""
	modelsCheckFlags.setFable = ""

	err := ExecuteArgs([]string{"models", "check", "--write"})
	if err == nil {
		t.Fatal("expected --write without any --set-* to error")
	}
	if !strings.Contains(err.Error(), "--write") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveSetFlags_CollectsOnlyNonEmpty(t *testing.T) {
	resetFlags()
	modelsCheckFlags.setOpus = "anthropic.claude-opus-4-9"
	modelsCheckFlags.setFable = ""
	defer func() {
		modelsCheckFlags.setOpus = ""
	}()

	got := resolveSetFlags()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 entry, got %v", got)
	}
	if got[discovery.TierOpus] != "anthropic.claude-opus-4-9" {
		t.Errorf("unexpected entry: %v", got)
	}
}

func TestApplyModelWrites_RejectsNonActiveID(t *testing.T) {
	cfg := testBedrockConfigForModels()
	sets := map[discovery.Tier]string{discovery.TierOpus: "anthropic.claude-opus-legacy"}
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-legacy", Status: "LEGACY"},
	}
	if err := applyModelWrites(cfg, sets, anthropic); err == nil {
		t.Fatal("expected error when pinning a LEGACY ID")
	}
	if cfg.Models.Opus != "global.anthropic.claude-opus-4-8" {
		t.Errorf("cfg must be unchanged on rejection, got %v", cfg.Models.Opus)
	}
}

func TestApplyModelWrites_RejectsUnknownID(t *testing.T) {
	cfg := testBedrockConfigForModels()
	sets := map[discovery.Tier]string{discovery.TierOpus: "anthropic.claude-opus-nonexistent"}
	if err := applyModelWrites(cfg, sets, nil); err == nil {
		t.Fatal("expected error when pinning an unknown ID")
	}
}

func TestApplyModelWrites_AllOrNothingAcrossMultipleTiers(t *testing.T) {
	cfg := testBedrockConfigForModels()
	sets := map[discovery.Tier]string{
		discovery.TierOpus:   "anthropic.claude-opus-4-9",   // ACTIVE, valid
		discovery.TierSonnet: "anthropic.claude-sonnet-bad", // not in catalog
	}
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-9", Status: "ACTIVE"},
	}
	if err := applyModelWrites(cfg, sets, anthropic); err == nil {
		t.Fatal("expected error because sonnet entry is invalid")
	}
	if cfg.Models.Opus != "global.anthropic.claude-opus-4-8" {
		t.Error("opus must be UNCHANGED because the batch failed on sonnet — all-or-nothing")
	}
}

func TestApplyModelWrites_SucceedsAndMutatesConfig(t *testing.T) {
	cfg := testBedrockConfigForModels()
	sets := map[discovery.Tier]string{discovery.TierOpus: "anthropic.claude-opus-4-9"}
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-9", Status: "ACTIVE"},
	}
	if err := applyModelWrites(cfg, sets, anthropic); err != nil {
		t.Fatalf("applyModelWrites: %v", err)
	}
	if cfg.Models.Opus != "anthropic.claude-opus-4-9" {
		t.Errorf("expected opus updated to anthropic.claude-opus-4-9, got %v", cfg.Models.Opus)
	}
}

func TestRunModelsCheck_EndToEnd_AllActiveExitsClean(t *testing.T) {
	origAnthropic, origProfiles := listAnthropicModels, listInferenceProfiles
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{
			{ID: "anthropic.claude-opus-4-8", Status: "ACTIVE"},
			{ID: "anthropic.claude-sonnet-5", Status: "ACTIVE"},
			{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
			{ID: "anthropic.claude-fable-5", Status: "ACTIVE"},
		}, nil
	}
	listInferenceProfiles = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}
	defer func() {
		listAnthropicModels = origAnthropic
		listInferenceProfiles = origProfiles
	}()

	// Explicitly reset the modelsCheckFlags struct fields that may have been modified by prior tests
	modelsCheckFlags.write = false
	modelsCheckFlags.setOpus = ""
	modelsCheckFlags.setSonnet = ""
	modelsCheckFlags.setHaiku = ""
	modelsCheckFlags.setFable = ""

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"models", "check"}); err != nil {
			t.Fatalf("expected clean exit when all tiers ACTIVE, got: %v", err)
		}
	})
	if !strings.Contains(out, "opus") {
		t.Errorf("expected report output, got:\n%s", out)
	}
}

func TestRunModelsCheck_EndToEnd_LegacyTierErrors(t *testing.T) {
	origAnthropic, origProfiles := listAnthropicModels, listInferenceProfiles
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{
			{ID: "anthropic.claude-opus-4-8", Status: "LEGACY"},
			{ID: "anthropic.claude-sonnet-5", Status: "ACTIVE"},
			{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
			{ID: "anthropic.claude-fable-5", Status: "ACTIVE"},
		}, nil
	}
	listInferenceProfiles = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}
	defer func() {
		listAnthropicModels = origAnthropic
		listInferenceProfiles = origProfiles
	}()

	// Explicitly reset the modelsCheckFlags struct fields that may have been modified by prior tests
	modelsCheckFlags.write = false
	modelsCheckFlags.setOpus = ""
	modelsCheckFlags.setSonnet = ""
	modelsCheckFlags.setHaiku = ""
	modelsCheckFlags.setFable = ""

	if err := ExecuteArgs([]string{"models", "check"}); err == nil {
		t.Fatal("expected non-zero exit (error) when a tier is LEGACY")
	}
}

func TestRunModelsCheck_SetWithoutWrite_PrintsValidationFeedback(t *testing.T) {
	origAnthropic, origProfiles := listAnthropicModels, listInferenceProfiles
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{
			{ID: "anthropic.claude-opus-4-8", Status: "ACTIVE"},
			{ID: "anthropic.claude-opus-4-9", Status: "ACTIVE"},
			{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE"},
			{ID: "anthropic.claude-sonnet-5", Status: "ACTIVE"},
			{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
			{ID: "anthropic.claude-fable-5", Status: "ACTIVE"},
		}, nil
	}
	listInferenceProfiles = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}
	defer func() {
		listAnthropicModels = origAnthropic
		listInferenceProfiles = origProfiles
	}()

	resetFlags()
	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"models", "check", "--set-opus=anthropic.claude-opus-4-9"}); err != nil {
			t.Fatalf("expected clean exit when --set-opus without --write, got: %v", err)
		}
	})
	if !strings.Contains(out, "opus: anthropic.claude-opus-4-9 is ACTIVE (pass --write to persist)") {
		t.Errorf("expected validation feedback message, got:\n%s", out)
	}
	if strings.Contains(out, "bedrock-config.json updated") {
		t.Errorf("should NOT persist without --write, but output contains 'updated':\n%s", out)
	}
}

func TestRunModelsCheck_SetWithoutWrite_RejectsInvalidID(t *testing.T) {
	origAnthropic, origProfiles := listAnthropicModels, listInferenceProfiles
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{
			{ID: "anthropic.claude-opus-4-8", Status: "ACTIVE"},
			{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE"},
			{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
			{ID: "anthropic.claude-fable-5", Status: "ACTIVE"},
		}, nil
	}
	listInferenceProfiles = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}
	defer func() {
		listAnthropicModels = origAnthropic
		listInferenceProfiles = origProfiles
	}()

	resetFlags()
	if err := ExecuteArgs([]string{"models", "check", "--set-opus=anthropic.claude-opus-legacy"}); err == nil {
		t.Fatal("expected error when --set-opus to an invalid ID, even without --write")
	}
}
