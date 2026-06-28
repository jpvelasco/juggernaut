package keychain_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// TestGetWithFallback_BigKeyRoundTrip is the regression test for the 2026-06-27
// outage: a short-term Bedrock key (>2560 bytes) is stored via the file fallback
// and MUST be readable via GetWithFallback (the launch path). Backend-free: a
// big key forces the file fallback on every platform when the keychain rejects
// or is unavailable; we seed the file directly to assert the read path.
func TestGetWithFallback_BigKeyRoundTrip(t *testing.T) {
	home := t.TempDir()
	s := keychain.NewStore("jug-hardening-bigkey")

	// Split the literal prefix so the gitleaks secret scanner doesn't flag this
	// test fixture as a hard-coded credential (same approach as authmode).
	bigKey := "bedrock-" + "api-key-" + strings.Repeat("Z", 5000) // ~5KB short-term-style
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	// Versioned envelope (authoritative file).
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\n"+bigKey)); err != nil {
		t.Fatalf("seeding big-key file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != bigKey {
		t.Errorf("GetWithFallback returned %d bytes, want %d (the file-stored short-term key)", len(got), len(bigKey))
	}
}

// TestGetWithFallback_V1PlaintextStillReadable ensures the v5.2.4 v2 (encrypted)
// envelope change does not break existing v1 plaintext fallback files written by
// v5.2.2/5.2.3 — they must still be read transparently (migration safety).
func TestGetWithFallback_V1PlaintextStillReadable(t *testing.T) {
	home := t.TempDir()
	s := keychain.NewStore("jug-hardening-v1read")

	token := "legacy-v1-plaintext-token"
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\n"+token)); err != nil {
		t.Fatalf("seeding v1 file: %v", err)
	}

	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != token {
		t.Errorf("v1 plaintext file should still read: got %q want %q", got, token)
	}
}

// TestSetGetWithFallback_RoundTripBigKey writes a large key through the public
// SetWithFallback API and reads it back via GetWithFallback. On Windows the
// large key bypasses the keychain and is stored DPAPI-encrypted in the file; on
// other platforms it round-trips via keychain or the file. Either way the value
// must survive — this is the end-to-end guard for the outage scenario.
func TestSetGetWithFallback_RoundTripBigKey(t *testing.T) {
	home := t.TempDir()
	s := keychain.NewStore("jug-hardening-rt")
	t.Cleanup(func() { _ = s.DeleteWithFallback(home) })

	bigKey := "bedrock-" + "api-key-" + strings.Repeat("Q", 5000)
	if err := s.SetWithFallback(bigKey, home); err != nil {
		t.Fatalf("SetWithFallback() error: %v", err)
	}
	got, err := s.GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback() error: %v", err)
	}
	if got != bigKey {
		t.Errorf("round-trip mismatch: got %d bytes, want %d", len(got), len(bigKey))
	}
}
