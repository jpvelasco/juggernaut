package keychain

import (
	"errors"
	"testing"
)

type stubBackend struct {
	val    string
	getErr error
	setErr error
}

func (s *stubBackend) Set(v string) error   { s.val = v; return s.setErr }
func (s *stubBackend) Get() (string, error) { return s.val, s.getErr }
func (s *stubBackend) Delete() error        { s.val = ""; return nil }

// migrateFromSources is the testable core of MigrateInto: it takes an explicit
// list of legacy sources so the cleanup-error path can be exercised without a
// real OS keyring or filesystem.
func TestMigrateFromSources_SurfacesCleanupErrorButStillMigrates(t *testing.T) {
	target := &stubBackend{}
	removeErr := errors.New("permission denied removing source")
	sources := []legacySource{
		{
			name:   "profile",
			read:   func() (string, error) { return "v3-secret", nil },
			remove: func() error { return removeErr },
		},
	}

	src, val, cleanupErr, err := migrateFromSources(target, sources)
	if err != nil {
		t.Fatalf("unexpected migration error: %v", err)
	}
	if src != "profile" || val != "v3-secret" {
		t.Errorf("expected (profile, v3-secret), got (%q, %q)", src, val)
	}
	if target.val != "v3-secret" {
		t.Errorf("expected target populated, got %q", target.val)
	}
	if cleanupErr == nil || !errors.Is(cleanupErr, removeErr) {
		t.Errorf("expected cleanup error surfaced, got %v", cleanupErr)
	}
}

func TestMigrateFromSources_NoCleanupErrorOnSuccess(t *testing.T) {
	target := &stubBackend{}
	sources := []legacySource{
		{
			name:   "profile",
			read:   func() (string, error) { return "v3-secret", nil },
			remove: func() error { return nil },
		},
	}
	_, _, cleanupErr, err := migrateFromSources(target, sources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanupErr != nil {
		t.Errorf("expected no cleanup error, got %v", cleanupErr)
	}
}
