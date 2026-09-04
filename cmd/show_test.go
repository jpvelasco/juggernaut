package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

func TestShow_Text_AfterApply(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user"}); err != nil {
			t.Fatalf("show error: %v", err)
		}
	})

	if !strings.Contains(out, "user scope") {
		t.Errorf("show should print the user scope header, got:\n%s", out)
	}
	if !strings.Contains(out, "managedBy") {
		t.Errorf("show should print the juggernaut block, got:\n%s", out)
	}
}

func TestShow_JSON_AfterApply(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user", "--json"}); err != nil {
			t.Fatalf("show --json error: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("show --json should emit valid JSON, got:\n%s\nerr: %v", out, err)
	}
	if _, ok := parsed["user"]; !ok {
		t.Errorf("show --json should contain the user scope key, got:\n%s", out)
	}
}

func TestShow_NotConfigured(t *testing.T) {
	_ = setupApplyTest(t)

	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user"}); err != nil {
			t.Fatalf("show on clean home error: %v", err)
		}
	})

	if !strings.Contains(out, "not configured") {
		t.Errorf("show on a clean home should report 'not configured', got:\n%s", out)
	}
}

func TestShow_CLI_TextAndJSON(t *testing.T) {
	cases := []struct {
		cli       string
		applyArgs []string
		wantText  []string
	}{
		{
			cli:       "claude",
			applyArgs: []string{"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight"},
			wantText:  []string{"managedBy", `"env"`},
		},
		{
			cli:       "codex",
			applyArgs: []string{"apply", "--cli=codex", "--auth=iam", "--region=us-east-1", "--skip-preflight"},
			wantText:  []string{"amazon-bedrock", "juggernaut"},
		},
		{
			cli:       "opencode",
			applyArgs: []string{"apply", "--cli=opencode", "--auth=iam", "--region=us-west-2", "--skip-preflight"},
			wantText:  []string{"amazon-bedrock"},
		},
		{
			cli:       "grok",
			applyArgs: []string{"apply", "--cli=grok", "--auth=iam", "--region=us-west-2", "--skip-preflight"},
			wantText:  []string{"bedrock-grok"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.cli, func(t *testing.T) {
			_ = setupApplyTest(t)
			if err := ExecuteArgs(tc.applyArgs); err != nil {
				t.Fatalf("apply --cli=%s: %v", tc.cli, err)
			}

			text := captureStreaming(t, func() {
				if err := ExecuteArgs([]string{"show", "--cli=" + tc.cli, "--scope=user"}); err != nil {
					t.Fatalf("show --cli=%s: %v", tc.cli, err)
				}
			})
			if !strings.Contains(text, "user scope") {
				t.Errorf("show --cli=%s missing user scope header:\n%s", tc.cli, text)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(text, want) {
					t.Errorf("show --cli=%s text omitted %q:\n%s", tc.cli, want, text)
				}
			}

			js := captureStreaming(t, func() {
				if err := ExecuteArgs([]string{"show", "--cli=" + tc.cli, "--scope=user", "--json"}); err != nil {
					t.Fatalf("show --cli=%s --json: %v", tc.cli, err)
				}
			})
			var parsed map[string]any
			if err := json.Unmarshal([]byte(js), &parsed); err != nil {
				t.Fatalf("show --cli=%s --json invalid JSON:\n%s\nerr: %v", tc.cli, js, err)
			}
			if _, ok := parsed["user"].(map[string]any); !ok {
				t.Fatalf("show --cli=%s --json missing user object:\n%s", tc.cli, js)
			}
			for _, want := range tc.wantText {
				if !strings.Contains(js, want) {
					t.Errorf("show --cli=%s --json omitted %q:\n%s", tc.cli, want, js)
				}
			}
		})
	}
}

func TestShow_UnknownCLI_Errors(t *testing.T) {
	_ = setupApplyTest(t)
	err := ExecuteArgs([]string{"show", "--cli=nonesuch"})
	if err == nil {
		t.Fatal("expected error for unknown --cli")
	}
	if !strings.Contains(err.Error(), "nonesuch") {
		t.Errorf("error should name the bad CLI, got: %v", err)
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should list supported CLI names, got: %v", err)
	}
}

func TestShow_DefaultCLI_IsClaude(t *testing.T) {
	_ = setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=iam", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=codex: %v", err)
	}
	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user"}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if !strings.Contains(out, "not configured") {
		t.Errorf("show without --cli should stay on claude, got:\n%s", out)
	}
	if strings.Contains(out, "amazon-bedrock") {
		t.Errorf("show without --cli must not dump Codex config, got:\n%s", out)
	}
}

