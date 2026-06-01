package keychain

import (
	keyring "github.com/zalando/go-keyring"
)

const (
	defaultService = "juggernaut-bedrock"
	account        = "api-key"
)

type Store struct {
	service string
}

func NewStore(service string) *Store {
	if service == "" {
		service = defaultService
	}
	return &Store{service: service}
}

func Default() *Store {
	return NewStore(defaultService)
}

func (s *Store) Set(token string) error {
	return keyring.Set(s.service, account, token)
}

func (s *Store) Get() (string, error) {
	token, err := keyring.Get(s.service, account)
	if err == keyring.ErrNotFound {
		return "", nil
	}
	return token, err
}

func (s *Store) Delete() error {
	err := keyring.Delete(s.service, account)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}
