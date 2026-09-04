package bedrock

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// swapProbeTransport points the package httpClient and endpoint builder at a
// local httptest server and records the model ID passed to bedrockEndpoint.
func swapProbeTransport(t *testing.T, handler http.HandlerFunc) *string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	var invoked string
	origClient, origEndpoint := httpClient, bedrockEndpoint
	httpClient = srv.Client()
	bedrockEndpoint = func(_, modelID string) string {
		invoked = modelID
		return srv.URL
	}
	t.Cleanup(func() {
		httpClient = origClient
		bedrockEndpoint = origEndpoint
	})
	return &invoked
}

// withTestServer points the package httpClient and endpoint builder at a local
// httptest server for the duration of fn, then restores them.
func withTestServer(t *testing.T, handler http.HandlerFunc, fn func()) {
	t.Helper()
	_ = swapProbeTransport(t, handler)
	fn()
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"content":[]}`))
}

func TestCheckAPIKeyConnectivity_Success(t *testing.T) {
	withTestServer(t, okHandler, func() {
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

func TestProbe_InvokesStoredModelIDUnchanged(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		iam     bool
	}{
		{"global inference profile", "global.anthropic.claude-haiku-4-5-20251001-v1:0", false},
		{"us inference profile", "us.anthropic.claude-sonnet-4-6", false},
		{"eu inference profile", "eu.anthropic.claude-sonnet-4-6", true},
		{"foundation ID", "anthropic.claude-sonnet-4-6", false},
		{"foundation ID IAM", "anthropic.claude-opus-4-8", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoked := swapProbeTransport(t, okHandler)
			var res *ConnectivityResult
			if tt.iam {
				res = CheckIAMConnectivity("us-west-2", tt.modelID)
			} else {
				res = CheckAPIKeyConnectivity("tok", "us-west-2", tt.modelID)
			}
			if *invoked != tt.modelID {
				t.Errorf("InvokeModel model ID = %q, want stored pin %q", *invoked, tt.modelID)
			}
			if res.ModelID != tt.modelID {
				t.Errorf("result.ModelID = %q, want stored pin %q", res.ModelID, tt.modelID)
			}
			if res.IsFailure() {
				t.Errorf("expected success, got: %+v", res)
			}
		})
	}
}

func TestProbe_InvokesBedrockConfigPinsUnchanged(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "bedrock-config.json"))
	if err != nil {
		t.Skipf("cannot load bedrock-config.json: %v", err)
	}
	for name, id := range map[string]string{
		"default": cfg.Models.Default,
		"fast":    cfg.Models.Fast,
		"opus":    cfg.Models.Opus,
		"sonnet":  cfg.Models.Sonnet,
		"haiku":   cfg.Models.Haiku,
	} {
		t.Run(name, func(t *testing.T) {
			if id == "" {
				t.Fatalf("bedrock-config.json %s model ID is empty", name)
			}
			invoked := swapProbeTransport(t, okHandler)
			res := CheckAPIKeyConnectivity("tok", "us-west-2", id)
			if *invoked != id {
				t.Errorf("InvokeModel model ID = %q, want config pin %q", *invoked, id)
			}
			if res.ModelID != id {
				t.Errorf("result.ModelID = %q, want config pin %q", res.ModelID, id)
			}
		})
	}
}
