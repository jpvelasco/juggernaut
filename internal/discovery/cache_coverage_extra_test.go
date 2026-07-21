package discovery

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSaveCachedModels_RenameError(t *testing.T) {
	home := t.TempDir()
	when := time.Now()

	// Save a valid cache first so the parent directory exists.
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2",
		[]Source{SourceMantle},
		[]DiscoveredModel{{ID: "xai.grok-4.3", Source: SourceMantle}},
		when); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Make the existing cache file read-only so the rename cannot overwrite it.
	cachePath, _ := CachePath(home)
	if err := os.Chmod(cachePath, 0o444); err != nil {
		t.Skipf("cannot chmod on this platform: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(cachePath, 0o644) })

	err := SaveCachedModels(home, testAccount, testScope, "us-west-2",
		[]Source{SourceMantle},
		[]DiscoveredModel{{ID: "openai.gpt-5.5", Source: SourceMantle}},
		when)
	if err == nil || !strings.Contains(err.Error(), "committing model catalog cache") {
		t.Fatalf("expected rename error, got %v", err)
	}
}

func TestLoadCache_ReadFileError(t *testing.T) {
	home := t.TempDir()
	path, err := CachePath(home)
	if err != nil {
		t.Fatal(err)
	}
	// Create the cache path as a directory — reading a directory as a file
	// triggers the ReadFile error path in loadCache.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err = loadCache(home)
	if err == nil || !strings.Contains(err.Error(), "reading model catalog cache") {
		t.Fatalf("expected read error for directory, got %v", err)
	}
}

func TestFoundationModelAvailability_NilOutput(t *testing.T) {
	got := foundationModelAvailability(nil)
	if got != "UNKNOWN" {
		t.Errorf("nil output = %q, want UNKNOWN", got)
	}
}