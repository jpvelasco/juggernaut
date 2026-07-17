package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
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
	collisions, err := detectForeignCollisions("/nonexistent/path/settings.json", prov, provider.ConfigPlan{Keys: map[string]any{"env": map[string]any{}}})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "checking existing configuration") {
		t.Errorf("unexpected error: %v", err)
	}
	if collisions != nil {
		t.Error("expected nil collisions on error")
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

	// With --preserve-key and no --bedrock-key, resolveCredential will try the
	// keychain. On CI/Linux without a keychain backend, the GetWithFallback call
	// will fail. The function should return an error containing "no existing key".
	token, err := resolveCredential(authmode.BedrockAPIKey, t.TempDir())
	if err == nil {
		// Keychain happened to be available and returned empty; preserve-key path
		// should error.
		t.Fatal("expected error when --preserve-key is set and no key in keychain")
	}
	if !strings.Contains(err.Error(), "no existing key found in keychain") {
		t.Errorf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token on error, got %q", token)
	}
}
