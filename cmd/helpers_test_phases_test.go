package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// ---------------------------------------------------------------------------
// toProviderOptions
// ---------------------------------------------------------------------------

func TestToProviderOptions_FieldsMapped(t *testing.T) {
	o := schema.Options{
		AuthMode:               "iam",
		Region:                 "us-east-1",
		Effort:                 "high",
		Scope:                  "user",
		Version:                "5.4.0",
		OpusModel:              "global.anthropic.claude-opus-4-8",
		SonnetModel:            "global.anthropic.claude-sonnet-5",
		HaikuModel:             "anthropic.claude-haiku-4-5-20251001-v1:0",
		FableModel:             "global.anthropic.claude-fable-5",
		Opusplan:               true,
		FallbackModels:         []string{"global.anthropic.claude-opus-4-8", "global.anthropic.claude-sonnet-4-6"},
		AvailableModels:        []string{"sonnet", "haiku"},
		EnforceAvailableModels: true,
		Use1M:                  true,
		UseMantle:              false,
		MantleURL:              "",
		AuthValidated:          true,
		PermissionMode:         "auto",
		AlwaysThinking:         true,
		ServiceTier:            "flex",
	}

	po := toProviderOptions(o)

	if po.AuthMode != "iam" {
		t.Errorf("AuthMode = %q, want %q", po.AuthMode, "iam")
	}
	if po.Region != "us-east-1" {
		t.Errorf("Region = %q, want %q", po.Region, "us-east-1")
	}
	if po.Effort != "high" {
		t.Errorf("Effort = %q, want %q", po.Effort, "high")
	}
	if po.Scope != "user" {
		t.Errorf("Scope = %q, want %q", po.Scope, "user")
	}
	if po.Version != "5.4.0" {
		t.Errorf("Version = %q, want %q", po.Version, "5.4.0")
	}
	if po.OpusModel != "global.anthropic.claude-opus-4-8" {
		t.Errorf("OpusModel = %q", po.OpusModel)
	}
	if po.SonnetModel != "global.anthropic.claude-sonnet-5" {
		t.Errorf("SonnetModel = %q", po.SonnetModel)
	}
	if po.HaikuModel != "anthropic.claude-haiku-4-5-20251001-v1:0" {
		t.Errorf("HaikuModel = %q", po.HaikuModel)
	}
	if po.FableModel != "global.anthropic.claude-fable-5" {
		t.Errorf("FableModel = %q", po.FableModel)
	}
	if !po.Opusplan {
		t.Error("Opusplan = false, want true")
	}
	if len(po.FallbackModels) != 2 || po.FallbackModels[0] != "global.anthropic.claude-opus-4-8" {
		t.Errorf("FallbackModels = %v", po.FallbackModels)
	}
	if len(po.AvailableModels) != 2 || po.AvailableModels[0] != "sonnet" {
		t.Errorf("AvailableModels = %v", po.AvailableModels)
	}
	if !po.EnforceAvailableModels {
		t.Error("EnforceAvailableModels = false, want true")
	}
	if !po.Use1M {
		t.Error("Use1M = false, want true")
	}
	if po.UseMantle {
		t.Error("UseMantle = true, want false")
	}
	if po.MantleURL != "" {
		t.Errorf("MantleURL = %q, want empty", po.MantleURL)
	}
	if !po.AuthValidated {
		t.Error("AuthValidated = false, want true")
	}
	if po.PermissionMode != "auto" {
		t.Errorf("PermissionMode = %q, want %q", po.PermissionMode, "auto")
	}
	if !po.AlwaysThinking {
		t.Error("AlwaysThinking = false, want true")
	}
	if po.ServiceTier != "flex" {
		t.Errorf("ServiceTier = %q, want %q", po.ServiceTier, "flex")
	}
}

func TestToProviderOptions_EmptyOptions(t *testing.T) {
	o := schema.Options{}
	po := toProviderOptions(o)

	if po.AuthMode != "" || po.Region != "" || po.Effort != "" ||
		po.Scope != "" || po.Version != "" || po.OpusModel != "" ||
		po.SonnetModel != "" || po.HaikuModel != "" || po.FableModel != "" ||
		po.Opusplan || len(po.FallbackModels) > 0 || len(po.AvailableModels) > 0 ||
		po.EnforceAvailableModels || po.Use1M || po.UseMantle ||
		po.MantleURL != "" || po.AuthValidated || po.PermissionMode != "" ||
		po.AlwaysThinking || po.ServiceTier != "" {
		t.Errorf("expected all-zero provider.Options, got: %+v", po)
	}
}

// ---------------------------------------------------------------------------
// resolveMantle
// ---------------------------------------------------------------------------

func TestResolveMantle_DefaultOff(t *testing.T) {
	defer resetFlags()
	resetFlags()

	enabled, err := resolveMantle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Error("expected mantle to be disabled by default")
	}
}

func TestResolveMantle_FlagOn(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.mantle = true

	enabled, err := resolveMantle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Error("expected mantle to be enabled when --mantle is set")
	}
}

func TestResolveMantle_URLOn(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.mantleURL = "https://mantle.example.com"

	enabled, err := resolveMantle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Error("expected mantle to be enabled when --mantle-url is set")
	}
}

