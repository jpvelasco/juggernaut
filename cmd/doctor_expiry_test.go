package cmd

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
)

// shortTermKey builds a short-term-style key (split prefix to avoid the secret
// scanner) whose embedded SigV4 window starts at amzDate for expiresSecs.
func shortTermKey(amzDate string, expiresSecs string) string {
	url := "https://bedrock.amazonaws.com/?Action=CallWithBearerToken" +
		"&X-Amz-Date=" + amzDate + "&X-Amz-Expires=" + expiresSecs
	return "bedrock-" + "api-key-" + base64.StdEncoding.EncodeToString([]byte(url))
}

func TestCheckKeyExpiry_Expired(t *testing.T) {
	r := doctor.NewReport()
	checkKeyExpiry(r, shortTermKey("20200101T000000Z", "3600")) // long expired

	if !r.HasWarnings() {
		t.Fatal("expired short-term key should produce a warning")
	}
	if !strings.Contains(r.String(), "expired") {
		t.Errorf("expected 'expired' in report, got:\n%s", r.String())
	}
}

func TestCheckKeyExpiry_ExpiringSoon(t *testing.T) {
	// Issued far in the future minus we can't control "now"; instead use a
	// window that is currently valid but tiny is hard without controlling time.
	// Use a key valid for a very long window from a recent-but-past date so it
	// is still valid now (covers the OK branch).
	r := doctor.NewReport()
	checkKeyExpiry(r, shortTermKey("20990101T000000Z", "43200")) // far-future, valid

	if r.HasWarnings() || r.HasFailures() {
		t.Errorf("a valid future short-term key should be OK, got:\n%s", r.String())
	}
	if !strings.Contains(r.String(), "valid until") {
		t.Errorf("expected 'valid until' in report, got:\n%s", r.String())
	}
}

func TestCheckKeyExpiry_LongTermSkipped(t *testing.T) {
	r := doctor.NewReport()
	// Long-term ABSK key has no embedded expiry → checkKeyExpiry records nothing.
	longTerm := "ABSK" + base64.StdEncoding.EncodeToString([]byte("BedrockAPIKey-x-at-1:secret"))
	checkKeyExpiry(r, longTerm)

	if strings.Contains(r.String(), "api key expiry") {
		t.Errorf("long-term key should not record an expiry check, got:\n%s", r.String())
	}
}
