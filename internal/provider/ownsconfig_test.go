package provider

import "testing"

// TestClaude_OwnsConfig: Claude recognizes its config by the managed juggernaut block.
func TestClaude_OwnsConfig(t *testing.T) {
	p, _ := Get("claude")
	if !p.OwnsConfig(map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
	}) {
		t.Error("claude should own a config with a juggernaut managed block")
	}
	// A bare model / unrelated keys are NOT ownership.
	if p.OwnsConfig(map[string]any{"model": "sonnet", "editor": "vim"}) {
		t.Error("claude must NOT claim a config lacking the juggernaut block")
	}
}

// TestCodex_OwnsConfig: Codex recognizes ONLY its amazon-bedrock provider
// routing, not a plain user config that merely has a `model` key. This is the
// P2 fix: a vanilla Codex config (model = "...") must NOT be mistaken for a
// Juggernaut-configured one.
func TestCodex_OwnsConfig(t *testing.T) {
	p, _ := Get("codex")
	if !p.OwnsConfig(map[string]any{
		"model":          "openai.gpt-5.5",
		"model_provider": "amazon-bedrock",
	}) {
		t.Error("codex should own a config routed to amazon-bedrock")
	}
	// Plain non-Juggernaut Codex config with just a model → NOT ours.
	if p.OwnsConfig(map[string]any{"model": "gpt-5.1-codex"}) {
		t.Error("codex must NOT claim a plain config that only has a model key")
	}
	// A different provider value → NOT ours.
	if p.OwnsConfig(map[string]any{"model": "x", "model_provider": "openai"}) {
		t.Error("codex must NOT claim a config pointing at a non-Mantle provider")
	}
}

// TestCodex_OwnsConfig_RuntimeProvider: the v6 built-in amazon-bedrock-runtime
// provider (and the v5 custom amazon-bedrock) are both recognized as ours —
// a re-apply over a migrated config must be a plain re-apply, not a fresh
// first-time auth prompt.
func TestCodex_OwnsConfig_RuntimeProvider(t *testing.T) {
	p, _ := Get("codex")
	if !p.OwnsConfig(map[string]any{
		"model":          "global.openai.gpt-5.6-sol",
		"model_provider": CodexBedrockRuntimeProviderID,
	}) {
		t.Error("codex should own a config routed to the built-in amazon-bedrock-runtime")
	}
}

// TestCodex_CleanLegacy: a migrate must delete the v5 custom
// model_providers.amazon-bedrock table (deep-merge would otherwise preserve
// it as a sibling, and it points at the dead Mantle endpoint), while keeping
// user-defined providers and dropping an emptied model_providers table.
func TestCodex_CleanLegacy(t *testing.T) {
	p, _ := Get("codex")
	// Legacy table alone → stripped, model_providers dropped.
	existing := map[string]any{
		"model_provider": CodexLegacyProviderID,
		"model_providers": map[string]any{
			CodexLegacyProviderID: map[string]any{"aws": map[string]any{"region": "us-west-2"}},
		},
	}
	p.(LegacyCleaner).CleanLegacy(existing)
	if _, ok := existing["model_providers"]; ok {
		t.Errorf("empty model_providers table should be dropped, got %v", existing["model_providers"])
	}
	// User's own provider must survive; only the legacy table goes.
	withUser := map[string]any{
		"model_providers": map[string]any{
			CodexLegacyProviderID: map[string]any{"aws": map[string]any{"region": "us-west-2"}},
			"my-own":              map[string]any{"base_url": "http://localhost:1/v1"},
		},
	}
	p.(LegacyCleaner).CleanLegacy(withUser)
	mp := withUser["model_providers"].(map[string]any)
	if _, ok := mp[CodexLegacyProviderID]; ok {
		t.Errorf("legacy table must be deleted")
	}
	if _, ok := mp["my-own"]; !ok {
		t.Error("user's own provider must survive")
	}
}

// TestCodex_OwnsConfig_OldBedrockMantle: the legacy custom bedrock-mantle
// provider (pre-amazon-bedrock) must NOT be claimed as currently owned.
// Plain apply still migrates it via isJuggernautLegacy (cmd/) — OwnsConfig
// stays false so a leftover Mantle table is not treated as a current v6
// re-apply. The leftover model_providers.bedrock-mantle sibling is stripped
// on migrate; deep merge alone would preserve it.
func TestCodex_OwnsConfig_OldBedrockMantle(t *testing.T) {
	p, _ := Get("codex")
	if p.OwnsConfig(map[string]any{
		"model":          "openai.gpt-5.5",
		"model_provider": "bedrock-mantle",
	}) {
		t.Error("codex must NOT claim old bedrock-mantle config (triggers fresh auth prompt)")
	}
}

// TestClaude_OwnsConfig_MalformedBlocks covers the defensive branches: a
// juggernaut key that isn't a map, or a block missing/!map meta, or a wrong
// owner, must not be mistaken for ownership.
func TestClaude_OwnsConfig_MalformedBlocks(t *testing.T) {
	p, _ := Get("claude")
	cases := []map[string]any{
		{},                               // no juggernaut key
		{"juggernaut": "not-a-map"},      // juggernaut not a map
		{"juggernaut": map[string]any{}}, // no meta
		{"juggernaut": map[string]any{"meta": "x"}},                                         // meta not a map
		{"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "someone-else"}}}, // wrong owner
	}
	for i, c := range cases {
		if p.OwnsConfig(c) {
			t.Errorf("case %d: claude must not claim ownership of %v", i, c)
		}
	}
}
