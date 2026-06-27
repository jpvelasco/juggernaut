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
	// force the file fallback path on Windows. On non-Windows the keychain
	// has no such limit, so the token may land in the keychain — the
	// GetWithFallback path still proves round-trip correctness.
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
}

func TestGetWithFallback_ReadsFromFile(t *testing.T) {
	home := t.TempDir()
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()

	token := "file-fallback-token"
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte(token)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != token {
		t.Errorf("expected %q, got %q", token, got)
	}
}

func TestGetWithFallback_FileWinsOverKeychain(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	defer func() { _ = s.Delete() }()

	// Write a short token to the keychain.
	if err := s.Set("keychain-token"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Write a different token to the file.
	home := t.TempDir()
	defer func() { _ = s.DeleteWithFallback(home) }()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("file-token")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "file-token" {
		t.Errorf("expected file-token (fallback file should win), got %q", got)
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

func TestSetWithFallback_ClearsStaleKeychainOnFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test: requires 2560-byte keychain limit")
	}

	s := testStore()
	skipIfUnavailable(t, s)
	home := t.TempDir()
	defer func() { _ = s.DeleteWithFallback(home) }()

	// Store a short token in the keychain.
	if err := s.Set("old-short-token"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Now store a long token — should fall back to file and clear the keychain.
	longToken := strings.Repeat("z", 2600)
	if err := s.SetWithFallback(longToken, home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}

	// Verify the keychain entry was cleared.
	old, err := s.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if old != "" {
		t.Errorf("expected stale keychain entry to be cleared, got %q", old)
	}

	// Verify the new token is readable via the file fallback.
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != longToken {
		t.Errorf("expected long token, got length %d", len(got))
	}
}

func TestDeleteWithFallback_RemovesFile(t *testing.T) {
	home := t.TempDir()
	s := testStore()
	skipIfUnavailable(t, s)

	// Write directly to the file to ensure it exists regardless of platform.
	token := "to-be-deleted"
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte(token)); err != nil {
		t.Fatalf("writing file: %v", err)
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

// TestSetWithFallback_FailsWhenCredentialPathCannotBeRemoved proves that
// writeCredentialFile does not silently continue after a failed os.Remove.
// If removal fails, the subsequent os.WriteFile could overwrite the credential
// while retaining permissive file permissions — this regression test ensures
// that scenario is rejected.
func TestSetWithFallback_FailsWhenCredentialPathCannotBeRemoved(t *testing.T) {
	home := t.TempDir()
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()

	// Make the credential path a non-empty directory so os.Remove fails
	// (directory with content cannot be removed).
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := os.MkdirAll(filePath, 0o700); err != nil {
		t.Fatalf("creating directory: %v", err)
	}
	// Put something inside so the directory is non-empty and cannot be removed.
	if err := os.WriteFile(filepath.Join(filePath, "blocker"), []byte("x"), 0o600); err != nil {
		t.Fatalf("writing blocker: %v", err)
	}

	// Use a short token so the keychain path is tried first; on non-Windows
	// the keychain will likely succeed and the test is meaningless. On
	// Windows with a working keychain, the keychain write succeeds and we
	// don't reach the file fallback. To force the file fallback on all
	// platforms, use an oversized token.
	token := strings.Repeat("x", 2600)
	if err := s.SetWithFallback(token, home); err == nil {
		t.Fatal("expected SetWithFallback to fail when credential path cannot be removed")
	}
}
