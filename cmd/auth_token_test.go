package cmd

import (
	"encoding/base64"
	"encoding/json"
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
