package discovery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
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

func TestListMantleModels_PublicEntryPoint(t *testing.T) {
	originalLoadConfig := loadDefaultAWSConfig
	originalHTTPClient := mantleHTTPClient
	t.Cleanup(func() {
		loadDefaultAWSConfig = originalLoadConfig
		mantleHTTPClient = originalHTTPClient
	})

	const region = "us-test-1"
	mantleHTTPClient = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://bedrock-mantle.us-test-1.api.aws/v1/models" {
			t.Fatalf("endpoint = %q", req.URL.String())
		}
		if req.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q, want bearer token", req.Header.Get("Authorization"))
		}
		return mantleResponse(http.StatusOK, `{"data":[{"id":"qwen.qwen3-coder-next"}]}`), nil
	})
	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		t.Fatal("bearer-token discovery must not load AWS credentials")
		return aws.Config{}, nil
	}
	models, err := ListMantleModels(context.Background(), region, "secret")
	if err != nil {
		t.Fatalf("ListMantleModels with bearer token: %v", err)
	}
	if len(models) != 1 || models[0].ID != "qwen.qwen3-coder-next" {
		t.Fatalf("models = %+v", models)
	}

	configErr := errors.New("no AWS configuration")
	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, configErr
	}
	if _, err := ListMantleModels(context.Background(), region, ""); !errors.Is(err, configErr) {
		t.Fatalf("configuration error = %v, want wrapped %v", err, configErr)
	}

	credentialErr := errors.New("credentials unavailable")
	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{}, credentialErr
		})}, nil
	}
	if _, err := ListMantleModels(context.Background(), region, ""); !errors.Is(err, credentialErr) {
		t.Fatalf("credential error = %v, want wrapped %v", err, credentialErr)
	}

	loadDefaultAWSConfig = func(_ context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
		options := config.LoadOptions{}
		for _, optFn := range optFns {
			if err := optFn(&options); err != nil {
				return aws.Config{}, err
			}
		}
		if options.Region != region {
			t.Fatalf("AWS region = %q, want %q", options.Region, region)
		}
		return aws.Config{Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret", SessionToken: "session"}, nil
		})}, nil
	}
	mantleHTTPClient = httpDoerFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			t.Fatalf("request was not SigV4-signed: %v", req.Header)
		}
		return mantleResponse(http.StatusOK, `{"data":[]}`), nil
	})
	if _, err := ListMantleModels(context.Background(), region, ""); err != nil {
		t.Fatalf("ListMantleModels with SigV4: %v", err)
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
