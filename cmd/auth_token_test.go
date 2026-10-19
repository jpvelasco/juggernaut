package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// makeShortTermKey builds a bedrock-api-key-<b64(presigned url)> whose embedded
// X-Amz-Date/X-Amz-Expires encode a known expiry, so ParseAPIKeyExpiry has
// something real to parse (mirrors the format apikey.go decodes).
func makeShortTermKey(issued time.Time, expiresSecs int) string {
	url := "bedrock.amazonaws.com/?Action=CallWithBearerToken" +
		"&X-Amz-Date=" + issued.UTC().Format("20060102T150405Z") +
		"&X-Amz-Expires=" + strconv.Itoa(expiresSecs)
	return "bedrock-api-key-" + base64.StdEncoding.EncodeToString([]byte(url))
}

// TestBuildAuthTokenJSON_BareToken: a token with no parseable expiry (long-term
// key) emits a BOUNDED expires_in so Grok re-runs the command periodically and
// picks up a rotated key, rather than caching the old token for ~30 days.
func TestBuildAuthTokenJSON_BareToken(t *testing.T) {
	out := buildAuthTokenJSON("plain-token-value", time.Now().UTC())
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v (%q)", err, out)
	}
	if m["access_token"] != "plain-token-value" {
		t.Errorf("access_token = %v, want plain-token-value", m["access_token"])
	}
	exp, ok := m["expires_in"].(float64)
	if !ok {
		t.Fatalf("expires_in must be present (bounded) for a long-term key, got %v", m["expires_in"])
	}
	if int(exp) != longTermRefreshSecs {
		t.Errorf("expires_in = %d, want bounded %d for a no-expiry key", int(exp), longTermRefreshSecs)
	}
}

// TestBuildAuthTokenJSON_WithExpiry: a short-term key emits expires_in as the
// seconds remaining until its embedded expiry.
func TestBuildAuthTokenJSON_WithExpiry(t *testing.T) {
	now := time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
	key := makeShortTermKey(now, 43200) // 12h lifetime, issued "now"
	out := buildAuthTokenJSON(key, now)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v (%q)", err, out)
	}
	if m["access_token"] != key {
		t.Errorf("access_token mismatch")
	}
	exp, ok := m["expires_in"].(float64)
	if !ok {
		t.Fatalf("expires_in missing/wrong type for a short-term key: %v", m["expires_in"])
	}
	if int(exp) != 43200 {
		t.Errorf("expires_in = %d, want 43200", int(exp))
	}
}

// TestBuildAuthTokenJSON_StdoutIsCleanJSON: the output is a single JSON object
// and nothing else — Grok reads stdout verbatim, so stray text would break it.
func TestBuildAuthTokenJSON_StdoutIsCleanJSON(t *testing.T) {
	out := buildAuthTokenJSON("tok", time.Now().UTC())
	if !strings.HasPrefix(out, "{") || !strings.HasSuffix(strings.TrimSpace(out), "}") {
		t.Errorf("output must be a bare JSON object, got %q", out)
	}
	if strings.Count(out, "{") != 1 {
		t.Errorf("output must be a single JSON object, got %q", out)
	}
}

// TestAuthToken_Command_EmitsKeychainToken drives the real command: store a
// token in an isolated keychain, run `auth-token`, and confirm stdout is the
// JSON carrying that token.
func TestAuthToken_Command_EmitsKeychainToken(t *testing.T) {
	home := testutil.NewTestHome(t)
	store := setupIsolatedKeychain(t) // skips if backend unavailable/hangs
	if err := store.SetWithFallback("kc-token-123", home); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWithFallback(home) })

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"auth-token"}); err != nil {
			t.Fatalf("auth-token: %v", err)
		}
	})
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("auth-token stdout not valid JSON: %v (%q)", err, out)
	}
	if m["access_token"] != "kc-token-123" {
		t.Errorf("access_token = %v, want kc-token-123", m["access_token"])
	}
}

