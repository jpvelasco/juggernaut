package schema_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

func testConfig() *bedrock.Config {
	return &bedrock.Config{
		Version: "4.1.0",
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-5",
			Sonnet: "global.anthropic.claude-sonnet-5",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			Fable:  "global.anthropic.claude-fable-5",
		},
		Environment: map[string]string{
			"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32768",
			"CLAUDE_CODE_EFFORT_LEVEL":      "xhigh",
		},
		EnvironmentBedrockAuth: map[string]string{
			"CLAUDE_CODE_USE_BEDROCK": "1",
		},
		Regions: []string{"us-east-1", "us-west-2"},
		Defaults: bedrock.Defaults{
			Region:   "us-west-2",
			AuthMode: "iam",
		},
	}
}

func TestBuild_IAM(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Auth.Mode != "iam" {
		t.Errorf("expected auth.mode=iam, got %s", block.Auth.Mode)
	}
	if block.Env["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Error("expected CLAUDE_CODE_USE_BEDROCK=1 for validated IAM")
	}
	if block.Meta.SchemaVersion != 2 {
		t.Errorf("expected schemaVersion=2, got %d", block.Meta.SchemaVersion)
	}
}

func TestBuild_InvalidRegion(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "eu-fake-99",
		Effort:   "xhigh",
		Scope:    "user",
		Version:  "4.0.0",
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Error("expected error for invalid region")
	}
}

func TestBuild_InvalidEffort(t *testing.T) {
	for _, effort := range []string{"turbo", "ultracode"} {
		t.Run(effort, func(t *testing.T) {
			opts := schema.Options{
				AuthMode: "iam",
				Region:   "us-west-2",
				Effort:   effort,
				Scope:    "user",
				Version:  "4.0.0",
			}

			_, err := schema.Build(testConfig(), opts)
			if err == nil {
				t.Errorf("expected error for invalid effort level %q", effort)
			}
		})
	}
}
func TestNativeKeys_Opusplan(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		Opusplan:      true,
		AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if native.Model != "opusplan" {
		t.Errorf("expected model=opusplan with opusplan=true, got %s", native.Model)
	}
}

