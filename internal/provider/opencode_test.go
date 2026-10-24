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
		{ID: "openai.gpt-5.6-sol", Status: "ACTIVE", Source: "mantle"},     // excluded: legacy mantle source
		{ID: "google.gemma-4-31b", Status: "ACTIVE", Source: "foundation"}, // excluded: mantle-only
		{ID: "openai.gpt-5.5", Status: "ACTIVE", Source: "foundation"},     // excluded: mantle-only
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
		"glm-5.3":       "global.zai.glm-5.3",
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

// TestOpenCode_BuildConfig_PreservesExistingDefaultModel: the root "model" is
// OpenCode's own native default, so a plain apply (--model omitted) over a
// config that already has one keeps it instead of pinning Juggernaut's
// convenience default. The plan then must not claim the key.
func TestOpenCode_BuildConfig_PreservesExistingDefaultModel(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Existing = map[string]any{"model": "batwing-coder/qwen-coder"}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if _, present := plan.Keys["model"]; present {
		t.Errorf("plan must not write the root model key when preserving, got: %v", plan.Keys["model"])
	}
	assertManagedKeys(t, plan, []string{"provider"})
	found := false
	for _, w := range plan.Warnings {
		if strings.Contains(w, "batwing-coder/qwen-coder") && strings.Contains(w, "preserved") {
			found = true
		}
	}
	if !found {
		t.Errorf("plan should warn that the existing default is preserved, got: %v", plan.Warnings)
	}
}

// TestOpenCode_BuildConfig_WritesDefaultWhenNoModelExists: a plain apply on a
// fresh (or model-less) config still seeds the convenience default —
// preservation only applies when a default exists to preserve.
func TestOpenCode_BuildConfig_WritesDefaultWhenNoModelExists(t *testing.T) {
	p, _ := Get("opencode")
	plan, err := p.BuildConfig(testConfig(), baseOpts()) // Existing nil, Model ""
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/openai.gpt-oss-120b-1:0" {
		t.Errorf("default model = %v, want amazon-bedrock/openai.gpt-oss-120b-1:0", plan.Keys["model"])
	}
	assertManagedKeys(t, plan, []string{"model", "provider"})
}

// TestOpenCode_BuildConfig_ExplicitModelOverridesPreservation: --model claims
// the root key; an existing default (even a foreign one) does not survive it.
func TestOpenCode_BuildConfig_ExplicitModelOverridesPreservation(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Model = "grok-4.6"
	opts.Existing = map[string]any{"model": "batwing-coder/qwen-coder"}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/global.xai.grok-4.6" {
		t.Errorf("explicit model = %v, want amazon-bedrock/global.xai.grok-4.6", plan.Keys["model"])
	}
	assertManagedKeys(t, plan, []string{"model", "provider"})
}

// TestOpenCode_BuildConfig_ReplacesStaleModels: a root model pointing at the
// provider entry this apply removes would dangle, so it is replaced with the
// apply's default instead of preserved. Covers the cross-endpoint switch and
// the v5 bedrock-mantle leftover for both endpoints.
func TestOpenCode_BuildConfig_ReplacesStaleModels(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		existing string
		wantKeys []string
	}{
		{"runtime apply over mantle model", "", "amazon-bedrock-mantle/openai.gpt-6-astra", []string{"model", "provider"}},
		{"runtime apply over v5 mantle model", "", "bedrock-mantle/openai.gpt-oss-120b-1:0", []string{"model", "provider"}},
		{"mantle apply over runtime model", "mantle", "amazon-bedrock/openai.gpt-oss-120b-1:0", []string{"model", "providers"}},
		{"mantle apply over v5 mantle model", "mantle", "bedrock-mantle/openai.gpt-oss-120b-1:0", []string{"model", "providers"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := Get("opencode")
			opts := baseOpts()
			opts.Endpoint = c.endpoint
			opts.Existing = map[string]any{"model": c.existing}

			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig: %v", err)
			}
			m, _ := plan.Keys["model"].(string)
			if m == "" {
				t.Fatalf("stale model %q must be replaced, not preserved; keys: %v", c.existing, plan.Keys)
			}
			if strings.Contains(m, c.existing) {
				t.Errorf("stale model %q must not be written back", c.existing)
			}
			assertManagedKeys(t, plan, c.wantKeys)
		})
	}
}

// TestOpenCode_Mantle_PreservesExistingDefaultModel: the mantle endpoint has
// the same contract — a foreign default survives a plain mantle apply.
func TestOpenCode_Mantle_PreservesExistingDefaultModel(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Endpoint = "mantle"
	opts.Existing = map[string]any{"model": "batwing-coder/qwen-coder"}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig (mantle): %v", err)
	}
	if _, present := plan.Keys["model"]; present {
		t.Errorf("mantle plan must not write the root model key when preserving, got: %v", plan.Keys["model"])
	}
	assertManagedKeys(t, plan, []string{"providers"})
}

