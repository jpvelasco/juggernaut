package provider

import "testing"

func TestRuntimeSupported(t *testing.T) {
	cases := []struct {
		id     string
		source string
		want   bool
	}{
		// Mantle-only Claude Mythos 5. Its successor 5.1 is runtime-only and
		// must not be caught by a naive family prefix.
		{"anthropic.claude-mythos-5", "foundation", false},
		{"global.anthropic.claude-mythos-5", "foundation", false},
		{"anthropic.claude-mythos-5-v1:0", "foundation", false},
		{"anthropic.claude-mythos-5-1", "foundation", true},
		{"global.anthropic.claude-mythos-5-1", "profile", true},

		// Gemma 4 is mantle-only; Gemma 3 stays runtime.
		{"google.gemma-4-31b", "foundation", false},
		{"google.gemma-4-31b-it", "foundation", false},
		{"google.gemma-4-26b-a4b", "foundation", false},
		{"google.gemma-4-e2b", "foundation", false},
		{"google.gemma-3-27b-it", "foundation", true},

		// GPT-5.4/5.5 and the Daybreak variants are mantle-only; GPT-5.6 and
		// GPT-6.x are runtime-served.
		{"openai.gpt-5.5", "foundation", false},
		{"openai.gpt-5.4", "foundation", false},
		{"openai.gpt-5.6-cyber", "foundation", false},
		{"openai.gpt-daybreak-blue-5.6-sol", "foundation", false},
		{"openai.gpt-5.6-sol", "foundation", true},
		{"global.openai.gpt-6.1-sol", "profile", true},

		// Grok 4.3 is mantle-only; 4.6 is dual-endpoint and 4.7 runtime-only.
		{"xai.grok-4.3", "foundation", false},
		{"xai.grok-4.6", "foundation", true},
		{"xai.grok-4.7", "foundation", true},

		// Profiles are runtime-only (mantle has no cross-region inference).
		{"us.anthropic.claude-opus-4-8", "profile", true},

		// Unknown/new models default to runtime so the catalog never hides a
		// newly launched runtime model by omission.
		{"unknown.model-1", "foundation", true},

		// Legacy mantle-source cache entries were never runtime callable.
		{"openai.gpt-5.6-sol", "mantle", false},
	}
	for _, tc := range cases {
		t.Run(tc.source+"/"+tc.id, func(t *testing.T) {
			if got := RuntimeSupported(tc.id, tc.source); got != tc.want {
				t.Fatalf("RuntimeSupported(%q, %q) = %v, want %v", tc.id, tc.source, got, tc.want)
			}
		})
	}
}

func TestBareModelID(t *testing.T) {
	cases := map[string]string{
		"global.anthropic.claude-opus-4-8": "anthropic.claude-opus-4-8",
		"us.openai.gpt-oss-120b-1:0":       "openai.gpt-oss-120b-1",
		"anthropic.claude-sonnet-4-6-v1:0": "anthropic.claude-sonnet-4-6",
		"anthropic.claude-mythos-5-1":      "anthropic.claude-mythos-5-1",
		"google.gemma-4-31b":               "google.gemma-4-31b",
	}
	for in, want := range cases {
		if got := bareModelID(in); got != want {
			t.Errorf("bareModelID(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestProviders_RejectMantleOnlyModels pins that every runtime-routed provider
// rejects mantle-only families at the SupportsModel gate, with the shared
// reason that points at the opt-in tracking issue.
func TestProviders_RejectMantleOnlyModels(t *testing.T) {
	cases := map[string]string{
		"claude":   "anthropic.claude-mythos-5",
		"codex":    "openai.gpt-5.6-cyber", // contains gpt-5.6, would pass the family filter
		"opencode": "google.gemma-4-31b",
		"grok":     "xai.grok-4.3", // matches the xai.grok- family filter
	}
	for cli, id := range cases {
		t.Run(cli, func(t *testing.T) {
			p, err := Get(cli)
			if err != nil {
				t.Fatal(err)
			}
			support := SupportsCatalogModel(p, catalogModel(id, "foundation"))
			if support.Supported {
				t.Fatalf("%s accepted mantle-only model %q", cli, id)
			}
			if support.Reason != mantleOnlyReason {
				t.Fatalf("reason = %q, want %q", support.Reason, mantleOnlyReason)
			}
		})
	}
}
