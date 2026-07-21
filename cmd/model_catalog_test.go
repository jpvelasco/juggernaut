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

func TestRefreshCatalog_IdentifiesProfileAndMantleFailures(t *testing.T) {
	origProfiles, origMantle := listInferenceProfiles, listMantleCatalog
	t.Cleanup(func() { listInferenceProfiles, listMantleCatalog = origProfiles, origMantle })
	listInferenceProfiles = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return nil, errors.New("profiles denied")
	}
	if _, err := refreshCatalog(context.Background(), "us-west-2", []discovery.Source{discovery.SourceProfile}, ""); err == nil || !strings.Contains(err.Error(), "inference profiles") {
		t.Fatalf("profile error = %v", err)
	}
	listMantleCatalog = func(context.Context, string, string) ([]discovery.DiscoveredModel, error) {
		return nil, errors.New("mantle denied")
	}
	if _, err := refreshCatalog(context.Background(), "us-west-2", []discovery.Source{discovery.SourceMantle}, ""); err == nil || !strings.Contains(err.Error(), "Mantle models") {
		t.Fatalf("Mantle error = %v", err)
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

	modelsListFlags.showUnsupported = true
	output.Reset()
	if err := runModelsList(command, nil); err != nil {
		t.Fatal(err)
	}
	got = output.String()
	if !strings.Contains(got, "openai.gpt-5.5") || !strings.Contains(got, "no") {
		t.Errorf("show-unsupported output omitted incompatible model:\n%s", got)
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

func TestModelsRefresh_ReportsInvalidSourceAndIdentityFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	origFlags := modelsRefreshFlags
	origScope := catalogCredentialScope
	t.Cleanup(func() {
		modelsRefreshFlags = origFlags
		catalogCredentialScope = origScope
	})
	modelsRefreshFlags.region = "us-west-2"
	modelsRefreshFlags.source = "invalid"
	if err := runModelsRefresh(&cobra.Command{}, nil); err == nil || !strings.Contains(err.Error(), "invalid catalog source") {
		t.Fatalf("invalid source error = %v", err)
	}

	modelsRefreshFlags.source = "mantle"
	catalogCredentialScope = func(string) (string, error) { return "", errors.New("scope failed") }
	if err := runModelsRefresh(&cobra.Command{}, nil); err == nil || !strings.Contains(err.Error(), "scope failed") {
		t.Fatalf("identity error = %v", err)
	}
}

func TestModelsRefresh_CachesAllSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)

	origFlags := modelsRefreshFlags
	origFoundation, origProfiles, origMantle := listFoundationCatalog, listInferenceProfiles, listMantleCatalog
	origAccount, origScope, origNow := catalogCallerAccount, catalogCredentialScope, catalogNow
	t.Cleanup(func() {
		modelsRefreshFlags = origFlags
		listFoundationCatalog, listInferenceProfiles, listMantleCatalog = origFoundation, origProfiles, origMantle
		catalogCallerAccount, catalogCredentialScope, catalogNow = origAccount, origScope, origNow
	})
	modelsRefreshFlags.region = "us-east-1"
	modelsRefreshFlags.source = "all"
	catalogCallerAccount = func(context.Context, string) (string, error) { return "111122223333", nil }
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	catalogNow = func() time.Time { return when }
	listFoundationCatalog = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "foundation", Source: discovery.SourceFoundation}}, nil
	}
	listInferenceProfiles = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "profile", Source: discovery.SourceProfile}}, nil
	}
	listMantleCatalog = func(context.Context, string, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "mantle", Source: discovery.SourceMantle}}, nil
	}

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsRefresh(command, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Cached 3 models for AWS account 111122223333 in us-east-1") {
		t.Fatalf("output = %q", output.String())
	}
	snapshot, found, err := discovery.LoadCachedModels(home, "scope", "us-east-1")
	if err != nil || !found || len(snapshot.Models) != 3 {
		t.Fatalf("snapshot = %+v, found %v, err %v", snapshot, found, err)
	}
}

func TestModelsList_RefreshesAndListsWithoutCLIFilter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := time.Date(2026, 7, 20, 16, 0, 0, 0, time.UTC)

	origFlags := modelsListFlags
	origMantle := listMantleCatalog
	origAccount, origScope, origNow := catalogCallerAccount, catalogCredentialScope, catalogNow
	t.Cleanup(func() {
		modelsListFlags = origFlags
		listMantleCatalog = origMantle
		catalogCallerAccount, catalogCredentialScope, catalogNow = origAccount, origScope, origNow
	})
	modelsListFlags.region = "us-west-1"
	modelsListFlags.source = "mantle"
	modelsListFlags.cli = ""
	modelsListFlags.refresh = true
	modelsListFlags.showUnsupported = false
	catalogCallerAccount = func(context.Context, string) (string, error) { return "444455556666", nil }
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	catalogNow = func() time.Time { return when }
	listMantleCatalog = func(context.Context, string, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{
			ID: "zai.glm-5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE",
		}}, nil
	}

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsList(command, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "SOURCE") || !strings.Contains(got, "zai.glm-5") ||
		!strings.Contains(got, "1 models for AWS account 444455556666") {
		t.Fatalf("output = %q", got)
	}
}

func TestCachedProviderModels_MapsCachedInventory(t *testing.T) {
	home := t.TempDir()
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	model := discovery.DiscoveredModel{
		ID: "zai.glm-5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE", Provider: "zai",
	}
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2", []discovery.Source{discovery.SourceMantle}, []discovery.DiscoveredModel{model}, time.Now()); err != nil {
		t.Fatal(err)
	}
	models, err := cachedProviderModels(home, "us-west-2")
	if err != nil || len(models) != 1 || models[0].ID != model.ID || models[0].Source != string(model.Source) {
		t.Fatalf("models = %+v, err %v", models, err)
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