func TestBuild_Use1MAnnotatesPinnedClaudeCodeModels(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "high",
		Scope:         "user",
		Version:       "4.1.0",
		Use1M:         true,
		Opusplan:      true,
		AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-5[1m]" {
		t.Errorf("expected Opus env model with [1m], got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-5[1m]" {
		t.Errorf("expected Sonnet env model with [1m], got %q", block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
		t.Errorf("Haiku should not get [1m], got %q", block.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "global.anthropic.claude-fable-5[1m]" {
		t.Errorf("expected Fable env model with [1m], got %q", block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
	if block.Env["ANTHROPIC_MODEL"] != "opusplan[1m]" {
		t.Errorf("expected opusplan env model with [1m], got %q", block.Env["ANTHROPIC_MODEL"])
	}
	if _, ok := block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"]; ok {
		t.Error("disable 1M env should not be set when Use1M=true")
	}

	native := block.NativeKeys()
	if native.Model != "opusplan[1m]" {
		t.Errorf("expected native model=opusplan[1m], got %q", native.Model)
	}
	if native.ModelOverrides["claude-sonnet-5[1m]"] != "global.anthropic.claude-sonnet-5" {
		t.Errorf("expected [1m] Sonnet override to map to unsuffixed provider ID, got %q", native.ModelOverrides["claude-sonnet-5[1m]"])
	}
	if native.ModelOverrides["claude-opus-5[1m]"] != "global.anthropic.claude-opus-5" {
		t.Errorf("expected [1m] Opus override to map to unsuffixed provider ID, got %q", native.ModelOverrides["claude-opus-5[1m]"])
	}
	if native.ModelOverrides["claude-fable-5[1m]"] != "global.anthropic.claude-fable-5" {
		t.Errorf("expected [1m] Fable override to map to unsuffixed provider ID, got %q", native.ModelOverrides["claude-fable-5[1m]"])
	}
}

func TestBuild_No1MDisablesClaudeCodeExtendedContext(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "high",
		Scope:         "user",
		Version:       "4.1.0",
		Use1M:         false,
		AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-5" {
		t.Errorf("expected Opus env model without [1m], got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-5" {
		t.Errorf("expected Sonnet env model without [1m], got %q", block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "global.anthropic.claude-fable-5" {
		t.Errorf("expected Fable env model without [1m], got %q", block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
	if block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"] != "1" {
		t.Errorf("expected CLAUDE_CODE_DISABLE_1M_CONTEXT=1, got %q", block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"])
	}
	if _, ok := block.NativeKeys().ModelOverrides["claude-sonnet-5[1m]"]; ok {
		t.Error("[1m] model override should be omitted when Use1M=false")
	}
	if _, ok := block.NativeKeys().ModelOverrides["claude-fable-5[1m]"]; ok {
		t.Error("[1m] Fable model override should be omitted when Use1M=false")
	}
}

func TestBuild_Use1MDoesNotAnnotateUnknownCustomModels(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "high",
		Scope:         "user",
		Version:       "4.1.0",
		Use1M:         true,
		OpusModel:     "custom.opus",
		SonnetModel:   "custom.sonnet",
		FableModel:    "custom.fable",
		AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "custom.opus" {
		t.Errorf("unknown Opus override should not get [1m], got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "custom.sonnet" {
		t.Errorf("unknown Sonnet override should not get [1m], got %q", block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "custom.fable" {
		t.Errorf("unknown Fable override should not get [1m], got %q", block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
}

func TestBuild_FableDefaultsAndNativeAliases(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Models.Fable != "global.anthropic.claude-fable-5" {
		t.Errorf("expected Fable model from config, got %q", block.Models.Fable)
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_NAME"] != "Fable 5" {
		t.Errorf("expected Fable display name, got %q", block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_NAME"])
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_DESCRIPTION"] == "" {
		t.Error("expected Fable description to be set")
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_SUPPORTED_CAPABILITIES"] == "" {
		t.Error("expected Fable supported capabilities to be set")
	}

	overrides := block.NativeKeys().ModelOverrides
	for _, key := range []string{"fable", "claude-fable-5", "anthropic.claude-fable-5"} {
		if overrides[key] != "global.anthropic.claude-fable-5" {
			t.Errorf("expected %s to map to global Fable profile, got %q", key, overrides[key])
		}
	}
}

func TestIsFable5Model(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"global.anthropic.claude-fable-5", true},
		{"us.anthropic.claude-fable-5", true},
		{"anthropic.claude-fable-5", true},
		{"global.anthropic.claude-fable-5[1m]", true},
		{"", false},
		{"global.anthropic.claude-sonnet-5", false},
		{"global.anthropic.claude-opus-4-8", false},
	}
	for _, c := range cases {
		if got := schema.IsFable5Model(c.model); got != c.want {
			t.Errorf("IsFable5Model(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

// TestBuild_FableDataRetentionWarning: Claude Fable 5's Bedrock model card
// requires opting in to provider_data_share before the model can be invoked
// (AWS docs: "in order to use Claude Fable 5, as required by Anthropic, you
// must opt in to sharing retained traffic with Anthropic for abuse detection
// and potential human review"). Juggernaut cannot check the account's actual
// retention setting (no queryable AWS API exists for it), so Build must
// surface a plain, no-promises warning whenever Fable is configured, rather
// than silently shipping calls that may be denied at runtime.
func TestBuild_FableDataRetentionWarning(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	found := false
	for _, w := range block.Warnings {
		if strings.Contains(w, "Fable") && strings.Contains(w, "data") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a Fable data-retention warning, got %v", block.Warnings)
	}
}

func TestBuild_NoFableDataRetentionWarningWhenFableNotConfigured(t *testing.T) {
	cfg := testConfig()
	cfg.Models.Fable = ""
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	for _, w := range block.Warnings {
		if strings.Contains(w, "Fable") {
			t.Errorf("expected no Fable warning when Fable is not configured, got %v", block.Warnings)
		}
	}
}

func TestBuild_FallbackModelsNativeKey(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		FallbackModels: []string{" global.anthropic.claude-opus-4-8 ", "global.anthropic.claude-sonnet-4-6"},
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	want := []string{"global.anthropic.claude-opus-4-8", "global.anthropic.claude-sonnet-4-6"}
	if !slices.Equal(block.Meta.FallbackModels, want) {
		t.Errorf("expected fallbackModels=%v, got %v", want, block.Meta.FallbackModels)
	}
	if !slices.Equal(block.NativeKeys().FallbackModel, want) {
		t.Errorf("expected native fallbackModel=%v, got %v", want, block.NativeKeys().FallbackModel)
	}
}

func TestBuild_FallbackModelsRejectEmptyEntries(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		FallbackModels: []string{"global.anthropic.claude-opus-4-8", " "},
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Fatal("expected empty fallback model ID to error")
	}
}

func TestBuild_AvailableModelsNativeKey(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		AvailableModels: []string{" sonnet ", "claude-opus-4-8", "haiku"},
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	want := []string{"sonnet", "claude-opus-4-8", "haiku"}
	if !slices.Equal(block.Meta.AvailableModels, want) {
		t.Errorf("expected meta.availableModels=%v, got %v", want, block.Meta.AvailableModels)
	}
	if !slices.Equal(block.NativeKeys().AvailableModels, want) {
		t.Errorf("expected native availableModels=%v, got %v", want, block.NativeKeys().AvailableModels)
	}
}

func TestBuild_AvailableModelsRejectEmptyEntries(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		AvailableModels: []string{"sonnet", " "},
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Fatal("expected empty available-models entry to error")
	}
}

func TestBuild_AvailableModelsPreservesOrderNoDedup(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		AvailableModels: []string{"haiku", "sonnet", "haiku"},
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	want := []string{"haiku", "sonnet", "haiku"}
	if !slices.Equal(block.Meta.AvailableModels, want) {
		t.Errorf("expected order preserved with no dedup, got %v", block.Meta.AvailableModels)
	}
}

func TestBuild_EnforceAvailableModelsWritesNativeKey(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		AvailableModels:        []string{"sonnet", "haiku"},
		EnforceAvailableModels: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if !block.Meta.EnforceAvailableModels {
		t.Error("expected meta.enforceAvailableModels=true")
	}
	if !block.NativeKeys().EnforceAvailableModels {
		t.Error("expected native enforceAvailableModels=true")
	}
}

func TestBuild_EnforceAvailableModelsWithoutListErrors(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		EnforceAvailableModels: true,
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Fatal("expected enforce-without-list to error")
	}
	if !strings.Contains(err.Error(), schema.ErrEnforceRequiresAvailable) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuild_EnforceAvailableModelsSucceedsWithNonEmptyList(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
		AvailableModels:        []string{"sonnet"},
		EnforceAvailableModels: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if !block.Meta.EnforceAvailableModels {
		t.Error("expected enforce to succeed with a valid non-empty list")
	}
}

func TestBuild_InvalidPermissionMode(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", PermissionMode: "bogus",
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Error("expected error for invalid permission mode")
	}
}

func TestBuild_InvalidServiceTier(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", ServiceTier: "turbo",
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Error("expected error for invalid service tier")
	}
}

func TestBuild_AutoMode_SetsBedrockEnvVar(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", PermissionMode: "auto", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_ENABLE_AUTO_MODE"] != "1" {
		t.Error("expected CLAUDE_CODE_ENABLE_AUTO_MODE=1 when mode=auto")
	}
}

func TestBuild_NonAutoMode_NoAutoModeEnvVar(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", PermissionMode: "plan", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_ENABLE_AUTO_MODE"] == "1" {
		t.Error("CLAUDE_CODE_ENABLE_AUTO_MODE should not be set for non-auto modes")
	}
}

func TestBuild_ServiceTier_WritesEnvVar(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", ServiceTier: "flex", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"] != "flex" {
		t.Errorf("expected ANTHROPIC_BEDROCK_SERVICE_TIER=flex, got %v", block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"])
	}
}

func TestNativeKeys_EffortLevel(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if native.EffortLevel != "xhigh" {
		t.Errorf("expected effortLevel=xhigh in NativeKeys, got %q", native.EffortLevel)
	}
}

func TestNativeKeys_MaxEffortIsEnvOnly(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "max",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if native.EffortLevel != "" {
		t.Errorf("expected max effort to be omitted from NativeKeys effortLevel, got %q", native.EffortLevel)
	}
	if native.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("expected CLAUDE_CODE_EFFORT_LEVEL=max, got %q", native.Env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
}

func TestNativeKeys_AutoEffortUsesEnvOnly(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "us-west-2",
		Effort:   "auto",
		Scope:    "user",
		Version:  "4.0.0",
	}

	built, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	native := built.NativeKeys()
	if native.EffortLevel != "" {
		t.Errorf("expected native effortLevel to be omitted for auto, got %q", native.EffortLevel)
	}
	if native.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "auto" {
		t.Errorf("expected CLAUDE_CODE_EFFORT_LEVEL=auto, got %q", native.Env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
}
func TestNativeKeys_ModelOverridesIncludeClaudeCodeVersionKeys(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	overrides := block.NativeKeys().ModelOverrides
	for _, key := range []string{"sonnet", "claude-sonnet-5", "anthropic.claude-sonnet-5"} {
		if overrides[key] != "global.anthropic.claude-sonnet-5" {
			t.Errorf("expected %s to map to global Sonnet profile, got %q", key, overrides[key])
		}
	}
	for _, key := range []string{"opus", "claude-opus-5", "anthropic.claude-opus-5"} {
		if overrides[key] != "global.anthropic.claude-opus-5" {
			t.Errorf("expected %s to map to global Opus profile, got %q", key, overrides[key])
		}
	}
	for _, key := range []string{"fable", "claude-fable-5", "anthropic.claude-fable-5"} {
		if overrides[key] != "global.anthropic.claude-fable-5" {
			t.Errorf("expected %s to map to global Fable profile, got %q", key, overrides[key])
		}
	}
}

func TestNativeKeys_SkipWebFetchPreflightAlwaysTrue(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if !native.SkipWebFetchPreflight {
		t.Error("expected skipWebFetchPreflight=true for all Bedrock configs")
	}
}

func TestNativeKeys_PermissionsMap(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", PermissionMode: "auto", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if native.Permissions == nil {
		t.Fatal("expected permissions map when PermissionMode is set")
	}
	if native.Permissions["defaultMode"] != "auto" {
		t.Errorf("expected permissions.defaultMode=auto, got %v", native.Permissions["defaultMode"])
	}
}

func TestNativeKeys_AlwaysThinking(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "xhigh",
		Scope: "user", Version: "4.1.0", AlwaysThinking: true, AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if !native.AlwaysThinking {
		t.Error("expected alwaysThinkingEnabled=true in NativeKeys")
	}
}

// TestBuild_Use1M_RecognizesGovCloudOpus covers the us-gov. prefix in the 1M
// context support check: a GovCloud Opus model must still be annotated [1m].
func TestBuild_Use1M_RecognizesGovCloudOpus(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		AuthValidated: true,
		Use1M:         true,
		OpusModel:     "us-gov.anthropic.claude-opus-4-8",
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	got := block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"]
	if got != "us-gov.anthropic.claude-opus-4-8[1m]" {
		t.Errorf("expected GovCloud Opus annotated with [1m], got %q", got)
	}
}

func TestBuild_NoBedrockFlagWithoutValidation(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		AuthValidated: false,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_USE_BEDROCK"] == "1" {
		t.Error("CLAUDE_CODE_USE_BEDROCK should NOT be set when AuthValidated=false")
	}
}

func TestIsAutoModeCapableModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		// Supported on Bedrock (verified against code.claude.com/docs/en/permission-modes):
		// Sonnet 5, and Opus 4.7 or later (4.7, 4.8, 5) — with any region prefix / [1m] suffix.
		{"global.anthropic.claude-opus-5", true},
		{"global.anthropic.claude-opus-5[1m]", true},
		{"global.anthropic.claude-opus-4-8", true},
		{"global.anthropic.claude-opus-4-8[1m]", true},
		{"us.anthropic.claude-opus-4-7", true},
		{"anthropic.claude-opus-4-7", true},
		{"claude-opus-4-8", true},
		{"claude-opus-5", true},
		{"global.anthropic.claude-sonnet-5", true},
		{"global.anthropic.claude-sonnet-5[1m]", true},
		{"us.anthropic.claude-sonnet-5", true},
		{"claude-sonnet-5", true},
		// Not supported: Sonnet 4.6, Haiku, older Opus, Fable, empty.
		{"global.anthropic.claude-sonnet-4-6", false},
		{"global.anthropic.claude-sonnet-4-6[1m]", false},
		{"us.anthropic.claude-opus-4-6", false},
		{"anthropic.claude-opus-4-5-20251101", false},
		{"global.anthropic.claude-haiku-4-5-20251001-v1:0", false},
		{"claude-fable-5", false},
		{"", false},
	}
	for _, c := range cases {
		if got := schema.IsAutoModeCapableModel(c.model); got != c.want {
			t.Errorf("IsAutoModeCapableModel(%q) = %v, want %v", c.model, got, c.want)
		}
	}
}

func TestBlock_AutoModeUsable_FalseWhenDefaultModelIsSonnet(t *testing.T) {
	// A Sonnet-4.6-tier active model => auto mode hidden on Bedrock. Pin it
	// explicitly (the default config now ships Sonnet 5, which IS capable).
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "5.2.0", PermissionMode: "auto", AuthValidated: true,
		SonnetModel: "global.anthropic.claude-sonnet-4-6",
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.AutoModeUsable() {
		t.Error("expected AutoModeUsable()=false when the active model is Sonnet 4.6")
	}
}

func TestBlock_AutoModeUsable_TrueWhenSonnetPinnedToOpus(t *testing.T) {
	// --model opus (or --sonnet-model opus) makes the active alias resolve to Opus.
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "5.2.0", PermissionMode: "auto", AuthValidated: true,
		SonnetModel: "global.anthropic.claude-opus-4-8",
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if !block.AutoModeUsable() {
		t.Error("expected AutoModeUsable()=true when the active model resolves to Opus 4.8")
	}
}

func TestBlock_AutoModeUsable_FalseWhenModeNotAuto(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "5.2.0", PermissionMode: "plan", AuthValidated: true,
		SonnetModel: "global.anthropic.claude-opus-4-8",
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.AutoModeUsable() {
		t.Error("expected AutoModeUsable()=false when PermissionMode is not auto")
	}
}

// autoOpts is the default-config auto-mode option set: Sonnet-tier default model
// (not capable) but the Opus override IS capable — the exact shape of JP's setup.
func autoOpts() schema.Options {
	return schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "5.3.0", PermissionMode: "auto", AuthValidated: true,
	}
}

// TestBlock_AutoModeAvailable_TrueWithDefaultConfig: the default config pins Sonnet
// as the default model but always configures Opus 5 as the opus override, which IS
// auto-capable — so auto mode is AVAILABLE (the enable var should be written) even
// though the default model isn't capable. This is JP's real scenario.
func TestBlock_AutoModeAvailable_TrueWithDefaultConfig(t *testing.T) {
	block, err := schema.Build(testConfig(), autoOpts())
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if !block.AutoModeAvailable() {
		t.Error("expected AutoModeAvailable()=true: the Opus override is auto-capable")
	}
}

// TestBlock_AutoModeAvailable_FalseWhenModeNotAuto: available only applies at mode=auto.
func TestBlock_AutoModeAvailable_FalseWhenModeNotAuto(t *testing.T) {
	opts := autoOpts()
	opts.PermissionMode = "plan"
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.AutoModeAvailable() {
		t.Error("expected AutoModeAvailable()=false when PermissionMode != auto")
	}
}

// TestBlock_AutoModeAvailable_FalseWhenNoCapableModel: force every model to a
// non-capable one → no configured model qualifies → not available.
func TestBlock_AutoModeAvailable_FalseWhenNoCapableModel(t *testing.T) {
	opts := autoOpts()
	// --model overrides all tiers → in schema.Options that's every per-tier field.
	opts.OpusModel = "global.anthropic.claude-sonnet-4-6"
	opts.SonnetModel = "global.anthropic.claude-sonnet-4-6"
	opts.HaikuModel = "global.anthropic.claude-sonnet-4-6"
	opts.FableModel = "global.anthropic.claude-sonnet-4-6"
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.AutoModeAvailable() {
		t.Error("expected AutoModeAvailable()=false when no configured model is auto-capable")
	}
}

// TestBuild_AutoMode_NoEnableVarWhenNoCapableModel: the enable var must NOT be written
// when no configured model can use auto mode (writing it would mislead — auto still
// wouldn't appear).
func TestBuild_AutoMode_NoEnableVarWhenNoCapableModel(t *testing.T) {
	opts := autoOpts()
	opts.OpusModel = "global.anthropic.claude-sonnet-4-6"
	opts.SonnetModel = "global.anthropic.claude-sonnet-4-6"
	opts.HaikuModel = "global.anthropic.claude-sonnet-4-6"
	opts.FableModel = "global.anthropic.claude-sonnet-4-6"
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_ENABLE_AUTO_MODE"] == "1" {
		t.Error("CLAUDE_CODE_ENABLE_AUTO_MODE must not be set when no configured model is capable")
	}
}
