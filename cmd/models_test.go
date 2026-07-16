package cmd

import (
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
	idx9 := strings.Index(report, "anthropic.claude-opus-4-10")
	idx10 := strings.Index(report, "anthropic.claude-opus-4-9")
	if idx9 == -1 || idx10 == -1 {
		t.Fatalf("expected both candidates listed, got:\n%s", report)
	}
	if idx9 > idx10 {
		t.Errorf("expected alphabetically sorted candidates (4-10 before 4-9 alphabetically), got:\n%s", report)
	}
}
