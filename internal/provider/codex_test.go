package provider

import (
	"runtime"
	"testing"
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

// --- Per-model table (all facts live-verified 2026-07-03 against Mantle) ---

// TestCodexModel_GPT55 pins the GPT-5.5 model ID and region availability.
func TestCodexModel_GPT55(t *testing.T) {
	m, ok := codexModel("gpt-5.5")
	if !ok {
		t.Fatal("gpt-5.5 not in Codex model table")
	}
	if m.ModelID != "openai.gpt-5.5" {
		t.Errorf("ModelID = %q, want openai.gpt-5.5", m.ModelID)
	}
	if len(m.Regions) == 0 {
		t.Error("gpt-5.5 should have non-empty regions")
	}
}

// TestCodexModel_GPTOSSExcluded: gpt-oss is NOT selectable for Codex. Current
// Codex is Responses-API-only (it rejects `wire_api = "chat"` at config load,
// see openai/codex CHAT_WIRE_API_REMOVED_ERROR), but gpt-oss on Mantle speaks
// only Chat Completions on /v1 — it has no Responses endpoint. So Codex cannot
// reach gpt-oss at all; offering it would write a config that fails to load.
// (OpenCode, which does speak Chat, still curates gpt-oss — that's unaffected.)
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
// non-empty region info — an empty list would cause resolveMantleRegion to
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

// TestCodex_DefaultModel is GPT-5.5 (the flagship, mirroring native Codex).
func TestCodex_DefaultModel(t *testing.T) {
	if got := codexDefaultModel(); got != "gpt-5.5" {
		t.Errorf("default Codex model = %q, want gpt-5.5", got)
	}
}

// TestCodex_OwnedSubKeys_LeafRegion: OwnedSubKeys must target the region leaf
// (amazon-bedrock.aws.region), not the entire aws sub-table. Users may configure
// their own profile/credentials under aws — uninstall must preserve them.
func TestCodex_OwnedSubKeys_LeafRegion(t *testing.T) {
	p, _ := Get("codex")
	subs := p.OwnedSubKeys()
	keys, ok := subs["model_providers"]
	if !ok {
		t.Fatal("model_providers not in OwnedSubKeys")
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 owned sub-key, got %d: %v", len(keys), keys)
	}
	if keys[0] != "amazon-bedrock.aws.region" {
		t.Errorf("owned sub-key = %q, want amazon-bedrock.aws.region (leaf-level to preserve user aws subkeys)", keys[0])
	}
}
