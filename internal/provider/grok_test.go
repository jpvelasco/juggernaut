package provider

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
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
	home := testutil.NewTestHome(t)
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
	if ls.NeedsToken {
		t.Error("Grok NeedsToken must be false — auth mode resolved from config at launch")
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
		"codex":    {[]string{"model_providers"}, map[string][]string{"model_providers": {"amazon-bedrock.aws.region"}}},
		"opencode": {[]string{"provider"}, map[string][]string{"provider": {"amazon-bedrock"}}},
		"grok":     {[]string{"model", "models", "auth"}, map[string][]string{"model": {"bedrock-grok"}, "models": {"default"}, "auth": {"auth_provider_command", "auth_provider_label"}}},
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

// TestGrok_BuildConfig writes the [model.bedrock-grok] block (base_url→
// bedrock-runtime /openai/v1, model xai.grok-4.6) + [models].default + an
// [auth] block. Crucially it does NOT set env_key: with env_key, Grok's
// credential order (api_key→env_key→XAI_API_KEY) still runs the interactive
// login for the session. The [auth] auth_provider_command is what replaces login.
func TestGrok_BuildConfig(t *testing.T) {
	p, _ := Get("grok")
	opts := baseOpts()
	opts.Region = "us-east-1"
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	bg, ok := testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap, ok := bg.(map[string]any)
	if !ok {
		t.Fatalf("[model.%s] not a map: %T", grokModelName, bg)
	}
	if bgMap["model"] != "xai.grok-4.6" {
		t.Errorf("model = %v, want xai.grok-4.6", bgMap["model"])
	}
	if base, _ := bgMap["base_url"].(string); base != "https://bedrock-runtime.us-east-1.amazonaws.com/openai/v1" {
		t.Errorf("base_url = %q, want .../openai/v1", base)
	}
	if _, hasEnvKey := bgMap["env_key"]; hasEnvKey {
		t.Errorf("env_key must NOT be set (it keeps Grok's login flow alive); got %v", bgMap["env_key"])
	}
	if _, hasBackend := bgMap["api_backend"]; hasBackend {
		t.Errorf("api_backend must NOT be set for native bedrock-runtime (was Mantle only), got %v", bgMap["api_backend"])
	}
	modelsDefault, ok := testutil.NestedMapChain(plan.Keys, "models", "default")
	if !ok || modelsDefault != "bedrock-grok" {
		t.Errorf("[models].default = %v, want bedrock-grok", plan.Keys["models"])
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan should validate: %v", err)
	}
}

// TestGrok_BuildConfig_AuthBlock: the [auth] block points auth_provider_command
// at `juggernaut auth-token` with a Bedrock label — this is what makes Grok skip
// its sign-in and use our keychain bearer token.
func TestGrok_BuildConfig_AuthBlock(t *testing.T) {
	p, _ := Get("grok")
	opts := baseOpts()
	opts.AuthMode = authmode.BedrockAPIKey
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	auth, ok := testutil.NestedMapChain(plan.Keys, "auth")
	if !ok {
		t.Fatalf("[auth] block missing")
	}
	authMap, ok := auth.(map[string]any)
	if !ok {
		t.Fatalf("[auth] not a map: %T", auth)
	}
	if cmd, _ := authMap["auth_provider_command"].(string); !strings.Contains(cmd, "auth-token") {
		t.Errorf("auth_provider_command = %v, want it to invoke `juggernaut auth-token`", authMap["auth_provider_command"])
	}
	if authMap["auth_provider_label"] != "Bedrock" {
		t.Errorf("auth_provider_label = %v, want Bedrock", authMap["auth_provider_label"])
	}
	// "auth" must be a managed key so uninstall removes it.
	found := false
	for _, k := range plan.ManagedKeys {
		if k == "auth" {
			found = true
		}
	}
	if !found {
		t.Error("auth must be in ManagedKeys")
	}
}

// TestGrok_BuildConfig_IAM_OmitsAuthProviderCommand: IAM apply must not write
// auth_provider_command (that command demands a keychain token) and must persist
// juggernaut.auth.mode so launch can skip token injection.
func TestGrok_BuildConfig_IAM_OmitsAuthProviderCommand(t *testing.T) {
	p, _ := Get("grok")
	plan, err := p.BuildConfig(testConfig(), baseOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	auth, ok := testutil.NestedMapChain(plan.Keys, "auth")
	if !ok {
		t.Fatal("[auth] block missing (needed so re-apply can strip API-key leaves)")
	}
	authMap, ok := auth.(map[string]any)
	if !ok {
		t.Fatalf("[auth] not a map: %T", auth)
	}
	if cmd, _ := authMap["auth_provider_command"].(string); cmd != "" {
		t.Errorf("IAM must not set auth_provider_command, got %q", cmd)
	}
	jb, ok := testutil.NestedMapChain(plan.Keys, "juggernaut", "auth", "mode")
	if !ok || jb != authmode.IAM {
		t.Errorf("juggernaut.auth.mode = %v, want iam", jb)
	}
}

// TestGrok_BuildConfig_RegionIronFist: xai.grok-4.3 is verified in us-east-1/2
// and us-west-2. An unlisted region is OVERRIDDEN to a known-good one (base_url
// reflects it) rather than written as-is; a known region is kept silently.
// TestGrok_BuildConfig_UnknownModel: BuildConfig errors on a model ID that
// doesn't start with xai.grok- after normalization.
func TestGrok_BuildConfig_UnknownModel(t *testing.T) {
	p, _ := Get("grok")
	opts := baseOpts()
	opts.Model = "anthropic.claude-sonnet-4-6"
	if _, err := p.BuildConfig(testConfig(), opts); err == nil {
		t.Fatal("expected error for non-Grok model")
	}
}

// TestGrok_BuildConfig_GrokPrefixNormalizes: a model key starting with "grok-"
// is normalized to "xai.grok-" and accepted.
func TestGrok_BuildConfig_GrokPrefixNormalizes(t *testing.T) {
	p, _ := Get("grok")
	opts := baseOpts()
	opts.Model = "grok-4.4"
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	bg, ok := testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap := bg.(map[string]any)
	if bgMap["model"] != "xai.grok-4.4" {
		t.Errorf("model = %v, want xai.grok-4.4", bgMap["model"])
	}
	// Non-4.6 models don't get context_window
	if _, hasContext := bgMap["context_window"]; hasContext {
		t.Error("non-4.6 model must not have context_window")
	}
}

func TestGrok_BuildConfig_RegionIronFist(t *testing.T) {
	p, _ := Get("grok")
	opts := baseOpts()
	opts.Region = "eu-west-1"
	opts.RegionExplicit = true
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig should not fail on an unlisted region: %v", err)
	}
	bg, ok := testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap := bg.(map[string]any)
	if base := bgMap["base_url"].(string); strings.Contains(base, "eu-west-1") {
		t.Errorf("base_url must not use the non-serving region eu-west-1, got %q", base)
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected a region-override message for grok-4.6 in eu-west-1")
	}
	// us-west-2 IS a known grok-4.6 region → kept, silent.
	opts.Region = "us-west-2"
	plan, _ = p.BuildConfig(testConfig(), opts)
	bg, ok = testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap = bg.(map[string]any)
	if base := bgMap["base_url"].(string); !strings.Contains(base, "us-west-2") {
		t.Errorf("us-west-2 serves grok-4.6 and must be kept, got %q", base)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("us-west-2 is a known region and must not warn, got %v", plan.Warnings)
	}
}
