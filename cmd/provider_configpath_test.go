package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
	"github.com/spf13/cobra"
)

// TestProviderConfigPath_UserAndProject covers the provider.Get("claude") +
// ConfigPath calls introduced in doctor.go and show.go when settingsPath was
// removed. These replace the dead helper with the provider interface method.
func TestProviderConfigPath_UserAndProject(t *testing.T) {
	home := testutil.NewTestHome(t)

	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("provider.Get(claude) error: %v", err)
	}

	// User scope should resolve to ~/.claude/settings.json.
	path, err := prov.ConfigPath(home, "user")
	if err != nil {
		t.Fatalf("ConfigPath(user) error: %v", err)
	}
	want := filepath.Join(home, ".claude", "settings.json")
	if path != want {
		t.Errorf("ConfigPath(user) = %q, want %q", path, want)
	}

	// Project scope should resolve to ./.claude/settings.json.
	path, err = prov.ConfigPath(home, "project")
	if err != nil {
		t.Fatalf("ConfigPath(project) error: %v", err)
	}
	want = filepath.Join(".", ".claude", "settings.json")
	if path != want {
		t.Errorf("ConfigPath(project) = %q, want %q", path, want)
	}
}

// TestCheckConnectivity_ProviderGetPath covers the checkConnectivity path
// that calls provider.Get("claude") and prov.ConfigPath — the replacement for
// the removed settingsPath helper in doctor.go.
func TestCheckConnectivity_ProviderGetPath(t *testing.T) {
	home := testutil.NewTestHome(t)

	r := doctor.NewReport()
	// checkConnectivity with an empty token exercises the provider.Get +
	// ConfigPath path without making network calls.
	checkConnectivity(r, home, "", []string{"user"})
}

// TestShow_ProviderGetPath covers the runShow path that calls
// provider.Get("claude") and prov.ConfigPath — the replacement for
// the removed settingsPath helper.
func TestShow_ProviderGetPath(t *testing.T) {
	_ = testutil.NewTestHome(t)

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old; r.Close(); w.Close() }()

	origScope := showFlags.scope
	defer func() { showFlags.scope = origScope }()
	showFlags.scope = "user"

	// runShow exercises provider.Get("claude") + ConfigPath for each scope.
	_ = runShow(&cobra.Command{}, []string{})
	w.Close()
	_ = r
}
