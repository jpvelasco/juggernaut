package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
	"github.com/spf13/cobra"
)

func TestRunModelsRefresh_NativeSources(t *testing.T) {
	home := testutil.NewTestHome(t)
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
	modelsRefreshFlags.source = "native"
	catalogCallerAccount = func(context.Context, string) (string, error) { return "111122223333", nil }
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	catalogNow = func() time.Time { return when }
	listFoundationCatalog = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "anthropic.claude-opus-4-8", Source: discovery.SourceFoundation, Status: "ACTIVE"}}, nil
	}
	listInferenceProfiles = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "global.anthropic.claude-sonnet-4-6", Source: discovery.SourceProfile, Status: "ACTIVE"}}, nil
	}
	listMantleCatalog = func(context.Context, string, string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsRefresh(command, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "Cached 2 models for AWS account 111122223333 in us-east-1") {
		t.Fatalf("refresh output wrong:\n%s", got)
	}
	if !strings.Contains(got, "model-catalog.json") {
		t.Fatalf("cache path not in output:\n%s", got)
	}
	// Verify the cache was saved correctly.
	snapshot, found, err := discovery.LoadCachedModels(home, "scope", "us-east-1")
	if err != nil || !found || len(snapshot.Models) != 2 {
		t.Fatalf("snapshot = %+v, found %v, err %v", snapshot, found, err)
	}
}

func TestRunModelsList_CliFilterWithProviderError(t *testing.T) {
	home := testutil.NewTestHome(t)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceMantle},
		[]discovery.DiscoveredModel{{ID: "xai.grok-4.3", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"}},
		when); err != nil {
		t.Fatal(err)
	}

	origFlags := modelsListFlags
	origScope := catalogCredentialScope
	t.Cleanup(func() {
		modelsListFlags = origFlags
		catalogCredentialScope = origScope
	})
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "mantle"
	modelsListFlags.cli = "grok"
	modelsListFlags.refresh = false
	modelsListFlags.showUnsupported = false
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsList(command, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "xai.grok-4.3") {
		t.Fatalf("grok list missing model:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "yes") {
		t.Fatalf("grok list missing support label:\n%s", output.String())
	}
}

func TestRunModelsList_NoModelsMatchSourceFilter(t *testing.T) {
	home := testutil.NewTestHome(t)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceFoundation},
		[]discovery.DiscoveredModel{{ID: "anthropic.claude-opus", Source: discovery.SourceFoundation}},
		when); err != nil {
		t.Fatal(err)
	}

	origFlags := modelsListFlags
	origScope := catalogCredentialScope
	t.Cleanup(func() {
		modelsListFlags = origFlags
		catalogCredentialScope = origScope
	})
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "mantle"
	modelsListFlags.cli = ""
	modelsListFlags.refresh = false
	modelsListFlags.showUnsupported = false
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsList(command, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "anthropic.claude-opus") {
		t.Fatalf("mantle filter should not show foundation model:\n%s", got)
	}
	if !strings.Contains(got, "0 models for AWS account 111122223333") {
		t.Fatalf("expected zero models summary:\n%s", got)
	}
}

func TestCachedProviderModels_LoadError(t *testing.T) {
	home := testutil.NewTestHome(t)
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	// No cache saved — LoadCachedModels returns found=false, err=nil.
	_, _, err := cachedProviderCatalog(home, "us-west-2")
	if err != nil {
		t.Fatalf("expected nil error for missing cache, got %v", err)
	}
}

func TestToProviderCatalogModel_PreservesFields(t *testing.T) {
	model := discovery.DiscoveredModel{
		ID: "anthropic.claude-opus-4-8", Status: "ACTIVE",
		Availability: "AVAILABLE", Provider: "Anthropic", Source: discovery.SourceFoundation,
	}
	cm := toProviderCatalogModel(model)
	if cm.ID != model.ID || cm.Status != model.Status ||
		cm.Availability != model.Availability || cm.Provider != model.Provider ||
		cm.Source != string(model.Source) {
		t.Fatalf("field mismatch: %+v vs %+v", cm, model)
	}
}

