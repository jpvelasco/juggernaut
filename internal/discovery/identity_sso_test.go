package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testhome"
)

// clearSSORelevantEnv isolates the credential-selection env vars so the
// shared-config branch of CredentialScope is exercised.
func clearSSORelevantEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_PROFILE", "AWS_DEFAULT_PROFILE",
		"AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI",
	} {
		t.Setenv(name, "")
	}
}

func ssoCacheFile(t *testing.T, home, name, body string) {
	t.Helper()
	dir := filepath.Join(home, ".aws", "sso", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- owner-only temp fixture dir
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Given an SSO user who logs in and selects account A (writing the aws-cli
// token cache), When the credential fingerprint is computed before and after,
// Then the cache write must change the scope — otherwise apply/doctor would
// serve another account's cached model inventory after an account switch.
func TestCredentialScope_IncludesSSOCacheContents(t *testing.T) {
	home := testhome.NewTestHome(t)
	clearSSORelevantEnv(t)

	beforeLogin, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope without sso cache: %v", err)
	}

	ssoCacheFile(t, home, "account-a.json", `{"accountId":"A1111222233"}`)

	afterLoginA, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope with account A: %v", err)
	}
	if afterLoginA == beforeLogin {
		t.Fatal("scope unchanged after first SSO login wrote its token cache")
	}

	// Given the user switches accounts in the SSO portal,
	// When aws-cli refreshes the cache for account B,
	// Then the scope must change so stale account-A caches are not served.
	ssoCacheFile(t, home, "account-b.json", `{"accountId":"B4444555566"}`)
	afterSwitchB, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope with account B: %v", err)
	}
	if afterSwitchB == afterLoginA {
		t.Fatal("scope unchanged after switching SSO accounts")
	}

	// Given nothing about the selection changes,
	// When the scope is recomputed,
	// Then it must stay stable.
	stable, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("stable rescope: %v", err)
	}
	if stable != afterSwitchB {
		t.Fatal("scope drifted without any input change")
	}
}

// Given an SSO cache entry that is a symlink escaping the cache directory,
// When the outside file's contents change,
// Then the credential scope must not move — cache reads must stay contained
// in the SSO cache root, so hostile or stray links cannot steer the
// fingerprint to arbitrary files.
func TestCredentialScope_SSOCacheSymlinkEscapeIsContained(t *testing.T) {
	home := testhome.NewTestHome(t)
	clearSSORelevantEnv(t)
	ssoCacheFile(t, home, "inside.json", `{"accountId":"A1111222233"}`)

	outside := filepath.Join(home, "outside-secret.txt")
	if err := os.WriteFile(outside, []byte("AAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".aws", "sso", "cache", "leak.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	beforeLeakChange, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope with escaped symlink entry: %v", err)
	}
	if err := os.WriteFile(outside, []byte("BBB"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterLeakChange, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("rescope after outside change: %v", err)
	}
	if beforeLeakChange != afterLeakChange {
		t.Fatal("outside content leaked into the fingerprint via a symlinked cache entry")
	}
}

// Given a cache entry that cannot be read (broken symlink),
// When the fingerprint is computed,
// Then the unreadable entry folds in deterministically instead of failing.
func TestCredentialScope_SSOCacheUnreadableEntryFoldsIn(t *testing.T) {
	home := testhome.NewTestHome(t)
	clearSSORelevantEnv(t)
	ssoCacheFile(t, home, "good.json", `{"accountId":"A1111222233"}`)

	broken := filepath.Join(home, ".aws", "sso", "cache", "broken.json")
	if err := os.Symlink(filepath.Join(home, ".aws", "sso", "cache", "missing"), broken); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	first, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope with unreadable cache entry: %v", err)
	}
	second, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("stable rescope with unreadable entry: %v", err)
	}
	if first != second {
		t.Fatal("scope drifted while the unreadable entry stayed constant")
	}

	ssoCacheFile(t, home, "another.json", `{}`)
	third, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("rescope after cache change: %v", err)
	}
	if third != first {
		t.Fatal("identity-less entry (no accountId/startUrl/region) must not alter the scope")
	}

	ssoCacheFile(t, home, "account-c.json", `{"accountId":"C1111222233"}`)
	fourth, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("rescope after identity-bearing entry: %v", err)
	}
	if fourth == first {
		t.Fatal("identity-bearing entry must alter the scope")
	}
}

// Given an SSO session whose aws-cli token rotates (the portal-keyed token
// model: accessToken/expiresAt refresh on every login, and client
// registrations churn), When the credential fingerprint is recomputed, Then
// the scope must stay stable — a rotating fingerprint orphans the catalog
// binding for the same credential selection after each login cycle, silently
// degrading apply to an empty catalog.
func TestCredentialScope_StableAcrossSSOTokenRotation(t *testing.T) {
	home := testhome.NewTestHome(t)
	clearSSORelevantEnv(t)

	ssoCacheFile(t, home, "3dacbce5.json", `{
		"startUrl": "https://example.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "token-AAA",
		"expiresAt": "2026-10-24T18:26:23.141Z",
		"clientId": "client-1",
		"clientSecret": "secret-1"
	}`)
	before, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope before rotation: %v", err)
	}

	// Every login cycle: fresh tokens, fresh expiry, client re-registration.
	ssoCacheFile(t, home, "3dacbce5.json", `{
		"startUrl": "https://example.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "token-BBB-new-session",
		"expiresAt": "2026-10-25T09:00:00.000Z",
		"clientId": "client-2-registered",
		"clientSecret": "secret-2"
	}`)
	ssoCacheFile(t, home, "0b46fd61.json", `{
		"clientId": "client-2-registered",
		"clientSecret": "secret-2",
		"expiresAt": "2027-01-21T03:12:21Z",
		"scopes": ["sso:account:access"],
		"grantTypes": ["authorization_code", "refresh_token"]
	}`)

	afterRotation, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope after rotation: %v", err)
	}
	if afterRotation != before {
		t.Fatal("scope changed across SSO token rotation — the catalog binding would be orphaned by routine logins")
	}

	// A portal switch is a real identity change and must re-scope.
	ssoCacheFile(t, home, "3dacbce5.json", `{
		"startUrl": "https://other-tenant.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "token-CCC",
		"expiresAt": "2026-10-25T09:00:00.000Z"
	}`)
	afterPortalSwitch, err := CredentialScope(home)
	if err != nil {
		t.Fatalf("scope after portal switch: %v", err)
	}
	if afterPortalSwitch == before {
		t.Fatal("scope unchanged after switching SSO portals")
	}
}
