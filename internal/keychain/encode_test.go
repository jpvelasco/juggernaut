package keychain

import (
	"bytes"
	"encoding/base64"
	"runtime"
	"testing"
)

// TestEncodeCredential_V1 writes a short token on non-Windows (no DPAPI)
// and verifies the v1 plaintext envelope is produced.
func TestEncodeCredential_V1(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 envelope is non-Windows only")
	}
	tok := "test-token-123"
	data, err := encodeCredential(tok)
	if err != nil {
		t.Fatalf("encodeCredential error: %v", err)
	}
	if !bytes.HasPrefix(data, []byte(credentialVersionPrefix)) {
		t.Errorf("expected v1 prefix, got %q", string(data[:min(30, len(data))]))
	}
	if !bytes.Contains(data, []byte(tok)) {
		t.Errorf("expected plaintext token in v1 envelope")
	}
}

// TestEncodeCredential_DPAPIUnavailable: when DPAPI reports unavailable,
// encodeCredential falls back to the v1 plaintext envelope even on Windows.
func TestEncodeCredential_DPAPIUnavailable(t *testing.T) {
	orig := dpapiAvailableFn
	dpapiAvailableFn = func() bool { return false }
	defer func() { dpapiAvailableFn = orig }()

	tok := "test-token-456"
	data, err := encodeCredential(tok)
	if err != nil {
		t.Fatalf("encodeCredential error: %v", err)
	}
	if !bytes.HasPrefix(data, []byte(credentialVersionPrefix)) {
		t.Errorf("expected v1 prefix when DPAPI unavailable, got %q", string(data[:min(30, len(data))]))
	}
}

// TestIsVersionedCredential_V1 recognizes a v1 prefix.
func TestIsVersionedCredential_V1(t *testing.T) {
	if !isVersionedCredential([]byte("juggernaut-credential-v1\nsome-token")) {
		t.Error("expected v1 data to be recognized as versioned")
	}
}

// TestIsVersionedCredential_V2 recognizes a v2 prefix.
func TestIsVersionedCredential_V2(t *testing.T) {
	if !isVersionedCredential([]byte("juggernaut-credential-v2\n" + base64.StdEncoding.EncodeToString([]byte("ciphertext")))) {
		t.Error("expected v2 data to be recognized as versioned")
	}
}

// TestIsVersionedCredential_Unrecognized returns false for data without a
// recognised prefix.
func TestIsVersionedCredential_Unrecognized(t *testing.T) {
	if isVersionedCredential([]byte("unrecognised-prefix\ntoken")) {
		t.Error("expected unrecognised prefix to not be versioned")
	}
}

// TestExtractTokenFromVersionedCredential_V1 strips the v1 prefix and
// returns the plaintext token.
func TestExtractTokenFromVersionedCredential_V1(t *testing.T) {
	tok := "plain-token-v1"
	data := []byte(credentialVersionPrefix + tok)
	got := extractTokenFromVersionedCredential(data)
	if got != tok {
		t.Errorf("expected %q, got %q", tok, got)
	}
}

// TestExtractTokenFromVersionedCredential_V2DecryptFails returns empty
// when the v2 ciphertext cannot be decoded (simulates a corrupted file).
func TestExtractTokenFromVersionedCredential_V2DecryptFails(t *testing.T) {
	// v2 prefix + invalid base64
	data := []byte("juggernaut-credential-v2\n!!!not-valid-base64!!!")
	got := extractTokenFromVersionedCredential(data)
	if got != "" {
		t.Errorf("expected empty string for corrupted v2 data, got %q", got)
	}
}

// TestExtractTokenFromVersionedCredential_V2DecryptError returns empty
// when DPAPI decryption fails (simulates a corrupted v2 envelope).
func TestExtractTokenFromVersionedCredential_V2DecryptError(t *testing.T) {
	// Build a valid base64 string that is not a valid DPAPI ciphertext.
	ciphertext := base64.StdEncoding.EncodeToString([]byte("not-valid-dpapi-ciphertext"))
	data := []byte(credentialVersionPrefixV2 + ciphertext)
	got := extractTokenFromVersionedCredential(data)
	if got != "" {
		t.Errorf("expected empty string for undecryptable v2 data, got %q", got)
	}
}
