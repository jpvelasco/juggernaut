package keychain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

func TestProfileBackend_SetGetDelete(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "juggernaut", "bearer-token")
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)

	b := keychain.NewProfileBackend(dir)

	if err := b.Set("profile-token"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, err := b.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "profile-token" {
		t.Errorf("expected profile-token, got %q", got)
	}

	if err := b.Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	got, err = b.Get()
	if err != nil {
		t.Fatalf("Get() after delete error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty after delete, got %q", got)
	}
}

func TestProfileBackend_GetMissingIsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", filepath.Join(dir, "nope", "bearer-token"))

	b := keychain.NewProfileBackend(dir)
	got, err := b.Get()
	if err != nil {
		t.Fatalf("Get() on missing returned error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty for missing token, got %q", got)
	}
}

// A v3 profile token file (plaintext, trailing newline) must be readable by v5
// so credentials migrate without manual intervention.
func TestProfileBackend_ReadsV3PlaintextWithWhitespace(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "juggernaut", "bearer-token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission, go_file-permissions_rule-fileperm -- test fixture under t.TempDir()
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("v3-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)

	got, err := keychain.NewProfileBackend(dir).Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "v3-token" {
		t.Errorf("expected trimmed v3-token, got %q", got)
	}
}

// JUGGERNAUT_PROFILE_TOKEN_PATH must win over XDG_CONFIG_HOME when both are set.
func TestProfileBackend_TokenPathEnvVarTakesPrecedenceOverXDG(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "explicit-token")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", explicit)

	if err := keychain.NewProfileBackend(home).Set("explicit-wins"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	data, err := os.ReadFile(explicit)
	if err != nil {
		t.Fatalf("expected token at explicit path %s: %v", explicit, err)
	}
	if string(data) != "explicit-wins" {
		t.Errorf("expected explicit-wins, got %q", string(data))
	}
	// XDG location must NOT have been written.
	if _, err := os.Stat(filepath.Join(xdg, "juggernaut", "bearer-token")); !os.IsNotExist(err) {
		t.Errorf("XDG path should be unused when JUGGERNAUT_PROFILE_TOKEN_PATH is set, stat err=%v", err)
	}
}

// XDG_CONFIG_HOME must take precedence over the home-derived default, matching v3.
func TestProfileBackend_RespectsXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if err := keychain.NewProfileBackend(home).Set("xdg-token"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	want := filepath.Join(xdg, "juggernaut", "bearer-token")
	data, err := os.ReadFile(want) // nosemgrep: gosec.G304-1, go_filesystem_rule-fileread -- test fixture under t.TempDir()
	if err != nil {
		t.Fatalf("expected token at %s: %v", want, err)
	}
	if string(data) != "xdg-token" {
		t.Errorf("expected xdg-token, got %q", string(data))
	}
}
