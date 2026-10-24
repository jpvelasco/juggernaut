package provider

import (
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestGet_Codex confirms the registry resolves the Codex provider.
func TestGet_Codex(t *testing.T) {
	p, err := Get("codex")
	if err != nil {
		t.Fatalf("Get(codex): %v", err)
	}
	if p.Name() != "codex" {
		t.Errorf("Name() = %q, want codex", p.Name())
	}
}

// TestCodex_ConfigFormatIsTOML pins the on-disk format (~/.codex/config.toml).
func TestCodex_ConfigFormatIsTOML(t *testing.T) {
	p, _ := Get("codex")
	if p.ConfigFormatName() != "toml" {
		t.Errorf("config format = %q, want toml", p.ConfigFormatName())
	}
}

// TestCodex_BinaryNames pins the real executable names.
func TestCodex_BinaryNames(t *testing.T) {
	p, _ := Get("codex")
	got := p.BinaryNames()
	if runtime.GOOS == "windows" {
		if len(got) == 0 || got[0] != "codex.exe" {
			t.Errorf("windows binary names = %v, want codex.exe first", got)
		}
	} else if len(got) != 1 || got[0] != "codex" {
		t.Errorf("unix binary names = %v, want [codex]", got)
	}
}

// TestCodex_ActivationMarkers are Codex-specific so they never collide with the
// Claude activation block in the same shell profile.
func TestCodex_ActivationMarkers(t *testing.T) {
	p, _ := Get("codex")
	begin, end := p.ActivationMarkers()
	if begin != "# BEGIN: Juggernaut Codex Activation" {
		t.Errorf("begin marker = %q", begin)
	}
	if end != "# END: Juggernaut Codex Activation" {
		t.Errorf("end marker = %q", end)
	}
	// Must differ from Claude's markers (shared-profile safety).
	cb, _ := (claude{}).ActivationMarkers()
	if begin == cb {
		t.Error("Codex markers must not equal Claude markers")
	}
}

// --- Per-model table (all facts live-verified 2026-10-19 native Bedrock) ---

// TestCodexModel_CurrentGeneration pins the refreshed roster: sol/luna/astra
// track the GPT-6 generation and terra stays on GPT-5.6 (there is no GPT-6
// Terra). Regions[0] must stay us-east-1 — it is the Iron Fist fallback.
func TestCodexModel_CurrentGeneration(t *testing.T) {
	want := map[string]string{
		"sol":   "global.openai.gpt-6.1-sol",
		"luna":  "global.openai.gpt-6-luna",
		"astra": "global.openai.gpt-6-astra",
		"terra": "global.openai.gpt-5.6-terra",
	}
	for key, wantID := range want {
		m, ok := codexModel(key)
		if !ok {
			t.Errorf("%s not in Codex model table", key)
			continue
		}
		if m.ModelID != wantID {
			t.Errorf("%s ModelID = %q, want %q", key, m.ModelID, wantID)
		}
		if len(m.Regions) == 0 {
			t.Errorf("%s should have non-empty regions", key)
			continue
		}
		if m.Regions[0] != "us-east-1" {
			t.Errorf("%s Regions[0] = %q, want us-east-1 (Iron Fist fallback)", key, m.Regions[0])
		}
	}
	// Old gpt-5.5 should be retired.
	if _, ok := codexModel("gpt-5.5"); ok {
		t.Error("gpt-5.5 should no longer be in Codex model table (retired)")
	}
}

// TestCodexModel_GPTOSSExcluded: gpt-oss is NOT selectable for Codex. Current
// Codex is Responses-API-only (it rejects `wire_api = "chat"` at config load,
// see openai/codex CHAT_WIRE_API_REMOVED_ERROR), but gpt-oss on Mantle speaks
// only Chat Completions on /v1 — it has no Responses endpoint. So Codex cannot
// reach gpt-oss at all; offering it would write a config that fails to load.
// (OpenCode, which does speak Chat, still supports gpt-oss — that's unaffected.)
func TestCodexModel_GPTOSSExcluded(t *testing.T) {
	for _, key := range []string{"gpt-oss-120b", "gpt-oss-20b"} {
		if _, ok := codexModel(key); ok {
			t.Errorf("%s must NOT be a Codex model (Codex is Responses-only; gpt-oss is Chat-only)", key)
		}
	}
}

// TestCodexModel_Unknown returns not-ok for an unlisted model.
func TestCodexModel_Unknown(t *testing.T) {
	if _, ok := codexModel("gpt-nonesuch"); ok {
		t.Error("expected unknown model to be absent from table")
	}
}

// TestCodexModel_AllModelsHaveRegions guards that every Codex model has
// non-empty region info — an empty list would cause resolveModelRegion to
// silently pass through any user region, potentially writing an unreachable config.
func TestCodexModel_AllModelsHaveRegions(t *testing.T) {
	for key, m := range codexModels {
		if len(m.Regions) == 0 {
			t.Errorf("%s has no regions (will skip region enforcement)", key)
		}
		if m.ModelID == "" {
			t.Errorf("%s has empty ModelID", key)
		}
	}
}

// TestCodex_DefaultModel is sol (the GPT-6.1 flagship).
func TestCodex_DefaultModel(t *testing.T) {
	if got := codexDefaultModel(); got != "sol" {
		t.Errorf("default Codex model = %q, want sol", got)
	}
}

// TestCodex_OwnedSubKeys_LeafRegion: OwnedSubKeys must target the region leaf
// under both the current amazon-bedrock-runtime.aws and the legacy v5
// amazon-bedrock.aws, not the entire aws sub-table. Users may configure their
// own profile/credentials under aws — uninstall must preserve them.
func TestCodex_OwnedSubKeys_LeafRegion(t *testing.T) {
	p, _ := Get("codex")
	subs := p.OwnedSubKeys()
	keys, ok := subs["model_providers"]
	if !ok {
		t.Fatal("model_providers not in OwnedSubKeys")
	}
	want := map[string]bool{
		"amazon-bedrock-runtime.aws.region": false,
		"amazon-bedrock.aws.region":         false,
	}
	for _, k := range keys {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected owned sub-key %q (want only the two region leaves): %v", k, keys)
		}
		want[k] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("owned sub-key %q missing (leaf-level, to preserve user aws subkeys): got %v", k, keys)
		}
	}
}

