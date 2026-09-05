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
	home := testutil.NewTestHome(t)
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
	// OpenCode routes via config; built-in amazon-bedrock uses AWS chain (IAM or bearer).
	if len(ls.StaticEnv) != 0 {
		t.Errorf("OpenCode should have no static enable flag, got %v", ls.StaticEnv)
	}
	if ls.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("TokenEnvVar = %q", ls.TokenEnvVar)
	}
	if ls.NeedsToken {
		t.Error("OpenCode NeedsToken must be false — auth mode resolved from config at launch")
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
		"provider": map[string]any{"amazon-bedrock": map[string]any{"options": map[string]any{"region": "us-west-2"}}},
	}) {
		t.Error("should own a config containing our amazon-bedrock provider")
	}
	// Plain user config with other providers → not ours.
	if p.OwnsConfig(map[string]any{"provider": map[string]any{"anthropic": map[string]any{}}}) {
		t.Error("must not claim a config without our amazon-bedrock provider")
	}
	if p.OwnsConfig(map[string]any{"model": "anthropic/claude"}) {
		t.Error("must not claim a plain config")
	}
}

// TestOpenCode_BuildConfig_DefaultAlias: default (gpt-oss) writes the built-in
// amazon-bedrock provider block (region + live models + whitelist) plus
// top-level model. The plan carries NO juggernaut key — OpenCode's strict
// schema rejects unknown top-level keys, so the auth-mode block lives in the
// sidecar file (see TestOpenCode_Sidecar).
func TestOpenCode_BuildConfig_DefaultAlias(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Region = "us-west-2"
	// All native sources are supported; a mantle source would be excluded.
	opts.ModelCatalog = []CatalogModel{
		{ID: "openai.gpt-oss-120b-1:0", Status: "ACTIVE", Source: "foundation"},
		{ID: "moonshotai.kimi-k2.5", Status: "ACTIVE", Source: "profile"},
		{ID: "zai.glm-4.7", Status: "ACTIVE", Source: "foundation"},
		{ID: "openai.gpt-5.6-sol", Status: "ACTIVE", Source: "mantle"}, // excluded: native-only
	}
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	bm, ok := testutil.NestedMapChain(plan.Keys, "provider", "amazon-bedrock")
	if !ok {
		t.Fatalf("provider.amazon-bedrock missing")
	}
	bmMap, ok := bm.(map[string]any)
	if !ok {
		t.Fatalf("amazon-bedrock provider not a map: %T", bm)
	}
	if _, hasNPM := bmMap["npm"]; hasNPM {
		t.Errorf("must not have npm for built-in provider")
	}
	optsMap, ok := testutil.NestedMapChain(bmMap, "options")
	if !ok {
		t.Fatalf("options missing under amazon-bedrock")
	}
	optsNested := optsMap.(map[string]any)
	if region, _ := optsNested["region"].(string); region != "us-west-2" {
		t.Errorf("region = %q, want us-west-2", region)
	}
	if _, hasBaseURL := optsNested["baseURL"]; hasBaseURL {
		t.Errorf("must not have baseURL for built-in provider")
	}
	if _, hasAPIKey := optsNested["apiKey"]; hasAPIKey {
		t.Errorf("must not have apiKey for built-in provider")
	}
	// models must be present; whitelist must equal the discovered non-mantle IDs
	if _, ok := bmMap["models"]; !ok {
		t.Error("models should be present")
	}
	wl, ok := bmMap["whitelist"].([]string)
	if !ok {
		t.Fatalf("whitelist should be a non-empty []string with a catalog, got %T", bmMap["whitelist"])
	}
	want := map[string]bool{"openai.gpt-oss-120b-1:0": true, "moonshotai.kimi-k2.5": true, "zai.glm-4.7": true}
	if len(wl) != len(want) {
		t.Fatalf("whitelist = %v, want the 3 discovered native models (mantle excluded)", wl)
	}
	for _, id := range wl {
		if !want[id] {
			t.Errorf("whitelist contains unexpected model %q", id)
		}
	}
	// top-level model must be provider_id/model_id with native ID
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/openai.gpt-oss-120b-1:0" {
		t.Errorf("model = %q, want amazon-bedrock/openai.gpt-oss-120b-1:0", m)
	}
	// RC1: no juggernaut key in the vendor-validated config
	if _, ok := plan.Keys["juggernaut"]; ok {
		t.Error("opencode.json plan must NOT contain a top-level juggernaut key (OpenCode schema: additionalProperties=false)")
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan should validate: %v", err)
	}
}

// TestOpenCode_BuildConfig_NoCatalogOmitsWhitelist: with no discovered models
// (no cached catalog — the normal fresh-install case), the whitelist key must
// be ABSENT entirely. A nil slice marshals to null and an empty array would
// hide every model in OpenCode's picker; OpenCode's schema requires array or
// absent (RC2).
func TestOpenCode_BuildConfig_NoCatalogOmitsWhitelist(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	bm, ok := testutil.NestedMapChain(plan.Keys, "provider", "amazon-bedrock")
	if !ok {
		t.Fatalf("provider.amazon-bedrock missing")
	}
	bmMap, ok := bm.(map[string]any)
	if !ok {
		t.Fatalf("provider.amazon-bedrock not a map: %T", bm)
	}
	if wl, present := bmMap["whitelist"]; present {
		t.Errorf("whitelist must be omitted when discovery finds no supported models, got %#v", wl)
	}
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/openai.gpt-oss-120b-1:0" {
		t.Errorf("model = %q, want amazon-bedrock/openai.gpt-oss-120b-1:0", m)
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("plan should validate: %v", err)
	}
}

// TestOpenCode_BuildConfig_Aliases: common convenience names resolve to their
// native Bedrock model IDs without acting as the authoritative availability roster.
func TestOpenCode_BuildConfig_Aliases(t *testing.T) {
	cases := map[string]string{
		"glm-4.7":       "zai.glm-4.7",
		"glm-5":         "zai.glm-5",
		"kimi-k2.5":     "moonshotai.kimi-k2.5",
		"deepseek-v3.2": "deepseek.v3.2",
		"qwen3-coder":   "qwen.qwen3-coder-480b-a35b-v1:0",
		"grok-4.3":      "global.xai.grok-4.6",
		"grok-4.6":      "global.xai.grok-4.6",
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
		if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/"+wantID {
			t.Errorf("%s: model = %q, want amazon-bedrock/%s", key, m, wantID)
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
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/some.exotic-model-v9" {
		t.Errorf("passthrough model = %q, want verbatim id", m)
	}
	if len(plan.Warnings) == 0 {
		t.Error("passthrough (unverified) model should emit a warning")
	}
}
