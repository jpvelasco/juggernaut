//go:build !windows

package keychain

import "errors"

// errDPAPIUnsupported is returned by the non-Windows crypter stubs. DPAPI is a
// Windows-only API; on other platforms large keys that exceed a keychain limit
// fall back to the (owner-only) plaintext file, and small keys use the OS
// keychain, which already encrypts at rest.
var errDPAPIUnsupported = errors.New("DPAPI encryption is only available on Windows")

func encryptForUser(_ []byte) ([]byte, error) { return nil, errDPAPIUnsupported }

func decryptForUser(_ []byte) ([]byte, error) { return nil, errDPAPIUnsupported }

// dpapiAvailable reports whether DPAPI-backed encryption is usable (never on
// non-Windows platforms).
func dpapiAvailable() bool { return false }
