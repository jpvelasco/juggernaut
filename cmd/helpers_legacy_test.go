package cmd

import "testing"

func TestIsJuggernautLegacy(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want bool
	}{
		{name: "nil", want: false},
		{name: "empty", in: map[string]any{}, want: false},
		{
			name: "grok mantle base_url",
			in: map[string]any{
				"model": map[string]any{
					"bedrock-grok": map[string]any{
						"base_url": "https://bedrock-mantle.us-west-2.api.aws/openai/v1",
					},
				},
			},
			want: true,
		},
		{
			name: "grok native runtime url is current",
			in: map[string]any{
				"model": map[string]any{
					"bedrock-grok": map[string]any{
						"base_url": "https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1",
					},
				},
			},
			want: false,
		},
		{
			name: "codex amazon-bedrock plus gpt-5.5",
			in: map[string]any{
				"model":          "openai.gpt-5.5",
				"model_provider": "amazon-bedrock",
			},
			want: true,
		},
		{
			name: "codex amazon-bedrock plus gpt-5.6 is current",
			in: map[string]any{
				"model":          "openai.gpt-5.6-sol",
				"model_provider": "amazon-bedrock",
			},
			want: false,
		},
		{
			name: "codex amazon-bedrock plus global gpt-5.6 is current",
			in: map[string]any{
				"model":          "global.openai.gpt-5.6-sol",
				"model_provider": "amazon-bedrock",
			},
			want: false,
		},
		{
			name: "codex model_provider bedrock-mantle",
			in: map[string]any{
				"model":          "openai.gpt-5.5",
				"model_provider": "bedrock-mantle",
			},
			want: true,
		},
		{
			name: "codex leftover model_providers.bedrock-mantle",
			in: map[string]any{
				"model":          "global.openai.gpt-5.6-sol",
				"model_provider": "amazon-bedrock",
				"model_providers": map[string]any{
					"amazon-bedrock": map[string]any{"aws": map[string]any{"region": "us-west-2"}},
					"bedrock-mantle": map[string]any{
						"base_url": "https://bedrock-mantle.us-west-2.api.aws/openai/v1",
					},
				},
			},
			want: true,
		},
		{
			name: "codex custom provider id with mantle url",
			in: map[string]any{
				"model":          "openai.gpt-5.5",
				"model_provider": "my-bedrock",
				"model_providers": map[string]any{
					"my-bedrock": map[string]any{
						"base_url": "https://bedrock-mantle.us-east-1.api.aws/openai/v1",
					},
				},
			},
			want: true,
		},
		{
			name: "opencode provider.bedrock-mantle",
			in: map[string]any{
				"model": "bedrock-mantle/openai.gpt-oss-120b-1:0",
				"provider": map[string]any{
					"bedrock-mantle": map[string]any{
						"options": map[string]any{"region": "us-west-2"},
					},
				},
			},
			want: true,
		},
		{
			name: "opencode amazon-bedrock is current",
			in: map[string]any{
				"model": "amazon-bedrock/openai.gpt-oss-120b-1:0",
				"provider": map[string]any{
					"amazon-bedrock": map[string]any{
						"options": map[string]any{"region": "us-west-2"},
					},
				},
			},
			want: false,
		},
		{
			name: "foreign openai provider",
			in: map[string]any{
				"model":          "gpt-5",
				"model_provider": "openai",
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isJuggernautLegacy(tc.in); got != tc.want {
				t.Fatalf("isJuggernautLegacy() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStripMantleLeftovers(t *testing.T) {
	existing := map[string]any{
		"model_provider": "amazon-bedrock",
		"model_providers": map[string]any{
			"amazon-bedrock": map[string]any{"aws": map[string]any{"region": "us-west-2"}},
			"bedrock-mantle": map[string]any{
				"base_url": "https://bedrock-mantle.us-west-2.api.aws/openai/v1",
			},
			"openai": map[string]any{"name": "openai"},
		},
		"provider": map[string]any{
			"amazon-bedrock": map[string]any{"options": map[string]any{"region": "us-west-2"}},
			"bedrock-mantle": map[string]any{"options": map[string]any{"region": "us-west-2"}},
		},
	}
	stripMantleLeftovers(existing)

	mp, _ := existing["model_providers"].(map[string]any)
	if _, ok := mp["bedrock-mantle"]; ok {
		t.Fatal("model_providers.bedrock-mantle must be stripped")
	}
	if _, ok := mp["amazon-bedrock"]; !ok {
		t.Fatal("model_providers.amazon-bedrock must be kept")
	}
	if _, ok := mp["openai"]; !ok {
		t.Fatal("unrelated model_providers.openai must be kept")
	}

	prov, _ := existing["provider"].(map[string]any)
	if _, ok := prov["bedrock-mantle"]; ok {
		t.Fatal("provider.bedrock-mantle must be stripped")
	}
	if _, ok := prov["amazon-bedrock"]; !ok {
		t.Fatal("provider.amazon-bedrock must be kept")
	}
}

func TestStripMantleLeftovers_CustomIDWithMantleURL(t *testing.T) {
	existing := map[string]any{
		"model_providers": map[string]any{
			"my-bedrock": map[string]any{
				"base_url": "https://bedrock-mantle.us-west-2.api.aws/openai/v1",
			},
		},
	}
	stripMantleLeftovers(existing)
	if _, ok := existing["model_providers"]; ok {
		t.Fatal("empty model_providers table should be dropped")
	}
}