// TestOpenCode_Mantle_ReplacesStaleRuntimeModel: switching an owned config to
// mantle replaces a runtime-prefixed default with the mantle default.
func TestOpenCode_Mantle_ReplacesStaleRuntimeModel(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Endpoint = "mantle"
	opts.Existing = map[string]any{"model": "amazon-bedrock/openai.gpt-oss-120b-1:0"}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig (mantle): %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock-mantle/openai.gpt-6-astra" {
		t.Errorf("mantle default = %v, want amazon-bedrock-mantle/openai.gpt-6-astra", plan.Keys["model"])
	}
	assertManagedKeys(t, plan, []string{"model", "providers"})
}

// TestOpenCode_NonStringModelNotPreserved: a malformed non-string root model
// is not a default to preserve; the plain default is written.
func TestOpenCode_NonStringModelNotPreserved(t *testing.T) {
	p, _ := Get("opencode")
	opts := baseOpts()
	opts.Existing = map[string]any{"model": 42}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "amazon-bedrock/openai.gpt-oss-120b-1:0" {
		t.Errorf("default model = %v, want amazon-bedrock/openai.gpt-oss-120b-1:0", plan.Keys["model"])
	}
}

// TestOpenCode_OwnsManagedValue: uninstall ownership of the root "model" is
// record-based — Juggernaut owns the default only while it still equals what
// the last apply wrote (sidecar record), or when it points at a
// Juggernaut-exclusive provider entry. A native runtime default
// (amazon-bedrock is OpenCode's own built-in provider id) is never claimed
// without a matching record.
func TestOpenCode_OwnsManagedValue(t *testing.T) {
	home := testutil.NewTestHome(t)
	p, _ := Get("opencode")
	// An apply that wrote a default records it in the auth sidecar.
	if err := WriteSidecar(p, home, Options{
		Scope: "user", AuthMode: "iam", Region: "us-west-2", Version: "test",
		WrittenModel: "amazon-bedrock/global.zai.glm-5.3",
	}); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	cases := []struct {
		name  string
		key   string
		value any
		want  bool
	}{
		{"written default still equals record", "model", "amazon-bedrock/global.zai.glm-5.3", true},
		{"pinned default later edited by user", "model", "amazon-bedrock/openai.gpt-oss-120b-1:0", false},
		{"native runtime default never written", "model", "amazon-bedrock/anthropic.claude-sonnet-5-5", false},
		{"foreign default", "model", "batwing-coder/qwen-coder", false},
		{"mantle default", "model", "amazon-bedrock-mantle/openai.gpt-6-astra", true},
		{"v5 mantle leftover", "model", "bedrock-mantle/openai.gpt-oss-120b-1:0", true},
		{"empty model is nothing to remove", "model", "", false},
		{"malformed model value", "model", 42, true},
		{"absent model value", "model", nil, true},
		{"other managed key", "provider", map[string]any{}, true},
		{"providers key", "providers", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OwnsManagedValueFor(p, home, "user", c.key, c.value); got != c.want {
				t.Errorf("OwnsManagedValue(%q, %v) = %v, want %v", c.key, c.value, got, c.want)
			}
		})
	}
}

// TestOpenCode_OwnsManagedValue_NoRecord: without a sidecar record (a
// pre-record install or a preserved default), only the seeded convenience
// default and Juggernaut-exclusive prefixes are owned; a native
// amazon-bedrock default and foreign values stay the user's.
func TestOpenCode_OwnsManagedValue_NoRecord(t *testing.T) {
	home := testutil.NewTestHome(t)
	p, _ := Get("opencode")
	cases := []struct {
		value any
		want  bool
	}{
		{"amazon-bedrock/openai.gpt-oss-120b-1:0", true}, // the pre-record seeded default
		{"amazon-bedrock/anthropic.claude-sonnet-5-5", false},
		{"amazon-bedrock/global.zai.glm-5.3", false},
		{"amazon-bedrock-mantle/openai.gpt-6-astra", true},
		{"bedrock-mantle/openai.gpt-oss-120b-1:0", true},
		{"batwing-coder/qwen-coder", false},
	}
	for _, c := range cases {
		if got := OwnsManagedValueFor(p, home, "user", "model", c.value); got != c.want {
			t.Errorf("OwnsManagedValue(no record, %v) = %v, want %v", c.value, got, c.want)
		}
	}
}

// TestOwnsManagedValueFor_DefaultOwnsEverything: providers without the
// ManagedValueOwner extension own every value of their managed keys (the
// pre-extension behavior).
func TestOwnsManagedValueFor_DefaultOwnsEverything(t *testing.T) {
	p, _ := Get("claude")
	home := testutil.NewTestHome(t)
	if !OwnsManagedValueFor(p, home, "user", "model", "whatever-the-user-set") {
		t.Error("claude does not implement ManagedValueOwner; every value must be owned")
	}
}

func assertManagedKeys(t *testing.T, plan ConfigPlan, want []string) {
	t.Helper()
	if len(plan.ManagedKeys) != len(want) {
		t.Fatalf("ManagedKeys = %v, want %v", plan.ManagedKeys, want)
	}
	for i, k := range plan.ManagedKeys {
		if k != want[i] {
			t.Fatalf("ManagedKeys = %v, want %v", plan.ManagedKeys, want)
		}
	}
}
