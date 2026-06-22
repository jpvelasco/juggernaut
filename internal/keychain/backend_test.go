package keychain_test

import (
	"runtime"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

func TestResolve_DefaultsToKeychain(t *testing.T) {
	for _, mode := range []string{"", "keychain"} {
		b, err := keychain.Resolve(mode, t.TempDir())
		if err != nil {
			t.Fatalf("Resolve(%q) error: %v", mode, err)
		}
		if _, ok := b.(*keychain.Store); !ok {
			t.Errorf("Resolve(%q) = %T, want *keychain.Store", mode, b)
		}
	}
}

func TestResolve_Profile(t *testing.T) {
	b, err := keychain.Resolve("profile", t.TempDir())
	if err != nil {
		t.Fatalf("Resolve(profile) error: %v", err)
	}
	if _, ok := b.(*keychain.ProfileBackend); !ok {
		t.Errorf("Resolve(profile) = %T, want *keychain.ProfileBackend", b)
	}
}

func TestResolve_UnknownMode(t *testing.T) {
	if _, err := keychain.Resolve("bogus", t.TempDir()); err == nil {
		t.Fatal("Resolve(bogus) expected error, got nil")
	}
}

func TestResolve_DPAPIPlatformGating(t *testing.T) {
	b, err := keychain.Resolve("dpapi", t.TempDir())
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Resolve(dpapi) on windows error: %v", err)
		}
		if b == nil {
			t.Fatal("Resolve(dpapi) on windows returned nil backend")
		}
	} else if err == nil {
		t.Fatal("Resolve(dpapi) on non-windows expected error, got nil")
	}
}
