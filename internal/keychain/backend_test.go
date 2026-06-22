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

func TestClearOthers_RemovesTokensFromNonSelectedBackends(t *testing.T) {
	home := t.TempDir()
	tokenPath := home + "/profile-token"
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)

	// Seed the profile backend, then "select" keychain and clear others.
	profile := keychain.NewProfileBackend(home)
	if err := profile.Set("stale-profile-token"); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	if err := keychain.ClearOthers("keychain", home); err != nil {
		t.Fatalf("ClearOthers error: %v", err)
	}

	got, err := profile.Get()
	if err != nil {
		t.Fatalf("profile Get error: %v", err)
	}
	if got != "" {
		t.Errorf("expected profile cleared, got %q", got)
	}
}

func TestClearOthers_DoesNotTouchSelectedBackend(t *testing.T) {
	home := t.TempDir()
	tokenPath := home + "/profile-token"
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", tokenPath)

	profile := keychain.NewProfileBackend(home)
	if err := profile.Set("keep-me"); err != nil {
		t.Fatalf("seeding profile: %v", err)
	}

	// Selecting profile must not delete the profile token.
	if err := keychain.ClearOthers("profile", home); err != nil {
		t.Fatalf("ClearOthers error: %v", err)
	}
	if got, _ := profile.Get(); got != "keep-me" {
		t.Errorf("expected selected profile backend untouched, got %q", got)
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
