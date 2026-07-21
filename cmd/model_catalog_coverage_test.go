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
	"github.com/spf13/cobra"
)

func TestRunModelsRefresh_NativeSources(t *testing.T) {
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	origScope := catalogCredentialScope
	t.Cleanup(func() { catalogCredentialScope = origScope })
	catalogCredentialScope = func(string) (string, error) { return "scope", nil }

	// No cache saved — LoadCachedModels returns found=false, err=nil.
	_, err := cachedProviderModels(home, "us-west-2")
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
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	home := t.TempDir()
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
	got, err := cachedProviderModels(home, "us-west-2")
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

func TestModelsList_WithGroKProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	models := []discovery.DiscoveredModel{
		{ID: "xai.grok-4.3", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "moonshotai.kimi-k2.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceMantle}, models, when); err != nil {
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
	got := output.String()
	if !strings.Contains(got, "xai.grok-4.3") {
		t.Fatalf("grok list missing supported model:\n%s", got)
	}
	if strings.Contains(got, "moonshotai.kimi-k2.5") {
		t.Fatalf("grok list should not include kimi without show-unsupported:\n%s", got)
	}
}

func TestModelsList_WithCodexProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	models := []discovery.DiscoveredModel{
		{ID: "openai.gpt-5.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "xai.grok-4.3", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceMantle}, models, when); err != nil {
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
	modelsListFlags.cli = "codex"
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
	if !strings.Contains(got, "openai.gpt-5.5") {
		t.Fatalf("codex list missing supported model:\n%s", got)
	}
	if strings.Contains(got, "xai.grok-4.3") {
		t.Fatalf("codex list should not include grok without show-unsupported:\n%s", got)
	}
}

func TestModelsList_WithClaudeProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	when := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	models := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-opus-4-8", Source: discovery.SourceFoundation, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "global.anthropic.claude-sonnet-4-6", Source: discovery.SourceProfile, Status: "ACTIVE", Availability: "AVAILABLE"},
		{ID: "openai.gpt-5.5", Source: discovery.SourceMantle, Status: "ACTIVE", Availability: "AVAILABLE"},
	}
	if err := discovery.SaveCachedModels(home, "111122223333", "scope", "us-west-2",
		[]discovery.Source{discovery.SourceFoundation, discovery.SourceProfile, discovery.SourceMantle},
		models, when); err != nil {
		t.Fatal(err)
	}

	origFlags := modelsListFlags
	origScope := catalogCredentialScope
	t.Cleanup(func() {
		modelsListFlags = origFlags
		catalogCredentialScope = origScope
	})
	modelsListFlags.region = "us-west-2"
	modelsListFlags.source = "native"
	modelsListFlags.cli = "claude"
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
	for _, id := range []string{"anthropic.claude-opus-4-8", "global.anthropic.claude-sonnet-4-6"} {
		if !strings.Contains(got, id) {
			t.Errorf("claude list missing model %q:\n%s", id, got)
		}
	}
	if strings.Contains(got, "openai.gpt-5.5") {
		t.Fatalf("claude native list should not include Mantle model:\n%s", got)
	}
}

func TestModelsList_SummaryLineFormat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
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
		cli   string
		id    string
		source string
		want  bool
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