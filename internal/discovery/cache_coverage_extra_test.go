package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestSaveCachedModels_RenameError(t *testing.T) {
	home := testutil.NewTestHome(t)
	when := time.Now()

	// Save a valid cache first so the parent directory exists.
	if err := SaveCachedModels(home, testAccount, testScope, "us-west-2",
		[]Source{SourceFoundation},
		[]DiscoveredModel{{ID: "xai.grok-4.6", Source: SourceFoundation}},
		when); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	// Make the parent directory read-only so the atomic write (write tmp +
	// rename) fails. On Linux this blocks the tmp file write. On Windows
	// Unix permission bits are not enforced so we skip there.
	cachePath, _ := CachePath(home)
	parent := filepath.Dir(cachePath)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Skipf("cannot chmod directory on this platform: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, safepath.DirPerm) })

	err := SaveCachedModels(home, testAccount, testScope, "us-west-2",
		[]Source{SourceFoundation},
		[]DiscoveredModel{{ID: "openai.gpt-5.6-sol", Source: SourceFoundation}},
		when)
	if err == nil {
		t.Skip("chmod on parent directory did not block write (Windows)")
	}
	// Accept either a write error or a rename error — both exercise the
	// error-return branch of SaveCachedModels.
	if !strings.Contains(err.Error(), "committing model catalog cache") &&
		!strings.Contains(err.Error(), "writing model catalog cache") {
		t.Fatalf("expected write/rename error, got: %v", err)
	}
}

func TestLoadCache_ReadFileError(t *testing.T) {
	home := testutil.NewTestHome(t)
	path, err := CachePath(home)
	if err != nil {
		t.Fatal(err)
	}
	// Create the cache path as a directory — reading a directory as a file
	// triggers the ReadFile error path in loadCache. Use safepath to avoid
	// explicit permission literals.
	if err := safepath.MkdirAll(path); err != nil {
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
