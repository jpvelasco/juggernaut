package keychain

import (
	"runtime"
	"strings"
	"testing"
)

// TestIsTooBigForKeychain_ExactBoundary covers the threshold arithmetic at the
// exact MaxWindowsKeychainSize limit. On non-Windows the function short-circuits
// to false, so the boundary logic is Windows-only.
func TestIsTooBigForKeychain_ExactBoundary(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("threshold branch is Windows-only")
	}
	atLimit := strings.Repeat("a", MaxWindowsKeychainSize)
	overLimit := strings.Repeat("a", MaxWindowsKeychainSize+1)
	underLimit := strings.Repeat("a", MaxWindowsKeychainSize-1)

	if IsTooBigForKeychain(atLimit) {
		t.Errorf("a token of exactly %d bytes must fit (got too-big)", MaxWindowsKeychainSize)
	}
	if IsTooBigForKeychain(underLimit) {
		t.Errorf("a token of %d bytes must fit", MaxWindowsKeychainSize-1)
	}
	if !IsTooBigForKeychain(overLimit) {
		t.Errorf("a token of %d bytes must be rejected", MaxWindowsKeychainSize+1)
	}
}

// TestExtractToken_LegacyV1 covers the v1 (plaintext) credential path: the
// prefix is stripped and the raw token returned.
func TestExtractToken_LegacyV1(t *testing.T) {
	got := extractTokenFromVersionedCredential([]byte(credentialVersionPrefix + "my-token"))
	if got != "my-token" {
		t.Errorf("v1 extract = %q, want %q", got, "my-token")
	}
}

// TestExtractToken_UnprefixedIsReturnedRaw covers the fallback branch: data with
// no known version prefix is returned as-is (legacy bare token).
func TestExtractToken_UnprefixedIsReturnedRaw(t *testing.T) {
	got := extractTokenFromVersionedCredential([]byte("bare-token"))
	if got != "bare-token" {
		t.Errorf("unprefixed extract = %q, want %q", got, "bare-token")
	}
}

// TestExtractToken_V2InvalidBase64FailsClosed covers the v2 branch where the
// body is not valid base64: the function must yield "" (no usable credential)
// rather than leaking ciphertext.
func TestExtractToken_V2InvalidBase64FailsClosed(t *testing.T) {
	got := extractTokenFromVersionedCredential([]byte(credentialVersionPrefixV2 + "!!!not-base64!!!"))
	if got != "" {
		t.Errorf("v2 with invalid base64 must fail closed, got %q", got)
	}
}
