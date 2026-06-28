// Package keychain provides cross-platform credential storage via go-keyring,
// with a file-based fallback for tokens that exceed the Windows Credential
// Manager 2560-byte limit.
package keychain

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	keyring "github.com/zalando/go-keyring"
)

const defaultService = "juggernaut-bedrock"

const account = "bedrock-credential"

// MaxWindowsKeychainSize is the hard limit enforced by go-keyring on Windows
// (CRED_MAX_CREDENTIAL_BLOB_SIZE). Tokens longer than this cannot be stored
// in the OS keychain and must use the file fallback.
const MaxWindowsKeychainSize = 2560

// credentialFile is the filename used for the file-based fallback storage
// inside ~/.claude/.
const credentialFile = "juggernaut-credential"

// credentialVersionPrefix is the line prefix that marks a v1 fallback credential
// file as versioned and authoritative. The token body is stored verbatim
// (plaintext, owner-only file perms). Files without any versioned prefix are
// treated as legacy raw fallback files (e.g., from v5.2.2) and may be stale
// compared to a keychain entry.
const credentialVersionPrefix = "juggernaut-credential-v1\n"

// credentialVersionPrefixV2 marks a v2 fallback credential file whose token body
// is DPAPI-encrypted (Windows) and base64-encoded. This protects large keys
// (e.g. short-term Bedrock keys that exceed the Windows keychain size limit)
// that would otherwise sit in a plaintext file. v1 files remain readable and are
// transparently migrated to v2 on the next write.
const credentialVersionPrefixV2 = "juggernaut-credential-v2\n"

// Store wraps go-keyring with a fixed account name and configurable service name.
type Store struct {
	service string
}

// NewStore creates a Store using the given service name.
// Pass an empty string to use the default production service name.
func NewStore(service string) *Store {
	if service == "" {
		service = defaultService
	}
	return &Store{service: service}
}

// Default returns a Store using the production service name, or the value of
// JUGGERNAUT_KEYCHAIN_SERVICE if set (used by tests to isolate keychain state).
func Default() *Store {
	if svc := os.Getenv("JUGGERNAUT_KEYCHAIN_SERVICE"); svc != "" {
		return NewStore(svc)
	}
	return NewStore(defaultService)
}

// Set stores the token in the OS keychain.
func (s *Store) Set(token string) error {
	return keyring.Set(s.service, account, token)
}

// legacyAccount is the v3 account name for the Bedrock API key. When
// upgrading from v3, the saved credential may exist only under this name.
const legacyAccount = "api-key"

// Get retrieves the token. Returns "" (not an error) if not found.
// When the new account is not found, it falls back to the legacy v3 account
// for upgrade compatibility.
func (s *Store) Get() (string, error) {
	token, err := keyring.Get(s.service, account)
	if err == nil {
		return token, nil
	}
	if err != keyring.ErrNotFound {
		return "", err
	}
	// Fallback: try the legacy v3 account name for upgrade compatibility.
	token, err = keyring.Get(s.service, legacyAccount)
	if err == nil {
		return token, nil
	}
	if err != keyring.ErrNotFound {
		return "", err
	}
	return "", nil
}

// Delete removes the token. Also removes the legacy v3 account if present.
// Silent if not found.
func (s *Store) Delete() error {
	err := keyring.Delete(s.service, account)
	if err != nil && err != keyring.ErrNotFound {
		return err
	}
	// Also clean up the legacy v3 account name so uninstall fully clears
	// stored credentials.
	err = keyring.Delete(s.service, legacyAccount)
	if err != nil && err != keyring.ErrNotFound {
		return err
	}
	return nil
}

// IsTooBigForKeychain reports whether the token exceeds the Windows keychain
// limit. On non-Windows platforms this always returns false.
func IsTooBigForKeychain(token string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return len(token) > MaxWindowsKeychainSize
}

// credentialFilePath returns the path to the file-based fallback credential
// store under ~/.claude/.
func credentialFilePath(home string) string {
	return filepath.Join(home, ".claude", credentialFile)
}

// SetWithFallback stores the token in the OS keychain. If the token is too
// large for the keychain (Windows 2560-byte limit), or if the keychain is
// unavailable, it falls back to a file at ~/.claude/juggernaut-credential
// with owner-only permissions.
//
// When falling back to file storage, any existing keychain entry is cleared
// when possible. The fallback file is authoritative while it exists, so a
// keychain backend outage cannot prevent storing or reading the new token.
// When the keychain write succeeds, any stale fallback file is removed.
func (s *Store) SetWithFallback(token, home string) error {
	filePath := credentialFilePath(home)

	// Skip the keychain write entirely if the token is known to exceed the
	// Windows limit — avoids a wasted keychain call that will always fail.
	if IsTooBigForKeychain(token) {
		deleteErr := s.Delete()
		return writeCredentialFallback(home, filePath, token, deleteErr)
	}

	// Try the OS keychain first.
	err := s.Set(token)
	if err == nil {
		// Keychain write succeeded — remove any stale fallback file.
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing stale credential fallback: %w", err)
		}
		return nil
	}
	// Fall back to file storage on keychain failure (unavailable, etc.).
	// Clear any stale keychain entry when the backend permits it. A failed
	// delete must not prevent the advertised file fallback.
	deleteErr := s.Delete()
	return writeCredentialFallback(home, filePath, token, deleteErr)
}

