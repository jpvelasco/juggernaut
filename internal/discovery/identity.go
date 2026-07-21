package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// CallerAccount returns the AWS account selected by the default credential
// chain. It is called only during an explicit catalog refresh.
func CallerAccount(ctx context.Context, region string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("loading AWS configuration: %w", err)
	}
	result, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("getting AWS caller identity: %w", err)
	}
	accountID := strings.TrimSpace(aws.ToString(result.Account))
	if accountID == "" {
		return "", fmt.Errorf("AWS caller identity returned an empty account ID")
	}
	return accountID, nil
}

// CredentialScope returns a non-secret fingerprint of the local AWS credential
// selection. This lets offline commands select the account cache populated by
// the most recent explicit refresh without contacting AWS.
func CredentialScope(home string) (string, error) {
	hash := sha256.New()
	writeHashField(hash, "profile", firstNonEmpty(os.Getenv("AWS_PROFILE"), os.Getenv("AWS_DEFAULT_PROFILE"), "default"))
	for _, name := range []string{
		"AWS_ROLE_ARN",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI",
	} {
		writeHashField(hash, name, os.Getenv(name))
	}

	if accessKeyID := strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")); accessKeyID != "" {
		writeHashField(hash, "access-key-id", accessKeyID)
	} else {
		paths := []string{
			firstNonEmpty(os.Getenv("AWS_CONFIG_FILE"), filepath.Join(home, ".aws", "config")),
			firstNonEmpty(os.Getenv("AWS_SHARED_CREDENTIALS_FILE"), filepath.Join(home, ".aws", "credentials")),
		}
		for _, path := range paths {
			writeHashField(hash, "path", filepath.Clean(path))
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				// Applying configuration must remain usable without AWS filesystem
				// access. Include the read failure in the fingerprint rather than
				// turning an optional cache lookup into a hard failure.
				writeHashField(hash, "read-error", err.Error())
				continue
			}
			writeHashField(hash, "contents", string(data))
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type hashWriter interface {
	Write([]byte) (int, error)
}

func writeHashField(hash hashWriter, name, value string) {
	_, _ = hash.Write([]byte(name))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(value))
	_, _ = hash.Write([]byte{0})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