func TestShow_Grok_SkipsDuplicateProjectPath(t *testing.T) {
	_ = setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=grok: %v", err)
	}
	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show", "--cli=grok"}); err != nil {
			t.Fatalf("show --cli=grok: %v", err)
		}
	})
	if strings.Count(out, "=== ") != 1 {
		t.Errorf("grok is user-only; expected one scope section, got:\n%s", out)
	}
	if !strings.Contains(out, "=== user scope ===") {
		t.Errorf("expected user scope for grok, got:\n%s", out)
	}
}

func TestShow_RedactsBearerToken(t *testing.T) {
	home := setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	raw := readSettingsJSON(t, home)
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
		settings["env"] = env
	}
	const leaked = "super-secret-value-not-a-real-key"
	env["AWS_BEARER_TOKEN_BEDROCK"] = leaked
	rewritten, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := safepath.WriteFile(filepath.Join(home, ".claude"), settingsPath, rewritten); err != nil {
		t.Fatalf("rewrite settings.json: %v", err)
	}

	out := captureStreaming(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user", "--json"}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if strings.Contains(out, leaked) {
		t.Errorf("show dumped bearer token:\n%s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Errorf("show should redact the token field, got:\n%s", out)
	}
}

func TestRedactSecrets_NestedAndNonSecrets(t *testing.T) {
	in := map[string]any{
		"model":                    "keep",
		"AWS_BEARER_TOKEN_BEDROCK": "hide-me",
		"nested": map[string]any{
			"api_key": "hide-nested",
			"region":  "us-west-2",
		},
		"list":   []any{map[string]any{"token": "hide-list", "name": "ok"}},
		"apikey": "",
	}
	got, ok := redactSecrets(in).(map[string]any)
	if !ok {
		t.Fatalf("redactSecrets should return a map, got %T", redactSecrets(in))
	}
	if got["model"] != "keep" {
		t.Errorf("non-secret model = %v, want keep", got["model"])
	}
	if got["AWS_BEARER_TOKEN_BEDROCK"] != "[redacted]" {
		t.Errorf("token field = %v, want [redacted]", got["AWS_BEARER_TOKEN_BEDROCK"])
	}
	if got["apikey"] != "" {
		t.Errorf("empty secret string should stay empty, got %v", got["apikey"])
	}
	nested, _ := got["nested"].(map[string]any)
	if nested["api_key"] != "[redacted]" {
		t.Errorf("nested api_key = %v, want [redacted]", nested["api_key"])
	}
	if nested["region"] != "us-west-2" {
		t.Errorf("nested region = %v, want us-west-2", nested["region"])
	}
	list, _ := got["list"].([]any)
	item, _ := list[0].(map[string]any)
	if item["token"] != "[redacted]" || item["name"] != "ok" {
		t.Errorf("list item = %v", item)
	}
}

func TestShowPayload_NotOwned(t *testing.T) {
	prov, err := provider.Get("codex")
	if err != nil {
		t.Fatalf("provider.Get: %v", err)
	}
	if payload := showPayload(prov, map[string]any{"model": "user-owned"}, true); payload != nil {
		t.Errorf("foreign config should be not-configured, got %#v", payload)
	}
	if payload := showPayload(prov, nil, true); payload != nil {
		t.Errorf("nil config should be not-configured, got %#v", payload)
	}
}

func TestSecretKeyName(t *testing.T) {
	secrets := []string{"token", "AWS_BEARER_TOKEN_BEDROCK", "api-key", "apiKey", "secret", "password", "access_key", "authorization", "credential"}
	for _, k := range secrets {
		if !secretKeyName(k) {
			t.Errorf("secretKeyName(%q) = false, want true", k)
		}
	}
	safe := []string{"model", "region", "env", "auth_provider_command", "model_provider"}
	for _, k := range safe {
		if secretKeyName(k) {
			t.Errorf("secretKeyName(%q) = true, want false", k)
		}
	}
}
