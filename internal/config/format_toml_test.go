package config

import (
	"strings"
	"testing"
)

// TestTOMLFormat_Name identifies the format.
func TestTOMLFormat_Name(t *testing.T) {
	if got := (tomlFormat{}).Name(); got != "toml" {
		t.Errorf("Name() = %q, want toml", got)
	}
}

// TestTOMLFormat_RoundTrip verifies TOML reads back what it writes, including a
// nested table (the [model_providers.<id>] shape Codex uses).
func TestTOMLFormat_RoundTrip(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{
		"model_provider": "bedrock-mantle",
		"model":          "openai.gpt-5.5",
		"model_providers": map[string]any{
			"bedrock-mantle": map[string]any{
				"name":     "Amazon Bedrock (Mantle)",
				"base_url": "https://bedrock-mantle.us-east-1.api.aws/openai/v1",
				"wire_api": "responses",
				"env_key":  "AWS_BEARER_TOKEN_BEDROCK",
			},
		},
	}
	encoded, err := f.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := f.Unmarshal(encoded)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out["model"] != "openai.gpt-5.5" {
		t.Errorf("lost model key: %v", out["model"])
	}
	mp, ok := out["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers not a table: %T", out["model_providers"])
	}
	bm, ok := mp["bedrock-mantle"].(map[string]any)
	if !ok {
		t.Fatalf("bedrock-mantle not a table: %T", mp["bedrock-mantle"])
	}
	if bm["wire_api"] != "responses" {
		t.Errorf("lost wire_api: %v", bm["wire_api"])
	}
}

// TestTOMLFormat_EmitsProviderTable confirms the encoded output contains the
// [model_providers.<id>] header Codex's config.toml expects.
func TestTOMLFormat_EmitsProviderTable(t *testing.T) {
	f := tomlFormat{}
	encoded, err := f.Marshal(map[string]any{
		"model_providers": map[string]any{
			"bedrock-mantle": map[string]any{"wire_api": "responses"},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(encoded)
	if !strings.Contains(got, "[model_providers.bedrock-mantle]") {
		t.Errorf("expected nested provider table header, got:\n%s", got)
	}
}

// TestTOMLFormat_UnmarshalRejectsGarbage confirms invalid TOML errors (not a
// silent empty map).
func TestTOMLFormat_UnmarshalRejectsGarbage(t *testing.T) {
	if _, err := (tomlFormat{}).Unmarshal([]byte("this is = = not toml")); err == nil {
		t.Error("expected error for invalid TOML")
	}
}
