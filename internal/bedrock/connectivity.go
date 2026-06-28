// Package bedrock provides connectivity diagnostics for the Bedrock API.
package bedrock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
)

const defaultTimeout = 15 * time.Second

// httpClient is the client used for connectivity probes. It is a package var so
// tests can substitute a client pointed at an httptest server.
var httpClient = &http.Client{Timeout: defaultTimeout}

// bedrockEndpoint builds the InvokeModel URL for a region/model. It is a var so
// tests can redirect probes to a local httptest server.
var bedrockEndpoint = func(region, modelID string) string {
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke", region, modelID)
}

// ConnectivityResult holds the outcome of a Bedrock connectivity check.
type ConnectivityResult struct {
	OK         bool
	AuthMode   string
	Region     string
	ModelID    string
	StatusCode int
	Message    string
	Elapsed    time.Duration
}

// CheckAPIKeyConnectivity makes a lightweight InvokeModel call to verify that
// the Bedrock API key is valid and can reach the configured region.
// It uses a minimal prompt ("hi") and a small max_tokens_to_sample to keep
// the request cheap.
func CheckAPIKeyConnectivity(token, region, modelID string) *ConnectivityResult {
	start := time.Now()

	modelID = stripRegionPrefix(modelID)
	url := bedrockEndpoint(region, modelID)

	body, err := json.Marshal(map[string]any{
		"messages":          []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens":        1,
		"anthropic_version": "bedrock-2023-05-31",
	})
	if err != nil {
		return &ConnectivityResult{
			OK:       false,
			AuthMode: authmode.BedrockAPIKey,
			Region:   region,
			ModelID:  modelID,
			Message:  fmt.Sprintf("failed to build request body: %v", err),
			Elapsed:  time.Since(start),
		}
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return &ConnectivityResult{
			OK:       false,
			AuthMode: authmode.BedrockAPIKey,
			Region:   region,
			ModelID:  modelID,
			Message:  fmt.Sprintf("failed to create request: %v", err),
			Elapsed:  time.Since(start),
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return &ConnectivityResult{
			OK:       false,
			AuthMode: authmode.BedrockAPIKey,
			Region:   region,
			ModelID:  modelID,
			Message:  fmt.Sprintf("request failed: %v", err),
			Elapsed:  time.Since(start),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		respBody = []byte{}
	}

	elapsed := time.Since(start)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &ConnectivityResult{
			OK:         true,
			AuthMode:   authmode.BedrockAPIKey,
			Region:     region,
			ModelID:    modelID,
			StatusCode: resp.StatusCode,
			Message:    "connected",
			Elapsed:    elapsed,
		}
	}

	// Try to extract a useful error message from the response.
	detail := formatResponseError(resp.StatusCode, respBody)
	return &ConnectivityResult{
		OK:         false,
		AuthMode:   authmode.BedrockAPIKey,
		Region:     region,
		ModelID:    modelID,
		StatusCode: resp.StatusCode,
		Message:    detail,
		Elapsed:    elapsed,
	}
}

// CheckIAMConnectivity verifies that AWS credentials are configured and can
// reach the Bedrock endpoint. This does NOT perform SigV4 signing (that
// requires the AWS SDK); instead it makes an unauthenticated request to
// confirm network reachability and that the region is valid.
// Returns a result indicating whether the endpoint is reachable, regardless
// of auth. IAM auth validation is left to Claude Code at runtime.
func CheckIAMConnectivity(region, modelID string) *ConnectivityResult {
	start := time.Now()

	modelID = stripRegionPrefix(modelID)
	url := bedrockEndpoint(region, modelID)

	body, err := json.Marshal(map[string]any{
		"messages":          []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens":        1,
		"anthropic_version": "bedrock-2023-05-31",
	})
	if err != nil {
		return &ConnectivityResult{
			OK:       false,
			AuthMode: "iam",
			Region:   region,
			ModelID:  modelID,
			Message:  fmt.Sprintf("failed to build request body: %v", err),
			Elapsed:  time.Since(start),
		}
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return &ConnectivityResult{
			OK:       false,
			AuthMode: "iam",
			Region:   region,
			ModelID:  modelID,
			Message:  fmt.Sprintf("failed to create request: %v", err),
			Elapsed:  time.Since(start),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return &ConnectivityResult{
			OK:       false,
			AuthMode: "iam",
			Region:   region,
			ModelID:  modelID,
			Message:  fmt.Sprintf("request failed: %v", err),
			Elapsed:  time.Since(start),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		respBody = []byte{}
	}

	elapsed := time.Since(start)

	// For IAM, a 403 is expected because we don't sign the request.
	// That means the endpoint is reachable and the region/model are valid.
	if resp.StatusCode == 403 {
		msg := string(respBody)
		if containsAuthError(msg) {
			return &ConnectivityResult{
				OK:         true,
				AuthMode:   "iam",
				Region:     region,
				ModelID:    modelID,
				StatusCode: resp.StatusCode,
				Message:    "endpoint reachable (IAM auth required — Claude Code will sign requests)",
				Elapsed:    elapsed,
			}
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &ConnectivityResult{
			OK:         true,
			AuthMode:   "iam",
			Region:     region,
			ModelID:    modelID,
			StatusCode: resp.StatusCode,
			Message:    "connected",
			Elapsed:    elapsed,
		}
	}

	detail := formatResponseError(resp.StatusCode, respBody)
	return &ConnectivityResult{
		OK:         false,
		AuthMode:   "iam",
		Region:     region,
		ModelID:    modelID,
		StatusCode: resp.StatusCode,
		Message:    detail,
		Elapsed:    elapsed,
	}
}

// IsFailure returns true if the connectivity check indicates a real problem.
func (r *ConnectivityResult) IsFailure() bool {
	return !r.OK
}

// IsAuthFailure returns true if the failure is an authentication or
// authorization error (401/403).
func (r *ConnectivityResult) IsAuthFailure() bool {
	return r.StatusCode == 401 || r.StatusCode == 403
}

// stripRegionPrefix removes the Bedrock inference profile region prefix
// (us., eu., apac.) so the model ID is valid for direct API calls.
// The global. prefix is preserved because global inference profiles
// are valid for cross-region invocation.
func stripRegionPrefix(modelID string) string {
	for _, prefix := range []string{"us.", "eu.", "apac."} {
		if rest, ok := strings.CutPrefix(modelID, prefix); ok {
			return rest
		}
	}
	return modelID
}

func formatResponseError(status int, body []byte) string {
	msg := string(body)

	// Try to parse as JSON to extract a message field.
	var v struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(body, &v); err == nil && v.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", status, v.Message)
	}
	if err := json.Unmarshal(body, &v); err == nil && v.Type != "" {
		return fmt.Sprintf("HTTP %d: %s", status, v.Type)
	}

	// Truncate long raw responses.
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return fmt.Sprintf("HTTP %d: %s", status, msg)
}

func containsAuthError(body string) bool {
	lower := strings.ToLower(body)
	for _, s := range []string{
		"authorization",
		"credential",
		"signature",
		"unauthorized",
		"forbidden",
		"access denied",
	} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// ErrNoAuthConfigured is returned when no auth mode is configured for the
// connectivity check.
var ErrNoAuthConfigured = errors.New("no Bedrock auth mode configured")
