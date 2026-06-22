//go:build windows

package keychain

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapiEntropyLabel must match the v3 PowerShell helper's entropy so v5 can
// decrypt blobs written by v3.
const dpapiEntropyLabel = "juggernaut-bedrock"

// cryptProtectUIForbidden suppresses any DPAPI UI prompt (CRYPTPROTECT_UI_FORBIDDEN).
const cryptProtectUIForbidden = 0x1

var (
	crypt32              = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect   = crypt32.NewProc("CryptUnprotectData")
)

// dataBlob mirrors the Win32 DATA_BLOB structure.
type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(b)), pbData: &b[0]}
}

func (b dataBlob) bytes() []byte {
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

// DPAPIBackend stores the bearer token as a DPAPI-protected file, matching the
// v3 PowerShell helper's path and format so v3 tokens migrate for free. The
// ciphertext only decrypts under the same Windows user account.
type DPAPIBackend struct {
	home string
}

func newDPAPIBackend(home string) (Backend, error) {
	return &DPAPIBackend{home: home}, nil
}

func (b *DPAPIBackend) path() string {
	if h := os.Getenv("JUGGERNAUT_HOME"); h != "" {
		return filepath.Join(h, ".juggernaut", "bearer-token.dpapi.bin")
	}
	return filepath.Join(b.home, ".juggernaut", "bearer-token.dpapi.bin")
}

// Set DPAPI-protects the token and writes it with owner-only permissions.
func (b *DPAPIBackend) Set(token string) error {
	enc, err := protect([]byte(token), []byte(dpapiEntropyLabel))
	if err != nil {
		return fmt.Errorf("dpapi protect: %w", err)
	}
	path := b.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, enc, 0o600)
}

// Get reads and decrypts the token, returning "" if the file is absent.
func (b *DPAPIBackend) Get() (string, error) {
	enc, err := os.ReadFile(b.path())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	plain, err := unprotect(enc, []byte(dpapiEntropyLabel))
	if err != nil {
		return "", fmt.Errorf("dpapi unprotect (file may be corrupt or from a different user): %w", err)
	}
	return string(plain), nil
}

// Delete removes the DPAPI file. Silent if absent.
func (b *DPAPIBackend) Delete() error {
	if err := os.Remove(b.path()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func protect(data, entropy []byte) ([]byte, error) {
	in := newBlob(data)
	ent := newBlob(entropy)
	var out dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		uintptr(unsafe.Pointer(&ent)),
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData))) //nolint:errcheck
	return out.bytes(), nil
}

func unprotect(data, entropy []byte) ([]byte, error) {
	in := newBlob(data)
	ent := newBlob(entropy)
	var out dataBlob
	r, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		uintptr(unsafe.Pointer(&ent)),
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData))) //nolint:errcheck
	return out.bytes(), nil
}
