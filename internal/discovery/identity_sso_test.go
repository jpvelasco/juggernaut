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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

	ssoCacheFile(t, home, "account-a.json", `{"accountId":"111122223333"}`)

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
	ssoCacheFile(t, home, "account-b.json", `{"accountId":"444455556666"}`)
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