// TestCodex_BuildConfig_PreservesExistingDefaultModel: a plain apply over a
// runtime-routed config keeps a resolvable, runtime-shaped default instead of
// resetting it to sol.
func TestCodex_BuildConfig_PreservesExistingDefaultModel(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Existing = map[string]any{
		"model":          "global.openai.gpt-6-luna",
		"model_provider": CodexBedrockRuntimeProviderID,
	}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if _, present := plan.Keys["model"]; present {
		t.Errorf("plan must not write the root model when preserving, got: %v", plan.Keys["model"])
	}
	for _, k := range plan.ManagedKeys {
		if k == "model" {
			t.Errorf("plan must not claim the model key when preserving, got: %v", plan.ManagedKeys)
		}
	}
	if len(plan.Warnings) == 0 || !strings.Contains(plan.Warnings[len(plan.Warnings)-1], "preserved") {
		t.Errorf("plan should report the preserved default, got: %v", plan.Warnings)
	}
	if plan.Keys["model_provider"] != CodexBedrockRuntimeProviderID {
		t.Errorf("routing keys must still be written, got: %v", plan.Keys)
	}
}

// TestCodex_BuildConfig_ReplacesUnservableDefault: a foreign default (the
// user's own OpenAI model) can't be served through the Bedrock runtime — a
// plain apply replaces it with the working convenience default instead of
// preserving a guaranteed 404.
func TestCodex_BuildConfig_ReplacesUnservableDefault(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Existing = map[string]any{
		"model":          "gpt-5.2-codex",
		"model_provider": CodexBedrockRuntimeProviderID,
	}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if m, _ := plan.Keys["model"].(string); m != "global.openai.gpt-6.1-sol" {
		t.Errorf("unservable default must reset to sol, got: %v", plan.Keys["model"])
	}
}

