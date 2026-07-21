package discovery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type httpDoerFunc func(*http.Request) (*http.Response, error)

func (f httpDoerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func mantleResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestListMantleModelsWith_BearerAndModelMapping(t *testing.T) {
	client := httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if req.Method != http.MethodGet || req.URL.String() != "https://example.test/v1/models" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
		}
		return mantleResponse(http.StatusOK, `{
			"data": [
				{"id":"moonshotai.kimi-k2.5","status":"available","owned_by":"moonshotai"},
				{"id":"zai.glm-5"},
				{"id":"","status":"available"}
			]
		}`), nil
	})

	models, err := listMantleModelsWith(context.Background(), "https://example.test/v1/models", "secret", client, nil)
	if err != nil {
		t.Fatalf("listMantleModelsWith: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v, want two non-empty IDs", models)
	}
	if models[0] != (DiscoveredModel{
		ID: "moonshotai.kimi-k2.5", Status: "ACTIVE", Availability: "AVAILABLE",
		Provider: "moonshotai", Source: SourceMantle,
	}) {
		t.Errorf("first model = %+v", models[0])
	}
	if models[1].Status != "ACTIVE" || models[1].Provider != "zai" {
		t.Errorf("fallback mapping = %+v, want ACTIVE status and zai provider", models[1])
	}
}

func TestListMantleModelsWith_SigV4Signer(t *testing.T) {
	signed := false
	signer := func(_ context.Context, req *http.Request) error {
		signed = true
		req.Header.Set("Authorization", "AWS4-HMAC-SHA256 test")
		return nil
	}
	client := httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			t.Fatalf("request was not signed: %v", req.Header)
		}
		return mantleResponse(http.StatusOK, `{"data":[]}`), nil
	})

	if _, err := listMantleModelsWith(context.Background(), "https://example.test/v1/models", "", client, signer); err != nil {
		t.Fatalf("listMantleModelsWith: %v", err)
	}
	if !signed {
		t.Fatal("expected SigV4 signer to be called")
	}
}

func TestListMantleModelsWith_RequiresAuthentication(t *testing.T) {
	_, err := listMantleModelsWith(context.Background(), "https://example.test/v1/models", "", httpDoerFunc(nil), nil)
	if err == nil || !strings.Contains(err.Error(), "no bearer token or AWS signer") {
		t.Fatalf("expected missing-auth error, got %v", err)
	}
}

func TestListMantleModelsWith_ReportsHTTPAndDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
		want string
	}{
		{"http", mantleResponse(http.StatusForbidden, `{"message":"denied"}`), "denied"},
		{"json", mantleResponse(http.StatusOK, `{`), "decoding Mantle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := httpDoerFunc(func(*http.Request) (*http.Response, error) { return tt.resp, nil })
			_, err := listMantleModelsWith(context.Background(), "https://example.test/v1/models", "token", client, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestListMantleModelsWith_PropagatesTransportAndSignerErrors(t *testing.T) {
	transportErr := errors.New("offline")
	client := httpDoerFunc(func(*http.Request) (*http.Response, error) { return nil, transportErr })
	if _, err := listMantleModelsWith(context.Background(), "https://example.test", "token", client, nil); !errors.Is(err, transportErr) {
		t.Fatalf("transport error = %v, want wrapped %v", err, transportErr)
	}

	signerErr := errors.New("credentials expired")
	signer := func(context.Context, *http.Request) error { return signerErr }
	if _, err := listMantleModelsWith(context.Background(), "https://example.test", "", client, signer); !errors.Is(err, signerErr) {
		t.Fatalf("signer error = %v, want wrapped %v", err, signerErr)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestListMantleModelsWith_RejectsInvalidAndOversizedResponses(t *testing.T) {
	if _, err := listMantleModelsWith(context.Background(), "://bad", "token", httpDoerFunc(nil), nil); err == nil || !strings.Contains(err.Error(), "creating Mantle") {
		t.Fatalf("invalid endpoint error = %v", err)
	}

	readErr := errors.New("read failed")
	readFailure := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{err: readErr})}, nil
	})
	if _, err := listMantleModelsWith(context.Background(), "https://example.test", "token", readFailure, nil); !errors.Is(err, readErr) {
		t.Fatalf("read error = %v, want wrapped %v", err, readErr)
	}

	oversized := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		body := bytes.NewReader(make([]byte, maxMantleCatalogBytes+1))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body)}, nil
	})
	if _, err := listMantleModelsWith(context.Background(), "https://example.test", "token", oversized, nil); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestListMantleModelsWith_FormatsEmptyAndLongHTTPErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: "no response body"},
		{name: "long", body: strings.Repeat("x", 600), want: "..."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := httpDoerFunc(func(*http.Request) (*http.Response, error) {
				return mantleResponse(http.StatusBadGateway, tt.body), nil
			})
			_, err := listMantleModelsWith(context.Background(), "https://example.test", "token", client, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
