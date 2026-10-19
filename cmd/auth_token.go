package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/spf13/cobra"
)

// longTermRefreshSecs bounds how long Grok caches a long-term (no-embedded-
// expiry) Bedrock key before re-running auth-token. Short enough to pick up a
// same-day key rotation, long enough that the re-run overhead is negligible.
const longTermRefreshSecs = 6 * 60 * 60 // 6 hours

// authTokenFormat selects the stdout shape (see runAuthToken). Reset by
// resetFlags between ExecuteArgs calls.
var authTokenFormat string

// authTokenIAM mints a short-term Bedrock API key from the default AWS
// credential chain instead of reading the keychain. authTokenRegion is required
// in that mode (or supplied via AWS_REGION/AWS_DEFAULT_REGION).
var (
	authTokenIAM    bool
	authTokenRegion string
)

// mintBedrockToken is a package var so command tests exercise the --iam branch
// without contacting AWS.
var mintBedrockToken = discovery.MintBedrockToken

// authTokenCmd is a hidden command invoked by a CLI's external auth provider to
// fetch a Bedrock bearer token. Three consumers, two shapes (--format):
//   - "json"  (default) → {"access_token":...,"expires_in":N} — the Grok CLI's
//     auth_provider_command reads this (see internal/provider/grok.go).
//   - "token"           → the BARE token on one line — the Codex CLI's
//     auth.command trims stdout and uses the whole thing as the bearer token
//     (verified in openai/codex external_bearer.rs), so it must NOT be JSON.
//
// By default the token comes from the keychain (--auth=bedrock-api-key). With
// --iam, auth-token mints a short-term Bedrock API key from the caller's AWS
// credentials; Grok uses this because it can only send a bearer token, not
// SigV4. Either way, stdout MUST contain ONLY the token payload — any stray
// text breaks parsing — so all diagnostics go to stderr.
var authTokenCmd = &cobra.Command{
	Use:          "auth-token",
	Short:        "Print the Bedrock bearer token (used by the Grok/Codex external auth providers)",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         runAuthToken,
}

func init() {
	authTokenCmd.Flags().StringVar(&authTokenFormat, "format", "json",
		"output format: json (Grok) or token (Codex, bare token)")
	authTokenCmd.Flags().BoolVar(&authTokenIAM, "iam", false,
		"mint a short-term Bedrock token from the AWS credential chain")
	authTokenCmd.Flags().StringVar(&authTokenRegion, "region", "",
		"AWS region for --iam (defaults to AWS_REGION or AWS_DEFAULT_REGION)")
	rootCmd.AddCommand(authTokenCmd)
}

func runAuthToken(_ *cobra.Command, _ []string) error {
	if authTokenFormat != "json" && authTokenFormat != "token" {
		return fmt.Errorf("invalid --format %q — must be json or token", authTokenFormat)
	}

	var token string
	var err error
	if authTokenIAM {
		region := authTokenIAMRegion()
		if region == "" {
			return errors.New("auth-token --iam requires --region (or AWS_REGION/AWS_DEFAULT_REGION)")
		}
		token, err = mintBedrockToken(context.Background(), region)
		if err != nil {
			// Diagnostics go to stderr; stdout stays clean so the caller never
			// mis-parses an error message as a token.
			return fmt.Errorf("minting short-term Bedrock token from IAM credentials: %w", err)
		}
	} else {
		home, homeErr := homeDir()
		if homeErr != nil {
			return homeErr
		}
		token, err = keychain.Default().GetWithFallback(home)
		if err != nil {
			// Diagnostics go to stderr; stdout stays clean so the caller never
			// mis-parses an error message as a token.
			return fmt.Errorf("%s: %w", keychain.ErrReadingKeychainMsg, err)
		}
	}
	if token == "" {
		if authTokenIAM {
			return errors.New("IAM token minting returned an empty token")
		}
		return errors.New("no Bedrock bearer token stored in the keychain — " +
			"run juggernaut apply with --auth=" + authmode.BedrockAPIKey + " to store one")
	}
	fmt.Println(buildAuthTokenOutput(token, authTokenFormat, time.Now().UTC()))
	return nil
}

// authTokenIAMRegion resolves the region used to mint a short-term token: the
// --region flag first, then the standard AWS region environment variables.
func authTokenIAMRegion() string {
	if region := strings.TrimSpace(authTokenRegion); region != "" {
		return region
	}
	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if region := strings.TrimSpace(os.Getenv(key)); region != "" {
			return region
		}
	}
	return ""
}

// buildAuthTokenOutput renders the token in the requested shape. "token" emits
// the bare token (Codex); anything else emits the Grok JSON.
func buildAuthTokenOutput(token, format string, now time.Time) string {
	if format == "token" {
		return token
	}
	return buildAuthTokenJSON(token, now)
}

// buildAuthTokenJSON renders the token in Grok's expected stdout shape:
// {"access_token":"<token>"} plus an optional "expires_in" (seconds until the
// key's embedded expiry) so Grok can refresh proactively. Short-term Bedrock
// keys carry an expiry; long-term keys don't, in which case expires_in is
// bounded (longTermRefreshSecs) so Grok periodically re-runs the command and
// picks up a rotated key from the keychain instead of caching the old token for
// ~30 days (Grok's default) and only refreshing on a 401.
func buildAuthTokenJSON(token string, now time.Time) string {
	payload := map[string]any{"access_token": token}
	if exp, ok := bedrock.ParseAPIKeyExpiry(token); ok {
		secs := int64(math.Floor(exp.Sub(now).Seconds()))
		if secs > 0 {
			payload["expires_in"] = secs
		}
	} else {
		// No embedded expiry (long-term key). Emit a bounded lifetime so Grok
		// re-runs auth-token periodically and picks up a key rotated via
		// `apply --cli=grok --bedrock-key <new>` — otherwise Grok caches the old
		// token for ~30 days and keeps sending it until a 401.
		payload["expires_in"] = longTermRefreshSecs
	}
	b, err := json.Marshal(payload)
	if err != nil {
		// A map[string]any of strings/ints cannot fail to marshal; fall back to
		// the bare-token form rather than emit nothing.
		return fmt.Sprintf(`{"access_token":%q}`, token)
	}
	return string(b)
}
