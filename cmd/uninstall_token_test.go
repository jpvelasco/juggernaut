package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// Given a Bedrock bearer token shared by every configured provider,
// When the user removes ONLY Claude's project-scope block,
// Then the shared token must survive: user-scope Claude and any non-Claude
// provider configs still reference it.
func TestUninstall_ProjectScope_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--scope=project", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after project-scope uninstall: %v", err)
	}
	if got != "shared-token" {
		t.Errorf("project-scope uninstall deleted the shared bearer token (got %q)\noutput:\n%s", got, out)
	}
}

// Given Claude is uninstalled outright (no scope restriction),
// When the uninstall completes,
// Then the primary's bearer token is removed as before.
func TestUninstall_FullClaude_RemovesToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("doomed-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after full uninstall: %v", err)
	}
	if got != "" {
		t.Errorf("full Claude uninstall should remove the bearer token, got %q", got)
	}
}

// Given a second CLI (Codex) is configured for Bedrock with a Juggernaut-owned
// config, and the user uninstalls Claude in full,
// Then the shared bearer token must survive — Codex's config still references
// it — and the output must name the surviving provider so the user knows the
// token is being kept on purpose.
func TestUninstall_FullClaude_WithCodexConfigured_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	// Seed a Juggernaut-owned Codex config (user scope).
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("creating codex dir: %v", err)
	}
	codexConfig := "model = \"global.openai.gpt-5.6-sol\"\nmodel_provider = \"amazon-bedrock-runtime\"\n[model_providers.amazon-bedrock-runtime.aws]\nregion = \"us-east-1\"\n"
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexConfig), 0o644); err != nil {
		t.Fatalf("seeding codex config: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after uninstall: %v", err)
	}
	if got != "shared-token" {
		t.Errorf("shared bearer token was removed while a Codex config still references it (got %q)\noutput:\n%s", got, out)
	}
	if !strings.Contains(out, "Shared Bedrock bearer token retained") {
		t.Errorf("expected a retain warning in output, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "codex") {
		t.Errorf("retain warning should name the surviving provider (codex), got:\n%s", out)
	}
}

// tokenSurveyProvider is a hand-built provider for exercising
// otherProviderNeedsToken's defensive branches that no real registered
// provider can reach: config construction failures, unreadable config, and
// re-registration under a real name. format defaults to the base provider's
// format when empty; set it to "bogus" to drive the newProviderManager
// continue branch.
type tokenSurveyProvider struct {
	base   provider.Provider
	format string
	owns   bool
}

func (p tokenSurveyProvider) Name() string          { return p.base.Name() }
func (p tokenSurveyProvider) BinaryNames() []string { return p.base.BinaryNames() }
func (p tokenSurveyProvider) ConfigFormatName() string {
	if p.format == "" {
		return p.base.ConfigFormatName()
	}
	return p.format
}
func (p tokenSurveyProvider) ConfigPath(h, _ string) (string, error) {
	return filepath.Join(h, ".codex", "config.toml"), nil
}
func (p tokenSurveyProvider) NativeManagedKeys() []string         { return p.base.NativeManagedKeys() }
func (p tokenSurveyProvider) DeepMergeKeys() []string             { return p.base.DeepMergeKeys() }
func (p tokenSurveyProvider) OwnedSubKeys() map[string][]string   { return p.base.OwnedSubKeys() }
func (p tokenSurveyProvider) OwnsConfig(map[string]any) bool      { return p.owns }
func (p tokenSurveyProvider) ActivationMarkers() (string, string) { return p.base.ActivationMarkers() }
func (p tokenSurveyProvider) BuildConfig(_ *bedrock.Config, _ provider.Options) (provider.ConfigPlan, error) {
	return provider.ConfigPlan{}, nil
}
func (p tokenSurveyProvider) LaunchSpec() provider.LaunchSpec     { return p.base.LaunchSpec() }
func (p tokenSurveyProvider) Supports(c provider.Capability) bool { return p.base.Supports(c) }
func (p tokenSurveyProvider) DisplayName() string                 { return p.base.DisplayName() }

// var asserts the full provider.Provider interface is satisfied at compile
// time.
var _ provider.Provider = tokenSurveyProvider{}

// TestOtherProviderNeedsToken_DefensiveBranches drives the error/defensive
// branches of otherProviderNeedsToken that real registered providers never
// reach: unknown-provider continue, config-construction continue, and the
// fail-safe retain on unreadable config.
func TestOtherProviderNeedsToken_DefensiveBranches(t *testing.T) {
	home := testutil.NewTestHome(t)
	orig := provider.MustGet("codex")
	t.Cleanup(func() { provider.ForceRegisterForTest("codex", orig) })

	exclude := provider.MustGet("claude")

	// A) empty home (no config files): Manager.Read maps IsNotExist → empty
	// map, so no provider owns anything and the survey returns "".
	provider.ForceRegisterForTest("codex", tokenSurveyProvider{base: orig})
	if res := otherProviderNeedsToken(home, exclude); res != "" {
		t.Fatalf("empty home: expected no retain, got %q", res)
	}

	// B) newProviderManager fails (unknown format) → continue branch.
	provider.ForceRegisterForTest("codex", tokenSurveyProvider{base: orig, format: "bogus"})
	_ = otherProviderNeedsToken(home, exclude)

	// C) unreadable config (invalid TOML) → fail-safe retain.
	badTOML := "[model\n"
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(badTOML), 0o644); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}
	provider.ForceRegisterForTest("codex", tokenSurveyProvider{base: orig})
	if res := otherProviderNeedsToken(home, exclude); res != "codex" {
		t.Errorf("unreadable config: expected fail-safe retain (got %q)", res)
	}

	// D) valid config the provider does NOT own → falls through to "".
	validTOML := "nonbedrock = true\n"
	if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte(validTOML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	provider.ForceRegisterForTest("codex", tokenSurveyProvider{base: orig, owns: false})
	if res := otherProviderNeedsToken(home, exclude); res != "" {
		t.Errorf("foreign config: expected no retain, got %q", res)
	}

	// E) valid config the provider DOES own → retain.
	provider.ForceRegisterForTest("codex", tokenSurveyProvider{base: orig, owns: true})
	if res := otherProviderNeedsToken(home, exclude); res != "codex" {
		t.Errorf("owned config: expected retain, got %q", res)
	}
}
