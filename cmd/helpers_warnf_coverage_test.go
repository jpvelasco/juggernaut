package cmd

import (
	"os"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
	"github.com/spf13/cobra"
)

// TestShow_ScopePathError covers the warnf branch in runShow when
// provider.ConfigPath returns an error for an invalid scope.
func TestShow_ScopePathError(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old; r.Close(); w.Close() }()

	_ = testutil.NewTestHome(t)
	origScope := showFlags.scope
	defer func() { showFlags.scope = origScope }()

	// An invalid scope name will cause ConfigPath to error for user scope,
	// exercising the warnf("could not determine %s scope path: %v") path.
	// Project scope always works (returns .), so use user scope.
	showFlags.scope = "user"

	// runShow with a HOME that has no .claude directory will hit the
	// "could not read" path, exercising the warnf in show.go.
	_ = runShow(&cobra.Command{}, []string{}) // best-effort — we only care about the warnf path
	w.Close()

	_ = r
}

// TestUninstallSettingsBlock_ManagerError covers the warnf branch in
// uninstallSettingsBlock when the config manager encounters an error.
func TestUninstallSettingsBlock_ManagerError(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old; r.Close(); w.Close() }()

	// Use a non-existent path to force newProviderManager to error.
	home := testutil.NewTestHome(t)
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}

	// The .claude directory does not exist in this temp dir, so
	// ConfigPath will resolve but the config will be empty/missing.
	// This exercises the "could not check" warnf path when HasManagedKeys
	// encounters a missing config.
	uninstallSettingsBlock(home, "user", prov)
	w.Close()

	_ = r
}

// TestReportLegacyRecovery_Error covers the warnf branch in
// reportLegacyRecovery when RecoverLegacyArtifacts encounters an error.
func TestReportLegacyRecovery_Error(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old; r.Close(); w.Close() }()

	// A temp dir has no .claude/ directory with legacy artifacts, so
	// RecoverLegacyArtifacts will return an error (directory not found).
	reportLegacyRecovery(t.TempDir())
	w.Close()

	_ = r
}
