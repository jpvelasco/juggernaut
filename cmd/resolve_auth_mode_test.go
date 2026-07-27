package cmd

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func mustProvider(t *testing.T, name string) provider.Provider {
	t.Helper()
	p, err := provider.Get(name)
	if err != nil {
		t.Fatalf("provider.Get(%q): %v", name, err)
	}
	return p
}

func TestResolveAuthMode_Table(t *testing.T) {
	claude := mustProvider(t, "claude")
	codex := mustProvider(t, "codex")

	bCfgIAM := &bedrock.Config{
		Defaults: bedrock.Defaults{AuthMode: authmode.IAM, Region: "us-west-2"},
	}
	bCfgAPI := &bedrock.Config{
		Defaults: bedrock.Defaults{AuthMode: authmode.BedrockAPIKey, Region: "us-west-2"},
	}

	existingIAM := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": authmode.IAM},
		},
	}
	existingAPI := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": authmode.BedrockAPIKey},
		},
	}
	existingEmptyAuth := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": ""},
		},
	}
	existingNoAuth := map[string]any{
		"juggernaut": map[string]any{},
	}

	for _, tc := range []struct {
		name     string
		flagVal  string
		prov     provider.Provider
		bCfg     *bedrock.Config
		existing map[string]any
		want     string
	}{
		// Flag value always wins
		{
			name: "flag_IAM_wins_Claude", flagVal: authmode.IAM,
			prov: claude, bCfg: bCfgAPI, existing: existingAPI,
			want: authmode.IAM,
		},
		{
			name: "flag_bedrock_api_key_wins_Claude", flagVal: authmode.BedrockAPIKey,
			prov: claude, bCfg: bCfgIAM, existing: existingIAM,
			want: authmode.BedrockAPIKey,
		},
		{
			name: "flag_wins_Codex", flagVal: authmode.BedrockAPIKey,
			prov: codex, bCfg: bCfgIAM, existing: nil,
			want: authmode.BedrockAPIKey,
		},
		// Codex supports CapNativeAuth — reads from existing block
		{
			name: "Codex_no_flag_existing_IAM", flagVal: "",
			prov: codex, bCfg: bCfgAPI, existing: existingIAM,
			want: authmode.IAM,
		},
		// Mantle-only CLIs (OpenCode, Grok) default to BedrockAPIKey when no flag
		{
			name: "OpenCode_no_flag_defaults_bedrock_api_key", flagVal: "",
			prov: mustProvider(t, "opencode"), bCfg: bCfgIAM, existing: nil,
			want: authmode.BedrockAPIKey,
		},
		{
			name: "Grok_no_flag_defaults_bedrock_api_key", flagVal: "",
			prov: mustProvider(t, "grok"), bCfg: bCfgIAM, existing: nil,
			want: authmode.BedrockAPIKey,
		},
		// Claude with no flag reads from existing block
		{
			name: "Claude_no_flag_existing_IAM", flagVal: "",
			prov: claude, bCfg: bCfgAPI, existing: existingIAM,
			want: authmode.IAM,
		},
		{
			name: "Claude_no_flag_existing_bedrock_api_key", flagVal: "",
			prov: claude, bCfg: bCfgIAM, existing: existingAPI,
			want: authmode.BedrockAPIKey,
		},
		// Empty mode in existing block falls through to bCfg default
		{
			name: "Claude_empty_mode_in_block_falls_to_bCfg", flagVal: "",
			prov: claude, bCfg: bCfgIAM, existing: existingEmptyAuth,
			want: authmode.IAM,
		},
		{
			name: "Claude_no_auth_in_block_falls_to_bCfg", flagVal: "",
			prov: claude, bCfg: bCfgAPI, existing: existingNoAuth,
			want: authmode.BedrockAPIKey,
		},
		// No existing config falls through to bCfg default
		{
			name: "Claude_no_flag_no_existing_uses_bCfg_IAM", flagVal: "",
			prov: claude, bCfg: bCfgIAM, existing: nil,
			want: authmode.IAM,
		},
		{
			name: "Claude_no_flag_no_existing_uses_bCfg_API", flagVal: "",
			prov: claude, bCfg: bCfgAPI, existing: nil,
			want: authmode.BedrockAPIKey,
		},
		{
			name: "Claude_no_flag_empty_existing_uses_bCfg", flagVal: "",
			prov: claude, bCfg: bCfgIAM, existing: map[string]any{},
			want: authmode.IAM,
		},
		// Nil bCfg returns empty string (shouldn't happen in practice)
		{
			name: "Claude_no_flag_nil_bCfg_empty", flagVal: "",
			prov: claude, bCfg: nil, existing: nil,
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAuthMode(tc.flagVal, tc.prov, tc.bCfg, tc.existing)
			if got != tc.want {
				t.Errorf("resolveAuthMode(%q, %s, bCfg=%v, existing=%v) = %q, want %q",
					tc.flagVal, tc.prov.Name(), tc.bCfg != nil, tc.existing != nil, got, tc.want)
			}
		})
	}
}

func TestDetectReapplyConfig_MissingFile(t *testing.T) {
	home := testutil.NewTestHome(t)
	prov := mustProvider(t, "claude")

	existing, isReapply, err := detectReapplyConfig(prov, home, "user")
	if err != nil {
		t.Fatalf("detectReapplyConfig error: %v", err)
	}
	if isReapply {
		t.Error("missing file should not be a re-apply")
	}
	if existing != nil {
		t.Error("expected nil existing for missing file")
	}
}

func TestDetectReapplyConfig_OwnedConfig(t *testing.T) {
	home := setupApplyTest(t)

	// First apply to create an owned config
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}

	prov := mustProvider(t, "claude")
	existing, isReapply, err := detectReapplyConfig(prov, home, "user")
	if err != nil {
		t.Fatalf("detectReapplyConfig error: %v", err)
	}
	if !isReapply {
		t.Error("owned config should be a re-apply")
	}
	if existing == nil {
		t.Error("expected non-nil existing for owned config")
	}
}

func TestDetectReapplyConfig_ForeignConfig(t *testing.T) {
	home := setupApplyTest(t)

	// Write a foreign config (no juggernaut block)
	prov := mustProvider(t, "claude")
	mgr, err := newProviderManager(prov, home, "user")
	if err != nil {
		t.Fatalf("newProviderManager error: %v", err)
	}
	_ = mgr.Write(map[string]any{
		"env": map[string]any{"MY_VAR": "value"},
	})

	existing, isReapply, err := detectReapplyConfig(prov, home, "user")
	if err != nil {
		t.Fatalf("detectReapplyConfig error: %v", err)
	}
	if isReapply {
		t.Error("foreign config should not be a re-apply")
	}
	if existing == nil {
		t.Error("expected non-nil existing for foreign config")
	}
}
