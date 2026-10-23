package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// TestValidateProfile_EmptyIsNoOp: empty profile means "use the ambient
// default chain" — valid by definition, nothing to check, and crucially no AWS
// config load should happen.
func TestValidateProfile_EmptyIsNoOp(t *testing.T) {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(ctx context.Context, _ ...func(*config.LoadOptions) error) (aws.Config, error) {
		t.Error("no profile was requested; no AWS config load should happen")
		return aws.Config{}, nil
	}
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	if err := ValidateProfile(context.Background(), "us-west-2", ""); err != nil {
		t.Errorf("ValidateProfile(\"\") = %v, want nil", err)
	}
}

// TestValidateProfile_ProfileNotFound: an explicitly requested profile that
// cannot be loaded from the shared config files must fail with an error naming
// the profile — apply must reject it before writing any config.
func TestValidateProfile_ProfileNotFound(t *testing.T) {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("profile 'missing' not found in the shared config files")
	}
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	err := ValidateProfile(context.Background(), "us-west-2", "missing")
	if err == nil {
		t.Fatal("ValidateProfile(missing) should fail when the profile does not exist")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %v, want a message naming the profile", err)
	}
}

// TestValidateProfile_NoCredentials: a profile that loads but resolves no
// credential provider must fail — have-config-but-no-creds otherwise fails far
// past the point of the typo it came from.
func TestValidateProfile_NoCredentials(t *testing.T) {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	err := ValidateProfile(context.Background(), "us-west-2", "no-creds")
	if err == nil {
		t.Fatal("ValidateProfile should fail when no credentials provider resolves")
	}
	if !strings.Contains(err.Error(), "no credentials provider") {
		t.Errorf("error = %v, want a no-credentials message", err)
	}
}

// TestValidateProfile_OK: the profile loads and carries a credentials provider.
func TestValidateProfile_OK(t *testing.T) {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Credentials: stubCredentialsProvider{}}, nil
	}
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	if err := ValidateProfile(context.Background(), "us-west-2", "developer"); err != nil {
		t.Errorf("ValidateProfile(developer) = %v, want nil", err)
	}
}

// TestMintBedrockToken_PassesProfile: with a named profile the mint loads the
// AWS config with that profile selected, so the short-term Bedrock key comes
// from the profile's credentials — not from the ambient default chain.
func TestMintBedrockToken_PassesProfile(t *testing.T) {
	var gotProfile string
	restore := stubProfileLoad(t, "developer", &gotProfile)
	defer restore()

	if _, err := MintBedrockToken(context.Background(), "us-west-2", "developer"); err != nil {
		t.Fatalf("MintBedrockToken: %v", err)
	}
	if gotProfile != "developer" {
		t.Errorf("profile passed to the AWS config load = %q, want developer", gotProfile)
	}
}

// TestMintBedrockToken_NoProfileKeepsAmbientChain: an empty profile must not
// force the shared config profile (it would discard AWS_PROFILE/defaults).
func TestMintBedrockToken_NoProfileKeepsAmbientChain(t *testing.T) {
	var gotProfile string
	restore := stubProfileLoad(t, "", &gotProfile)
	defer restore()

	if _, err := MintBedrockToken(context.Background(), "us-west-2", ""); err != nil {
		t.Fatalf("MintBedrockToken: %v", err)
	}
	if gotProfile != "" {
		t.Errorf("profile passed to the AWS config load = %q, want empty (ambient chain)", gotProfile)
	}
}

// stubProfileLoad replaces loadDefaultAWSConfig with a stub that applies the
// options Juggernaut built (so the profile is observable via cfg) and returns
// static expiring credentials. Storing the applied profile in *out.
func stubProfileLoad(t *testing.T, profile string, out *string) func() {
	orig := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		var lo config.LoadOptions
		for _, opt := range opts {
			if err := opt(&lo); err != nil {
				return aws.Config{}, err
			}
		}
		*out = lo.SharedConfigProfile
		var cfg aws.Config
		cfg.Credentials = stubCredentialsProvider{creds: aws.Credentials{
			AccessKeyID:     "AKIDEXAMPLE",
			SecretAccessKey: "secret",
			SessionToken:    "session-token",
			CanExpire:       true,
			Expires:         time.Now().Add(2 * time.Hour),
		}}
		return cfg, nil
	}
	return func() { loadDefaultAWSConfig = orig }
}
