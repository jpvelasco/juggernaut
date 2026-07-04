package provider

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGet_Grok(t *testing.T) {
	p, err := Get("grok")
	if err != nil {
		t.Fatalf("Get(grok): %v", err)
	}
	if p.Name() != "grok" {
		t.Errorf("Name() = %q, want grok", p.Name())
	}
}

func TestGrok_ConfigFormatIsTOML(t *testing.T) {
	p, _ := Get("grok")
	if p.ConfigFormatName() != "toml" {
		t.Errorf("format = %q, want toml", p.ConfigFormatName())
	}
}

// TestGrok_ConfigPath: ~/.grok/config.toml. Grok's model config is USER-ONLY
// (project .grok/config.toml accepts only mcp/plugins/permission), so BOTH
// scopes resolve to the user config.
func TestGrok_ConfigPath(t *testing.T) {
	p, _ := Get("grok")
	home := t.TempDir()
	for _, scope := range []string{"user", "project"} {
		got, err := p.ConfigPath(home, scope)
		if err != nil {
			t.Fatalf("scope %s: %v", scope, err)
		}
		if !strings.Contains(filepath.ToSlash(got), ".grok/config.toml") {
			t.Errorf("scope %s path = %q, want ~/.grok/config.toml", scope, got)
		}
	}
}

func TestGrok_BinaryNames(t *testing.T) {
	p, _ := Get("grok")
	got := p.BinaryNames()
	if len(got) == 0 || (got[0] != "grok" && got[0] != "grok.exe") {
		t.Errorf("binary names = %v, want grok first", got)
	}
}

func TestGrok_ActivationMarkers(t *testing.T) {
	p, _ := Get("grok")
	begin, end := p.ActivationMarkers()
	if begin != "# BEGIN: Juggernaut Grok Activation" {
		t.Errorf("begin = %q", begin)
	}
	if end != "# END: Juggernaut Grok Activation" {
		t.Errorf("end = %q", end)
	}
	for _, other := range []Provider{claude{}, codex{}, opencode{}} {
		ob, _ := other.ActivationMarkers()
		if begin == ob {
			t.Errorf("Grok markers must differ from %s", other.Name())
		}
	}
}

func TestGrok_LaunchSpec(t *testing.T) {
	p, _ := Get("grok")
	ls := p.LaunchSpec()
	if len(ls.StaticEnv) != 0 {
		t.Errorf("Grok should have no static enable flag, got %v", ls.StaticEnv)
	}
	if ls.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("TokenEnvVar = %q", ls.TokenEnvVar)
	}
	if !ls.NeedsToken {
		t.Error("Grok via Mantle needs a token")
	}
}

// TestDeepMergeContract pins each provider's deep-merge keys + owned sub-keys —
// the contract the data-loss fix depends on.
func TestDeepMergeContract(t *testing.T) {
	cases := map[string]struct {
		deep  []string
		owned map[string][]string
	}{
		"claude":   {nil, nil},
		"codex":    {[]string{"model_providers"}, map[string][]string{"model_providers": {"bedrock-mantle"}}},
		"opencode": {[]string{"provider"}, map[string][]string{"provider": {"bedrock-mantle"}}},
		"grok":     {[]string{"model", "models"}, map[string][]string{"model": {"bedrock-grok"}, "models": {"default"}}},
	}
	for name, want := range cases {
		p, _ := Get(name)
		got := p.DeepMergeKeys()
		if len(got) != len(want.deep) {
			t.Errorf("%s DeepMergeKeys = %v, want %v", name, got, want.deep)
		}
		os := p.OwnedSubKeys()
		for k, subs := range want.owned {
			if len(os[k]) != len(subs) || (len(subs) > 0 && os[k][0] != subs[0]) {
				t.Errorf("%s OwnedSubKeys[%s] = %v, want %v", name, k, os[k], subs)
			}
		}
	}
}

func TestGrok_Supports_None(t *testing.T) {
	p, _ := Get("grok")
	for _, c := range []Capability{CapAutoMode, Cap1MContext, CapOpusplan, CapThinking, CapServiceTiers, CapEffortLevels} {
		if p.Supports(c) {
			t.Errorf("Grok should not claim capability %d", c)
		}
	}
}

// TestGrok_OwnsConfig: recognizes our named [model.bedrock-grok] block, not a
// user's own grok model profiles.
func TestGrok_OwnsConfig(t *testing.T) {
	p, _ := Get("grok")
	if !p.OwnsConfig(map[string]any{
		"model": map[string]any{"bedrock-grok": map[string]any{"model": "xai.grok-4.3"}},
	}) {
		t.Error("should own a config containing [model.bedrock-grok]")
	}
	// A plain grok config with other model profiles → not ours.
	if p.OwnsConfig(map[string]any{
		"model": map[string]any{"my-local": map[string]any{"base_url": "http://localhost"}},
	}) {
		t.Error("must not claim a config without our bedrock-grok model")
	}
	if p.OwnsConfig(map[string]any{"models": map[string]any{"default": "grok-4.3"}}) {
		t.Error("must not claim a plain config")
	}
}

// TestGrok_BuildConfig writes the [model.bedrock-grok] block (base_url→Mantle
// /openai/v1, env_key, model xai.grok-4.3) + [models].default.
func TestGrok_BuildConfig(t *testing.T) {
	p, _ := Get("grok")
	opts := baseOpts()
	opts.Region = "us-east-1"
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	modelSection, ok := plan.Keys["model"].(map[string]any)
	if !ok {
		t.Fatalf("[model] table missing: %T", plan.Keys["model"])
	}
	bg, ok := modelSection["bedrock-grok"].(map[string]any)
	if !ok {
		t.Fatalf("[model.bedrock-grok] missing: %v", modelSection)
	}
	if bg["model"] != "xai.grok-4.3" {
		t.Errorf("model = %v, want xai.grok-4.3", bg["model"])
	}
	if base, _ := bg["base_url"].(string); base != "https://bedrock-mantle.us-east-1.api.aws/openai/v1" {
		t.Errorf("base_url = %q, want .../openai/v1", base)
	}
	if bg["env_key"] != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("env_key = %v, want AWS_BEARER_TOKEN_BEDROCK", bg["env_key"])
	}
	if bg["api_backend"] != "chat_completions" {
		t.Errorf("api_backend = %v, want chat_completions", bg["api_backend"])
	}
	models, ok := plan.Keys["models"].(map[string]any)
	if !ok || models["default"] != "bedrock-grok" {
		t.Errorf("[models].default = %v, want bedrock-grok", plan.Keys["models"])
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan should validate: %v", err)
	}
}
