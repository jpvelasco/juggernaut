package keychain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

// fakeBackend is an in-memory Backend for exercising MigrateInto without touching
// the real OS keyring.
type fakeBackend struct {
	val string
	set bool
}

func (f *fakeBackend) Set(token string) error { f.val = token; f.set = true; return nil }
func (f *fakeBackend) Get() (string, error)   { return f.val, nil }
func (f *fakeBackend) Delete() error          { f.val = ""; return nil }

func TestMigrateInto_NoopWhenTargetAlreadyHasValue(t *testing.T) {
	home := t.TempDir()
	// A v3 profile token exists, but the target is already populated.
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", filepath.Join(home, "bearer-token"))
	if err := os.WriteFile(filepath.Join(home, "bearer-token"), []byte("v3-value"), 0o600); err != nil {
		t.Fatal(err)
	}

	target := &fakeBackend{val: "already-here"}
	src, err := keychain.MigrateInto(target, home)
	if err != nil {
		t.Fatalf("MigrateInto error: %v", err)
	}
	if src != "" {
		t.Errorf("expected no migration, got source %q", src)
	}
	if target.set {
		t.Error("target should not have been written when already populated")
	}
}

func TestMigrateInto_FromProfileFile(t *testing.T) {
	home := t.TempDir()
	// Isolate the keychain/CredMan service name so the migrator can't pick up a
	// real v3 Credential Manager entry on the developer's machine.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-migrate-isolated")
	t.Setenv("JUGGERNAUT_HOME", filepath.Join(home, "no-juggernaut-home"))
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", filepath.Join(home, "bearer-token"))
	if err := os.WriteFile(filepath.Join(home, "bearer-token"), []byte("v3-profile-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	target := &fakeBackend{}
	src, err := keychain.MigrateInto(target, home)
	if err != nil {
		t.Fatalf("MigrateInto error: %v", err)
	}
	if src != "profile" {
		t.Errorf("expected migration from profile, got %q", src)
	}
	if target.val != "v3-profile-token" {
		t.Errorf("expected imported value v3-profile-token, got %q", target.val)
	}
}

func TestMigrateInto_NothingToMigrate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-migrate-isolated-empty")
	t.Setenv("JUGGERNAUT_HOME", filepath.Join(home, "no-juggernaut-home"))
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", filepath.Join(home, "absent"))

	target := &fakeBackend{}
	src, err := keychain.MigrateInto(target, home)
	if err != nil {
		t.Fatalf("MigrateInto error: %v", err)
	}
	if src != "" {
		t.Errorf("expected no source, got %q", src)
	}
	if target.set {
		t.Error("target should not be written when no v3 credential exists")
	}
}
