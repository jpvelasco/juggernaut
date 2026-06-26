package bedrock

import (
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------
// stripRegionPrefix
// -----------------------------------------------------------------------

func TestStripRegionPrefix_PreservesGlobal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "global inference profile preserved",
			input:    "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			expected: "global.anthropic.claude-haiku-4-5-20251001-v1:0",
		},
		{
			name:     "global opus preserved",
			input:    "global.anthropic.claude-opus-4-8",
			expected: "global.anthropic.claude-opus-4-8",
		},
		{
			name:     "global sonnet preserved",
			input:    "global.anthropic.claude-sonnet-4-6",
			expected: "global.anthropic.claude-sonnet-4-6",
		},
		{
			name:     "us. prefix stripped",
			input:    "us.anthropic.claude-sonnet-4-6",
			expected: "anthropic.claude-sonnet-4-6",
		},
		{
			name:     "eu. prefix stripped",
			input:    "eu.anthropic.claude-sonnet-4-6",
			expected: "anthropic.claude-sonnet-4-6",
		},
		{
			name:     "apac. prefix stripped",
			input:    "apac.anthropic.claude-sonnet-4-6",
			expected: "anthropic.claude-sonnet-4-6",
		},
		{
			name:     "no prefix unchanged",
			input:    "anthropic.claude-sonnet-4-6",
			expected: "anthropic.claude-sonnet-4-6",
		},
		{
			name:     "custom model unchanged",
			input:    "custom-model-id",
			expected: "custom-model-id",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripRegionPrefix(tt.input)
			if got != tt.expected {
				t.Errorf("stripRegionPrefix(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestStripRegionPrefix_BedrockConfigModelIDs verifies that every model ID
// in bedrock-config.json survives stripRegionPrefix unchanged. This catches
// regressions if the config switches to a different model ID format.
func TestStripRegionPrefix_BedrockConfigModelIDs(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	cfg, err := Load(filepath.Join(repoRoot, "bedrock-config.json"))
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
			got := stripRegionPrefix(id)
			if got != id {
				t.Errorf("stripRegionPrefix(%q) = %q, want %q", id, got, id)
			}
			if strings.HasPrefix(id, "global.") && !strings.HasPrefix(got, "global.") {
				t.Errorf("global prefix stripped for %s model: got %q", name, got)
			}
		})
	}
}

// -----------------------------------------------------------------------
// ConnectivityResult
// -----------------------------------------------------------------------

func TestConnectivityResult_IsFailure(t *testing.T) {
	ok := &ConnectivityResult{OK: true}
	fail := &ConnectivityResult{OK: false}
	if ok.IsFailure() {
		t.Error("OK result should not be a failure")
	}
	if !fail.IsFailure() {
		t.Error("non-OK result should be a failure")
	}
}

