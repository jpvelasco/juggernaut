// Package keychain provides cross-platform credential storage via go-keyring.
package keychain

import (
	keyring "github.com/zalando/go-keyring"
)

const (
	defaultService = "juggernaut-bedrock"
	account        = "bedrock-credential"
	legacyAccount  = "api-key"
)

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

// Default returns a Store using the production service name.
func Default() *Store {
	return NewStore(defaultService)
}

// Set stores the token in the OS keychain.
func (s *Store) Set(token string) error {
	if err := keyring.Set(s.service, account, token); err != nil {
		return err
	}
	_ = keyring.Delete(s.service, legacyAccount)
	return nil
}

// Get retrieves the token. Returns "" (not an error) if not found.
func (s *Store) Get() (string, error) {
	token, err := keyring.Get(s.service, account)
	if err == nil {
		return token, nil
	}
	if err != keyring.ErrNotFound {
		return "", err
	}
	token, err = keyring.Get(s.service, legacyAccount)
	if err == keyring.ErrNotFound {
		return "", nil
	}
	return token, err
}

// Delete removes the token. Silent if not found.
func (s *Store) Delete() error {
	if err := keyring.Delete(s.service, account); err != nil && err != keyring.ErrNotFound {
		return err
	}
	err := keyring.Delete(s.service, legacyAccount)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}