// Package keychain provides cross-platform credential storage via go-keyring,
// with a file-based fallback for tokens that exceed the Windows Credential
// Manager 2560-byte limit.
package keychain

import (
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
// so that GetWithFallback does not return a stale token.
// When the keychain write succeeds, any stale fallback file is removed.
func (s *Store) SetWithFallback(token, home string) error {
	filePath := credentialFilePath(home)

	// Skip the keychain write entirely if the token is known to exceed the
	// Windows limit — avoids a wasted keychain call that will always fail.
	if IsTooBigForKeychain(token) {
		if err := s.Delete(); err != nil {
			return fmt.Errorf("clearing keychain before file fallback: %w", err)
		}
		return writeCredentialFile(home, filePath, token)
	}

	// Try the OS keychain first.
	err := s.Set(token)
	if err == nil {
		// Keychain write succeeded — remove any stale fallback file.
		_ = os.Remove(filePath)
		return nil
	}
	// Fall back to file storage on keychain failure (unavailable, etc.).
	// Clear any stale keychain entry before falling back.
	if err := s.Delete(); err != nil {
		return fmt.Errorf("clearing keychain before file fallback: %w", err)
	}
	return writeCredentialFile(home, filePath, token)
}

// GetWithFallback retrieves the token from the OS keychain. If the keychain
// has no entry, it falls back to the file at ~/.claude/juggernaut-credential.
func (s *Store) GetWithFallback(home string) (string, error) {
	token, err := s.Get()
	if err == nil && token != "" {
		return token, nil
	}
	// Fall back to file storage.
	data, err := safepath.ReadFile(home, credentialFilePath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// writeCredentialFile writes the credential to the given path with owner-only
// permissions. If the file already exists, it is removed first so that a
// subsequent write creates a new inode with the correct mode rather than
// preserving a potentially lax mode on the existing file.
func writeCredentialFile(base, filePath string, token string) error {
	_ = os.Remove(filePath)
	return safepath.WriteFile(base, filePath, []byte(token))
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