// TestAuthToken_Command_FormatToken: --format=token prints the BARE token (no
// JSON), which is what Codex's auth.command reads (it trims stdout and uses the
// whole thing as the bearer token — verified in openai/codex external_bearer.rs).
func TestAuthToken_Command_FormatToken(t *testing.T) {
	home := testutil.NewTestHome(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("kc-bare-456", home); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteWithFallback(home) })

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"auth-token", "--format=token"}); err != nil {
			t.Fatalf("auth-token --format=token: %v", err)
		}
	})
	got := strings.TrimSpace(out)
	if got != "kc-bare-456" {
		t.Errorf("bare output = %q, want exactly the token kc-bare-456", got)
	}
	if strings.Contains(got, "{") || strings.Contains(got, "access_token") {
		t.Errorf("--format=token must emit no JSON, got %q", got)
	}
}

// TestBuildAuthTokenOutput_Formats pins the pure renderer for both formats.
func TestBuildAuthTokenOutput_Formats(t *testing.T) {
	now := time.Now().UTC()
	if got := buildAuthTokenOutput("tok", "token", now); got != "tok" {
		t.Errorf("token format = %q, want bare tok", got)
	}
	jsonOut := buildAuthTokenOutput("tok", "json", now)
	if !strings.HasPrefix(jsonOut, "{") || !strings.Contains(jsonOut, "access_token") {
		t.Errorf("json format = %q, want a JSON object", jsonOut)
	}
}

// TestAuthToken_Command_ErrorsWhenNoToken: with no stored token, the command
// errors (non-zero) and prints nothing parseable as a token to stdout, so Grok
// falls through to its normal login.
func TestAuthToken_Command_ErrorsWhenNoToken(t *testing.T) {
	_ = testutil.NewTestHome(t)
	testutil.SkipIfNoKeychain(t) // isolated empty store

	var err error
	out := captureStdout(t, func() {
		err = ExecuteArgs([]string{"auth-token"})
	})
	if err == nil {
		t.Fatal("expected an error when no token is stored")
	}
	if strings.Contains(out, "access_token") {
		t.Errorf("stdout must stay clean on error, got %q", out)
	}
}

// TestAuthToken_Command_InvalidFormat: an unrecognised --format value must
// error immediately without touching the keychain.
func TestAuthToken_Command_InvalidFormat(t *testing.T) {
	out := captureStdout(t, func() {
		err := ExecuteArgs([]string{"auth-token", "--format=xml"})
		if err == nil {
			t.Fatal("expected error for invalid --format")
		}
		if !strings.Contains(err.Error(), "invalid --format") {
			t.Errorf("expected 'invalid --format' in error, got: %v", err)
		}
	})
	if out != "" {
		t.Errorf("expected no stdout on format error, got %q", out)
	}
}

// stubMintBedrockToken replaces the AWS-backed minter for one command test.
func stubMintBedrockToken(t *testing.T, fn func(context.Context, string) (string, error)) {
	t.Helper()
	original := mintBedrockToken
	mintBedrockToken = fn
	t.Cleanup(func() { mintBedrockToken = original })
}

// TestAuthToken_Command_IAM_MintsShortTermToken drives the --iam path used by
// Grok: mint a short-term key from the credential chain and emit Grok's JSON
// shape with the key's embedded expiry.
func TestAuthToken_Command_IAM_MintsShortTermToken(t *testing.T) {
	var gotRegion string
	key := makeShortTermKey(time.Now().UTC(), 3600)
	stubMintBedrockToken(t, func(_ context.Context, region string) (string, error) {
		gotRegion = region
		return key, nil
	})

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"auth-token", "--iam", "--region=us-west-2"}); err != nil {
			t.Fatalf("auth-token --iam: %v", err)
		}
	})
	if gotRegion != "us-west-2" {
		t.Errorf("minted for region %q, want us-west-2", gotRegion)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		t.Fatalf("stdout not valid JSON: %v (%q)", err, out)
	}
	if m["access_token"] != key {
		t.Errorf("access_token mismatch")
	}
	exp, ok := m["expires_in"].(float64)
	if !ok || int(exp) < 3590 || int(exp) > 3600 {
		t.Errorf("expires_in = %v, want about 3600", m["expires_in"])
	}
}

