// Package keychain provides cross-platform credential storage via go-keyring.
package keychain

import (
	"os"

	keyring "github.com/zalando/go-keyring"
)

const defaultService = "juggernaut-bedrock"

const account = "bedrock-credential"

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
