package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	cfg, err := loadAWSConfig(ctx, region, "")
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
//
// The fingerprint hashes identity sources, not volatile bytes: the profile
// name, the assumed-role/web-identity/container env vars, the contents of
// ~/.aws/config and ~/.aws/credentials (a static key rotation or a profile
// re-point is a real credential change and legitimately re-scopes), and the
// SSO cache's identity fields only (accountId, startUrl, region). Hashing the
// SSO cache's raw bytes — access tokens, client registrations, expiries —
// would orphan the catalog binding for the same credential selection after
// every `aws sso login` refresh cycle.
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
		hashSSOIdentity(hash, home)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashSSOIdentity folds the aws-cli SSO token cache's identity fields into the
// credential fingerprint. Only identity-bearing fields are hashed: accountId
// (legacy account-keyed tokens, where the portal-selected account is not in
// ~/.aws/config) and startUrl with region (portal-keyed tokens). Access
// tokens, refresh tokens, client registrations, and expiries rotate on every
// login cycle, and hashing them would orphan the catalog binding for the same
// credential selection after each refresh. Files without identity fields
// (client registrations) and unreadable or unparseable entries contribute
// nothing rather than hashing volatile bytes.
func hashSSOIdentity(hash hashWriter, home string) {
	dir := filepath.Join(home, ".aws", "sso", "cache")
	root, err := os.OpenRoot(dir)
	if err != nil {
		writeHashField(hash, "sso-identity", "<absent>")
		return
	}
	defer root.Close()
	var identities []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		data, readErr := root.ReadFile(rel)
		if readErr != nil {
			return nil
		}
		var token struct {
			AccountID string `json:"accountId"`
			StartURL  string `json:"startUrl"`
			Region    string `json:"region"`
		}
		if json.Unmarshal(data, &token) != nil {
			return nil
		}
		if token.AccountID == "" && token.StartURL == "" && token.Region == "" {
			return nil
		}
		identities = append(identities, token.AccountID+"\x00"+token.StartURL+"\x00"+token.Region)
		return nil
	})
	sort.Strings(identities)
	for _, identity := range identities {
		writeHashField(hash, "sso-identity", identity)
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
