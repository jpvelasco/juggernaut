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

// Given a Bedrock bearer token is present and no other provider keeps a
// config,
// When the user runs a full Claude uninstall with --dry-run,
// Then the survey runs but nothing is removed: the token survives and no
// removal is reported.
func TestUninstall_DryRun_LastProvider_KeepsToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("probe-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--force", "--dry-run"}); err != nil {
			t.Fatalf("uninstall dry-run error: %v", err)
		}
	})

	got, _ := store.GetWithFallback(home)
	if got != "probe-token" {
		t.Errorf("dry-run must not remove the bearer token, got %q", got)
	}
	if strings.Contains(out, "Removed bearer token") {
		t.Errorf("dry-run must not report token removal:\n%s", out)
	}
}

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

// claudeOwnedSettingsJSON is a minimal Claude settings.json carrying a
// Juggernaut block (juggernaut.meta.managedBy == "juggernaut"), which is what
// (claude).OwnsConfig recognizes.
const claudeOwnedSettingsJSON = `{"juggernaut":{"auth":{"mode":"iam","region":"us-west-2"},"meta":{"managedBy":"juggernaut"}}}`

// grokOwnedTOML is a minimal Grok config.toml whose [model.bedrock-grok]
// profile is what (grok).OwnsConfig recognizes.
const grokOwnedTOML = "[model.bedrock-grok]\nbase_url = \"https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1\"\n"

// seedJuggernautConfig writes a Juggernaut-owned config file for the given
// provider name (claude or grok) under home.
func seedJuggernautConfig(t *testing.T, home, provName string) {
	t.Helper()
	var dir, file, body string
	switch provName {
	case "claude":
		dir, file, body = filepath.Join(home, ".claude"), "settings.json", claudeOwnedSettingsJSON
	case "grok":
		dir, file, body = filepath.Join(home, ".grok"), "config.toml", grokOwnedTOML
	default:
		t.Fatalf("unknown provider for fixture: %s", provName)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatalf("seed %s config: %v", provName, err)
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

	// F) a nil exclude surveys EVERY provider — the provider "excluded"
	// from the caller's perspective (claude) must still be surveyed when it
	// owns a config.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(claudeOwnedSettingsJSON), 0o644); err != nil {
		t.Fatalf("seed claude config: %v", err)
	}
	provider.ForceRegisterForTest("codex", orig)
	if res := otherProviderNeedsToken(home, nil); res != "claude" {
		t.Errorf("nil exclude: expected claude retained, got %q", res)
	}
	if err := os.RemoveAll(filepath.Join(home, ".claude")); err != nil {
		t.Fatalf("remove .claude: %v", err)
	}

	// G) same-provider user-scope retain: the survey must consider scopes the
	// uninstall is NOT removing. An opencode uninstall with claude configured
	// in user scope keeps the token.
	provider.ForceRegisterForTest("claude", tokenSurveyProvider{base: provider.MustGet("claude")})
	if res := otherProviderNeedsToken(home, provider.MustGet("opencode")); res != "claude" {
		t.Errorf("claude user-scope config present: expected claude retained, got %q", res)
	}
}

// Given the user uninstalls Claude with --scope=user (a partial removal),
// When the uninstall completes,
// Then the shared token must survive: the project scope and any other CLI
// still reference it.
func TestUninstall_UserScope_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--scope=user", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after user-scope uninstall: %v", err)
	}
	if got != "shared-token" {
		t.Errorf("user-scope uninstall deleted the shared bearer token (got %q)\noutput:\n%s", got, out)
	}
}

// Given the LAST CLI (Grok) is uninstalled outright and no other
// Juggernaut-owned config remains anywhere,
// When the uninstall completes,
// Then the shared bearer token is removed — the old Claude-only gate never
// ran the survey for a non-Claude last uninstall.
func TestUninstall_LastNonClaude_RemovesSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	chdirTo(t, home) // project-scope probe must find nothing under the cwd
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("last-cli-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=grok", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after last-CLI uninstall: %v", err)
	}
	if got != "" {
		t.Errorf("last-CLI uninstall should remove the shared bearer token, got %q\noutput:\n%s", got, out)
	}
	if !strings.Contains(out, "Removed bearer token from keychain") {
		t.Errorf("expected a token-removal line in output, got:\n%s", out)
	}
}

// Given a Grok config is configured and the user uninstalls Claude in full,
// When the uninstall completes,
// Then the shared token must survive — the retention survey must consider
// Grok (and every provider), not just the providers historically known.
func TestUninstall_FullClaude_WithGrokConfigured_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	chdirTo(t, home)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}
	seedJuggernautConfig(t, home, "grok")

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
		t.Errorf("shared bearer token was removed while a Grok config still references it (got %q)\noutput:\n%s", got, out)
	}
	if !strings.Contains(out, "Shared Bedrock bearer token retained") {
		t.Errorf("expected a retain warning in output, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "grok") {
		t.Errorf("retain warning should name the surviving provider (grok), got:\n%s", out)
	}
}

// Given a Grok config is configured and the user dries a full Claude
// uninstall,
// When the dry run completes,
// Then nothing is removed — the token survives and no removal is announced.
func TestUninstall_DryRunClaude_WithGrokConfigured_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	chdirTo(t, home)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}
	seedJuggernautConfig(t, home, "grok")

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--dry-run"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after dry run: %v", err)
	}
	if got != "shared-token" {
		t.Errorf("dry run removed the shared bearer token (got %q)\noutput:\n%s", got, out)
	}
	if strings.Contains(out, "Removed bearer token") {
		t.Errorf("dry run must not announce a token removal, got:\n%s", out)
	}
}

// Given no Juggernaut-owned config exists for any provider and the user
// uninstalls Grok outright,
// When the uninstall completes,
// Then the shared token is still removed — the survey runs for every last-CLI
// uninstall, not just Claude's.
func TestUninstall_LastGrok_NoConfigs_RemovesSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	chdirTo(t, home)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("orphan-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=grok", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after uninstall: %v", err)
	}
	if got != "" {
		t.Errorf("orphan token should be removed on last-CLI uninstall, got %q\noutput:\n%s", got, out)
	}
}
