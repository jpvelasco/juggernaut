package schema

import (
	"testing"
)

// TestStripRegionPrefix_TableDriven covers all prefix variants including
// global. and us-gov. which were missing from the stale bedrock version.
func TestStripRegionPrefix_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no-prefix", "anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"global-prefix", "global.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"us-prefix", "us.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"us-gov-prefix", "us-gov.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"eu-prefix", "eu.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"apac-prefix", "apac.anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"empty", "", ""},
		{"partial-match-us", "user.anthropic.claude-opus-4-8", "user.anthropic.claude-opus-4-8"},
		{"example-model", "xai.grok-4-3", "xai.grok-4-3"},
		{"gpt-model", "gpt-5-codex", "gpt-5-codex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripRegionPrefix(tt.in)
			if got != tt.want {
				t.Errorf("StripRegionPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeModelID_UsesStripRegionPrefix verifies normalizeModelID
// correctly combines [1m] suffix stripping with region prefix stripping.
func TestNormalizeModelID_UsesStripRegionPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "anthropic.claude-opus-4-8", "anthropic.claude-opus-4-8"},
		{"with-1m", "anthropic.claude-opus-4-8[1m]", "anthropic.claude-opus-4-8"},
		{"global-with-1m", "global.anthropic.claude-opus-4-8[1m]", "anthropic.claude-opus-4-8"},
		{"us-with-1m", "us.anthropic.claude-sonnet-4-20250514[1m]", "anthropic.claude-sonnet-4-20250514"},
		{"no-strip", "xai.grok-4-3", "xai.grok-4-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeModelID(tt.in)
			if got != tt.want {
				t.Errorf("normalizeModelID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
