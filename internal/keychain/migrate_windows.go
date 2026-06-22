//go:build windows

package keychain

import (
	"unicode/utf16"

	"github.com/danieljoos/wincred"
)

// legacySources lists the v3 credential locations to probe on Windows, in v3's
// own read order: DPAPI file, then Credential Manager, then profile file.
//
// The Credential Manager entry is the case the v5 keychain backend cannot read
// on its own: v3 wrote a bare target name (no ":account" suffix) with a UTF-16
// blob, whereas go-keyring expects "<service>:<account>" with a UTF-8 blob.
func legacySources(home string) []legacySource {
	dpapi, _ := newDPAPIBackend(home)
	profile := NewProfileBackend(home)
	sources := []legacySource{
		{name: "dpapi", read: dpapi.Get},
		{name: "credential-manager", read: readLegacyCredManToken},
		{name: "profile", read: profile.Get},
	}
	return sources
}

// readLegacyCredManToken reads the v3 bare-target Credential Manager entry and
// decodes its UTF-16 blob. Returns "" when no such entry exists.
func readLegacyCredManToken() (string, error) {
	cred, err := wincred.GetGenericCredential(legacyServiceName())
	if err != nil {
		// Not found (or unreadable) — treat as absent so migration falls through.
		return "", nil
	}
	return decodeUTF16(cred.CredentialBlob), nil
}

// decodeUTF16 decodes a little-endian UTF-16 byte blob (as written by the v3
// PowerShell CredWrite helper) into a Go string. An odd trailing byte is dropped.
func decodeUTF16(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u16 = append(u16, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(u16))
}
