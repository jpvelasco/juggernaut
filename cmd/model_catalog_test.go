package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
	"github.com/spf13/cobra"
)

func TestParseCatalogSources(t *testing.T) {
	tests := []struct {
		input string
		want  []discovery.Source
	}{
		{"all", []discovery.Source{discovery.SourceFoundation, discovery.SourceProfile, discovery.SourceMantle}},
		{" NATIVE ", []discovery.Source{discovery.SourceFoundation, discovery.SourceProfile}},
		{"Mantle", []discovery.Source{discovery.SourceMantle}},
	}
	for _, tt := range tests {
		got, err := parseCatalogSources(tt.input)
		if err != nil {
			t.Errorf("parseCatalogSources(%q): %v", tt.input, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseCatalogSources(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseCatalogSources(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
	if _, err := parseCatalogSources("curated"); err == nil {
		t.Fatal("expected invalid-source error")
	}
}

func TestRefreshCatalog_QueriesOnlyRequestedSources(t *testing.T) {
	origFoundation, origProfiles, origMantle := listFoundationCatalog, listInferenceProfiles, listMantleCatalog
	t.Cleanup(func() {
		listFoundationCatalog, listInferenceProfiles, listMantleCatalog = origFoundation, origProfiles, origMantle
	})
	calls := map[string]int{}
	listFoundationCatalog = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		calls["foundation"]++
		return []discovery.DiscoveredModel{{ID: "native", Source: discovery.SourceFoundation}}, nil
	}
	listInferenceProfiles = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		calls["profile"]++
		return []discovery.DiscoveredModel{{ID: "profile", Source: discovery.SourceProfile}}, nil
	}
	listMantleCatalog = func(_ context.Context, region, token string) ([]discovery.DiscoveredModel, error) {
		calls["mantle"]++
		if region != "us-west-2" || token != "token" {
			t.Fatalf("Mantle args = region %q token %q", region, token)
		}
		return []discovery.DiscoveredModel{{ID: "mantle", Source: discovery.SourceMantle}}, nil
	}

	models, err := refreshCatalog(context.Background(), "us-west-2", []discovery.Source{discovery.SourceMantle}, "token")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "mantle" {
		t.Fatalf("models = %+v", models)
	}
	if calls["mantle"] != 1 || calls["foundation"] != 0 || calls["profile"] != 0 {
		t.Fatalf("source calls = %v", calls)
	}
}

func TestRefreshCatalog_IdentifiesFailingSource(t *testing.T) {
	orig := listFoundationCatalog
	t.Cleanup(func() { listFoundationCatalog = orig })
	listFoundationCatalog = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return nil, errors.New("denied")
	}
	_, err := refreshCatalog(context.Background(), "us-west-2", []discovery.Source{discovery.SourceFoundation}, "")
	if err == nil || !strings.Contains(err.Error(), "querying Bedrock foundation models") {
		t.Fatalf("error = %v", err)
	}
}

func TestModelsList_FiltersLiveCatalogByCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	models := []discovery.DiscoveredModel{
		{ID: "moonshotai.kimi-k2.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "zai.glm-5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "qwen.qwen3-coder-next", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "openai.gpt-5.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	credentialScope, err := discovery.CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := discovery.SaveCachedModels(home, "111122223333", credentialScope, "us-west-2", []discovery.Source{discovery.SourceMantle}, models, when); err != nil {
		t.Fatal(err)
	}

	origFlags := modelsListFlags
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "mantle"
	modelsListFlags.cli = "opencode"
	modelsListFlags.refresh = false
	modelsListFlags.showUnsupported = false
	t.Cleanup(func() { modelsListFlags = origFlags })

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsList(command, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, id := range []string{"moonshotai.kimi-k2.5", "zai.glm-5", "qwen.qwen3-coder-next"} {
		if !strings.Contains(got, id) {
			t.Errorf("list output omitted compatible model %q:\n%s", id, got)
		}
	}
	if strings.Contains(got, "openai.gpt-5.5") {
		t.Errorf("OpenCode list included Responses-only model:\n%s", got)
	}
	if !strings.Contains(got, "3 models for AWS account 111122223333; refreshed 2026-07-20T12:00:00Z") {
		t.Errorf("missing summary:\n%s", got)
	}
}

func TestModelsList_MissingCacheIsActionable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origFlags := modelsListFlags
	modelsListFlags = struct {
		region          string
		source          string
		cli             string
		refresh         bool
		showUnsupported bool
	}{region: "us-east-2", source: "all"}
	t.Cleanup(func() { modelsListFlags = origFlags })
	err := runModelsList(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "models refresh --region us-east-2") {
		t.Fatalf("error = %v", err)
	}
}

func TestResetFlags_ResetsNestedModelCommands(t *testing.T) {
	if err := modelsRefreshCmd.Flags().Set("source", "mantle"); err != nil {
		t.Fatal(err)
	}
	if err := modelsListCmd.Flags().Set("show-unsupported", "true"); err != nil {
		t.Fatal(err)
	}
	resetFlags()
	if modelsRefreshFlags.source != "all" || modelsRefreshCmd.Flags().Lookup("source").Changed {
		t.Errorf("refresh source was not reset: value=%q changed=%v", modelsRefreshFlags.source, modelsRefreshCmd.Flags().Lookup("source").Changed)
	}
	if modelsListFlags.showUnsupported || modelsListCmd.Flags().Lookup("show-unsupported").Changed {
		t.Errorf("list flag was not reset: value=%v changed=%v", modelsListFlags.showUnsupported, modelsListCmd.Flags().Lookup("show-unsupported").Changed)
	}
}
