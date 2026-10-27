package discovery

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

type stubCredentialsProvider struct {
	creds aws.Credentials
	err   error
}

func (p stubCredentialsProvider) Retrieve(context.Context) (aws.Credentials, error) {
	return p.creds, p.err
}

// stubTokenAWSConfig replaces the AWS config loader for one test. Tests in this
// package do not run in parallel, so the package-level seam is safe.
func stubTokenAWSConfig(t *testing.T, cfg aws.Config, err error) {
	t.Helper()
	original := loadDefaultAWSConfig
	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return cfg, err
	}
	t.Cleanup(func() { loadDefaultAWSConfig = original })
}

func decodeMintedToken(t *testing.T, token string) *url.URL {
	t.Helper()
	body, ok := strings.CutPrefix(token, "bedrock-api-key-")
	if !ok {
		t.Fatalf("token missing bedrock-api-key- prefix: %q", token)
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("token body is not base64: %v", err)
	}
	u, err := url.Parse("https://" + string(raw))
	if err != nil {
		t.Fatalf("decoded token is not a URL: %v", err)
	}
	return u
}

// TestMintBedrockToken_StaticCredentials pins the AWS token-generator format:
// a presigned CallWithBearerToken URL, base64-encoded, with Version=1 appended
// after signing. Static credentials have no session expiry, so the key uses the
// 12-hour cap.
func TestMintBedrockToken_StaticCredentials(t *testing.T) {
	stubTokenAWSConfig(t, aws.Config{
		Credentials: stubCredentialsProvider{creds: aws.Credentials{
			AccessKeyID:     "AKIDEXAMPLE",
			SecretAccessKey: "secret",
			SessionToken:    "session-token",
		}},
	}, nil)

	token, err := MintBedrockToken(context.Background(), "us-west-2", "")
	if err != nil {
		t.Fatalf("MintBedrockToken: %v", err)
	}
	u := decodeMintedToken(t, token)
	if u.Host != bedrockTokenHost || u.Path != "/" {
		t.Errorf("token host/path = %q/%q, want %q/", u.Host, u.Path, bedrockTokenHost)
	}
	q := u.Query() // nosemgrep: go_sql_rule-concat-sqli -- net/url query parsing, not SQL
	for key, want := range map[string]string{
		"Action":               "CallWithBearerToken",
		"Version":              "1",
		"X-Amz-Expires":        "43200",
		"X-Amz-SignedHeaders":  "host",
		"X-Amz-Security-Token": "session-token",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if !strings.Contains(q.Get("X-Amz-Credential"), "/us-west-2/bedrock/aws4_request") {
		t.Errorf("X-Amz-Credential = %q, want a us-west-2/bedrock scope", q.Get("X-Amz-Credential"))
	}
	if q.Get("X-Amz-Signature") == "" {
		t.Error("X-Amz-Signature is empty")
	}
	if q.Get("X-Amz-Date") == "" {
		t.Error("X-Amz-Date is empty")
	}
}

// TestMintBedrockToken_ExpiringCredentials caps the key at the credentials'
// remaining lifetime instead of the 12-hour maximum, so Grok refreshes before
// the underlying SSO session expires.
func TestMintBedrockToken_ExpiringCredentials(t *testing.T) {
	remaining := 2 * time.Hour
	stubTokenAWSConfig(t, aws.Config{
		Credentials: stubCredentialsProvider{creds: aws.Credentials{
			AccessKeyID:     "AKIDEXAMPLE",
			SecretAccessKey: "secret",
			SessionToken:    "session-token",
			CanExpire:       true,
			Expires:         time.Now().Add(remaining),
		}},
	}, nil)

	token, err := MintBedrockToken(context.Background(), "us-east-1", "")
	if err != nil {
		t.Fatalf("MintBedrockToken: %v", err)
	}
	expires := decodeMintedToken(t, token).Query().Get("X-Amz-Expires") // nosemgrep: go_sql_rule-concat-sqli -- net/url query parsing, not SQL
	secs, err := time.ParseDuration(expires + "s")
	if err != nil {
		t.Fatalf("X-Amz-Expires = %q: %v", expires, err)
	}
	if secs > remaining || secs < remaining-time.Minute {
		t.Errorf("X-Amz-Expires = %s, want approximately %s", secs, remaining)
	}
}

// TestMintBedrockToken_RejectsShortLivedCredentials: a key minted minutes
// before the SSO session ends would send Grok into a refresh loop, so fail with
// an actionable error instead.
func TestMintBedrockToken_RejectsShortLivedCredentials(t *testing.T) {
	tests := []struct {
		name    string
		expires time.Time
		want    string
	}{
		{name: "expired", expires: time.Now().Add(-time.Minute), want: "expired"},
		{name: "too soon", expires: time.Now().Add(5 * time.Minute), want: "too soon"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stubTokenAWSConfig(t, aws.Config{
				Credentials: stubCredentialsProvider{creds: aws.Credentials{
					AccessKeyID:     "AKIDEXAMPLE",
					SecretAccessKey: "secret",
					CanExpire:       true,
					Expires:         tc.expires,
				}},
			}, nil)
			_, err := MintBedrockToken(context.Background(), "us-west-2", "")
			if err == nil {
				t.Fatal("expected an error for short-lived credentials")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestMintBedrockToken_ErrorPaths(t *testing.T) {
	t.Run("region required", func(t *testing.T) {
		if _, err := MintBedrockToken(context.Background(), "  ", ""); err == nil ||
			!strings.Contains(err.Error(), "region is required") {
			t.Fatalf("error = %v, want a region-required error", err)
		}
	})
	t.Run("config load", func(t *testing.T) {
		stubTokenAWSConfig(t, aws.Config{}, errors.New("no config"))
		if _, err := MintBedrockToken(context.Background(), "us-west-2", ""); err == nil ||
			!strings.Contains(err.Error(), "loading AWS config") {
			t.Fatalf("error = %v, want a loading-AWS-config error", err)
		}
	})
	t.Run("no credentials provider", func(t *testing.T) {
		stubTokenAWSConfig(t, aws.Config{}, nil)
		if _, err := MintBedrockToken(context.Background(), "us-west-2", ""); err == nil ||
			!strings.Contains(err.Error(), "no credentials provider") {
			t.Fatalf("error = %v, want a no-credentials-provider error", err)
		}
	})
	t.Run("credentials retrieve", func(t *testing.T) {
		stubTokenAWSConfig(t, aws.Config{
			Credentials: stubCredentialsProvider{err: errors.New("sso expired")},
		}, nil)
		if _, err := MintBedrockToken(context.Background(), "us-west-2", ""); err == nil ||
			!strings.Contains(err.Error(), "sso expired") {
			t.Fatalf("error = %v, want the credentials error", err)
		}
	})
}
