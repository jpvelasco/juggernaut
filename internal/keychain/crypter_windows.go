//go:build windows

package keychain

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapiDescription is the human-readable label stored with the DPAPI blob.
const dpapiDescription = "juggernaut-bedrock-credential"

// encryptForUser encrypts plaintext with Windows DPAPI scoped to the current
// user account (CRYPTPROTECT_LOCAL_MACHINE is intentionally NOT set, so only
// this user can decrypt). It returns the opaque ciphertext.
func encryptForUser(plaintext []byte) ([]byte, error) {
	in := newBlob(plaintext)
	desc, err := windows.UTF16PtrFromString(dpapiDescription)
	if err != nil {
		return nil, fmt.Errorf("dpapi description: %w", err)
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, desc, nil, 0, nil, 0, &out); err != nil {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer freeBlob(&out)
	return copyBlob(&out), nil
}

// decryptForUser reverses encryptForUser.
func decryptForUser(ciphertext []byte) ([]byte, error) {
	in := newBlob(ciphertext)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer freeBlob(&out)
	return copyBlob(&out), nil
}

// dpapiAvailable reports whether DPAPI-backed encryption is usable. It is always
// true on Windows.
func dpapiAvailable() bool { return true }

// newBlob wraps a byte slice as a DATA_BLOB without copying. The caller must
// keep the backing slice alive for the duration of the syscall.
func newBlob(b []byte) windows.DataBlob {
	if len(b) == 0 {
		return windows.DataBlob{Size: 0, Data: nil}
	}
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// copyBlob copies the bytes referenced by a DATA_BLOB into a Go-owned slice.
func copyBlob(b *windows.DataBlob) []byte {
	if b.Size == 0 || b.Data == nil {
		return []byte{}
	}
	out := make([]byte, b.Size)
	copy(out, unsafe.Slice(b.Data, b.Size))
	return out
}

// freeBlob releases the buffer DPAPI allocated for an output DATA_BLOB.
func freeBlob(b *windows.DataBlob) {
	if b.Data != nil {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	}
}
