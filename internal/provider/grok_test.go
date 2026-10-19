package provider

import (
	"path/filepath"
	"slices"
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
		"codex":    {[]string{"model_providers"}, map[string][]string{"model_providers": {CodexBedrockRuntimeProviderID + ".aws.region", CodexLegacyProviderID + ".aws.region"}}},
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
			if len(os[k]) != len(subs) || !containsAll(os[k], subs) {
				t.Errorf("%s OwnedSubKeys[%s] = %v, want %v", name, k, os[k], subs)
			}
		}
	}
}

// containsAll is an order-independent set-membership check for owned sub-key
// lists (map-slice order is nondeterministic).
func containsAll(got, want []string) bool {
	set := make(map[string]bool, len(got))
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
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
// bedrock-runtime /openai/v1, model xai.grok-4.7) + [models].default + an
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
	// The bare default is normalized to the global. CRIS profile ID that the
	// bedrock-runtime endpoint actually serves.
	if bgMap["model"] != "global.xai.grok-4.7" {
		t.Errorf("model = %v, want global.xai.grok-4.7", bgMap["model"])
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
// is normalized to a global.xai.grok-* CRIS profile ID and accepted.
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
	if bgMap["model"] != "global.xai.grok-4.4" {
		t.Errorf("model = %v, want global.xai.grok-4.4", bgMap["model"])
	}
	// Models outside the documented roster don't get context_window.
	if _, hasContext := bgMap["context_window"]; hasContext {
		t.Error("model outside the documented roster must not have context_window")
	}
}

// TestGrok_SupportsModel_CRISProfileIDs: the catalog carries CRIS profile IDs
// (global.xai.grok-4.6, us.xai.grok-4.6) — the SupportsModel gate must accept
// them, not just the bare xai.grok-* foundation form. Regression: the old
// HasPrefix("xai.grok-") check hid every live profile ID from
// `models list --cli=grok`. Grok 4.3 is mantle-only, so it is rejected even
// though its ID matches the xai.grok- family filter.
func TestGrok_SupportsModel_CRISProfileIDs(t *testing.T) {
	p, _ := Get("grok")
	cases := []struct {
		id   string
		want bool
	}{
		{id: "global.xai.grok-4.7", want: true},
		{id: "us.xai.grok-4.7", want: true},
		{id: "xai.grok-4.7", want: true},
		{id: "global.xai.grok-4.6", want: true},
		{id: "us.xai.grok-4.6", want: true},
		{id: "xai.grok-4.6", want: true},
		{id: "global.xai.grok-4.3", want: false}, // mantle-only
		{id: "xai.grok-4.3", want: false},        // mantle-only
		{id: "openai.gpt-5.6-sol", want: false},
		{id: "xai.grok-4.6-eu", want: true}, // still the xai.grok- family after prefix strip
	}
	for _, c := range cases {
		s := SupportsCatalogModel(p, CatalogModel{ID: c.id, Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"})
		if s.Supported != c.want {
			t.Errorf("SupportsModel(%q) = %v, want %v (%s)", c.id, s.Supported, c.want, s.Reason)
		}
	}
}

// TestGrok_BuildConfig_CRISForms: bare and regional forms normalize to the
// global. CRIS ID the runtime serves; an explicit regional profile ID is kept
// verbatim (a us. pin stays us.); each documented model gets its AWS card's
// context window (4.7 = 524288, 4.6 = 500000).
func TestGrok_BuildConfig_CRISForms(t *testing.T) {
	p, _ := Get("grok")
	cases := []struct {
		in, want      string
		contextWindow int
	}{
		{in: "", want: "global.xai.grok-4.7", contextWindow: 524288},
		{in: "xai.grok-4.7", want: "global.xai.grok-4.7", contextWindow: 524288},
		{in: "grok-4.7", want: "global.xai.grok-4.7", contextWindow: 524288},
		{in: "us.xai.grok-4.7", want: "us.xai.grok-4.7", contextWindow: 524288},
		{in: "global.xai.grok-4.7", want: "global.xai.grok-4.7", contextWindow: 524288},
		{in: "us.xai.grok-4.6", want: "us.xai.grok-4.6", contextWindow: 500000},
		{in: "global.xai.grok-4.6", want: "global.xai.grok-4.6", contextWindow: 500000},
		{in: "us.xai.grok-4.4", want: "us.xai.grok-4.4", contextWindow: 0},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			opts := baseOpts()
			opts.Model = c.in
			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig(%q): %v", c.in, err)
			}
			bg, ok := testutil.NestedMapChain(plan.Keys, "model", grokModelName)
			if !ok {
				t.Fatalf("[model.%s] missing", grokModelName)
			}
			bgMap := bg.(map[string]any)
			if bgMap["model"] != c.want {
				t.Errorf("model = %v, want %v", bgMap["model"], c.want)
			}
			gotContext, hasContext := bgMap["context_window"]
			if c.contextWindow == 0 {
				if hasContext {
					t.Errorf("context_window present = %v, want absent (model %v)", gotContext, c.want)
				}
				return
			}
			if !hasContext {
				t.Fatalf("context_window missing for %v", c.want)
			}
			if gotContext != c.contextWindow {
				t.Errorf("context_window = %v, want %d (AWS model card)", gotContext, c.contextWindow)
			}
		})
	}
}

