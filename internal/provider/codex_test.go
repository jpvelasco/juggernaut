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