func TestRefreshCatalog_AllSourcesCombined(t *testing.T) {
	origFoundation, origProfiles, origMantle := listFoundationCatalog, listInferenceProfiles, listMantleCatalog
	t.Cleanup(func() {
		listFoundationCatalog, listInferenceProfiles, listMantleCatalog = origFoundation, origProfiles, origMantle
	})
	listFoundationCatalog = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "native", Source: discovery.SourceFoundation}}, nil
	}
	listInferenceProfiles = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "profile", Source: discovery.SourceProfile}}, nil
	}
	listMantleCatalog = func(context.Context, string, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "mantle", Source: discovery.SourceMantle}}, nil
	}

	models, err := refreshCatalog(context.Background(), "us-west-2",
		[]discovery.Source{discovery.SourceFoundation, discovery.SourceProfile, discovery.SourceMantle}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models from all sources, got %d", len(models))
	}
}

func TestRefreshCatalog_EmptySourceList(t *testing.T) {
	models, err := refreshCatalog(context.Background(), "us-west-2", []discovery.Source{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("expected 0 models for empty source list, got %d", len(models))
	}
}

func TestCatalogIdentity_PropagatesAccountError(t *testing.T) {
	origAccount, origScope := catalogCallerAccount, catalogCredentialScope
	t.Cleanup(func() {
		catalogCallerAccount, catalogCredentialScope = origAccount, origScope
	})
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	catalogCallerAccount = func(context.Context, string) (string, error) {
		return "", errors.New("sts denied")
	}
	_, _, err := catalogIdentity(context.Background(), "/tmp", "us-west-2")
	if err == nil || !strings.Contains(err.Error(), "sts denied") {
		t.Fatalf("expected sts error, got %v", err)
	}
}

func TestParseCatalogSources_CaseInsensitive(t *testing.T) {
	for _, input := range []string{"ALL", "All", "native", "NATIVE", "Mantle", "MANTLE"} {
		_, err := parseCatalogSources(input)
		if err != nil {
			t.Errorf("parseCatalogSources(%q) should succeed: %v", input, err)
		}
	}
}

func TestModelsList_RefreshAccountError(t *testing.T) {
	_ = testutil.NewTestHome(t)

	origFlags := modelsListFlags
	origAccount, origScope := catalogCallerAccount, catalogCredentialScope
	t.Cleanup(func() {
		modelsListFlags = origFlags
		catalogCallerAccount, catalogCredentialScope = origAccount, origScope
	})
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "mantle"
	modelsListFlags.refresh = true
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	catalogCallerAccount = func(context.Context, string) (string, error) {
		return "", errors.New("sts unavailable")
	}

	if err := runModelsList(&cobra.Command{}, nil); err == nil || !strings.Contains(err.Error(), "sts unavailable") {
		t.Fatalf("expected sts error, got %v", err)
	}
}

func TestCatalogProviderModels_MapsAllFields(t *testing.T) {
	home := testutil.NewTestHome(t)
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	models := []discovery.DiscoveredModel{
		{ID: "native", Source: discovery.SourceFoundation, Status: "ACTIVE", Availability: "AVAILABLE", Provider: "Anthropic"},
		{ID: "profile", Source: discovery.SourceProfile, Status: "ACTIVE", Availability: "AVAILABLE", Provider: ""},
		{ID: "mantle", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE", Provider: "zai"},
	}
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceFoundation, discovery.SourceProfile, discovery.SourceMantle},
		models, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _, err := cachedProviderCatalog(home, "us-west-2")
	if err != nil || len(got) != 3 {
		t.Fatalf("models = %+v, err %v", got, err)
	}
	// Models are sorted by source then ID, so check by ID set rather than position.
	gotIDs := make(map[string]bool, len(got))
	for _, m := range got {
		gotIDs[m.ID] = true
	}
	for _, want := range []string{"native", "profile", "mantle"} {
		if !gotIDs[want] {
			t.Errorf("missing model %q in cached models: %+v", want, got)
		}
	}
}

// TestModelsList_FiltersByProvider covers models list for grok, codex, and
// claude: each provider's compatible models must appear in its list output,
// while models exclusive to other protocols stay hidden without
// --show-unsupported.
func TestModelsList_FiltersByProvider(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		cli           string
		cachedSources []discovery.Source
		models        []discovery.DiscoveredModel
		wantPresent   []string
		wantAbsent    []string
	}{
		{
			name:          "grok",
			source:        "mantle",
			cli:           "grok",
			cachedSources: []discovery.Source{discovery.SourceMantle},
			models: []discovery.DiscoveredModel{
				{ID: "xai.grok-4.3", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
				{ID: "moonshotai.kimi-k2.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
			},
			wantPresent: []string{"xai.grok-4.3"},
			wantAbsent:  []string{"moonshotai.kimi-k2.5"},
		},
		{
			name:          "codex",
			source:        "mantle",
			cli:           "codex",
			cachedSources: []discovery.Source{discovery.SourceMantle},
			models: []discovery.DiscoveredModel{
				{ID: "openai.gpt-5.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
				{ID: "xai.grok-4.3", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
			},
			wantPresent: []string{"openai.gpt-5.5"},
			wantAbsent:  []string{"xai.grok-4.3"},
		},
		{
			name:          "claude",
			source:        "native",
			cli:           "claude",
			cachedSources: []discovery.Source{discovery.SourceFoundation, discovery.SourceProfile, discovery.SourceMantle},
			models: []discovery.DiscoveredModel{
				{ID: "anthropic.claude-opus-4-8", Source: discovery.SourceFoundation, Status: "ACTIVE", Availability: "AVAILABLE"},
				{ID: "global.anthropic.claude-sonnet-4-6", Source: discovery.SourceProfile, Status: "ACTIVE", Availability: "AVAILABLE"},
				{ID: "openai.gpt-5.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
			},
			wantPresent: []string{"anthropic.claude-opus-4-8", "global.anthropic.claude-sonnet-4-6"},
			wantAbsent:  []string{"openai.gpt-5.5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := testutil.NewTestHome(t)
			when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
			if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
				tt.cachedSources, tt.models, when); err != nil {
				t.Fatal(err)
			}

			origFlags := modelsListFlags
			origScope := catalogCredentialScope
			t.Cleanup(func() {
				modelsListFlags = origFlags
				catalogCredentialScope = origScope
			})
			modelsListFlags.region = "us-west-2"
			modelsListFlags.source = tt.source
			modelsListFlags.cli = tt.cli
			modelsListFlags.refresh = false
			modelsListFlags.showUnsupported = false
			catalogCredentialScope = func(string) (string, error) { return "scope", nil }

			var output bytes.Buffer
			command := &cobra.Command{}
			command.SetOut(&output)
			if err := runModelsList(command, nil); err != nil {
				t.Fatal(err)
			}
			got := output.String()
			for _, id := range tt.wantPresent {
				if !strings.Contains(got, id) {
					t.Errorf("%s list missing model %q:\n%s", tt.cli, id, got)
				}
			}
			for _, id := range tt.wantAbsent {
				if strings.Contains(got, id) {
					t.Errorf("%s list should not include %q without show-unsupported:\n%s", tt.cli, id, got)
				}
			}
		})
	}
}

func TestModelsList_SummaryLineFormat(t *testing.T) {
	home := testutil.NewTestHome(t)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if err := discovery.SaveCachedModels(home, "999988887777", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceMantle},
		[]discovery.DiscoveredModel{{ID: "xai.grok-4.3", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"}},
		when); err != nil {
		t.Fatal(err)
	}

	origFlags := modelsListFlags
	origScope := catalogCredentialScope
	t.Cleanup(func() {
		modelsListFlags = origFlags
		catalogCredentialScope = origScope
	})
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "mantle"
	modelsListFlags.cli = ""
	modelsListFlags.refresh = false
	modelsListFlags.showUnsupported = false
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsList(command, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "1 models for AWS account 999988887777") {
		t.Fatalf("summary line missing or wrong:\n%s", got)
	}
	if !strings.Contains(got, "refreshed 2026-07-20T12:00:00Z") {
		t.Fatalf("refreshed timestamp missing:\n%s", got)
	}
}

func TestModelsRefresh_OutputFormat(t *testing.T) {
	_ = testutil.NewTestHome(t)
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
	modelsRefreshFlags.source = "native"
	catalogCallerAccount = func(context.Context, string) (string, error) { return "123456789012", nil }
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }
	catalogNow = func() time.Time { return when }
	listFoundationCatalog = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "native", Source: discovery.SourceFoundation}}, nil
	}
	listInferenceProfiles = func(context.Context, string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{{ID: "profile", Source: discovery.SourceProfile}}, nil
	}
	listMantleCatalog = func(context.Context, string, string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}

	var output bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&output)
	if err := runModelsRefresh(command, nil); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "Cached 2 models for AWS account 123456789012 in us-east-1") {
		t.Fatalf("refresh output wrong:\n%s", got)
	}
	// Verify the cache path is printed
	if !strings.Contains(got, "model-catalog.json") {
		t.Fatalf("cache path not in output:\n%s", got)
	}
}

