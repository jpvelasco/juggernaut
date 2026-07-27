package discovery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

const (
	testAccount = "111122223333"
	testScope   = "profile-a"
)

func TestCatalogCache_MissingAndSourceAwareMerge(t *testing.T) {
	home := testutil.NewTestHome(t)
	if _, found, err := LoadCachedModels(home, testScope, "us-west-2"); err != nil || found {
		t.Fatalf("missing cache = found %v, err %v; want false, nil", found, err)
	}

	mantleTime := time.Date(2026, 7, 20, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2", []Source{SourceMantle}, []DiscoveredModel{
		{ID: "zai.glm-5", Source: SourceMantle, Status: "ACTIVE"},
		{ID: "moonshotai.kimi-k2.5", Source: SourceMantle, Status: "ACTIVE"},
	}, mantleTime); err != nil {
		t.Fatalf("saving Mantle cache: %v", err)
	}

	nativeTime := mantleTime.Add(time.Hour)
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2", []Source{SourceFoundation, SourceProfile}, []DiscoveredModel{
		{ID: "anthropic.claude-opus", Source: SourceFoundation, Status: "ACTIVE"},
		{ID: "global.anthropic.claude-opus", Source: SourceProfile, Status: "ACTIVE"},
	}, nativeTime); err != nil {
		t.Fatalf("saving native cache: %v", err)
	}

	snapshot, found, err := LoadCachedModels(home, testScope, "us-west-2")
	if err != nil || !found {
		t.Fatalf("loading merged cache = found %v, err %v", found, err)
	}
	if snapshot.AccountID != testAccount {
		t.Errorf("AccountID = %q, want %q", snapshot.AccountID, testAccount)
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

	if err := SaveCachedModels(home, testAccount, testScope, "us-east-1", []Source{SourceMantle}, []DiscoveredModel{
		{ID: "openai.gpt-5.5", Source: SourceMantle},
	}, nativeTime); err != nil {
		t.Fatalf("saving second region: %v", err)
	}
	if _, found, err := LoadCachedModels(home, testScope, "us-west-2"); err != nil || !found {
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

func TestCatalogCache_IsolatesAccountsAndRebindsCredentialScope(t *testing.T) {
	home := testutil.NewTestHome(t)
	when := time.Now()
	modelA := []DiscoveredModel{{ID: "account-a-model", Source: SourceMantle}}
	modelB := []DiscoveredModel{{ID: "account-b-model", Source: SourceMantle}}

	if err := SaveCachedModels(home, "account-a", "scope-a", "us-west-2", []Source{SourceMantle}, modelA, when); err != nil {
		t.Fatal(err)
	}
	if err := SaveCachedModels(home, "account-b", "scope-b", "us-west-2", []Source{SourceMantle}, modelB, when); err != nil {
		t.Fatal(err)
	}
	for scope, want := range map[string]string{"scope-a": "account-a-model", "scope-b": "account-b-model"} {
		snapshot, found, err := LoadCachedModels(home, scope, "us-west-2")
		if err != nil || !found || len(snapshot.Models) != 1 || snapshot.Models[0].ID != want {
			t.Errorf("scope %q snapshot = %+v, found %v, err %v; want %q", scope, snapshot, found, err, want)
		}
	}
	if _, found, err := LoadCachedModels(home, "unknown-scope", "us-west-2"); err != nil || found {
		t.Errorf("unknown scope = found %v, err %v; want false, nil", found, err)
	}

	if err := SaveCachedModels(home, "account-b", "scope-a", "us-west-2", []Source{SourceMantle}, modelB, when); err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := LoadCachedModels(home, "scope-a", "us-west-2")
	if err != nil || !found || snapshot.AccountID != "account-b" || snapshot.Models[0].ID != "account-b-model" {
		t.Fatalf("rebound scope snapshot = %+v, found %v, err %v", snapshot, found, err)
	}
}

func TestCatalogCache_RejectsInvalidInputsAndVersion(t *testing.T) {
	home := testutil.NewTestHome(t)
	validSources := []Source{SourceMantle}
	if err := SaveCachedModels(home, "", testScope, "us-west-2", validSources, nil, time.Now()); err == nil {
		t.Fatal("expected empty-account error")
	}
	if err := SaveCachedModels(home, testAccount, "", "us-west-2", validSources, nil, time.Now()); err == nil {
		t.Fatal("expected empty-scope error")
	}
	if err := SaveCachedModels(home, testAccount, testScope, "", validSources, nil, time.Now()); err == nil {
		t.Fatal("expected empty-region error")
	}
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2", nil, nil, time.Now()); err == nil {
		t.Fatal("expected empty-sources error")
	}
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2", validSources, nil, time.Now()); err != nil {
		t.Fatalf("creating valid cache: %v", err)
	}
	path, _ := CachePath(home)
	if err := os.WriteFile(path, []byte(`{"version":99,"accounts":{},"bindings":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadCachedModels(home, testScope, "us-west-2")
	if err == nil || !strings.Contains(err.Error(), "unsupported model catalog cache version 99") {
		t.Fatalf("version error = %v", err)
	}
}

func TestCatalogCache_ReportsMalformedCache(t *testing.T) {
	home := testutil.NewTestHome(t)
	path, err := CachePath(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2", []Source{SourceMantle}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadCachedModels(home, testScope, "us-west-2"); err == nil || !strings.Contains(err.Error(), "parsing model-catalog.json") {
		t.Fatalf("malformed cache error = %v", err)
	}
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2", []Source{SourceMantle}, nil, time.Now()); err == nil || !strings.Contains(err.Error(), "parsing model-catalog.json") {
		t.Fatalf("save with malformed cache error = %v", err)
	}
}

func TestCatalogCache_MissingAccountAndReadFailure(t *testing.T) {
	home := testutil.NewTestHome(t)
	path, err := CachePath(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":2,"accounts":{},"bindings":{"profile-a":"missing-account"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := LoadCachedModels(home, testScope, "us-west-2"); err != nil || found {
		t.Fatalf("binding to missing account = found %v, err %v; want false, nil", found, err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadCachedModels(home, testScope, "us-west-2"); err == nil || !strings.Contains(err.Error(), "reading model catalog cache") {
		t.Fatalf("read failure error = %v", err)
	}
}