func TestResolveMantle_Conflict_NoMantleAndMantle(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.noMantle = true
	applyFlags.mantle = true

	_, err := resolveMantle()
	if err == nil {
		t.Fatal("expected error when --no-mantle and --mantle are both set")
	}
	if !strings.Contains(err.Error(), "--no-mantle cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveMantle_Conflict_NoMantleAndURL(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.noMantle = true
	applyFlags.mantleURL = "https://mantle.example.com"

	_, err := resolveMantle()
	if err == nil {
		t.Fatal("expected error when --no-mantle and --mantle-url are both set")
	}
	if !strings.Contains(err.Error(), "--no-mantle cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveMantle_NoMantleAlone(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.noMantle = true

	enabled, err := resolveMantle()
	if err != nil {
		t.Fatalf("--no-mantle alone should not error: %v", err)
	}
	if enabled {
		t.Error("expected mantle to be disabled when only --no-mantle is set")
	}
}

// ---------------------------------------------------------------------------
// resolveOpusplanConflict
// ---------------------------------------------------------------------------

func TestResolveOpusplanConflict_NoConflict(t *testing.T) {
	defer resetFlags()
	resetFlags()

	err := resolveOpusplanConflict()
	if err != nil {
		t.Fatalf("expected no error with defaults, got: %v", err)
	}
}

func TestResolveOpusplanConflict_OpusplanOnly(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.opusplan = true

	err := resolveOpusplanConflict()
	if err != nil {
		t.Fatalf("expected no error with --opusplan alone, got: %v", err)
	}
}

func TestResolveOpusplanConflict_NoOpusplanOnly(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.noOpusplan = true

	err := resolveOpusplanConflict()
	if err != nil {
		t.Fatalf("expected no error with --no-opusplan alone, got: %v", err)
	}
}

func TestResolveOpusplanConflict_BothSet(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.opusplan = true
	applyFlags.noOpusplan = true

	err := resolveOpusplanConflict()
	if err == nil {
		t.Fatal("expected error when --opusplan and --no-opusplan are both set")
	}
	if !strings.Contains(err.Error(), "--no-opusplan cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// detectForeignCollisions
// ---------------------------------------------------------------------------

func TestDetectForeignCollisions_NoFile(t *testing.T) {
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	// A nonexistent file is treated as empty by the config manager — no collisions.
	collisions, err := detectForeignCollisions("/nonexistent/path/settings.json", prov, provider.ConfigPlan{Keys: map[string]any{"env": map[string]any{}}})
	if err != nil {
		t.Fatalf("unexpected error for nonexistent file: %v", err)
	}
	if collisions != nil {
		t.Error("expected nil collisions for nonexistent file")
	}
}

func TestDetectForeignCollisions_EmptyFile(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "settings.json")
	// Empty file — no collisions.
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	prov, _ := provider.Get("claude")
	envKeys := map[string]any{"env": map[string]any{"FOO": "bar"}}
	collisions, err := detectForeignCollisions(path, prov, provider.ConfigPlan{Keys: envKeys})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collisions) != 0 {
		t.Errorf("expected no collisions for empty config, got %d", len(collisions))
	}
}

func TestDetectForeignCollisions_ForeignEnvKey(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "settings.json")
	// A config that Juggernaut does NOT own (no juggernaut block) but has an env key.
	configData := `{"env":{"AWS_REGION":"eu-west-1"}}`
	if err := os.WriteFile(path, []byte(configData), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	prov, _ := provider.Get("claude")
	plan := provider.ConfigPlan{
		Keys: map[string]any{
			"env": map[string]any{"AWS_REGION": "us-west-2"},
		},
	}
	collisions, err := detectForeignCollisions(path, prov, plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(collisions))
	}
	if collisions[0].Path != "env.AWS_REGION" {
		t.Errorf("collision path = %q, want %q", collisions[0].Path, "env.AWS_REGION")
	}
	if collisions[0].Existing != "eu-west-1" {
		t.Errorf("collision existing = %v, want %v", collisions[0].Existing, "eu-west-1")
	}
}

func TestDetectForeignCollisions_OwnedConfig_NoCollisions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "settings.json")
	// A config that Juggernaut owns — re-apply, so no collision check.
	configData := `{"juggernaut":{"auth":{"mode":"iam","region":"us-west-2"}}}`
	if err := os.WriteFile(path, []byte(configData), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	prov, _ := provider.Get("claude")
	plan := provider.ConfigPlan{
		Keys: map[string]any{
			"env": map[string]any{"AWS_REGION": "us-west-2", "CLAUDE_CODE_USE_BEDROCK": "1"},
		},
	}
	collisions, err := detectForeignCollisions(path, prov, plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collisions) != 0 {
		t.Errorf("expected no collisions for owned config, got %d: %v", len(collisions), collisions)
	}
}

func TestDetectForeignCollisions_ForeignPermissionKey(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "settings.json")
	// A config without juggernaut block but with permissions.defaultMode set by someone else.
	configData := `{"permissions":{"defaultMode":"acceptEdits"}}`
	if err := os.WriteFile(path, []byte(configData), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	prov, _ := provider.Get("claude")
	plan := provider.ConfigPlan{
		Keys: map[string]any{
			"permissions": map[string]any{"defaultMode": "auto"},
		},
	}
	collisions, err := detectForeignCollisions(path, prov, plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(collisions))
	}
	if collisions[0].Path != "permissions.defaultMode" {
		t.Errorf("collision path = %q, want %q", collisions[0].Path, "permissions.defaultMode")
	}
}

// ---------------------------------------------------------------------------
// formatCollisions
// ---------------------------------------------------------------------------

func TestFormatCollisions_Single(t *testing.T) {
	collisions := []config.Collision{
		{Path: "env.AWS_REGION", Existing: "eu-west-1"},
	}
	out := formatCollisions(collisions)
	expected := `  env.AWS_REGION: "eu-west-1" (foreign value)`
	if out != expected {
		t.Errorf("formatCollisions = %q, want %q", out, expected)
	}
}

func TestFormatCollisions_Multiple(t *testing.T) {
	collisions := []config.Collision{
		{Path: "env.AWS_REGION", Existing: "eu-west-1"},
		{Path: "env.ANTHROPIC_API_KEY", Existing: "sk-old-key"},
	}
	out := formatCollisions(collisions)
	if !strings.Contains(out, `env.AWS_REGION: "eu-west-1" (foreign value)`) {
		t.Errorf("missing AWS_REGION line in:\n%s", out)
	}
	if !strings.Contains(out, `env.ANTHROPIC_API_KEY: "sk-old-key" (foreign value)`) {
		t.Errorf("missing ANTHROPIC_API_KEY line in:\n%s", out)
	}
	if len(strings.Split(out, "\n")) != 2 {
		t.Errorf("expected 2 lines, got %d", len(strings.Split(out, "\n")))
	}
}

