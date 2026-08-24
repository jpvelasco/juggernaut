package discovery

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type stalledDoer struct{ delay time.Duration }

func (d stalledDoer) Do(*http.Request) (*http.Response, error) {
	time.Sleep(d.delay)
	body := io.NopCloser(strings.NewReader(`{"data":[{"id":"anthropic.claude-sonnet-4-5"}]}`))
	return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
}

// Given the Mantle endpoint stalls (VPN black-hole, DROP-mode firewall),
// When models refresh queries it with a context that has no deadline,
// Then the request is bounded by an internal deadline (tunable via
// mantleRequestTimeout) instead of blocking indefinitely.
func TestListMantleModels_BoundedByInternalDeadline(t *testing.T) {
	origClient := mantleHTTPClient
	origTimeout := mantleRequestTimeout
	mantleHTTPClient = stalledDoer{delay: 5 * time.Second}
	mantleRequestTimeout = 50 * time.Millisecond
	defer func() {
		mantleHTTPClient = origClient
		mantleRequestTimeout = origTimeout
	}()

	start := time.Now()
	_, err := ListMantleModels(context.Background(), "us-west-2", "tok")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("ListMantleModels succeeded against a stalled endpoint after %s; want internal deadline error", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("deadline not applied promptly: blocked for %s", elapsed)
	}
}

// Given the endpoint responds normally,
// When the internal deadline is generous,
// Then a healthy fetch still succeeds.
func TestListMantleModels_HealthyFetchUnaffected(t *testing.T) {
	origClient := mantleHTTPClient
	mantleHTTPClient = stalledDoer{delay: 10 * time.Millisecond}
	defer func() { mantleHTTPClient = origClient }()

	models, err := ListMantleModels(context.Background(), "us-west-2", "tok")
	if err != nil {
		t.Fatalf("healthy fetch failed: %v", err)
	}
	if len(models) != 1 || models[0].ID != "anthropic.claude-sonnet-4-5" {
		t.Errorf("unexpected models: %+v", models)
	}
}
