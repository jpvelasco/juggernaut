package keychain

import "fmt"

// Backend is a credential storage backend. The keychain (OS keyring), profile
// (plaintext file), and dpapi (Windows DPAPI file) backends all satisfy it.
type Backend interface {
	Set(token string) error
	Get() (string, error)
	Delete() error
}

// Resolve returns the credential backend for the given storage mode. An empty
// mode or "keychain" yields the OS keyring; "profile" yields a plaintext file
// under the user's config dir; "dpapi" yields the Windows DPAPI file backend
// (and errors on non-Windows platforms). home roots the file-based backends.
func Resolve(mode, home string) (Backend, error) {
	switch mode {
	case "", "keychain":
		return Default(), nil
	case "profile":
		return NewProfileBackend(home), nil
	case "dpapi":
		return newDPAPIBackend(home)
	default:
		return nil, fmt.Errorf("unknown credential storage %q — must be one of: keychain, profile, dpapi", mode)
	}
}
