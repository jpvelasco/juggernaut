package keychain_test

import (
	"os"
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
