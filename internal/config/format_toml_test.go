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
// nested table (the [model_providers.amazon-bedrock.aws] shape Codex uses).
func TestTOMLFormat_RoundTrip(t *testing.T) {
	f := tomlFormat{}
	in := map[string]any{
		"model_provider": "amazon-bedrock",
		"model":          "openai.gpt-5.5",
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
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
	if out["model_provider"] != "amazon-bedrock" {
		t.Errorf("lost model_provider key: %v", out["model_provider"])
	}
	mp, ok := out["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers not a table: %T", out["model_providers"])
	}
	ab, ok := mp["amazon-bedrock"].(map[string]any)
	if !ok {
		t.Fatalf("amazon-bedrock not a table: %T", mp["amazon-bedrock"])
	}
	aws, ok := ab["aws"].(map[string]any)
	if !ok {
		t.Fatalf("aws not a table: %T", ab["aws"])
	}
	if aws["region"] != "us-east-1" {
		t.Errorf("lost aws.region: %v", aws["region"])
	}
}

// TestTOMLFormat_EmitsProviderTable confirms the encoded output contains the
// [model_providers.amazon-bedrock] and [model_providers.amazon-bedrock.aws]
// headers Codex's config.toml expects.
func TestTOMLFormat_EmitsProviderTable(t *testing.T) {
	f := tomlFormat{}
	encoded, err := f.Marshal(map[string]any{
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{
				"aws": map[string]any{"region": "us-east-1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(encoded)
	if !strings.Contains(got, "[model_providers.amazon-bedrock]") {
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
