//go:build !windows

package keychain

// legacySources lists the v3 credential locations to probe on non-Windows
// platforms. v3 stored to the OS keyring under the same go-keyring target the
// v5 keychain backend already reads (via the legacy-account fallback), so the
// only extra location to cover here is the plaintext profile token file.
func legacySources(home string) []legacySource {
	profile := NewProfileBackend(home)
	return []legacySource{
		{name: "profile", read: profile.Get, remove: profile.Delete},
	}
}
