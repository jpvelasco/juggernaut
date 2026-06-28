package bedrock

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// withTestServer points the package httpClient and endpoint builder at a local
// httptest server for the duration of fn, then restores them.
func withTestServer(t *testing.T, handler http.HandlerFunc, fn func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	origClient, origEndpoint := httpClient, bedrockEndpoint
	httpClient = srv.Client()
	bedrockEndpoint = func(_, _ string) string { return srv.URL }
	t.Cleanup(func() {
		httpClient = origClient
		bedrockEndpoint = origEndpoint
	})
	fn()
}

func TestCheckAPIKeyConnectivity_Success(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[]}`))
	}, func() {
		res := CheckAPIKeyConnectivity("tok", "us-west-2", "model")
		if res.IsFailure() {
			t.Errorf("expected success, got failure: %+v", res)
		}
		if res.Message != "connected" {
			t.Errorf("expected 'connected', got %q", res.Message)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}
	})
}

func TestCheckAPIKeyConnectivity_AuthError(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid bearer token"}`))
	}, func() {
		res := CheckAPIKeyConnectivity("bad", "us-west-2", "model")
		if !res.IsFailure() {
			t.Error("expected failure on 401")
		}
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", res.StatusCode)
		}
		if res.Message == "" {
			t.Error("expected a non-empty error detail")
		}
	})
}

func TestCheckIAMConnectivity_403WithAuthError_IsReachable(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		// A body containing an auth-style keyword => endpoint reachable for IAM.
		_, _ = w.Write([]byte(`{"message":"missing authorization credential for signature"}`))
	}, func() {
		res := CheckIAMConnectivity("us-west-2", "model")
		if res.IsFailure() {
			t.Errorf("403 with auth error should be treated as reachable, got: %+v", res)
		}
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", res.StatusCode)
		}
	})
}

func TestCheckIAMConnectivity_Success2xx(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}, func() {
		res := CheckIAMConnectivity("us-west-2", "model")
		if res.IsFailure() {
			t.Errorf("2xx should be success, got: %+v", res)
		}
		if res.Message != "connected" {
			t.Errorf("expected 'connected', got %q", res.Message)
		}
	})
}

func TestCheckIAMConnectivity_ServerError(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}, func() {
		res := CheckIAMConnectivity("us-west-2", "model")
		if !res.IsFailure() {
			t.Error("expected failure on 500")
		}
		if res.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", res.StatusCode)
		}
	})
}

func TestCheckIAMConnectivity_403WithoutAuthError_IsFailure(t *testing.T) {
	withTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		// No auth-style keyword in the body => falls through to the failure path.
		_, _ = w.Write([]byte(`{"message":"quota exceeded for this model"}`))
	}, func() {
		res := CheckIAMConnectivity("us-west-2", "model")
		// A 403 that is NOT an auth error falls through to the failure path.
		if !res.IsFailure() {
			t.Errorf("403 without auth-error body should be a failure, got: %+v", res)
		}
	})
}