func TestProviderSupportsCatalogModel_UsesProviderCheck(t *testing.T) {
	tests := []struct {
		cli    string
		id     string
		source string
		want   bool
	}{
		{"claude", "anthropic.claude-opus-4-8", "foundation", true},
		{"claude", "openai.gpt-5.5", "mantle", false},
		{"codex", "openai.gpt-5.5", "mantle", true},
		{"codex", "anthropic.claude-opus-4-8", "foundation", false},
		{"grok", "xai.grok-4.3", "mantle", true},
		{"grok", "moonshotai.kimi-k2.5", "mantle", false},
		{"opencode", "moonshotai.kimi-k2.5", "mantle", true},
	}
	for _, tt := range tests {
		t.Run(tt.cli+"/"+tt.id, func(t *testing.T) {
			p, _ := provider.Get(tt.cli)
			model := provider.CatalogModel{ID: tt.id, Source: tt.source, Status: "ACTIVE", Availability: "AVAILABLE"}
			support := provider.SupportsCatalogModel(p, model)
			if support.Supported != tt.want {
				t.Errorf("SupportsCatalogModel(%s, %+v) = %+v, want supported=%v", tt.cli, model, support, tt.want)
			}
		})
	}
}

func TestCatalogSourcesFor_ReturnsExpected(t *testing.T) {
	tests := []struct {
		cli  string
		want []string
	}{
		{"claude", []string{"foundation", "profile"}},
		{"codex", []string{"mantle"}},
		{"grok", []string{"mantle"}},
		{"opencode", []string{"mantle"}},
	}
	for _, tt := range tests {
		p, _ := provider.Get(tt.cli)
		got := provider.CatalogSourcesFor(p)
		if len(got) != len(tt.want) {
			t.Errorf("%s sources = %v, want %v", tt.cli, got, tt.want)
		}
	}
}

