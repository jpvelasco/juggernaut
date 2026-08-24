package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

type stsClient interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

var makeSTSClient = func(cfg aws.Config) stsClient { return sts.NewFromConfig(cfg) }

// CallerAccount returns the AWS account selected by the default credential
// chain. It is called only during an explicit catalog refresh.
func CallerAccount(ctx context.Context, region string) (string, error) {
	cfg, err := loadAWSConfig(ctx, region)
	if err != nil {
		return "", err
	}
	result, err := makeSTSClient(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
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
			data, err := safepath.ReadFile(filepath.Dir(path), path)
			if err != nil && !os.IsNotExist(err) {
				// Applying configuration must remain usable without AWS filesystem
				// access. Include the read failure in the fingerprint rather than
				// turning an optional cache lookup into a hard failure.
				writeHashField(hash, "read-error", err.Error())
				continue
			}
			writeHashField(hash, "contents", string(data))
		}
		hashSSOCache(hash, home)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashSSOCache folds the aws-cli SSO token cache (~/.aws/sso/cache) into the
// credential fingerprint. Switching accounts in the SSO portal refreshes that
// cache while leaving ~/.aws/config and credentials untouched, so without it
// the scope cannot distinguish accounts and apply/doctor would serve another
// account's cached model inventory. Contents are hashed with sorted relative
// paths for determinism; a missing directory hashes as an explicit marker.
func hashSSOCache(hash hashWriter, home string) {
	dir := filepath.Join(home, ".aws", "sso", "cache")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		writeHashField(hash, "sso-cache", "<absent>")
		return
	}

	type cachedFile struct{ rel, body string }
	var files []cachedFile
	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			files = append(files, cachedFile{rel: path, body: "<walk-error: " + err.Error() + ">"})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		data, readErr := os.ReadFile(path) // #nosec G304 -- path is walked from a fixed root, not user input
		if readErr != nil {
			files = append(files, cachedFile{rel: rel, body: "<read-error: " + readErr.Error() + ">"})
			return nil
		}
		files = append(files, cachedFile{rel: rel, body: string(data)})
		return nil
	})
	if walkErr != nil {
		writeHashField(hash, "sso-cache-walk-error", walkErr.Error())
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	for _, f := range files {
		writeHashField(hash, "sso-cache:"+f.rel, f.body)
	}
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
