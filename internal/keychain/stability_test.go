package keychain_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestSetWithFallback_PreservesKeychainWhenFileWriteFails is the core guarantee
// of the write-before-delete fix: if the file fallback write fails, a working
// keychain credential must NOT be wiped. Previously the keychain was deleted
// before the (failing) file write, leaving the user with no credential at all.
func TestSetWithFallback_PreservesKeychainWhenFileWriteFails(t *testing.T) {
	home := t.TempDir()
	s := keychain.NewStore("jug-stability-preserve")
	skipIfUnavailableStab(t, s)
	t.Cleanup(func() { _ = s.DeleteWithFallback(home) })

	// Seed a working small credential in the keychain.
	if err := s.Set("existing-good-token"); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	// Force the file-fallback write to fail by making the credential path an
	// un-removable non-empty directory, then store an oversized token (which
	// always routes to the file fallback).
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.MkdirAll(filePath); err != nil {
		t.Fatalf("creating blocking dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filePath, "blocker"), []byte("x"), 0o600); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}

	bigToken := strings.Repeat("x", 2600)
	if err := s.SetWithFallback(bigToken, home); err == nil {
		t.Fatal("expected SetWithFallback to fail when the file path is unwritable")
	}

	// THE GUARANTEE: the previous keychain credential must still be present.
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() after failed fallback write: %v", err)
	}
	if got != "existing-good-token" {
		t.Errorf("keychain credential was wiped by a failed fallback write; got %q, want %q",
			got, "existing-good-token")
	}
}

// TestGet_FallsBackToLegacyV3Account verifies that a user upgrading from v3
// (credential stored only under the legacy "api-key" account) is still found.
func TestGet_FallsBackToLegacyV3Account(t *testing.T) {
	s := keychain.NewStore("jug-stability-v3")
	skipIfUnavailableStab(t, s)
	t.Cleanup(func() { _ = s.Delete() })

	// Seed ONLY the legacy v3 account, leaving the modern account empty.
	if err := s.SetLegacyForTesting("v3-era-token"); err != nil {
		t.Fatalf("seeding legacy account: %v", err)
	}

	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "v3-era-token" {
		t.Errorf("v3 legacy account fallback failed: got %q, want %q", got, "v3-era-token")
	}
}

// TestSetWithFallback_V1FileMigratesOnWrite verifies the v1->v2 migration: a
// user upgrading from v5.2.2/5.2.3 has a v1 plaintext fallback file; writing a
// new credential must produce a v2 (encrypted) envelope on Windows, and a v1
// envelope on non-Windows, round-tripping correctly either way.
func TestSetWithFallback_V1FileMigratesOnWrite(t *testing.T) {
	home := t.TempDir()
	s := keychain.NewStore("jug-stability-migrate")
	t.Cleanup(func() { _ = s.DeleteWithFallback(home) })

	// Seed a v1 plaintext file (simulates a v5.2.2/5.2.3 large-key state).
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\nold-big-token")); err != nil {
		t.Fatalf("seeding v1 file: %v", err)
	}

	// Write a new oversized token (routes to the file fallback on every OS).
	newToken := strings.Repeat("y", 2600)
	if err := s.SetWithFallback(newToken, home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}

	raw, err := safepath.ReadFile(home, filePath)
	if err != nil {
		t.Fatalf("reading migrated file: %v", err)
	}
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(string(raw), "juggernaut-credential-v2\n") {
			t.Errorf("on Windows the file should migrate to a v2 envelope, got prefix %q", first25(raw))
		}
		if strings.Contains(string(raw), newToken) {
			t.Error("on Windows the token must not be present in plaintext after migration")
		}
	} else {
		if !strings.HasPrefix(string(raw), "juggernaut-credential-v1\n") {
			t.Errorf("on non-Windows the file should be a v1 envelope, got prefix %q", first25(raw))
		}
	}

	// Either way, it must round-trip back to the new token.
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != newToken {
		t.Errorf("migrated file did not round-trip: got %d bytes, want %d", len(got), len(newToken))
	}
}

func first25(b []byte) string {
	if len(b) > 25 {
		return string(b[:25])
	}
	return string(b)
}

// skipIfUnavailableStab is a local probe (the package's skipIfUnavailable lives
// in keychain_test.go in the same package; redeclared name avoided).
func skipIfUnavailableStab(t *testing.T, s *keychain.Store) {
	t.Helper()
	if err := s.Set("probe"); err != nil {
		t.Skipf("keychain backend unavailable: %v", err)
	}
	_ = s.Delete()
}
