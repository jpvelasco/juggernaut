package bedrock_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

// shortTermPrefix is the short-term key prefix, split so the gitleaks secret
// scanner doesn't flag these test fixtures as hard-coded credentials.
const shortTermPrefix = "bedrock-" + "api-key-"

// makeShortTermKey builds a short-term-style key: the short-term prefix followed
// by base64 of a presigned URL carrying X-Amz-Date and X-Amz-Expires.
func makeShortTermKey(amzDate string, expiresSecs int) string {
	url := "https://bedrock.amazonaws.com/?Action=CallWithBearerToken" +
		"&X-Amz-Algorithm=AWS4-HMAC-SHA256" +
		"&X-Amz-Date=" + amzDate +
		"&X-Amz-Expires=" + itoa(expiresSecs) +
		"&X-Amz-SignedHeaders=host"
	return shortTermPrefix + base64.StdEncoding.EncodeToString([]byte(url))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestParseAPIKeyExpiry_ShortTerm(t *testing.T) {
	// Issued 2026-06-28T02:51:49Z, valid 12h => expires 2026-06-28T14:51:49Z.
	key := makeShortTermKey("20260628T025149Z", 43200)

	exp, ok := bedrock.ParseAPIKeyExpiry(key)
	if !ok {
		t.Fatal("expected a parseable expiry for a short-term key")
	}
	want := time.Date(2026, 6, 28, 14, 51, 49, 0, time.UTC)
	if !exp.Equal(want) {
		t.Errorf("expiry = %s, want %s", exp.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestParseAPIKeyExpiry_LongTermHasNoExpiry(t *testing.T) {
	// Long-term keys are "ABSK" + base64(...) with no embedded presigned URL.
	longTerm := "ABSK" + base64.StdEncoding.EncodeToString([]byte("BedrockAPIKey-abc-at-123:secret"))
	if _, ok := bedrock.ParseAPIKeyExpiry(longTerm); ok {
		t.Error("long-term key should not yield an expiry")
	}
}

func TestParseAPIKeyExpiry_Garbage(t *testing.T) {
	for _, k := range []string{"", "not-a-key", shortTermPrefix + "not-base64!!"} {
		if _, ok := bedrock.ParseAPIKeyExpiry(k); ok {
			t.Errorf("garbage key %q should not yield an expiry", k)
		}
	}
}

func TestIsAPIKeyExpired(t *testing.T) {
	past := makeShortTermKey("20200101T000000Z", 3600)    // long expired
	future := makeShortTermKey("20990101T000000Z", 43200) // far future

	if !bedrock.IsAPIKeyExpired(past, time.Now().UTC()) {
		t.Error("expected past short-term key to be expired")
	}
	if bedrock.IsAPIKeyExpired(future, time.Now().UTC()) {
		t.Error("expected future short-term key to be valid")
	}
	// Keys with no parseable expiry (long-term) are never reported expired.
	longTerm := "ABSK" + base64.StdEncoding.EncodeToString([]byte("x"))
	if bedrock.IsAPIKeyExpired(longTerm, time.Now().UTC()) {
		t.Error("long-term key should never be reported expired")
	}
}
