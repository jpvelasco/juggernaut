package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// Short-term Bedrock API key generation. Some clients (notably the Grok CLI)
// can only authenticate with a bearer token and cannot sign SigV4 requests.
// AWS's answer is a short-term Bedrock API key: a SigV4-presigned
// CallWithBearerToken URL, base64-encoded and prefixed with
// "bedrock-api-key-". Minting is a local presign operation on the caller's own
// credentials — it does not call AWS — and the resulting key inherits the
// principal's permissions and expires with its session.
const (
	bedrockTokenHost        = "bedrock.amazonaws.com"
	bedrockTokenService     = "bedrock"
	bedrockTokenMaxLifetime = 12 * time.Hour
	bedrockTokenMinLifetime = 10 * time.Minute
)

// emptyPayloadHash is the hex SHA-256 of an empty request body, the payload
// hash used by AWS's token generator for its bodyless presigned POST.
var emptyPayloadHash = func() string {
	sum := sha256.Sum256(nil)
	return hex.EncodeToString(sum[:])
}()

// signBedrockToken is a package var so tests can inspect the presigned URL
// without relying on wall-clock time or real credentials.
var signBedrockToken = func(ctx context.Context, creds aws.Credentials, region string, lifetime time.Duration) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, "https://"+bedrockTokenHost+"/?Action=CallWithBearerToken", nil)
	if err != nil {
		return "", fmt.Errorf("building Bedrock token request: %w", err)
	}
	// PresignHTTP reads the requested validity window from this query param;
	// it does not derive it from signingTime.
	q := req.URL.Query()
	q.Set("X-Amz-Expires", strconv.FormatInt(int64(lifetime/time.Second), 10))
	req.URL.RawQuery = q.Encode()

	signedURL, _, err := v4.NewSigner().PresignHTTP(
		ctx, creds, req, emptyPayloadHash, bedrockTokenService, region, time.Now().UTC())
	if err != nil {
		return "", fmt.Errorf("presigning Bedrock token: %w", err)
	}
	raw := strings.TrimPrefix(signedURL, "https://")
	if raw == signedURL {
		return "", fmt.Errorf("presigned Bedrock token has an unexpected scheme")
	}
	// Version=1 is appended after signing (matching AWS's generator), so it is
	// deliberately outside the signature.
	return "bedrock-api-key-" + base64.StdEncoding.EncodeToString([]byte(raw+"&Version=1")), nil
}

// MintBedrockToken mints a short-term Bedrock API key from the default AWS
// credential chain for region. The key is valid for at most 12 hours and never
// longer than the underlying credentials, so callers must treat it as a
// short-lived secret and refresh it before expiry.
//
// This is the bridge for clients that speak bearer auth but cannot sign SigV4
// (the Grok CLI). The IAM/SSO principal must have the Bedrock permissions the
// eventual request needs; minting itself only presigns locally.
func MintBedrockToken(ctx context.Context, region string) (string, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return "", errors.New("region is required to mint a short-term Bedrock token")
	}
	cfg, err := loadAWSConfig(ctx, region)
	if err != nil {
		return "", err
	}
	if cfg.Credentials == nil {
		return "", errors.New("AWS config has no credentials provider")
	}
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("retrieving AWS credentials: %w", err)
	}

	lifetime := bedrockTokenMaxLifetime
	if creds.CanExpire {
		remaining := time.Until(creds.Expires)
		switch {
		case remaining <= 0:
			return "", errors.New("AWS credentials have expired; refresh them and retry")
		case remaining < bedrockTokenMinLifetime:
			return "", fmt.Errorf(
				"AWS credentials expire in %s, too soon to mint a usable Bedrock token; refresh them and retry",
				remaining.Round(time.Second))
		case remaining < lifetime:
			lifetime = remaining
		}
	}
	return signBedrockToken(ctx, creds, region, lifetime)
}
