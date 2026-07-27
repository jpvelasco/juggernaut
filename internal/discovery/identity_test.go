package discovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

type fakeSTSClient struct {
	out *sts.GetCallerIdentityOutput
	err error
}

func (f *fakeSTSClient) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return f.out, f.err
}

func TestCallerAccount(t *testing.T) {
	originalLoadConfig := loadDefaultAWSConfig
	originalMakeClient := makeSTSClient
	t.Cleanup(func() {
		loadDefaultAWSConfig = originalLoadConfig
		makeSTSClient = originalMakeClient
	})

	configErr := errors.New("no AWS configuration")
	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, configErr
	}
	if _, err := CallerAccount(context.Background(), "us-test-1"); !errors.Is(err, configErr) {
		t.Fatalf("configuration error = %v, want wrapped %v", err, configErr)
	}

	const region = "us-test-1"
	loadDefaultAWSConfig = func(_ context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
		options := config.LoadOptions{}
		for _, optFn := range optFns {
			if err := optFn(&options); err != nil {
				return aws.Config{}, err
			}
		}
		return aws.Config{Region: options.Region}, nil
	}
	client := &fakeSTSClient{}
	makeSTSClient = func(cfg aws.Config) stsClient {
		if cfg.Region != region {
			t.Fatalf("AWS region = %q, want %q", cfg.Region, region)
		}
		return client
	}

	identityErr := errors.New("identity denied")
	client.err = identityErr
	if _, err := CallerAccount(context.Background(), region); !errors.Is(err, identityErr) {
		t.Fatalf("identity error = %v, want wrapped %v", err, identityErr)
	}

	client.err = nil
	client.out = &sts.GetCallerIdentityOutput{}
	if _, err := CallerAccount(context.Background(), region); err == nil || !strings.Contains(err.Error(), "empty account ID") {
		t.Fatalf("empty account error = %v", err)
	}

	client.out = &sts.GetCallerIdentityOutput{Account: aws.String(" 123456789012 ")}
	account, err := CallerAccount(context.Background(), region)
	if err != nil {
		t.Fatalf("CallerAccount: %v", err)
	}
	if account != "123456789012" {
		t.Fatalf("account = %q, want trimmed account ID", account)
	}
}

func TestCredentialScope_TracksProfileAndCredentialsWithoutSecrets(t *testing.T) {
	home := testutil.NewTestHome(t)
	awsDir := filepath.Join(home, ".aws")
	if err := safepath.MkdirAll(awsDir); err != nil {
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
	home := testutil.NewTestHome(t)
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

func TestCredentialScope_FingerprintsUnreadableCredentialSelectors(t *testing.T) {
	home := testutil.NewTestHome(t)
	for _, name := range []string{"AWS_ACCESS_KEY_ID", "AWS_PROFILE", "AWS_DEFAULT_PROFILE"} {
		t.Setenv(name, "")
	}
	t.Setenv("AWS_CONFIG_FILE", home) // Reading a directory exercises the non-fatal read-error path.
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, "missing"))
	scope, err := CredentialScope(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope) != 64 {
		t.Fatalf("scope length = %d, want SHA-256 hex", len(scope))
	}
	if got := firstNonEmpty(" ", ""); got != "" {
		t.Fatalf("firstNonEmpty returned %q", got)
	}
}