// TestCodex_BuildConfig_ReplacesStaleShapes: cross-endpoint model shapes and
// v5 legacy routing are migrated, not preserved.
func TestCodex_BuildConfig_ReplacesStaleShapes(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		existing map[string]any
	}{
		{
			"runtime apply over mantle-shaped model", "",
			map[string]any{"model": "openai.gpt-6-astra", "model_provider": CodexBedrockRuntimeProviderID},
		},
		{
			"mantle apply over runtime-shaped model", "mantle",
			map[string]any{"model": "global.openai.gpt-6-astra", "model_provider": CodexLegacyProviderID},
		},
		{
			"runtime apply over v5 legacy routing", "",
			map[string]any{"model": "global.openai.gpt-5.6-sol", "model_provider": CodexLegacyProviderID},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := Get("codex")
			opts := baseOpts()
			opts.Endpoint = c.endpoint
			opts.Existing = c.existing

			plan, err := p.BuildConfig(testConfig(), opts)
			if err != nil {
				t.Fatalf("BuildConfig: %v", err)
			}
			m, _ := plan.Keys["model"].(string)
			if m == "" {
				t.Fatalf("stale model must be replaced, not preserved; keys: %v", plan.Keys)
			}
			if m == c.existing["model"] {
				t.Errorf("stale model %v must not be written back", c.existing["model"])
			}
		})
	}
}

// TestCodex_Mantle_PreservesExistingDefaultModel: the mantle endpoint has the
// same contract — a mantle-shaped default survives a plain mantle apply.
func TestCodex_Mantle_PreservesExistingDefaultModel(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Endpoint = "mantle"
	opts.Existing = map[string]any{
		"model":          "openai.gpt-6-astra",
		"model_provider": CodexLegacyProviderID,
	}

	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig (mantle): %v", err)
	}
	if _, present := plan.Keys["model"]; present {
		t.Errorf("mantle plan must not write the root model when preserving, got: %v", plan.Keys["model"])
	}
}

// TestCodex_OwnsManagedValue: ownership of the root "model" is record-based —
// Juggernaut owns it only while it still equals what the last apply wrote.
func TestCodex_OwnsManagedValue(t *testing.T) {
	home := testutil.NewTestHome(t)
	p, _ := Get("codex")
	if err := WriteSidecar(p, home, Options{
		Scope: "user", AuthMode: "iam", Region: "us-west-2", Version: "test",
		WrittenModel: "global.openai.gpt-6.1-sol",
	}); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{"written default still equals record", "global.openai.gpt-6.1-sol", true},
		{"default later edited by user", "global.openai.gpt-6-luna", false},
		{"foreign default never written", "gpt-5.2-codex", false},
		{"empty model", "", false},
		{"malformed model value", 42, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OwnsManagedValueFor(p, home, "user", "model", c.value); got != c.want {
				t.Errorf("OwnsManagedValue(%v) = %v, want %v", c.value, got, c.want)
			}
		})
	}
	// Non-model managed keys are owned regardless of value.
	if !OwnsManagedValueFor(p, home, "user", "model_provider", "anything") {
		t.Error("non-model managed keys must be owned")
	}
}

// TestCodex_OwnsManagedValue_NoRecord: without a sidecar record (a
// pre-record install or a preserved default), any value in the ID space
// Juggernaut writes for Codex is owned — the provider entry uninstall
// removes is what made it routable. A foreign value stays the user's.
func TestCodex_OwnsManagedValue_NoRecord(t *testing.T) {
	home := testutil.NewTestHome(t)
	p, _ := Get("codex")
	cases := []struct {
		value any
		want  bool
	}{
		{"global.openai.gpt-6.1-sol", true}, // the pre-record seeded default
		{"global.openai.gpt-6-luna", true},  // any roster ID from an explicit pre-record pin
		{"openai.gpt-6-astra", true},        // the mantle-era default's bare shape
		{"gpt-5.2-codex", false},            // the user's own OpenAI model
		{"", false},
	}
	for _, c := range cases {
		if got := OwnsManagedValueFor(p, home, "user", "model", c.value); got != c.want {
			t.Errorf("OwnsManagedValue(no record, %v) = %v, want %v", c.value, got, c.want)
		}
	}
}