// GetWithFallback retrieves the token using the following precedence:
//
//  1. Versioned fallback file: return its token immediately (authoritative).
//  2. Legacy unversioned fallback file + keychain has token: return the
//     keychain token and remove the stale legacy file.
//  3. Legacy unversioned fallback file + keychain empty/unavailable: return
//     the file token.
//  4. No file: return the keychain token (or empty string).
func (s *Store) GetWithFallback(home string) (string, error) {
	filePath := credentialFilePath(home)
	data, err := safepath.ReadFile(home, filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		// No file at all — use keychain. Surface real backend errors so callers
		// can distinguish "no credential stored" (empty, nil) from "keychain
		// backend is broken" (non-nil error). s.Get already maps ErrNotFound to
		// ("", nil), so any error here is a genuine backend failure.
		return s.Get()
	}

	// File exists — check if it's versioned (authoritative).
	if isVersionedCredential(data) {
		return extractTokenFromVersionedCredential(data), nil
	}

	// Legacy unversioned file: keychain may have a newer token.
	token, err := s.Get()
	if err != nil {
		// Keychain error (not ErrNotFound) — treat as unavailable, fall back to file.
		if !errors.Is(err, keyring.ErrNotFound) {
			return string(data), nil
		}
		// Keychain empty — no newer token available.
		return string(data), nil
	}
	if token != "" {
		// Keychain has a newer token — remove the stale legacy file.
		_ = os.Remove(filePath)
		return token, nil
	}

	// Keychain exists but is empty; fall back to file.
	return string(data), nil
}

// writeCredentialFile writes the credential to the given path with owner-only
// permissions, using a versioned envelope. If the file already exists, it is
// removed first so that a subsequent write creates a new inode with the
// correct mode rather than preserving a potentially lax mode on the existing
// file.
//
// When DPAPI is available (Windows), the token is encrypted to the current user
// account and stored as a v2 envelope, so large keys that bypass the OS keychain
// are still encrypted at rest. Otherwise it falls back to the v1 plaintext
// envelope (owner-only perms; the OS keychain already covers small keys).
func writeCredentialFile(base, filePath string, token string) error {
	payload, err := encodeCredential(token)
	if err != nil {
		return err
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing existing credential file: %w", err)
	}
	return safepath.WriteFile(base, filePath, payload)
}

// Indirection seams so tests can exercise the fail-closed path without a real
// DPAPI backend. Default to the build-tagged platform implementations.
var (
	dpapiAvailableFn = dpapiAvailable
	encryptFn        = encryptForUser
)

// encodeCredential builds the on-disk envelope for a token. On platforms where
// DPAPI is available (Windows) it MUST encrypt — it fails closed rather than
// downgrading to the v1 plaintext envelope, so a CryptProtectData failure can
// never reintroduce plaintext-at-rest storage of the bearer token. On platforms
// without DPAPI it uses the v1 plaintext envelope (owner-only perms; the OS
// keychain already covers small keys, and large-key file fallback is the
// documented behavior there).
func encodeCredential(token string) ([]byte, error) {
	if dpapiAvailableFn() {
		ciphertext, err := encryptFn([]byte(token))
		if err != nil {
			return nil, fmt.Errorf("encrypting credential for file fallback: %w", err)
		}
		enc := base64.StdEncoding.EncodeToString(ciphertext)
		return []byte(credentialVersionPrefixV2 + enc), nil
	}
	return []byte(credentialVersionPrefix + token), nil
}

// isVersionedCredential reports whether the file data starts with a recognised
// versioned-credential prefix (v1 plaintext or v2 encrypted).
func isVersionedCredential(data []byte) bool {
	return bytes.HasPrefix(data, []byte(credentialVersionPrefixV2)) ||
		bytes.HasPrefix(data, []byte(credentialVersionPrefix))
}

// extractTokenFromVersionedCredential strips the version prefix from the
// credential data and returns the raw token, decrypting a v2 (DPAPI) envelope.
// A v2 envelope that cannot be decoded/decrypted yields an empty token so the
// caller treats it as "no usable credential" rather than returning ciphertext.
func extractTokenFromVersionedCredential(data []byte) string {
	if body, ok := bytes.CutPrefix(data, []byte(credentialVersionPrefixV2)); ok {
		ciphertext, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			return ""
		}
		plaintext, err := decryptForUser(ciphertext)
		if err != nil {
			return ""
		}
		return string(plaintext)
	}
	return string(bytes.TrimPrefix(data, []byte(credentialVersionPrefix)))
}

// writeCredentialFallback writes the authoritative fallback token. Failure to
// clear the keychain is reported only if the fallback itself cannot be stored;
// an unavailable keychain backend is the reason this path exists.
func writeCredentialFallback(base, filePath, token string, deleteErr error) error {
	if err := writeCredentialFile(base, filePath, token); err != nil {
		if deleteErr != nil {
			return fmt.Errorf("writing credential fallback: %w (keychain cleanup also failed: %v)", err, deleteErr)
		}
		return fmt.Errorf("writing credential fallback: %w", err)
	}
	return nil
}

// DeleteWithFallback removes the token from both the OS keychain and the
// file-based fallback. Returns the last error if both fail.
func (s *Store) DeleteWithFallback(home string) error {
	err := s.Delete()
	if rmErr := os.Remove(credentialFilePath(home)); rmErr != nil && !os.IsNotExist(rmErr) {
		return rmErr
	}
	return err
}
