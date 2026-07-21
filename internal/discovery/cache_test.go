package discovery

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCatalogCache_MissingAndSourceAwareMerge(t *testing.T) {
	home := t.TempDir()
	if _, found, err := LoadCachedModels(home, "us-west-2"); err != nil || found {
		t.Fatalf("missing cache = found %v, err %v; want false, nil", found, err)
	}

	mantleTime := time.Date(2026, 7, 20, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	if err := SaveCachedModels(home, "us-west-2", []Source{SourceMantle}, []DiscoveredModel{
		{ID: "zai.glm-5", Source: SourceMantle, Status: "ACTIVE"},
		{ID: "moonshotai.kimi-k2.5", Source: SourceMantle, Status: "ACTIVE"},
	}, mantleTime); err != nil {
		t.Fatalf("saving Mantle cache: %v", err)
	}

	nativeTime := mantleTime.Add(time.Hour)
	if err := SaveCachedModels(home, "us-west-2", []Source{SourceFoundation, SourceProfile}, []DiscoveredModel{
		{ID: "anthropic.claude-opus", Source: SourceFoundation, Status: "ACTIVE"},
		{ID: "global.anthropic.claude-opus", Source: SourceProfile, Status: "ACTIVE"},
	}, nativeTime); err != nil {
		t.Fatalf("saving native cache: %v", err)
	}

	snapshot, found, err := LoadCachedModels(home, "us-west-2")
	if err != nil || !found {
		t.Fatalf("loading merged cache = found %v, err %v", found, err)
	}
	if !snapshot.RefreshedAt.Equal(nativeTime.UTC()) {
		t.Errorf("RefreshedAt = %s, want %s", snapshot.RefreshedAt, nativeTime.UTC())
	}
	if len(snapshot.Models) != 4 {
		t.Fatalf("merged models = %+v, want four", snapshot.Models)
	}
	wantIDs := []string{"anthropic.claude-opus", "moonshotai.kimi-k2.5", "zai.glm-5", "global.anthropic.claude-opus"}
	for i, want := range wantIDs {
		if snapshot.Models[i].ID != want {
			t.Errorf("models[%d].ID = %q, want %q (models=%+v)", i, snapshot.Models[i].ID, want, snapshot.Models)
		}
	}

	if err := SaveCachedModels(home, "us-east-1", []Source{SourceMantle}, []DiscoveredModel{
		{ID: "openai.gpt-5.5", Source: SourceMantle},
	}, nativeTime); err != nil {
		t.Fatalf("saving second region: %v", err)
	}
	if _, found, err := LoadCachedModels(home, "us-west-2"); err != nil || !found {
		t.Fatalf("second region overwrote first: found %v, err %v", found, err)
	}

	path, err := CachePath(home)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if got := info.Mode().Perm() & 0o077; got != 0 {
			t.Errorf("cache permissions expose owner data: %o", info.Mode().Perm())
		}
	}
}

func TestCatalogCache_RejectsInvalidInputsAndVersion(t *testing.T) {
	home := t.TempDir()
	if err := SaveCachedModels(home, "", []Source{SourceMantle}, nil, time.Now()); err == nil {
		t.Fatal("expected empty-region error")
	}
	if err := SaveCachedModels(home, "us-west-2", nil, nil, time.Now()); err == nil {
		t.Fatal("expected empty-sources error")
	}
	if err := SaveCachedModels(home, "us-west-2", []Source{SourceMantle}, nil, time.Now()); err != nil {
		t.Fatalf("creating valid cache: %v", err)
	}
	path, _ := CachePath(home)
	if err := os.WriteFile(path, []byte(`{"version":99,"regions":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadCachedModels(home, "us-west-2")
	if err == nil || !strings.Contains(err.Error(), "unsupported model catalog cache version 99") {
		t.Fatalf("version error = %v", err)
	}
}
