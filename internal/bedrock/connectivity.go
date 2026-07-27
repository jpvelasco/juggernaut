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

// RegionalInferencePrefixes are the Bedrock cross-region inference profile
// prefixes stripped to recover the bare provider model ID. This is the single
// source of truth — StripRegionPrefix, schema, and all callers use it.
var RegionalInferencePrefixes = []string{"global.", "us.", "us-gov.", "eu.", "apac."}

// StripRegionPrefix removes a cross-region inference profile prefix from a
// model ID, recovering the bare provider model identifier. This is the single
// exported function for region-prefix stripping — callers in cmd, bedrock, and
// schema all use it. Uses RegionalInferencePrefixes as the authoritative list.
func StripRegionPrefix(modelID string) string {
	for _, prefix := range RegionalInferencePrefixes {
		if rest, ok := strings.CutPrefix(modelID, prefix); ok {
			return rest
		}
	}
	return modelID
}

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

// probeOpts carries the per-auth-mode parameters for a connectivity probe.
type probeOpts struct {
	authMode string
	token    string // non-empty for API key mode
	isIAM    bool   // if true, treat 403+auth-error as success
}

// CheckAPIKeyConnectivity makes a lightweight InvokeModel call to verify that
// the Bedrock API key is valid and can reach the configured region.
func CheckAPIKeyConnectivity(token, region, modelID string) *ConnectivityResult {
	return probe(probeOpts{authMode: authmode.BedrockAPIKey, token: token}, region, modelID)
}

// CheckIAMConnectivity verifies that AWS credentials are configured and can
// reach the Bedrock endpoint. This does NOT perform SigV4 signing (that
// requires the AWS SDK); instead it makes an unauthenticated request to
// confirm network reachability and that the region is valid.
// A 403 with an auth-related error body means the endpoint is reachable.
func CheckIAMConnectivity(region, modelID string) *ConnectivityResult {
	return probe(probeOpts{authMode: "iam", isIAM: true}, region, modelID)
}

// probe sends a minimal InvokeModel request and interprets the response
// according to the auth mode. It is the shared implementation of
// CheckAPIKeyConnectivity and CheckIAMConnectivity.
func probe(opts probeOpts, region, modelID string) *ConnectivityResult {
	start := time.Now()
	modelID = stripRegionPrefix(modelID)

	body, err := json.Marshal(map[string]any{
		"messages":          []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens":        1,
		"anthropic_version": "bedrock-2023-05-31",
	})
	if err != nil {
		return failResult(opts.authMode, region, modelID,
			fmt.Sprintf("failed to build request body: %v", err), time.Since(start))
	}

	url := bedrockEndpoint(region, modelID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return failResult(opts.authMode, region, modelID,
			fmt.Sprintf("failed to create request: %v", err), time.Since(start))
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.token != "" {
		req.Header.Set("Authorization", "Bearer "+opts.token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return failResult(opts.authMode, region, modelID,
			fmt.Sprintf("request failed: %v", err), time.Since(start))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	elapsed := time.Since(start)

	// For IAM, a 403 with an auth-related error means the endpoint is reachable.
	if opts.isIAM && resp.StatusCode == 403 && containsAuthError(string(respBody)) {
		return &ConnectivityResult{
			OK:         true,
			AuthMode:   opts.authMode,
			Region:     region,
			ModelID:    modelID,
			StatusCode: resp.StatusCode,
			Message:    "endpoint reachable (IAM auth required — Claude Code will sign requests)",
			Elapsed:    elapsed,
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &ConnectivityResult{
			OK:         true,
			AuthMode:   opts.authMode,
			Region:     region,
			ModelID:    modelID,
			StatusCode: resp.StatusCode,
			Message:    "connected",
			Elapsed:    elapsed,
		}
	}

	return &ConnectivityResult{
		OK:         false,
		AuthMode:   opts.authMode,
		Region:     region,
		ModelID:    modelID,
		StatusCode: resp.StatusCode,
		Message:    formatResponseError(resp.StatusCode, respBody),
		Elapsed:    elapsed,
	}
}

func failResult(authMode, region, modelID, message string, elapsed time.Duration) *ConnectivityResult {
	return &ConnectivityResult{
		OK:       false,
		AuthMode: authMode,
		Region:   region,
		ModelID:  modelID,
		Message:  message,
		Elapsed:  elapsed,
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

// stripRegionPrefix delegates to the exported StripRegionPrefix.
func stripRegionPrefix(modelID string) string {
	return StripRegionPrefix(modelID)
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
