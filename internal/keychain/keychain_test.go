package keychain_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func testStore() *keychain.Store {
	svc := os.Getenv("JUGGERNAUT_KEYCHAIN_SERVICE")
	if svc == "" {
		svc = "juggernaut-test-isolated"
	}
	return keychain.NewStore(svc)
}

func TestDefault_UsesEnvServiceOverride(t *testing.T) {
	// With the override set, Default() must return a usable store. We can't
	// inspect the private service field, so assert behaviour: a Get on a clean
	// isolated service returns empty without error.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-default-test")
	s := keychain.Default()
	if s == nil {
		t.Fatal("Default() returned nil")
	}
	skipIfUnavailable(t, s)
	_ = s.Delete()
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() on clean isolated store: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty token on clean store, got %q", got)
	}
}

func TestDefault_NoOverrideReturnsStore(t *testing.T) {
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "")
	if keychain.Default() == nil {
		t.Fatal("Default() with no override returned nil")
	}
}

// skipIfUnavailable skips the test if the keychain backend is not available.
// On headless Linux CI (no Secret Service daemon), all keychain ops fail.
// On macOS CI the 'security' command blocks indefinitely waiting for keychain
// unlock, so Set() never returns — skip on darwin as well.
func skipIfUnavailable(t *testing.T, s *keychain.Store) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("keychain security command hangs on macOS CI")
	}
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
	if runtime.GOOS != "windows" {
		t.Skip("keychain Set hangs on macOS CI; file fallback only triggered on Windows")
	}
	home := testutil.NewTestHome(t)
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()
	skipIfUnavailable(t, s)

	// Use a token longer than the Windows keychain limit (2560 bytes) to
	// force the file fallback path on Windows.
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

