package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The model catalog cache binds to the local credential scope. When apply
// runs with a cache present but no binding for the CURRENT scope (stale
// binding after an account switch, or a fingerprint the catalog was never
// refreshed under), the config would silently degrade to the seeded model
// only — apply must say so instead.

// TestApply_OpenCode_WarnsOnCatalogScopeMiss: a cache file exists but its
// bindings don't cover the current credential scope; apply completes and
// warns with the models refresh remediation.
func TestApply_OpenCode_WarnsOnCatalogScopeMiss(t *testing.T) {
	home := setupApplyTest(t)

	cacheDir := filepath.Join(home, ".juggernaut")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
		t.Fatalf("mkdir .juggernaut: %v", err)
	}
	cache := `{"version": 2, "accounts": {}, "bindings": {"some-other-scope": "123456789012"}}`
	if err := os.WriteFile(filepath.Join(cacheDir, "model-catalog.json"), []byte(cache), 0o600); err != nil {
		t.Fatalf("write model-catalog.json: %v", err)
	}

	var out string
	stderr := captureStderr(t, func() {
		out = applyOpenCodeIAM(t)
	})

	if !strings.Contains(stderr, "models refresh") {
		t.Errorf("apply should warn about the scope miss with the refresh remediation, stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "us-west-2") {
		t.Errorf("warning should name the region, stderr:\n%s", stderr)
	}
	// The config is still written (degraded, not blocked).
	cfg := readOpenCodeConfig(t, home)
	if _, ok := cfg["provider"]; !ok {
		t.Error("apply must still write the config on a catalog scope miss")
	}
	if !strings.Contains(out, "Configuration written successfully") {
		t.Errorf("apply should complete, got:\n%s", out)
	}
}

// TestApply_OpenCode_NoWarnWithoutCache: a fresh install with no cache at
// all is the expected state — no warning.
func TestApply_OpenCode_NoWarnWithoutCache(t *testing.T) {
	home := setupApplyTest(t)

	stderr := captureStderr(t, func() {
		_ = applyOpenCodeIAM(t)
	})

	if strings.Contains(stderr, "model catalog") {
		t.Errorf("no warning without any cache file, stderr:\n%s", stderr)
	}
	_ = home
}
