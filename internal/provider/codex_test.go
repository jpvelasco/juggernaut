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

// TestCodexModel_GPT55 pins the load-bearing GPT-5.5 facts: Responses-only on the
// /openai/v1 base path. Getting wire_api wrong is a live-confirmed 400.
func TestCodexModel_GPT55(t *testing.T) {
	m, ok := codexModel("gpt-5.5")
	if !ok {
		t.Fatal("gpt-5.5 not in Codex model table")
	}
	if m.ModelID != "openai.gpt-5.5" {
		t.Errorf("ModelID = %q, want openai.gpt-5.5", m.ModelID)
	}
	if m.BasePath != "/openai/v1" {
		t.Errorf("BasePath = %q, want /openai/v1", m.BasePath)
	}
	if m.WireAPI != "responses" {
		t.Errorf("WireAPI = %q, want responses (live: gpt-5.5 rejects chat)", m.WireAPI)
	}
}

// TestCodexModel_GPTOSS pins gpt-oss-120b: Chat on the /v1 base path.
func TestCodexModel_GPTOSS(t *testing.T) {
	m, ok := codexModel("gpt-oss-120b")
	if !ok {
		t.Fatal("gpt-oss-120b not in Codex model table")
	}
	if m.ModelID != "openai.gpt-oss-120b" {
		t.Errorf("ModelID = %q, want openai.gpt-oss-120b", m.ModelID)
	}
	if m.BasePath != "/v1" {
		t.Errorf("BasePath = %q, want /v1", m.BasePath)
	}
	if m.WireAPI != "chat" {
		t.Errorf("WireAPI = %q, want chat", m.WireAPI)
	}
}

// TestCodexModel_Unknown returns not-ok for an unlisted model.
func TestCodexModel_Unknown(t *testing.T) {
	if _, ok := codexModel("gpt-nonesuch"); ok {
		t.Error("expected unknown model to be absent from table")
	}
}

// TestCodexModel_PathsMatchVerifiedSplit guards the core finding that base path
// is per-model: gpt-5.x on /openai/v1, gpt-oss on /v1.
func TestCodexModel_PathsMatchVerifiedSplit(t *testing.T) {
	cases := map[string]string{
		"gpt-5.5":      "/openai/v1",
		"gpt-5.4":      "/openai/v1",
		"gpt-oss-120b": "/v1",
		"gpt-oss-20b":  "/v1",
	}
	for key, wantPath := range cases {
		m, ok := codexModel(key)
		if !ok {
			t.Errorf("%s missing from table", key)
			continue
		}
		if m.BasePath != wantPath {
			t.Errorf("%s BasePath = %q, want %q", key, m.BasePath, wantPath)
		}
	}
}

// TestCodex_DefaultModel is GPT-5.5 (the flagship, mirroring native Codex).
func TestCodex_DefaultModel(t *testing.T) {
	if got := codexDefaultModel(); got != "gpt-5.5" {
		t.Errorf("default Codex model = %q, want gpt-5.5", got)
	}
}