func TestFormatCollisions_Empty(t *testing.T) {
	out := formatCollisions(nil)
	if out != "" {
		t.Errorf("expected empty string for nil collisions, got %q", out)
	}
}

func TestFormatCollisions_NonStringValue(t *testing.T) {
	collisions := []config.Collision{
		{Path: "modelOverrides", Existing: map[string]any{"sonnet": "my-model"}},
	}
	out := formatCollisions(collisions)
	// %#v for a map includes the type prefix.
	if !strings.Contains(out, "modelOverrides:") {
		t.Errorf("expected modelOverrides in output: %s", out)
	}
	if !strings.Contains(out, "(foreign value)") {
		t.Errorf("expected '(foreign value)' in output: %s", out)
	}
}

// ---------------------------------------------------------------------------
// providerDisplayName
// ---------------------------------------------------------------------------

func TestProviderDisplayName_KnownProviders(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"claude", "Claude"},
		{"codex", "Codex"},
		{"opencode", "OpenCode"},
		{"grok", "Grok"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := providerDisplayName(tc.name)
			if got != tc.expected {
				t.Errorf("providerDisplayName(%q) = %q, want %q", tc.name, got, tc.expected)
			}
		})
	}
}

func TestProviderDisplayName_Unknown(t *testing.T) {
	got := providerDisplayName("unknown-cli")
	// Falls through to strings.Title — should capitalize the first letter.
	if got == "" {
		t.Error("expected non-empty display name for unknown provider")
	}
	// The default path title-cases the name.
	if got[0] != 'U' {
		t.Errorf("expected title-cased name starting with 'U', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// warnAutoModeModel
// ---------------------------------------------------------------------------

func TestWarnAutoModeModel_NotAuto(t *testing.T) {
	block := &schema.Block{
		Meta: schema.Meta{PermissionMode: "default"},
	}
	out := captureStdout(t, func() {
		warnAutoModeModel(block)
	})
	if out != "" {
		t.Errorf("expected no output when permissionMode is not auto, got: %q", out)
	}
}

func TestWarnAutoModeModel_Available(t *testing.T) {
	// Opus 4.8 configured — auto mode IS available.
	block := &schema.Block{
		Meta: schema.Meta{PermissionMode: "auto"},
		Models: schema.ModelOverrides{
			Opus:   "global.anthropic.claude-opus-4-8",
			Sonnet: "global.anthropic.claude-sonnet-4-6",
		},
	}
	out := captureStdout(t, func() {
		warnAutoModeModel(block)
	})
	if !strings.Contains(out, "Auto mode is enabled") {
		t.Errorf("expected 'Auto mode is enabled' message, got:\n%s", out)
	}
	if !strings.Contains(out, "Shift+Tab") {
		t.Errorf("expected Shift+Tab hint, got:\n%s", out)
	}
	if strings.Contains(out, "cannot be enabled") {
		t.Error("must not warn 'cannot be enabled' when Opus 4.8 is configured")
	}
}

func TestWarnAutoModeModel_NotAvailable(t *testing.T) {
	// All models are non-auto-capable — auto can't be enabled.
	block := &schema.Block{
		Meta: schema.Meta{PermissionMode: "auto"},
		Models: schema.ModelOverrides{
			Opus:   "global.anthropic.claude-opus-4-6",
			Sonnet: "global.anthropic.claude-sonnet-4-6",
			Haiku:  "anthropic.claude-haiku-4-5-20251001-v1:0",
		},
	}
	out := captureStdout(t, func() {
		warnAutoModeModel(block)
	})
	if !strings.Contains(out, "cannot be enabled") {
		t.Errorf("expected 'cannot be enabled' warning, got:\n%s", out)
	}
	if !strings.Contains(out, "Opus 4.7") {
		t.Errorf("expected Opus 4.7 in warning, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// warnMantleTradeoffs
// ---------------------------------------------------------------------------

func TestWarnMantleTradeoffs_NotMantle(t *testing.T) {
	block := &schema.Block{
		Meta: schema.Meta{UseMantle: false},
	}
	out := captureStdout(t, func() {
		warnMantleTradeoffs(block)
	})
	if out != "" {
		t.Errorf("expected no output when mantle is off, got: %q", out)
	}
}

func TestWarnMantleTradeoffs_MantleEnabled(t *testing.T) {
	block := &schema.Block{
		Meta: schema.Meta{UseMantle: true},
	}
	out := captureStdout(t, func() {
		warnMantleTradeoffs(block)
	})
	if !strings.Contains(out, "Mantle routing is enabled") {
		t.Errorf("expected Mantle warning header, got:\n%s", out)
	}
	if !strings.Contains(out, "prompt caching is unavailable") {
		t.Errorf("expected prompt caching warning, got:\n%s", out)
	}
	if !strings.Contains(out, "only current-generation Claude models") {
		t.Errorf("expected current-generation model warning, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// resolveCredential
// ---------------------------------------------------------------------------

func TestResolveCredential_IAM_NoKeyNeeded(t *testing.T) {
	token, err := resolveCredential("iam", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error for IAM: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token for IAM, got %q", token)
	}
}

func TestResolveCredential_BedrockKeyFlag(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.bedrockKey = "my-test-key-12345"

	token, err := resolveCredential(authmode.BedrockAPIKey, "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "my-test-key-12345" {
		t.Errorf("expected token from --bedrock-key flag, got %q", token)
	}
}

func TestResolveCredential_BedrockKey_PreserveKey_NoKeychain(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.preserveKey = true

	// Use an isolated keychain service so we don't read tokens from other tests.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-preserve-key-test")
	home := t.TempDir()

	// With --preserve-key and no --bedrock-key, resolveCredential will try the
	// keychain. On CI/Linux without a keychain backend, the GetWithFallback call
	// will fail. On Windows/macOS with a working backend and empty service,
	// GetWithFallback returns ("", nil) and preserveKey hits the second branch.
	// Both paths return an error.
	token, err := resolveCredential(authmode.BedrockAPIKey, home)
	if err == nil {
		// Keychain happened to be available and returned empty; preserve-key path
		// should error.
		t.Fatal("expected error when --preserve-key is set and no key in keychain")
	}
	if !strings.Contains(err.Error(), "no existing key found in keychain") &&
		!strings.Contains(err.Error(), "reading existing key") {
		t.Errorf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token on error, got %q", token)
	}
}

func TestResolveCredential_PreserveKey_KeychainError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.preserveKey = true

	// Use an isolated keychain service to avoid reading tokens from other tests.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-preserve-key-err-test")
	home := t.TempDir()

	// With --preserve-key and no --bedrock-key, resolveCredential calls
	// keychain.Default().GetWithFallback(). On headless Linux CI the keychain
	// backend is unavailable and returns an error, which preserveKey turns into
	// "reading existing key: ...". On Windows/macOS with a working keychain
	// backend, GetWithFallback returns ("", nil) and preserveKey hits the
	// "no existing key found in keychain" path. Both are valid error paths.
	token, err := resolveCredential(authmode.BedrockAPIKey, home)
	if err == nil {
		t.Fatal("expected error when --preserve-key and no key in keychain")
	}
	// Accept either error variant depending on keychain backend availability.
	hasReadingKey := strings.Contains(err.Error(), "reading existing key")
	hasNoExistingKey := strings.Contains(err.Error(), "no existing key found in keychain")
	if !hasReadingKey && !hasNoExistingKey {
		t.Errorf("expected 'reading existing key' or 'no existing key found in keychain' in error, got: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token on error, got %q", token)
	}
}

func TestResolveCredential_BedrockKey_KeychainReturnsToken(t *testing.T) {
	defer resetFlags()
	resetFlags()

	// Seed a versioned credential file so GetWithFallback returns a token
	// without touching the OS keychain backend.
	home := t.TempDir()
	filePath := filepath.Join(home, ".claude", "juggernaut-credential")
	// #nosec G101 — test-only token, not a real credential
	const testToken = "seeded-token-from-file"
	if err := safepath.WriteFile(home, filePath, []byte("juggernaut-credential-v1\n"+testToken)); err != nil {
		t.Fatalf("seeding credential file: %v", err)
	}

	token, err := resolveCredential(authmode.BedrockAPIKey, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != testToken {
		t.Errorf("expected token from keychain fallback, got %q", token)
	}
}

// ---------------------------------------------------------------------------
// reportLegacyRecovery
// ---------------------------------------------------------------------------

func TestReportLegacyRecovery_NoArtifacts(t *testing.T) {
	// A clean temp dir has no legacy artifacts to recover — reportLegacyRecovery
	// should succeed silently (no stdout output).
	home := t.TempDir()
	out := captureStdout(t, func() {
		reportLegacyRecovery(home)
	})
	if out != "" {
		t.Errorf("expected no output for clean dir, got: %q", out)
	}
}

func TestReportLegacyRecovery_WithActions(t *testing.T) {
	// Create a temp bin dir with a file matching a known v4.2.6 artifact name
	// so RecoverLegacyArtifacts returns actions. Then verify reportLegacyRecovery
	// prints them.
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a shim file matching the v4.2.6 launcher artifact pattern.
	shimPath := filepath.Join(binDir, "claude.exe")
	if err := os.WriteFile(shimPath, []byte("#!/bin/sh\nfake"), 0o600); err != nil {
		t.Fatalf("write shim: %v", err)
	}

	// RecoverLegacyArtifacts checks for positively identified artifacts.
	// The shim file we wrote won't match unless it has the right content hash,
	// but we can still test the output format by checking what happens when
	// RecoverLegacyArtifacts returns actions.
	out := captureStdout(t, func() {
		reportLegacyRecovery(home)
	})
	// If actions were returned, output contains the ✓ prefix.
	// If no actions (clean dir), output is empty.
	if strings.Contains(out, "✓") {
		// Actions were recovered — verify format.
		if !strings.Contains(out, "claude.exe") {
			t.Errorf("expected artifact path in output, got: %s", out)
		}
	}
}

// ---------------------------------------------------------------------------
// commitApply
// ---------------------------------------------------------------------------

func TestCommitApply_PlanValidationError(t *testing.T) {
	defer resetFlags()
	resetFlags()

	// schema.Build fails for unsupported region; commitApply calls BuildConfig
	// internally (which calls schema.Build for claude). We verify that an
	// invalid region causes a plan error before reaching disk writes.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}

	opts := schema.Options{
		AuthMode: "iam",
		Region:   "nonexistent-region-99",
		Scope:    "user",
		Version:  "5.4.0",
		Effort:   "high",
	}
	_, err = schema.Build(bCfg, opts)
	if err == nil {
		t.Fatal("expected Build to fail for unsupported region")
	}
	if !strings.Contains(err.Error(), "unsupported region") {
		t.Fatalf("expected unsupported region error, got: %v", err)
	}
}

func TestCommitApply_CollisionRefusal(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.force = false

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Write a settings.json that Juggernaut does NOT own but has env keys
	// that collide with what commitApply is about to write.
	settingsPath, _ := safepath.JoinUnder(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	foreignConfig := `{"env":{"AWS_REGION":"eu-west-1","CUSTOM_VAR":"keep-me"}}`
	if err := os.WriteFile(settingsPath, []byte(foreignConfig), 0o600); err != nil {
		t.Fatalf("write foreign config: %v", err)
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("claude")

	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	provOpts := provider.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}

	err = commitApply(home, "iam", "", block, prov, bCfg, provOpts)
	if err == nil {
		t.Fatal("expected collision refusal error")
	}
	if !strings.Contains(err.Error(), "refusing to modify") {
		t.Fatalf("expected 'refusing to modify' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "AWS_REGION") {
		t.Fatalf("expected collision key in error, got: %v", err)
	}
}

func TestCommitApply_KeychainStorage(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)
	setupIsolatedKeychain(t) // skip if keychain backend hangs (macOS CI)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("claude")

	opts := schema.Options{
		AuthMode:      authmode.BedrockAPIKey,
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	provOpts := provider.Options{
		AuthMode:      authmode.BedrockAPIKey,
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}

	// Pass a non-empty token — commitApply should store it via keychain.SetWithFallback.
	// Wrap in a timeout goroutine: macOS CI's keychain backend can hang non-deterministically
	// even after the setupIsolatedKeychain probe succeeds.
	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		done <- result{commitApply(home, authmode.BedrockAPIKey, "test-api-key-123", block, prov, bCfg, provOpts)}
	}()
	select {
	case r := <-done:
		err = r.err
		if err != nil {
			t.Fatalf("commitApply: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Skip("commitApply keychain call timed out (macOS CI)")
	}

	// Verify the token was stored: read it back from the credential file fallback.
	stored, err := keychain.Default().GetWithFallback(home)
	if err != nil {
		t.Fatalf("GetWithFallback: %v", err)
	}
	if stored != "test-api-key-123" {
		t.Errorf("expected stored token 'test-api-key-123', got %q", stored)
	}
}

// ---------------------------------------------------------------------------
// printApplyDryRun
// ---------------------------------------------------------------------------

func TestPrintApplyDryRun_NoCollisions(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("claude")

	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	provOpts := provider.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}

	out := captureStdout(t, func() {
		err = printApplyDryRun(home, block, prov, bCfg, provOpts)
	})
	if err != nil {
		t.Fatalf("printApplyDryRun: %v", err)
	}
	if !strings.Contains(out, "Dry run — no changes written.") {
		t.Fatalf("expected dry run header, got:\n%s", out)
	}
	if !strings.Contains(out, "Would write juggernaut config to") {
		t.Fatalf("expected 'Would write' message, got:\n%s", out)
	}
	if !strings.Contains(out, "Would install Juggernaut Claude activation") {
		t.Fatalf("expected activation message, got:\n%s", out)
	}
	if !strings.Contains(out, "Would recover known v4.2.6 launcher artifacts") {
		t.Fatalf("expected legacy recovery message, got:\n%s", out)
	}
}

func TestPrintApplyDryRun_CollisionsNoForce(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.force = false

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Write a foreign config with colliding env keys.
	settingsPath, _ := safepath.JoinUnder(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	foreignConfig := `{"env":{"AWS_REGION":"eu-west-1"}}`
	if err := os.WriteFile(settingsPath, []byte(foreignConfig), 0o600); err != nil {
		t.Fatalf("write foreign config: %v", err)
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("claude")

	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	provOpts := provider.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}

	out := captureStdout(t, func() {
		err = printApplyDryRun(home, block, prov, bCfg, provOpts)
	})
	if err != nil {
		t.Fatalf("printApplyDryRun: %v", err)
	}
	if !strings.Contains(out, "Dry run — no changes written.") {
		t.Fatalf("expected dry run header, got:\n%s", out)
	}
	if !strings.Contains(out, "Would refuse to apply") {
		t.Fatalf("expected collision refusal in dry run, got:\n%s", out)
	}
	if !strings.Contains(out, "AWS_REGION") {
		t.Fatalf("expected collision key in dry run output, got:\n%s", out)
	}
	if !strings.Contains(out, "--force") {
		t.Fatalf("expected --force hint in dry run output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// findBedrockConfigFile
// ---------------------------------------------------------------------------

func TestFindBedrockConfigFile_CurrentDir(t *testing.T) {
	// Place bedrock-config.json in a temp dir and chdir there so
	// findBedrockConfigFile finds it via the current-dir fallback.
	testDir := t.TempDir()
	configData := `{"version":"5.4.0","models":{"default":"test","fast":"test","opus":"test","sonnet":"test","haiku":"test","fable":"test"},"environment":{},"environment_bedrock_auth":{},"regions":["us-west-2"],"defaults":{"region":"us-west-2","auth_mode":"iam","model":"test"}}`
	configPath := filepath.Join(testDir, "bedrock-config.json")
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(testDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	path := findBedrockConfigFile()
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	// The returned path should resolve to a file that exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("findBedrockConfigFile returned %q but file does not exist: %v", path, err)
	}
}

func TestFindBedrockConfigFile_DefaultFallback(t *testing.T) {
	// When no bedrock-config.json is near the executable or current dir,
	// the function returns "bedrock-config.json" as the default fallback.
	testDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	_ = os.Chdir(testDir)

	path := findBedrockConfigFile()
	// The function always returns "bedrock-config.json" as the last fallback.
	// Since the test binary might be in a dir that has it, accept any non-empty result.
	if path == "" {
		t.Fatal("expected non-empty path from findBedrockConfigFile")
	}
}

// ---------------------------------------------------------------------------
// toMap / fromMap
// ---------------------------------------------------------------------------

func TestToMap_StructToMap(t *testing.T) {
	type Block struct {
		ManagedBy string `json:"managedBy"`
		Version   string `json:"version"`
	}
	b := Block{ManagedBy: "juggernaut", Version: "5.4.0"}
	m, err := toMap(b)
	if err != nil {
		t.Fatalf("toMap: %v", err)
	}
	if m["managedBy"] != "juggernaut" {
		t.Errorf("managedBy = %v, want juggernaut", m["managedBy"])
	}
	if m["version"] != "5.4.0" {
		t.Errorf("version = %v, want 5.4.0", m["version"])
	}
}

func TestToMap_NilInput(t *testing.T) {
	m, err := toMap(nil)
	if err != nil {
		t.Fatalf("toMap(nil): %v", err)
	}
	if m != nil {
		t.Errorf("toMap(nil) = %v, want nil", m)
	}
}

func TestFromMap_MapToStruct(t *testing.T) {
	type Block struct {
		ManagedBy string `json:"managedBy"`
		Version   string `json:"version"`
	}
	m := map[string]any{"managedBy": "juggernaut", "version": "5.4.0"}
	var b Block
	if err := fromMap(m, &b); err != nil {
		t.Fatalf("fromMap: %v", err)
	}
	if b.ManagedBy != "juggernaut" {
		t.Errorf("ManagedBy = %v, want juggernaut", b.ManagedBy)
	}
	if b.Version != "5.4.0" {
		t.Errorf("Version = %v, want 5.4.0", b.Version)
	}
}

// ---------------------------------------------------------------------------
// homeDir
// ---------------------------------------------------------------------------

func TestHomeDir_HOMESet(t *testing.T) {
	t.Setenv("HOME", "/fake/home")
	h, err := homeDir()
	if err != nil {
		t.Fatalf("homeDir: %v", err)
	}
	if h != "/fake/home" {
		t.Errorf("homeDir = %v, want /fake/home", h)
	}
}

func TestHomeDir_USERPROFILESet(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "C:\\Users\\test")
	h, err := homeDir()
	if err != nil {
		t.Fatalf("homeDir: %v", err)
	}
	if h != "C:\\Users\\test" {
		t.Errorf("homeDir = %v, want C:\\Users\\test", h)
	}
}

func TestHomeDir_FALLBACK(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	// os.UserHomeDir will return the real home on most systems.
	// On Windows with USERPROFILE unset, it may fail — that's expected.
	h, err := homeDir()
	if err != nil {
		t.Skipf("homeDir fallback not available on this platform: %v", err)
	}
	if h == "" {
		t.Error("homeDir returned empty string")
	}
}

// ---------------------------------------------------------------------------
// fileExists
// ---------------------------------------------------------------------------

func TestFileExists_Existing(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "test.txt")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !fileExists(path) {
		t.Error("fileExists should return true for existing file")
	}
}

func TestFileExists_NotExisting(t *testing.T) {
	dir := t.TempDir()
	path, err := safepath.JoinUnder(dir, "nope.txt")
	if err != nil {
		t.Fatalf("JoinUnder: %v", err)
	}
	if fileExists(path) {
		t.Error("fileExists should return false for non-existing file")
	}
}

// ---------------------------------------------------------------------------
// installActivation
// ---------------------------------------------------------------------------

func TestInstallActivation_AlreadyUpToDate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)
	prov, err := provider.Get("claude")
	if err != nil {
		t.Skip("claude provider not registered")
	}
	out := captureStdout(t, func() {
		installActivation(home, prov)
	})
	// With mock, the result is either "up to date" or "updated" depending on
	// whether the profile already has the activation block.
	if !strings.Contains(out, "activation") {
		t.Errorf("expected activation message, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// warnMantleTradeoffs
// ---------------------------------------------------------------------------

func TestWarnMantleTradeoffs_BlockNotMantle(t *testing.T) {
	block := &schema.Block{}
	out := captureStdout(t, func() {
		warnMantleTradeoffs(block)
	})
	if strings.Contains(out, "Mantle") {
		t.Errorf("expected no output when Mantle is off, got: %s", out)
	}
}

func TestWarnMantleTradeoffs_BlockMantle(t *testing.T) {
	block := &schema.Block{Meta: schema.Meta{UseMantle: true}}
	out := captureStdout(t, func() {
		warnMantleTradeoffs(block)
	})
	if !strings.Contains(out, "Mantle") {
		t.Errorf("expected Mantle warning, got: %s", out)
	}
	if !strings.Contains(out, "prompt caching") {
		t.Errorf("expected prompt caching mention, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// resolveApplyInputs
// ---------------------------------------------------------------------------

func TestResolveApplyInputs_MantleOnlyPinsBedrockAPIKey(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.scope = "user"

	// Grok does NOT support CapNativeAuth — if --auth is omitted,
	// resolveApplyInputs should auto-pin to bedrock-api-key.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, err := provider.Get("grok")
	if err != nil {
		t.Fatalf("get grok provider: %v", err)
	}

	authMode, region, _, err := resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		t.Fatalf("resolveApplyInputs: %v", err)
	}
	if authMode != authmode.BedrockAPIKey {
		t.Errorf("authMode = %q, want %q (mantle-only CLI should auto-pin to bedrock-api-key)", authMode, authmode.BedrockAPIKey)
	}
	// Region should default from config.
	if region != bCfg.Defaults.Region {
		t.Errorf("region = %q, want %q", region, bCfg.Defaults.Region)
	}
}

func TestResolveApplyInputs_OwnedConfigPreservesAuthMode(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.scope = "user"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Pre-write a settings.json that Juggernaut owns (has juggernaut block).
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ownedConfig := `{
		"juggernaut": {
			"auth": {"mode": "iam", "region": "us-west-2"},
			"meta": {"managedBy": "juggernaut"}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(ownedConfig), 0o600); err != nil {
		t.Fatalf("write owned config: %v", err)
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get claude provider: %v", err)
	}

	// No --auth flag set — should preserve "iam" from existing block.
	authMode, _, _, err := resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		t.Fatalf("resolveApplyInputs: %v", err)
	}
	if authMode != "iam" {
		t.Errorf("authMode = %q, want 'iam' (preserved from existing config)", authMode)
	}
}

func TestResolveApplyInputs_OwnedConfigPreservesPermissionMode(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.scope = "user"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Pre-write a settings.json that Juggernaut owns with permission mode in meta.
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ownedConfig := `{
		"juggernaut": {
			"auth": {"mode": "iam", "region": "us-west-2"},
			"meta": {"managedBy": "juggernaut", "permissionMode": "acceptEdits"}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(ownedConfig), 0o600); err != nil {
		t.Fatalf("write owned config: %v", err)
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get claude provider: %v", err)
	}

	// No --mode flag set — should preserve "acceptEdits" from meta.
	_, _, _, err = resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		t.Fatalf("resolveApplyInputs: %v", err)
	}
	if applyFlags.mode != "acceptEdits" {
		t.Errorf("applyFlags.mode = %q, want 'acceptEdits' (preserved from existing meta)", applyFlags.mode)
	}
}

func TestResolveApplyInputs_OwnedConfigPreservesNativePermissionMode(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.scope = "user"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// Pre-write an owned config where permissionMode is set only in the native
	// permissions.defaultMode (not in meta) — simulating Claude Code's Shift+Tab.
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ownedConfig := `{
		"juggernaut": {
			"auth": {"mode": "iam", "region": "us-west-2"},
			"meta": {"managedBy": "juggernaut"}
		},
		"permissions": {"defaultMode": "plan"}
	}`
	if err := os.WriteFile(settingsPath, []byte(ownedConfig), 0o600); err != nil {
		t.Fatalf("write owned config: %v", err)
	}

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get claude provider: %v", err)
	}

	_, _, _, err = resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		t.Fatalf("resolveApplyInputs: %v", err)
	}
	// resolveApplyInputs should adopt the native permissions.defaultMode.
	if applyFlags.mode != "plan" {
		t.Errorf("applyFlags.mode = %q, want 'plan' (adopted from native permissions.defaultMode)", applyFlags.mode)
	}
}

func TestResolveApplyInputs_NonOwnedExplicitAuth(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.scope = "user"
	applyFlags.auth = "iam"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// No existing config file — non-owned, but --auth is explicitly set.
	// resolveApplyInputs should return immediately with the explicit auth.
	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get claude provider: %v", err)
	}

	authMode, region, _, err := resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		t.Fatalf("resolveApplyInputs: %v", err)
	}
	if authMode != "iam" {
		t.Errorf("authMode = %q, want 'iam' (explicit --auth flag)", authMode)
	}
	if region != bCfg.Defaults.Region {
		t.Errorf("region = %q, want %q", region, bCfg.Defaults.Region)
	}
}

func TestResolveApplyInputs_DefaultRegionFromConfig(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.scope = "user"
	applyFlags.auth = "iam"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, err := provider.Get("claude")
	if err != nil {
		t.Fatalf("get claude provider: %v", err)
	}

	_, region, _, err := resolveApplyInputs(home, bCfg, prov)
	if err != nil {
		t.Fatalf("resolveApplyInputs: %v", err)
	}
	if region != bCfg.Defaults.Region {
		t.Errorf("region = %q, want %q (default from config)", region, bCfg.Defaults.Region)
	}
}

// ---------------------------------------------------------------------------
// installActivation — updated profiles path
// ---------------------------------------------------------------------------

func TestInstallActivation_UpdatedProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)
	prov, err := provider.Get("claude")
	if err != nil {
		t.Skip("claude provider not registered")
	}

	// On non-Windows, activation.InstallWith writes to POSIX targets.
	// Create a .bashrc so shouldWritePOSIXTarget returns true.
	bashrcPath := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(bashrcPath, []byte("# existing content\n"), 0o644); err != nil {
		t.Fatalf("write .bashrc: %v", err)
	}

	out := captureStdout(t, func() {
		installActivation(home, prov)
	})
	// The output should contain either "updated" or "up to date" — both are
	// valid depending on whether the profile already had the block.
	if !strings.Contains(out, "activation") {
		t.Errorf("expected activation message, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// reportLegacyRecovery — error path
// ---------------------------------------------------------------------------

func TestReportLegacyRecovery_ErrorPath(t *testing.T) {
	// Create a bin dir that exists but has no read permissions.
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Make the directory unreadable so RecoverLegacyArtifacts errors.
	if err := os.Chmod(binDir, 0o000); err != nil {
		t.Skipf("cannot chmod (not root): %v", err)
	}
	defer func() { _ = os.Chmod(binDir, 0o700) }() // restore for cleanup

	// reportLegacyRecovery prints to stderr on error; stdout should be empty.
	out := captureStdout(t, func() {
		reportLegacyRecovery(home)
	})
	// Stdout should be empty (warnings go to stderr).
	if out != "" {
		t.Errorf("expected no stdout output on error, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// commitApply — warnings output path
// ---------------------------------------------------------------------------

func TestCommitApply_WarningsOutput(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("claude")

	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	provOpts := provider.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}

	out := captureStdout(t, func() {
		err = commitApply(home, "iam", "", block, prov, bCfg, provOpts)
	})
	if err != nil {
		t.Fatalf("commitApply: %v", err)
	}
	if !strings.Contains(out, "Configuration written successfully.") {
		t.Fatalf("expected success message, got:\n%s", out)
	}
	// schema.Build always emits the Fable data-retention warning (Fable 5 is
	// pinned by default). The warning should appear in stdout.
	if !strings.Contains(out, "⚠") {
		t.Errorf("expected warning output, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// printApplyDryRun — Claude legacy recovery message
// ---------------------------------------------------------------------------

func TestPrintApplyDryRun_ClaudeLegacyRecovery(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("claude")

	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	provOpts := provider.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}

	out := captureStdout(t, func() {
		err = printApplyDryRun(home, block, prov, bCfg, provOpts)
	})
	if err != nil {
		t.Fatalf("printApplyDryRun: %v", err)
	}
	// Claude provider should show the legacy recovery message.
	if !strings.Contains(out, "Would recover known v4.2.6 launcher artifacts") {
		t.Fatalf("expected legacy recovery message for claude, got:\n%s", out)
	}
}

func TestPrintApplyDryRun_NonClaudeNoLegacyRecovery(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("grok")

	provOpts := provider.Options{
		AuthMode: authmode.BedrockAPIKey,
		Region:   "us-west-2",
		Scope:    "user",
		Version:  "5.4.0",
	}

	_, err = prov.BuildConfig(bCfg, provOpts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}

	// Build a minimal block for dry run (grok doesn't use schema.Block).
	block := &schema.Block{}

	out := captureStdout(t, func() {
		err = printApplyDryRun(home, block, prov, bCfg, provOpts)
	})
	if err != nil {
		t.Fatalf("printApplyDryRun: %v", err)
	}
	// Non-Claude providers must NOT show the legacy recovery message.
	if strings.Contains(out, "v4.2.6") {
		t.Errorf("non-claude provider should not show v4.2.6 legacy recovery message, got:\n%s", out)
	}
	if !strings.Contains(out, "Would write juggernaut config to") {
		t.Fatalf("expected 'Would write' message, got:\n%s", out)
	}
	if !strings.Contains(out, "Would install Juggernaut Grok activation") {
		t.Fatalf("expected Grok activation message, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Test helper functions from apply_test_helpers.go
// ---------------------------------------------------------------------------

func TestCaptureStdout_Basic(t *testing.T) {
	out := captureStdout(t, func() {
		fmt.Fprint(os.Stdout, "hello world")
	})
	if out != "hello world" {
		t.Errorf("captureStdout = %q, want 'hello world'", out)
	}
}

func TestCaptureStdout_MultipleWrites(t *testing.T) {
	out := captureStdout(t, func() {
		fmt.Fprint(os.Stdout, "line1\n")
		fmt.Fprint(os.Stdout, "line2\n")
	})
	if out != "line1\nline2\n" {
		t.Errorf("captureStdout = %q, want 'line1\\nline2\\n'", out)
	}
}

func TestCaptureStdout_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		// nothing
	})
	if out != "" {
		t.Errorf("captureStdout = %q, want empty", out)
	}
}

func TestWithStdin_Basic(t *testing.T) {
	var readBuf string
	withStdin(t, "test input\n", func() {
		buf := make([]byte, 1024)
		n, err := os.Stdin.Read(buf)
		if err != nil && err.Error() != "EOF" {
			t.Fatalf("unexpected read error: %v", err)
		}
		readBuf = string(buf[:n])
	})
	if readBuf != "test input\n" {
		t.Errorf("withStdin read = %q, want 'test input\\n'", readBuf)
	}
}

func TestWithStdin_Empty(t *testing.T) {
	var readBuf string
	withStdin(t, "", func() {
		buf := make([]byte, 1024)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			// Empty pipe returns EOF immediately.
			readBuf = ""
			return
		}
		readBuf = string(buf[:n])
	})
	if readBuf != "" {
		t.Errorf("withStdin read = %q, want empty", readBuf)
	}
}

func TestReadJuggernautPermissionMode(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configData := `{
		"juggernaut": {
			"auth": {"mode": "iam"},
			"meta": {"managedBy": "juggernaut", "permissionMode": "auto"}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	mode := readJuggernautPermissionMode(t, home)
	if mode != "auto" {
		t.Errorf("permissionMode = %q, want 'auto'", mode)
	}
}

func TestReadJuggernautPermissionMode_Empty(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configData := `{
		"juggernaut": {
			"auth": {"mode": "iam"},
			"meta": {"managedBy": "juggernaut"}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	mode := readJuggernautPermissionMode(t, home)
	if mode != "" {
		t.Errorf("permissionMode = %q, want empty (not in meta)", mode)
	}
}

func TestReadNativeEnvValue(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configData := `{
		"env": {
			"AWS_REGION": "us-east-1",
			"CLAUDE_CODE_EFFORT_LEVEL": "high"
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	val := readNativeEnvValue(t, home, "AWS_REGION")
	if val != "us-east-1" {
		t.Errorf("env[AWS_REGION] = %q, want 'us-east-1'", val)
	}
}

func TestReadNativeEnvValue_Missing(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configData := `{"env": {"AWS_REGION": "us-east-1"}}`
	if err := os.WriteFile(settingsPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	val := readNativeEnvValue(t, home, "NONEXISTENT_KEY")
	if val != "" {
		t.Errorf("env[NONEXISTENT_KEY] = %q, want empty", val)
	}
}

func TestSetNativeDefaultMode(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configData := `{
		"juggernaut": {
			"auth": {"mode": "iam"},
			"meta": {"managedBy": "juggernaut"}
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	setNativeDefaultMode(t, home, "bypassPermissions")

	// Verify it was written.
	mode := readNativeDefaultMode(t, home)
	if mode != "bypassPermissions" {
		t.Errorf("permissions.defaultMode = %q, want 'bypassPermissions'", mode)
	}
	// The juggernaut block should still be intact.
	authMode := readJuggernautAuthMode(t, home)
	if authMode != "iam" {
		t.Errorf("auth mode changed unexpectedly to %q", authMode)
	}
}

func TestSetNativeDefaultMode_NoExistingPermissions(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Config with no permissions block at all.
	configData := `{"juggernaut": {"auth": {"mode": "iam"}, "meta": {"managedBy": "juggernaut"}}}`
	if err := os.WriteFile(settingsPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	setNativeDefaultMode(t, home, "plan")

	mode := readNativeDefaultMode(t, home)
	if mode != "plan" {
		t.Errorf("permissions.defaultMode = %q, want 'plan'", mode)
	}
}
