package provider

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestGet_OpenCode(t *testing.T) {
	p, err := Get("opencode")
	if err != nil {
		t.Fatalf("Get(opencode): %v", err)
	}
	if p.Name() != "opencode" {
		t.Errorf("Name() = %q, want opencode", p.Name())
	}
}

func TestOpenCode_ConfigFormatIsJSON(t *testing.T) {
	p, _ := Get("opencode")
	if p.ConfigFormatName() != "json" {
		t.Errorf("format = %q, want json", p.ConfigFormatName())
	}
}

func TestOpenCode_ConfigPath(t *testing.T) {
	p, _ := Get("opencode")
	home := t.TempDir()
	user, err := p.ConfigPath(home, "user")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	// Global config lives at ~/.config/opencode/opencode.json
	if !strings.Contains(filepath.ToSlash(user), ".config/opencode/opencode.json") {
		t.Errorf("user path = %q, want ~/.config/opencode/opencode.json", user)
	}
	proj, err := p.ConfigPath(home, "project")
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if filepath.Base(proj) != "opencode.json" {
		t.Errorf("project path = %q, want ./opencode.json", proj)
	}
}

func TestOpenCode_BinaryNames(t *testing.T) {
	p, _ := Get("opencode")
	got := p.BinaryNames()
	if len(got) == 0 || (got[0] != "opencode" && got[0] != "opencode.exe") {
		t.Errorf("binary names = %v, want opencode first", got)
	}
}

func TestOpenCode_ActivationMarkers(t *testing.T) {
	p, _ := Get("opencode")
	begin, end := p.ActivationMarkers()
	if begin != "# BEGIN: Juggernaut OpenCode Activation" {
		t.Errorf("begin = %q", begin)
	}
	if end != "# END: Juggernaut OpenCode Activation" {
		t.Errorf("end = %q", end)
	}
	cb, _ := (claude{}).ActivationMarkers()
	xb, _ := (codex{}).ActivationMarkers()
	if begin == cb || begin == xb {
		t.Error("OpenCode markers must be distinct from claude/codex")
	}
}

func TestOpenCode_LaunchSpec(t *testing.T) {
	p, _ := Get("opencode")
	ls := p.LaunchSpec()
	// OpenCode routes via config; token supplied through {env:VAR}. No static
	// enable flag, but the bearer token is still needed (Mantle).
	if len(ls.StaticEnv) != 0 {
		t.Errorf("OpenCode should have no static enable flag, got %v", ls.StaticEnv)
	}
	if ls.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("TokenEnvVar = %q", ls.TokenEnvVar)
	}
	if !ls.NeedsToken {
		t.Error("OpenCode via Mantle needs a token")
	}
}

func TestOpenCode_Supports_None(t *testing.T) {
	p, _ := Get("opencode")
	for _, c := range []Capability{CapAutoMode, Cap1MContext, CapOpusplan, CapThinking, CapServiceTiers, CapEffortLevels} {
		if p.Supports(c) {
			t.Errorf("OpenCode should not claim Claude/Codex capability %d", c)
		}
	}
}

// TestOpenCode_OwnsConfig: recognizes a config we wrote by our provider id under
// the "provider" map, not a plain user opencode.json.
func TestOpenCode_OwnsConfig(t *testing.T) {
	p, _ := Get("opencode")
	if !p.OwnsConfig(map[string]any{
		"provider": map[string]any{"bedrock-mantle": map[string]any{"npm": "@ai-sdk/openai-compatible"}},
	}) {
		t.Error("should own a config containing our bedrock-mantle provider")
	}
	// Plain user config with other providers → not ours.
	if p.OwnsConfig(map[string]any{"provider": map[string]any{"anthropic": map[string]any{}}}) {
		t.Error("must not claim a config without our bedrock-mantle provider")
	}
	if p.OwnsConfig(map[string]any{"model": "anthropic/claude"}) {
		t.Error("must not claim a plain config")
	}
}

// TestOpenCode_BuildConfig_DefaultAlias: default (gpt-oss) writes a custom
// openai-compatible provider block pointing at Mantle /v1, plus top-level model.
func TestOpenCode_BuildConfig_DefaultAlias(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Region = "us-west-2"
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	bm, ok := testutil.NestedMapChain(plan.Keys, "provider", "bedrock-mantle")
	if !ok {
		t.Fatalf("provider.bedrock-mantle missing")
	}
	bmMap, ok := bm.(map[string]any)
	if !ok {
		t.Fatalf("bedrock-mantle provider not a map: %T", bm)
	}
	if bmMap["npm"] != "@ai-sdk/openai-compatible" {
		t.Errorf("npm = %v, want @ai-sdk/openai-compatible", bmMap["npm"])
	}
	optsMap, ok := testutil.NestedMapChain(bmMap, "options")
	if !ok {
		t.Fatalf("options missing under bedrock-mantle")
	}
	optsNested := optsMap.(map[string]any)
	if base, _ := optsNested["baseURL"].(string); base != "https://bedrock-mantle.us-west-2.api.aws/v1" {
		t.Errorf("baseURL = %q, want .../v1", base)
	}
	if api, _ := optsNested["apiKey"].(string); api != "{env:AWS_BEARER_TOKEN_BEDROCK}" {
		t.Errorf("apiKey = %q, want {env:AWS_BEARER_TOKEN_BEDROCK}", api)
	}
	// top-level model must be provider_id/model_id
	if m, _ := plan.Keys["model"].(string); m != "bedrock-mantle/openai.gpt-oss-120b" {
		t.Errorf("model = %q, want bedrock-mantle/openai.gpt-oss-120b", m)
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan should validate: %v", err)
	}
}

// TestOpenCode_BuildConfig_Aliases: common convenience names resolve to their
// Mantle model IDs without acting as the authoritative availability roster.
func TestOpenCode_BuildConfig_Aliases(t *testing.T) {
	cases := map[string]string{
		"glm-4.7":       "zai.glm-4.7",
		"glm-5":         "zai.glm-5",
		"kimi-k2.5":     "moonshotai.kimi-k2.5",
		"deepseek-v3.2": "deepseek.v3.2",
		"qwen3-coder":   "qwen.qwen3-coder-480b-a35b-instruct",
		"grok-4.3":      "xai.grok-4.3",
	}
	p, _ := Get("opencode")
	for key, wantID := range cases {
		opts := baseOpts()
		opts.Model = key
		plan, err := p.BuildConfig(testConfig(), opts)
		if err != nil {
			t.Errorf("%s: BuildConfig: %v", key, err)
			continue
		}
		if m, _ := plan.Keys["model"].(string); m != "bedrock-mantle/"+wantID {
			t.Errorf("%s: model = %q, want bedrock-mantle/%s", key, m, wantID)
		}
	}
}

// TestOpenCode_BuildConfig_Passthrough: an unknown model id is accepted verbatim
// (BYO) and surfaces an "unverified" warning.
func TestOpenCode_BuildConfig_Passthrough(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Model = "some.exotic-model-v9"
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("passthrough should not error: %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "bedrock-mantle/some.exotic-model-v9" {
		t.Errorf("passthrough model = %q, want verbatim id", m)
	}
	if len(plan.Warnings) == 0 {
		t.Error("passthrough (unverified) model should emit a warning")
	}
}
