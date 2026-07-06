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
