package bedrock

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// swapMantleEndpoint points the mantle probe at a local httptest server for the
// duration of the test.
func swapMantleEndpoint(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	origClient, origEndpoint := httpClient, mantleEndpoint
	httpClient = srv.Client()
	mantleEndpoint = func(string) string { return srv.URL }
	t.Cleanup(func() {
		httpClient = origClient
		mantleEndpoint = origEndpoint
	})
}

func TestCheckMantleReachability_ReachableStatuses(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			swapMantleEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			})
			res := CheckMantleReachability("us-west-2")
			if res.IsFailure() {
				t.Fatalf("HTTP %d should prove reachability, got: %+v", code, res)
			}
			if res.StatusCode != code {
				t.Errorf("StatusCode = %d, want %d", res.StatusCode, code)
			}
			if res.AuthMode != EndpointMantle {
				t.Errorf("AuthMode = %q, want %q", res.AuthMode, EndpointMantle)
			}
			if res.Region != "us-west-2" {
				t.Errorf("Region = %q, want us-west-2", res.Region)
			}
		})
	}
}

func TestCheckMantleReachability_FailureStatuses(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			swapMantleEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			})
			res := CheckMantleReachability("us-west-2")
			if !res.IsFailure() {
				t.Fatalf("HTTP %d should fail, got: %+v", code, res)
			}
			if !strings.Contains(res.Message, "unexpected HTTP") {
				t.Errorf("Message = %q, want an unexpected-HTTP error", res.Message)
			}
		})
	}
}

func TestCheckMantleReachability_RequestCreationError(t *testing.T) {
	orig := mantleEndpoint
	t.Cleanup(func() { mantleEndpoint = orig })
	mantleEndpoint = func(string) string { return "://not-a-url" }

	res := CheckMantleReachability("us-west-2")
	if !res.IsFailure() {
		t.Fatal("expected request-creation failure")
	}
	if !strings.Contains(res.Message, "failed to create request") {
		t.Errorf("Message = %q, want failed-to-create-request", res.Message)
	}
}

func TestCheckMantleReachability_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // make every request fail at the transport layer

	origClient, origEndpoint := httpClient, mantleEndpoint
	t.Cleanup(func() {
		httpClient = origClient
		mantleEndpoint = origEndpoint
	})
	httpClient = &http.Client{Timeout: 2 * time.Second}
	mantleEndpoint = func(string) string { return url }

	res := CheckMantleReachability("us-west-2")
	if !res.IsFailure() {
		t.Fatal("expected transport failure")
	}
	if !strings.Contains(res.Message, "request failed") {
		t.Errorf("Message = %q, want request-failed", res.Message)
	}
}