func TestConnectivityResult_IsAuthFailure(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"401", 401, true},
		{"403", 403, true},
		{"200", 200, false},
		{"500", 500, false},
		{"400", 400, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ConnectivityResult{StatusCode: tt.statusCode}
			got := r.IsAuthFailure()
			if got != tt.want {
				t.Errorf("IsAuthFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------
// formatResponseError
// -----------------------------------------------------------------------

func TestFormatResponseError_WithMessageField(t *testing.T) {
	msg := formatResponseError(400, []byte(`{"message":"something went wrong"}`))
	if !strings.Contains(msg, "HTTP 400") {
		t.Errorf("expected status in message, got %s", msg)
	}
	if !strings.Contains(msg, "something went wrong") {
		t.Errorf("expected error detail in message, got %s", msg)
	}
}

func TestFormatResponseError_WithTypeField(t *testing.T) {
	msg := formatResponseError(400, []byte(`{"type":"InvalidRequestException"}`))
	if !strings.Contains(msg, "InvalidRequestException") {
		t.Errorf("expected type in message, got %s", msg)
	}
}

func TestFormatResponseError_RawText(t *testing.T) {
	msg := formatResponseError(500, []byte("internal server error"))
	if !strings.Contains(msg, "HTTP 500") {
		t.Errorf("expected status in message, got %s", msg)
	}
	if !strings.Contains(msg, "internal server error") {
		t.Errorf("expected raw text in message, got %s", msg)
	}
}

func TestFormatResponseError_TruncatesLongBody(t *testing.T) {
	body := strings.Repeat("x", 300)
	msg := formatResponseError(500, []byte(body))
	if len(msg) > 250 {
		t.Errorf("message should be truncated, got %d chars", len(msg))
	}
	if !strings.HasSuffix(msg, "...") {
		t.Error("truncated message should end with ...")
	}
}

// -----------------------------------------------------------------------
// containsAuthError
// -----------------------------------------------------------------------

func TestContainsAuthError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"authorization keyword", "The authorization header is missing", true},
		{"credential keyword", "invalid credentials provided", true},
		{"signature keyword", "The request signature we calculated does not match", true},
		{"unauthorized keyword", "User is unauthorized", true},
		{"forbidden keyword", "Access forbidden", true},
		{"access denied keyword", "Access Denied", true},
		{"not authorized (no match)", "The security token you provided is not authorized", false},
		{"not invalid (no match)", "The security token included in the request is invalid", false},
		{"no match", "Resource not found", false},
		{"model error", "Invocation of model ID isn't supported", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAuthError(tt.body)
			if got != tt.want {
				t.Errorf("containsAuthError(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------
// contains (lowercase helper)
// -----------------------------------------------------------------------

func TestContains(t *testing.T) {
	tests := []struct {
		name, s, substr string
		want            bool
	}{
		{"found", "hello world", "world", true},
		{"not found", "hello world", "xyz", false},
		{"empty string", "", "a", false},
		{"empty substr", "hello", "", true},
		{"case insensitive (source uppercased)", "Hello World", "hello", true},
		{"source lowercased but search not", "hello world", "HELLO", false},
		{"exact match", "abc", "abc", true},
		{"prefix match", "abcdef", "abc", true},
		{"suffix match", "abcdef", "def", true},
		{"middle match", "abcdef", "bcd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------
// CheckAPIKeyConnectivity
// -----------------------------------------------------------------------

func TestCheckAPIKeyConnectivity_BadToken(t *testing.T) {
	result := CheckAPIKeyConnectivity("invalid-token", "us-west-2",
		"global.anthropic.claude-haiku-4-5-20251001-v1:0")
	if result.OK {
		t.Error("expected connectivity check to fail with invalid token")
	}
	if result.Message == "" {
		t.Error("expected error message")
	}
	if result.Elapsed < 0 {
		t.Error("elapsed time should not be negative")
	}
}

func TestCheckAPIKeyConnectivity_InvalidRegion(t *testing.T) {
	result := CheckAPIKeyConnectivity("some-token", "not-a-region",
		"global.anthropic.claude-haiku-4-5-20251001-v1:0")
	if result.OK {
		t.Error("expected connectivity check to fail with invalid region")
	}
}

// -----------------------------------------------------------------------
// CheckIAMConnectivity
// -----------------------------------------------------------------------

func TestCheckIAMConnectivity_InvalidRegion(t *testing.T) {
	result := CheckIAMConnectivity("not-a-region",
		"global.anthropic.claude-haiku-4-5-20251001-v1:0")
	if result.OK {
		t.Error("expected IAM connectivity check to fail with invalid region")
	}
}

// -----------------------------------------------------------------------
// ErrNoAuthConfigured
// -----------------------------------------------------------------------

func TestErrNoAuthConfigured(t *testing.T) {
	if ErrNoAuthConfigured == nil {
		t.Error("ErrNoAuthConfigured should not be nil")
	}
	if !strings.Contains(ErrNoAuthConfigured.Error(), "auth") {
		t.Error("error message should mention auth")
	}
}

// -----------------------------------------------------------------------
// Helper: findBedrockConfigFile (used by Load for test fallback)
// -----------------------------------------------------------------------

func TestFindBedrockConfigFile(t *testing.T) {
	// Verify the function can locate the config in the repo root.
	repoRoot := filepath.Join("..", "..")
	cfg, err := Load(filepath.Join(repoRoot, "bedrock-config.json"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Models.Haiku == "" {
		t.Error("expected Haiku model ID to be set")
	}
	// Ensure the model ID starts with global.
	if !strings.HasPrefix(cfg.Models.Haiku, "global.") {
		t.Errorf("expected Haiku model ID to start with global., got %q", cfg.Models.Haiku)
	}
}

// -----------------------------------------------------------------------
// Regression: old behavior would have stripped global. prefix
// -----------------------------------------------------------------------

func TestStripRegionPrefix_OldBehaviorReggression(t *testing.T) {
	// The old code stripped "global." from model IDs, producing raw model
	// IDs that Bedrock rejects with HTTP 400. This test ensures that
	// behavior cannot regress.
	modelID := "global.anthropic.claude-haiku-4-5-20251001-v1:0"
	got := stripRegionPrefix(modelID)
	if got != modelID {
		t.Errorf("regression: global prefix was stripped, got %q", got)
	}
	// Verify the stripped result would NOT be the raw model ID (old bug).
	rawModelID := "anthropic.claude-haiku-4-5-20251001-v1:0"
	if got == rawModelID {
		t.Error("regression: global prefix was stripped to raw model ID (old bug)")
	}
}
