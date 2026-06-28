package keychain

import (
	"errors"
	"strings"
	"testing"
)

// TestEncodeCredential_FailsClosedWhenDPAPIErrors verifies the P1 fix: when
// DPAPI is available but encryption fails, encodeCredential must return an error
// rather than silently downgrading to the v1 plaintext envelope (which would
// reintroduce plaintext-at-rest storage of the bearer token).
func TestEncodeCredential_FailsClosedWhenDPAPIErrors(t *testing.T) {
	origAvail, origEnc := dpapiAvailableFn, encryptFn
	t.Cleanup(func() { dpapiAvailableFn, encryptFn = origAvail, origEnc })

	dpapiAvailableFn = func() bool { return true }
	encryptFn = func([]byte) ([]byte, error) { return nil, errors.New("CryptProtectData boom") }

	payload, err := encodeCredential("super-secret-bearer-token")
	if err == nil {
		t.Fatal("expected encodeCredential to fail closed when DPAPI encryption errors")
	}
	if payload != nil {
		t.Errorf("expected nil payload on failure, got %q", payload)
	}
	// Critically: the plaintext token must NOT appear in any returned bytes.
	if strings.Contains(string(payload), "super-secret-bearer-token") {
		t.Error("plaintext token must never be returned when encryption fails")
	}
}

// TestEncodeCredential_EncryptsWhenDPAPIAvailable verifies the happy path uses
// the v2 envelope and does not leak plaintext.
func TestEncodeCredential_EncryptsWhenDPAPIAvailable(t *testing.T) {
	origAvail, origEnc := dpapiAvailableFn, encryptFn
	t.Cleanup(func() { dpapiAvailableFn, encryptFn = origAvail, origEnc })

	dpapiAvailableFn = func() bool { return true }
	encryptFn = func(b []byte) ([]byte, error) { return append([]byte("ENC:"), b...), nil }

	payload, err := encodeCredential("tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(string(payload), credentialVersionPrefixV2) {
		t.Errorf("expected v2 envelope, got %q", payload)
	}
}

// TestEncodeCredential_PlaintextWhenNoDPAPI verifies non-Windows uses v1.
func TestEncodeCredential_PlaintextWhenNoDPAPI(t *testing.T) {
	origAvail := dpapiAvailableFn
	t.Cleanup(func() { dpapiAvailableFn = origAvail })

	dpapiAvailableFn = func() bool { return false }

	payload, err := encodeCredential("tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(payload) != credentialVersionPrefix+"tok" {
		t.Errorf("expected v1 plaintext envelope, got %q", payload)
	}
}
