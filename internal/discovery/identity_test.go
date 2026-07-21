package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialScope_TracksProfileAndCredentialsWithoutSecrets(t *testing.T) {
	home := t.TempDir()
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(awsDir, "credentials")
	if err := os.WriteFile(credentials, []byte("[one]\naws_access_key_id=first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_DEFAULT_PROFILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_PROFILE", "one")

	first, err := CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_PROFILE", "two")
	second, err := CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("credential scope did not change with AWS_PROFILE")
	}

	t.Setenv("AWS_PROFILE", "one")
	if err := os.WriteFile(credentials, []byte("[one]\naws_access_key_id=second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("credential scope did not change when credentials file changed")
	}
	if len(third) != 64 {
		t.Fatalf("scope length = %d, want SHA-256 hex", len(third))
	}
}

func TestCredentialScope_TracksEnvironmentAccessKey(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "first")
	first, err := CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "second")
	second, err := CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("credential scope did not change with AWS_ACCESS_KEY_ID")
	}
}
