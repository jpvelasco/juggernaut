package keychain_test

import (
	"os"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/keychain"
)

func testStore() *keychain.Store {
	svc := os.Getenv("JUGGERNAUT_KEYCHAIN_SERVICE")
	if svc == "" {
		svc = "juggernaut-test-isolated"
	}
	return keychain.NewStore(svc)
}

func TestStoreAndGet(t *testing.T) {
	s := testStore()
	defer s.Delete()

	if err := s.Set("test-token-value"); err != nil {
		t.Skipf("keychain unavailable: %v", err)
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
	s.Delete()

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
	_ = s.Set("to-delete")

	if err := s.Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	got, _ := s.Get()
	if got != "" {
		t.Error("expected empty after delete")
	}
}
