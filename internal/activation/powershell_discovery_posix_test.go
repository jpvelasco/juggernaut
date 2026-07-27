//go:build !windows

package activation

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestDiscoverPowerShellProfiles_POSIX(t *testing.T) {
	testutil.NewTestHome(t)
	result := discoverPowerShellProfiles()
	if len(result.ActiveTargets) == 0 {
		t.Fatal("expected non-empty ActiveTargets on POSIX")
	}
	if len(result.EditionsDiscovered) != 1 || result.EditionsDiscovered[0] != "non-windows" {
		t.Errorf("expected editions [non-windows], got %v", result.EditionsDiscovered)
	}
}

func TestDiscoverPowerShellProfilesScoped_POSIX(t *testing.T) {
	home := testutil.NewTestHome(t)
	scoped := discoverPowerShellProfilesScoped(home)
	unscoped := discoverPowerShellProfiles()
	// On POSIX the scoped variant delegates to the unscoped one.
	if len(scoped.ActiveTargets) != len(unscoped.ActiveTargets) {
		t.Errorf("scoped target count %d != unscoped %d",
			len(scoped.ActiveTargets), len(unscoped.ActiveTargets))
	}
}

func TestContainsPathCI_POSIX(t *testing.T) {
	paths := []string{"/a/b", "/c/d"}
	if !containsPathCI(paths, "/a/b") {
		t.Error("expected exact path to be found")
	}
	if containsPathCI(paths, "/x/y") {
		t.Error("absent path should not be found")
	}
}

func TestDeduplicatePathsCI_POSIX(t *testing.T) {
	in := []string{"/a", "/b", "/a", "/c", "/b"}
	got := deduplicatePathsCI(in)
	want := []string{"/a", "/b", "/c"}
	if len(got) != len(want) {
		t.Fatalf("deduplicatePathsCI(%v) = %v, want %v", in, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("at %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidateAndCanonicalizePath_POSIX(t *testing.T) {
	if got := validateAndCanonicalizePath("/a/b/../b/c", ""); got != "/a/b/c" {
		t.Errorf("validateAndCanonicalizePath cleaned = %q, want /a/b/c", got)
	}
	if got := validateAndCanonicalizePath(".", ""); got != "" {
		t.Errorf("validateAndCanonicalizePath(\".\") = %q, want empty", got)
	}
}

func TestPSRunnerNoOps_POSIX(t *testing.T) {
	// These are no-ops on POSIX; assert they don't panic.
	SetPSRunnerForTesting(nil)
	ResetPSRunnerForTesting()
}
