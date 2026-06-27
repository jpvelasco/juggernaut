package keychain_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

func testStore() *keychain.Store {
	svc := os.Getenv("JUGGERNAUT_KEYCHAIN_SERVICE")
	if svc == "" {
		svc = "juggernaut-test-isolated"
	}
	return keychain.NewStore(svc)
}

// skipIfUnavailable skips the test if the keychain backend is not available.
// On headless Linux CI (no Secret Service daemon), all keychain ops fail.
func skipIfUnavailable(t *testing.T, s *keychain.Store) {
	t.Helper()
	if err := s.Set("probe"); err != nil {
		t.Skipf("keychain backend unavailable: %v", err)
	}
	if err := s.Delete(); err != nil {
		t.Skipf("keychain backend unavailable: %v", err)
	}
}

func TestStoreAndGet(t *testing.T) {
	s := testStore()
	defer func() { _ = s.Delete() }()
	skipIfUnavailable(t, s)

	if err := s.Set("test-token-value"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "test-token-value" {
		t.Errorf("expected test-token-value, got %q", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	_ = s.Delete()

	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() on missing key returned error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestDelete(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	_ = s.Set("to-delete")

	if err := s.Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	got, _ := s.Get()
	if got != "" {
		t.Error("expected empty after delete")
	}
}

func TestSetWithFallback_FallsBackToFile(t *testing.T) {
	home := t.TempDir()
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()

	// Use a token longer than the Windows keychain limit (2560 bytes) to
	// force the file fallback path.
	token := strings.Repeat("x", 2600)
	if err := s.SetWithFallback(token, home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}

	// Read it back via the fallback getter.
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != token {
		t.Errorf("expected token of length %d, got %d", len(token), len(got))
	}

	// Verify the file exists and has the right content.
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading fallback file: %v", err)
	}
	if string(data) != token {
		t.Errorf("file content mismatch: expected length %d, got %d", len(token), len(data))
	}
}

func TestGetWithFallback_ReturnsEmptyWhenNothingStored(t *testing.T) {
	home := t.TempDir()
	s := testStore()

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got length %d", len(got))
	}
}

func TestDeleteWithFallback_RemovesFile(t *testing.T) {
	home := t.TempDir()
	s := testStore()

	// Force file fallback with a long token.
	token := strings.Repeat("y", 2600)
	if err := s.SetWithFallback(token, home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}

	if err := s.DeleteWithFallback(home); err != nil {
		t.Fatalf("DeleteWithFallback() error: %v", err)
	}

	// File should be gone.
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty after delete, got length %d", len(got))
	}
}

func TestIsTooBigForKeychain(t *testing.T) {
	// On non-Windows, always false regardless of size.
	if runtime.GOOS != "windows" {
		if keychain.IsTooBigForKeychain("short") {
			t.Error("expected false for short token on non-Windows")
		}
		if keychain.IsTooBigForKeychain(strings.Repeat("a", 3000)) {
			t.Error("expected false for long token on non-Windows")
		}
		return
	}

	// On Windows, short tokens are fine, long ones are rejected.
	if keychain.IsTooBigForKeychain("short") {
		t.Error("expected false for short token on Windows")
	}
	if !keychain.IsTooBigForKeychain(strings.Repeat("a", 3000)) {
		t.Error("expected true for long token on Windows")
	}
}
