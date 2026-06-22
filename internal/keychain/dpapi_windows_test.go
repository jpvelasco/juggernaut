//go:build windows

package keychain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

func TestDPAPIBackend_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JUGGERNAUT_HOME", "")

	b, err := keychain.Resolve("dpapi", home)
	if err != nil {
		t.Fatalf("Resolve(dpapi) error: %v", err)
	}

	if err := b.Set("dpapi-secret"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Ciphertext must actually be encrypted on disk, not stored in cleartext.
	raw, err := os.ReadFile(filepath.Join(home, ".juggernaut", "bearer-token.dpapi.bin"))
	if err != nil {
		t.Fatalf("reading dpapi file: %v", err)
	}
	if string(raw) == "dpapi-secret" {
		t.Fatal("token stored in cleartext; expected DPAPI ciphertext")
	}

	got, err := b.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "dpapi-secret" {
		t.Errorf("expected dpapi-secret, got %q", got)
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

func TestDPAPIBackend_GetMissingIsEmpty(t *testing.T) {
	t.Setenv("JUGGERNAUT_HOME", "")
	b, err := keychain.Resolve("dpapi", t.TempDir())
	if err != nil {
		t.Fatalf("Resolve(dpapi) error: %v", err)
	}
	got, err := b.Get()
	if err != nil {
		t.Fatalf("Get() on missing error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty for missing dpapi file, got %q", got)
	}
}