func TestModelsList_HomeDirError(t *testing.T) {
	// HOME is normally set on all test platforms, but clear it to exercise the error path.
	origHome := os.Getenv("HOME")
	origUser := os.Getenv("USERPROFILE")
	os.Setenv("HOME", "")
	os.Setenv("USERPROFILE", "")
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUser)
	})

	origFlags := modelsListFlags
	t.Cleanup(func() { modelsListFlags = origFlags })
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "mantle"
	modelsListFlags.refresh = false
	modelsListFlags.cli = ""
	modelsListFlags.showUnsupported = false

	err := runModelsList(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "home") {
		t.Fatalf("expected home error, got %v", err)
	}
}

func TestModelsRefresh_HomeDirError(t *testing.T) {
	origHome := os.Getenv("HOME")
	origUser := os.Getenv("USERPROFILE")
	os.Setenv("HOME", "")
	os.Setenv("USERPROFILE", "")
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUser)
	})

	origFlags := modelsRefreshFlags
	t.Cleanup(func() { modelsRefreshFlags = origFlags })
	modelsRefreshFlags.region = "us-west-2"
	modelsRefreshFlags.source = "mantle"

	err := runModelsRefresh(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "home") {
		t.Fatalf("expected home error, got %v", err)
	}
}

func TestCachedProviderSources_ReturnsSources(t *testing.T) {
	home := testutil.NewTestHome(t)
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceMantle},
		[]discovery.DiscoveredModel{{ID: "xai.grok-4.3", Source: discovery.SourceMantle}},
		time.Now()); err != nil {
		t.Fatal(err)
	}
	_, got, err := cachedProviderCatalog(home, "us-west-2")
	if err != nil || len(got) != 1 || got[0] != "mantle" {
		t.Fatalf("sources = %+v, err %v", got, err)
	}
}

func TestCachedProviderSources_NoCache(t *testing.T) {
	home := testutil.NewTestHome(t)
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	// No cache saved — found=false returns nil.
	_, sources, err := cachedProviderCatalog(home, "us-west-2")
	if err != nil {
		t.Fatalf("expected nil error for missing cache, got %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("expected empty sources, got %+v", sources)
	}
}

func TestCachedProviderSources_ScopeError(t *testing.T) {
	home := testutil.NewTestHome(t)
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "", errors.New("scope unavailable") }

	if _, _, err := cachedProviderCatalog(home, "us-west-2"); err == nil || !strings.Contains(err.Error(), "scope unavailable") {
		t.Fatalf("expected scope error, got %v", err)
	}
}