// TestAuthToken_Command_IAM_RegionFromEnv: the Grok command embeds --region,
// but the env fallback keeps standalone invocations working.
func TestAuthToken_Command_IAM_RegionFromEnv(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-2")
	t.Setenv("AWS_DEFAULT_REGION", "")
	var gotRegion string
	stubMintBedrockToken(t, func(_ context.Context, region string) (string, error) {
		gotRegion = region
		return "iam-token", nil
	})
	if err := ExecuteArgs([]string{"auth-token", "--iam"}); err != nil {
		t.Fatalf("auth-token --iam: %v", err)
	}
	if gotRegion != "us-east-2" {
		t.Errorf("minted for region %q, want us-east-2", gotRegion)
	}
}

// TestAuthToken_Command_IAM_RequiresRegion: without a flag or env region the
// command must fail before calling AWS rather than minting for an empty region.
func TestAuthToken_Command_IAM_RequiresRegion(t *testing.T) {
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	var err error
	out := captureStdout(t, func() {
		err = ExecuteArgs([]string{"auth-token", "--iam"})
	})
	if err == nil || !strings.Contains(err.Error(), "requires --region") {
		t.Fatalf("error = %v, want a requires-region error", err)
	}
	if out != "" {
		t.Errorf("stdout must stay clean, got %q", out)
	}
}

// TestAuthToken_Command_IAM_ErrorsKeepStdoutClean: mint failures must not leak
// diagnostics into the token stream Grok parses.
func TestAuthToken_Command_IAM_ErrorsKeepStdoutClean(t *testing.T) {
	stubMintBedrockToken(t, func(context.Context, string) (string, error) {
		return "", errors.New("sso expired")
	})
	out := captureStdout(t, func() {
		err := ExecuteArgs([]string{"auth-token", "--iam", "--region=us-west-2"})
		if err == nil {
			t.Fatal("expected the mint error")
		}
		if !strings.Contains(err.Error(), "minting short-term Bedrock token") ||
			!strings.Contains(err.Error(), "sso expired") {
			t.Errorf("error = %v, want the wrapped mint error", err)
		}
	})
	if out != "" {
		t.Errorf("stdout must stay clean on mint error, got %q", out)
	}
}

// TestAuthToken_Command_IAM_FormatToken: Codex-style bare output also works for
// a minted IAM token (no JSON wrapper).
func TestAuthToken_Command_IAM_FormatToken(t *testing.T) {
	stubMintBedrockToken(t, func(context.Context, string) (string, error) {
		return "iam-token", nil
	})
	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"auth-token", "--iam", "--region=us-west-2", "--format=token"}); err != nil {
			t.Fatalf("auth-token --iam --format=token: %v", err)
		}
	})
	if got := strings.TrimSpace(out); got != "iam-token" {
		t.Errorf("bare IAM output = %q, want iam-token", got)
	}
}

// TestAuthToken_Command_IAM_RejectsEmptyToken covers the defensive empty-token
// branch (a minter regression must not print a blank line as a bearer token).
func TestAuthToken_Command_IAM_RejectsEmptyToken(t *testing.T) {
	stubMintBedrockToken(t, func(context.Context, string) (string, error) {
		return "", nil
	})
	var err error
	out := captureStdout(t, func() {
		err = ExecuteArgs([]string{"auth-token", "--iam", "--region=us-west-2"})
	})
	if err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Fatalf("error = %v, want an empty-token error", err)
	}
	if out != "" {
		t.Errorf("stdout must stay clean, got %q", out)
	}
}
