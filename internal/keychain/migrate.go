package keychain

import "fmt"

// legacySource is one v3-era credential storage location probed during migration.
type legacySource struct {
	name   string
	read   func() (string, error)
	remove func() error
}

// MigrateInto imports a v3-era Bedrock API key into target when target is empty.
// It probes the v3 storage locations (in v3's own read order) and, on the first
// non-empty hit, writes the value into target. Returns the name of the source it
// migrated from and the imported value, or ("", "") if target already had a
// value or no v3 credential exists.
//
// cleanupErr is non-nil when the credential was imported successfully but the v3
// source could not be removed afterward (e.g. a locked file). It is informational
// only — migration still succeeded — but callers should surface it because a
// lingering plaintext profile token is a minor security concern. err is reserved
// for failures that abort migration (read/write/probe faults).
//
// Targets that share v3's path/format (profile, dpapi) read the v3 value directly
// via their own Get, so this becomes a no-op for them — the case it genuinely
// repairs is a v3 Windows Credential Manager entry (bare target, UTF-16) that the
// v5 keychain backend cannot otherwise see.
//
// A real error from target.Get (e.g. a locked keychain or a corrupt DPAPI blob)
// is surfaced immediately rather than masked by overwriting the target — the
// backends return ("", nil) for "not found" and an error only for genuine faults.
func MigrateInto(target Backend, home string) (source, value string, cleanupErr, err error) {
	existing, gerr := target.Get()
	if gerr != nil {
		return "", "", nil, fmt.Errorf("checking existing credential: %w", gerr)
	}
	if existing != "" {
		return "", "", nil, nil
	}
	return migrateFromSources(target, legacySources(home))
}

// migrateFromSources is the testable core of MigrateInto: it assumes target is
// already known to be empty and probes the given sources in order.
func migrateFromSources(target Backend, sources []legacySource) (source, value string, cleanupErr, err error) {
	for _, src := range sources {
		val, rerr := src.read()
		if rerr != nil {
			return "", "", nil, fmt.Errorf("reading legacy %s credential: %w", src.name, rerr)
		}
		if val == "" {
			continue
		}
		if serr := target.Set(val); serr != nil {
			return "", "", nil, fmt.Errorf("importing legacy %s credential: %w", src.name, serr)
		}
		// Remove the v3 source so a stale credential isn't read later from a
		// different backend, and plaintext tokens don't linger on disk. A failure
		// here doesn't undo the successful import, so report it via cleanupErr
		// rather than failing the migration.
		if src.remove != nil {
			if derr := src.remove(); derr != nil {
				cleanupErr = fmt.Errorf("removing legacy %s credential after import: %w", src.name, derr)
			}
		}
		return src.name, val, cleanupErr, nil
	}
	return "", "", nil, nil
}
