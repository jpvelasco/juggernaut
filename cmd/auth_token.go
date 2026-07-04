package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/spf13/cobra"
)

// authTokenCmd is a hidden command invoked by the Grok CLI's
// `auth_provider_command` (see internal/provider/grok.go). Grok runs it via
// `sh -c`, reads a token from stdout, stores it in ~/.grok/auth.json, and
// re-runs it to refresh. Its stdout MUST contain ONLY the token JSON — any other
// text breaks Grok's parsing — so all diagnostics go to stderr.
var authTokenCmd = &cobra.Command{
	Use:          "auth-token",
	Short:        "Print the Bedrock bearer token as JSON (used by the Grok CLI auth provider)",
	Hidden:       true,
	SilenceUsage: true,
	RunE:         runAuthToken,
}

func init() {
	rootCmd.AddCommand(authTokenCmd)
}

func runAuthToken(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	token, err := keychain.Default().GetWithFallback(home)
	if err != nil {
		// Diagnostics go to stderr; stdout stays clean so Grok never mis-parses
		// an error message as a token.
		return fmt.Errorf("reading Bedrock API key from keychain: %w", err)
	}
	if token == "" {
		return fmt.Errorf("no Bedrock API key found; run `juggernaut apply --cli=grok --auth=bedrock-api-key`")
	}
	out := buildAuthTokenJSON(token, time.Now().UTC())
	fmt.Println(out)
	return nil
}

// buildAuthTokenJSON renders the token in Grok's expected stdout shape:
// {"access_token":"<token>"} plus an optional "expires_in" (seconds until the
// key's embedded expiry) so Grok can refresh proactively. Short-term Bedrock
// keys carry an expiry; long-term keys don't, in which case expires_in is
// omitted and Grok assumes a long lifetime (refreshing on a 401).
func buildAuthTokenJSON(token string, now time.Time) string {
	payload := map[string]any{"access_token": token}
	if exp, ok := bedrock.ParseAPIKeyExpiry(token); ok {
		secs := int64(math.Floor(exp.Sub(now).Seconds()))
		if secs > 0 {
			payload["expires_in"] = secs
		}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		// A map[string]any of strings/ints cannot fail to marshal; fall back to
		// the bare-token form rather than emit nothing.
		return fmt.Sprintf(`{"access_token":%q}`, token)
	}
	return string(b)
}
