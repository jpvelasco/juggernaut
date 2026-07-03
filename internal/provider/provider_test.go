package provider

import (
	"runtime"
	"testing"
)

// TestGet_Claude confirms the registry resolves the default Claude provider.
func TestGet_Claude(t *testing.T) {
	p, err := Get("claude")
	if err != nil {
		t.Fatalf("Get(claude): %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("Name() = %q, want claude", p.Name())
	}
}

// TestGet_Default confirms an empty name resolves to Claude (back-compat: every
// existing caller passes no --cli and must keep getting Claude behavior).
func TestGet_Default(t *testing.T) {
	p, err := Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if p.Name() != "claude" {
		t.Errorf("default provider = %q, want claude", p.Name())
	}
}

// TestGet_Unknown rejects an unregistered CLI with an actionable error.
func TestGet_Unknown(t *testing.T) {
	_, err := Get("nonesuch")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

// TestClaude_ManagedKeysMatchLegacy pins the exact native managed-key set the
// config package hardcoded before the provider seam, so uninstall/merge behavior
// is unchanged.
func TestClaude_ManagedKeysMatchLegacy(t *testing.T) {
	p, _ := Get("claude")
	want := []string{
		"env", "model", "modelOverrides", "fallbackModel",
		"effortLevel", "alwaysThinkingEnabled", "skipWebFetchPreflight",
	}
	got := p.NativeManagedKeys()
	if len(got) != len(want) {
		t.Fatalf("managed keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("managed key[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestClaude_ActivationMarkers pins the exact marker strings activation.go used,
// so shell-profile block detection/removal is unchanged.
func TestClaude_ActivationMarkers(t *testing.T) {
	p, _ := Get("claude")
	begin, end := p.ActivationMarkers()
	if begin != "# BEGIN: Juggernaut Claude Activation" {
		t.Errorf("begin marker = %q", begin)
	}
	if end != "# END: Juggernaut Claude Activation" {
		t.Errorf("end marker = %q", end)
	}
}

// TestClaude_BinaryNames pins the platform-specific binary names activation.go
// searched for when resolving the real Claude Code executable.
func TestClaude_BinaryNames(t *testing.T) {
	p, _ := Get("claude")
	got := p.BinaryNames()
	if runtime.GOOS == "windows" {
		want := []string{"claude.exe", "claude.cmd", "claude.bat"}
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("windows binary names = %v, want %v", got, want)
		}
	} else if len(got) != 1 || got[0] != "claude" {
		t.Errorf("unix binary names = %v, want [claude]", got)
	}
}

// TestClaude_BedrockEnvVar pins the launch-time Bedrock activation env var.
func TestClaude_BedrockEnvVar(t *testing.T) {
	p, _ := Get("claude")
	k, v := p.BedrockEnvVar()
	if k != "CLAUDE_CODE_USE_BEDROCK" || v != "1" {
		t.Errorf("BedrockEnvVar() = (%q,%q), want (CLAUDE_CODE_USE_BEDROCK,1)", k, v)
	}
}

// TestClaude_ConfigFormatIsJSON confirms Claude writes JSON.
func TestClaude_ConfigFormatIsJSON(t *testing.T) {
	p, _ := Get("claude")
	if p.ConfigFormatName() != "json" {
		t.Errorf("config format = %q, want json", p.ConfigFormatName())
	}
}
