//go:build !windows

package keychain

import "fmt"

// newDPAPIBackend errors on non-Windows platforms; DPAPI is Windows-only.
func newDPAPIBackend(string) (Backend, error) {
	return nil, fmt.Errorf("dpapi storage is only available on Windows")
}
