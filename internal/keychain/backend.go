package keychain

import (
	"errors"
	"fmt"
)

// ErrCredentialTooBig is returned when a credential exceeds the OS credential
// store's blob size limit (Windows Credential Manager / go-keyring cap is
// ~2560 bytes). Callers should suggest a file-based backend (dpapi or profile).
var ErrCredentialTooBig = errors.New("credential too large for OS credential store")

// Backend is a credential storage backend. The keychain (OS keyring), profile
// (plaintext file), and dpapi (Windows DPAPI file) backends all satisfy it.
type Backend interface {
	Set(token string) error
	Get() (string, error)
	Delete() error
}

// allStorageModes lists the canonical storage mode names.
var allStorageModes = []string{"keychain", "profile", "dpapi"}

// normalizeMode maps the empty default to "keychain".
func normalizeMode(mode string) string {
	if mode == "" {
		return "keychain"
	}
	return mode
}

// ClearOthers deletes any stored credential from every backend except the
// selected one. Use after storing into the selected backend so switching
// storage modes does not orphan a credential in the previously-used backend.
//
// It is best-effort: a backend that can't be resolved (e.g. dpapi off Windows)
// or whose Delete fails because the backend is simply unreachable (e.g. no
// Secret Service on headless Linux) is skipped rather than treated as an error —
// an unreachable backend holds no credential this process could have written.
// Returns the joined errors only when a reachable backend fails to delete.
func ClearOthers(selected, home string) error {
	keep := normalizeMode(selected)
	var errs []error
	for _, mode := range allStorageModes {
		if mode == keep {
			continue
		}
		backend, err := Resolve(mode, home)
		if err != nil {
			// Backend not available on this platform — nothing to clear.
			continue
		}
		// Probe reachability via Get: an unreachable backend (no keyring daemon)
		// holds nothing for us to clear, so skip it instead of erroring.
		if _, gerr := backend.Get(); gerr != nil {
			continue
		}
		if err := backend.Delete(); err != nil {
			errs = append(errs, fmt.Errorf("clearing %s credential: %w", mode, err))
		}
	}
	return errors.Join(errs...)
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