// TestGrokModelFacts pins the per-model AWS card facts that drive
// context_window and region routing: counts (so roster updates are deliberate)
// plus representative members and non-members of each profile's region set.
func TestGrokModelFacts(t *testing.T) {
	tests := []struct {
		id            string
		contextWindow int
		globalCount   int
		globalIn      string
		globalOut     string
	}{
		{"xai.grok-4.7", 524288, 28, "eu-west-1", "me-south-1"},
		{"xai.grok-4.6", 500000, 29, "me-south-1", "ca-west-1"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			facts, ok := grokModelFacts(tt.id)
			if !ok {
				t.Fatalf("%s not in Grok model table", tt.id)
			}
			if facts.ContextWindow != tt.contextWindow {
				t.Errorf("ContextWindow = %d, want %d", facts.ContextWindow, tt.contextWindow)
			}
			if len(facts.GlobalRegions) != tt.globalCount {
				t.Errorf("len(GlobalRegions) = %d, want %d", len(facts.GlobalRegions), tt.globalCount)
			}
			if !slices.Contains(facts.GlobalRegions, tt.globalIn) {
				t.Errorf("GlobalRegions missing documented region %s", tt.globalIn)
			}
			if slices.Contains(facts.GlobalRegions, tt.globalOut) {
				t.Errorf("GlobalRegions must not contain non-serving region %s", tt.globalOut)
			}
			if len(facts.USRegions) != 6 {
				t.Errorf("len(USRegions) = %d, want 6", len(facts.USRegions))
			}
			for name, regions := range map[string][]string{"GlobalRegions": facts.GlobalRegions, "USRegions": facts.USRegions} {
				if len(regions) == 0 || regions[0] != "us-east-1" {
					t.Errorf("%s[0] = %v, want us-east-1 (Iron Fist fallback)", name, regions)
				}
			}
		})
	}
	// A regional profile form resolves to the same facts as its bare ID.
	for _, id := range []string{"global.xai.grok-4.7", "us.xai.grok-4.7"} {
		if facts, ok := grokModelFacts(id); !ok || facts.ContextWindow != 524288 {
			t.Errorf("grokModelFacts(%q) = %+v, %v; want the 4.7 card facts", id, facts, ok)
		}
	}
	if _, ok := grokModelFacts("xai.grok-4.4"); ok {
		t.Error("undocumented model must not resolve to card facts")
	}
}

// TestGrok_BuildConfig_UnknownCRISFamily: a regional prefix on a non-Grok
// family must be rejected, not blindly accepted.
func TestGrok_BuildConfig_UnknownCRISFamily(t *testing.T) {
	p, _ := Get("grok")
	for _, in := range []string{"global.openai.gpt-5.6-sol", "us.anthropic.claude-sonnet-4-6"} {
		opts := baseOpts()
		opts.Model = in
		if _, err := p.BuildConfig(testConfig(), opts); err == nil {
			t.Errorf("BuildConfig(%q) should error", in)
		}
	}
}

// TestGrok_BuildConfig_RegionIronFist: the default 4.7 global. profile is
// callable from eu-west-1 (per the AWS card), but not from me-south-1. An
// unlisted region is OVERRIDDEN to us-east-1 (base_url reflects it) rather
// than written as-is; a serving region is kept silently.
func TestGrok_BuildConfig_RegionIronFist(t *testing.T) {
	p, _ := Get("grok")

	// eu-west-1 serves the Grok 4.7 global. profile → kept, silent.
	opts := baseOpts()
	opts.Region = "eu-west-1"
	opts.RegionExplicit = true
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig should not fail for a serving region: %v", err)
	}
	bg, ok := testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap := bg.(map[string]any)
	if base := bgMap["base_url"].(string); !strings.Contains(base, "eu-west-1") {
		t.Errorf("eu-west-1 serves the grok-4.7 global profile and must be kept, got %q", base)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("eu-west-1 is a known region and must not warn, got %v", plan.Warnings)
	}

	// me-south-1 does not serve the 4.7 global. profile → override, warn.
	opts.Region = "me-south-1"
	plan, err = p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig should not fail on a non-serving region: %v", err)
	}
	bg, ok = testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap = bg.(map[string]any)
	if base := bgMap["base_url"].(string); strings.Contains(base, "me-south-1") {
		t.Errorf("base_url must not use the non-serving region me-south-1, got %q", base)
	}
	if len(plan.Warnings) == 0 || !strings.Contains(strings.Join(plan.Warnings, " "), "us-east-1") {
		t.Errorf("expected a us-east-1 region-override message for grok-4.7 in me-south-1, got %v", plan.Warnings)
	}

	// A us. profile uses the US source-region set: eu-west-1 serves the global
	// 4.7 profile but is not a us. source region, so it is overridden.
	opts.Model = "us.xai.grok-4.7"
	plan, err = p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig should not fail for a us. profile: %v", err)
	}
	bg, ok = testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap = bg.(map[string]any)
	if model := bgMap["model"]; model != "us.xai.grok-4.7" {
		t.Errorf("model = %v, want us.xai.grok-4.7 (explicit profile kept)", model)
	}
	if base := bgMap["base_url"].(string); strings.Contains(base, "eu-west-1") {
		t.Errorf("base_url must not use eu-west-1 for a us. profile, got %q", base)
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected a region-override message for us.xai.grok-4.7 in eu-west-1")
	}
	opts.Model = ""

	// us-west-2 IS a known grok-4.7 region → kept, silent.
	opts.Region = "us-west-2"
	plan, _ = p.BuildConfig(testConfig(), opts)
	bg, ok = testutil.NestedMapChain(plan.Keys, "model", grokModelName)
	if !ok {
		t.Fatalf("[model.%s] missing", grokModelName)
	}
	bgMap = bg.(map[string]any)
	if base := bgMap["base_url"].(string); !strings.Contains(base, "us-west-2") {
		t.Errorf("us-west-2 serves grok-4.7 and must be kept, got %q", base)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("us-west-2 is a known region and must not warn, got %v", plan.Warnings)
	}
}
