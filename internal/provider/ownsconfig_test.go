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

// TestCodex_OwnsConfig: Codex recognizes ONLY its Bedrock-Mantle routing, not a
// plain user config that merely has a `model` key. This is the P2 fix: a vanilla
// Codex config (model = "...") must NOT be mistaken for a Juggernaut-configured one.
func TestCodex_OwnsConfig(t *testing.T) {
	p, _ := Get("codex")
	if !p.OwnsConfig(map[string]any{
		"model":          "openai.gpt-5.5",
		"model_provider": "bedrock-mantle",
	}) {
		t.Error("codex should own a config routed to bedrock-mantle")
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