func TestGetWithFallback_ReadsVersionedFile(t *testing.T) {
	home := testutil.NewTestHome(t)
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()

	token := "file-fallback-token"
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\n"+token)); err != nil {
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

func TestGetWithFallback_VersionedFileWinsOverKeychain(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	defer func() { _ = s.Delete() }()

	// Write a short token to the keychain.
	if err := s.Set("keychain-token"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Write a different token to the file with the versioned envelope.
	home := testutil.NewTestHome(t)
	defer func() { _ = s.DeleteWithFallback(home) }()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\nfile-token")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "file-token" {
		t.Errorf("expected file-token (versioned fallback file should win), got %q", got)
	}
}

func TestGetWithFallback_ReturnsEmptyWhenNothingStored(t *testing.T) {
	home := testutil.NewTestHome(t)
	s := testStore()
	// With no fallback file and nothing stored, GetWithFallback consults the
	// keychain. As of v5.2.4 a broken keychain backend (e.g. headless Linux with
	// no Secret Service) surfaces as an error rather than being silently
	// swallowed — that is the intended behavior, so skip when unavailable and
	// assert the clean "nothing stored" case only when the backend works.
	skipIfUnavailable(t, s)

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
	home := testutil.NewTestHome(t)
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
	home := testutil.NewTestHome(t)
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
	home := testutil.NewTestHome(t)
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()

	// Make the credential path a non-empty directory so os.Remove fails
	// (directory with content cannot be removed).
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.MkdirAll(filePath); err != nil {
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

// TestGetWithFallback_LegacyFilePlusKeychain_ReturnsKeychainToken verifies the
// migration scenario: a user upgrading from v5.2.2 has a stale unversioned
// fallback file (token A) and a newer token B in the keychain. The keychain
// token B should be returned and the legacy file removed.
func TestGetWithFallback_LegacyFilePlusKeychain_ReturnsKeychainToken(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	defer func() { _ = s.Delete() }()

	// Write token B to the keychain.
	if err := s.Set("keychain-token-B"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Write legacy unversioned token A to the file.
	home := testutil.NewTestHome(t)
	defer func() { _ = s.DeleteWithFallback(home) }()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("legacy-token-A")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "keychain-token-B" {
		t.Errorf("expected keychain-token-B, got %q", got)
	}

	// The stale legacy file should have been removed.
	_, err = safepath.ReadFile(home, filePath)
	if !os.IsNotExist(err) {
		t.Errorf("expected legacy file to be removed after keychain token returned")
	}
}

// TestGetWithFallback_VersionedFilePlusKeychain_ReturnsFileVersion verifies
// that a versioned fallback file is authoritative even when the keychain has
// a different token.
func TestGetWithFallback_VersionedFilePlusKeychain_ReturnsFileVersion(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	defer func() { _ = s.Delete() }()

	// Write a different token to the keychain.
	if err := s.Set("keychain-token-B"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Write versioned token A to the file.
	home := testutil.NewTestHome(t)
	defer func() { _ = s.DeleteWithFallback(home) }()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\nversioned-token-A")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "versioned-token-A" {
		t.Errorf("expected versioned-token-A, got %q", got)
	}
}

// TestGetWithFallback_LegacyFilePlusEmptyKeychain_ReturnsLegacyFile verifies
// that when the keychain is empty, the legacy unversioned file is used as a
// fallback.
func TestGetWithFallback_LegacyFilePlusEmptyKeychain_ReturnsLegacyFile(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	defer func() { _ = s.Delete() }()

	// Ensure keychain is empty.
	if err := s.Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	// Write legacy unversioned token to the file.
	home := testutil.NewTestHome(t)
	defer func() { _ = s.DeleteWithFallback(home) }()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("legacy-token-A")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "legacy-token-A" {
		t.Errorf("expected legacy-token-A, got %q", got)
	}
}

// TestSetWithFallback_KeychainSuccessRemovesFallback verifies that a
// successful keychain write removes any existing fallback file, preventing
// stale credential rotation.
func TestSetWithFallback_KeychainSuccessRemovesFallback(t *testing.T) {
	s := testStore()
	skipIfUnavailable(t, s)
	defer func() { _ = s.Delete() }()

	home := testutil.NewTestHome(t)
	defer func() { _ = s.DeleteWithFallback(home) }()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")

	// Write a legacy file first.
	if err := safepath.WriteFile(home, filePath, []byte("legacy-token")); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	// Write a short token to the keychain — should succeed and remove the file.
	if err := s.SetWithFallback("new-keychain-token", home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}

	// File should be gone.
	_, err := safepath.ReadFile(home, filePath)
	if !os.IsNotExist(err) {
		t.Errorf("expected fallback file to be removed after keychain write")
	}

	// Keychain should have the new token.
	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "new-keychain-token" {
		t.Errorf("expected new-keychain-token, got %q", got)
	}
}

// TestSetWithFallback_WritesVersionedCredential verifies that
// SetWithFallback writes a versioned envelope to the fallback file, not a
// raw token. This is Windows-only because only Windows enforces the 2560-byte
// keychain limit that forces the file fallback.
func TestSetWithFallback_WritesVersionedCredential(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only: requires 2560-byte keychain limit to force file fallback")
	}

	home := testutil.NewTestHome(t)
	s := testStore()
	defer func() { _ = s.DeleteWithFallback(home) }()

	// Force file fallback with an oversized token.
	token := strings.Repeat("x", 2600)
	if err := s.SetWithFallback(token, home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}

	// Read the raw file contents.
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	data, err := safepath.ReadFile(home, filePath)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}

	// On Windows the fallback file is now a v2 (DPAPI-encrypted) envelope.
	if !bytes.HasPrefix(data, []byte("juggernaut-credential-v2\n")) {
		t.Errorf("expected v2 encrypted credential file, got raw data starting with %q", string(data[:min(25, len(data))]))
	}

	// The plaintext token must NOT appear on disk (it is DPAPI-encrypted).
	if bytes.Contains(data, []byte(token)) {
		t.Error("plaintext token must not be present in the v2 credential file")
	}

	// And it must round-trip back to the original token via the read path.
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != token {
		t.Errorf("round-trip mismatch: got %d bytes, want %d", len(got), len(token))
	}
}

// The tests below exercise the file-fallback layer without requiring an OS
// keychain backend, so they run on headless CI (Linux) where the backend is
// unavailable and other keychain tests skip.

func TestDeleteWithFallback_RemovesFileNoBackend(t *testing.T) {
	home := testutil.NewTestHome(t)
	s := testStore()

	// Seed an authoritative versioned fallback file directly (no backend needed).
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\nsecret")); err != nil {
		t.Fatalf("seeding fallback file: %v", err)
	}

	// Sanity: the file is read back via GetWithFallback (versioned path, no backend).
	if got, err := s.GetWithFallback(home); err != nil || got != "secret" {
		t.Fatalf("GetWithFallback() = %q, %v; want \"secret\", nil", got, err)
	}

	// DeleteWithFallback removes the file even if the OS keychain backend is
	// unavailable (it returns the backend error but still deletes the file).
	// We assert the file removal, not the backend result.
	_ = s.DeleteWithFallback(home)

	// The fallback file must be gone regardless of backend availability.
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected fallback file removed, stat err = %v", err)
	}
}

func TestGetWithFallback_VersionedFileNoBackend(t *testing.T) {
	home := testutil.NewTestHome(t)
	s := testStore()

	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	token := "versioned-authoritative"
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\n"+token)); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	// A versioned file is authoritative and returns without touching the backend.
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != token {
		t.Errorf("GetWithFallback() = %q, want %q", got, token)
	}
}

func TestGetWithFallback_EmptyVersionedFile(t *testing.T) {
	home := testutil.NewTestHome(t)
	s := testStore()

	// Versioned envelope with an empty token body.
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\n")); err != nil {
		t.Fatalf("seeding file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty token from empty versioned file, got %q", got)
	}
}
