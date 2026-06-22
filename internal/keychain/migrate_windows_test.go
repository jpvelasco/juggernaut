//go:build windows

package keychain_test

import (
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/danieljoos/wincred"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
)

// writeLegacyCredMan writes a v3-style bare-target Credential Manager entry with
// a UTF-16LE blob, mirroring the v3 PowerShell CredWrite helper.
func writeLegacyCredMan(t *testing.T, target, value string) {
	t.Helper()
	cred := wincred.NewGenericCredential(target)
	u16 := utf16.Encode([]rune(value))
	blob := make([]byte, len(u16)*2)
	for i, c := range u16 {
		blob[i*2] = byte(c)
		blob[i*2+1] = byte(c >> 8)
	}
	cred.CredentialBlob = blob
	if err := cred.Write(); err != nil {
		t.Fatalf("writing legacy credential: %v", err)
	}
	t.Cleanup(func() { _ = cred.Delete() })
}

func TestMigrateInto_FromLegacyCredManUTF16(t *testing.T) {
	// Isolate under a test service name so we never touch the real credential.
	const svc = "jug-migrate-test"
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", svc)

	home := t.TempDir()
	// Ensure no profile/dpapi file shadows the Credential Manager source.
	t.Setenv("JUGGERNAUT_PROFILE_TOKEN_PATH", filepath.Join(home, "no-profile"))
	t.Setenv("JUGGERNAUT_HOME", filepath.Join(home, "no-juggernaut-home"))

	writeLegacyCredMan(t, svc, "v3-credman-secret")

	target := &fakeBackend{}
	src, val, err := keychain.MigrateInto(target, home)
	if err != nil {
		t.Fatalf("MigrateInto error: %v", err)
	}
	if src != "credential-manager" {
		t.Errorf("expected migration from credential-manager, got %q", src)
	}
	if val != "v3-credman-secret" {
		t.Errorf("expected returned value v3-credman-secret, got %q", val)
	}
	if target.val != "v3-credman-secret" {
		t.Errorf("expected imported value v3-credman-secret, got %q", target.val)
	}
}
