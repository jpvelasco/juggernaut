package keychain

import (
	"fmt"
	"os"
)

// legacySource is one v3-era credential storage location probed during migration.
type legacySource struct {
	name   string
	read   func() (string, error)
	remove func() error
}

// MigrateInto imports a v3-era Bedrock API key into target when target is empty.
// It probes the v3 storage locations (in v3's own read order) and, on the first
// non-empty hit, writes the value into target. Returns the name of the source it
// migrated from and the imported value, or ("", "", nil) if target already had a
// value or no v3 credential exists.
//
// Targets that share v3's path/format (profile, dpapi) read the v3 value directly
// via their own Get, so this becomes a no-op for them — the case it genuinely
// repairs is a v3 Windows Credential Manager entry (bare target, UTF-16) that the
// v5 keychain backend cannot otherwise see.
//
// A real error from target.Get (e.g. a locked keychain or a corrupt DPAPI blob)
// is surfaced immediately rather than masked by overwriting the target — the
// backends return ("", nil) for "not found" and an error only for genuine faults.
func MigrateInto(target Backend, home string) (source, value string, err error) {
	existing, gerr := target.Get()
	if gerr != nil {
		return "", "", fmt.Errorf("checking existing credential: %w", gerr)
	}
	if existing != "" {
		return "", "", nil
	}
	for _, src := range legacySources(home) {
		val, rerr := src.read()
		if rerr != nil {
			return "", "", fmt.Errorf("reading legacy %s credential: %w", src.name, rerr)
		}
		if val != "" {
			if serr := target.Set(val); serr != nil {
				return "", "", fmt.Errorf("importing legacy %s credential: %w", src.name, serr)
			}
			// Remove the v3 source so a stale credential isn't read later from a
			// different backend, and plaintext tokens don't linger on disk.
			if src.remove != nil {
				_ = src.remove()
			}
			return src.name, val, nil
		}
	}
	return "", "", nil
}

// legacyServiceName is the v3 keychain service name (also the bare Windows
// Credential Manager target name v3 wrote to). Honors JUGGERNAUT_KEYCHAIN_SERVICE
// for test isolation, matching Default().
func legacyServiceName() string {
	if svc := os.Getenv("JUGGERNAUT_KEYCHAIN_SERVICE"); svc != "" {
		return svc
	}
	return defaultService
}
