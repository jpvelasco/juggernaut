package schema_test

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

func testConfig() *bedrock.Config {
	return &bedrock.Config{
		Version: "4.1.0",
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-4-8",
			Sonnet: "global.anthropic.claude-sonnet-4-6",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
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
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "us-west-2",
		Effort:   "turbo",
		Scope:    "user",
		Version:  "4.0.0",
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Error("expected error for invalid effort level")
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
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-4-8[1m]" {
		t.Errorf("expected Opus env model with [1m], got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-4-6[1m]" {
		t.Errorf("expected Sonnet env model with [1m], got %q", block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
		t.Errorf("Haiku should not get [1m], got %q", block.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
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
	if native.ModelOverrides["claude-sonnet-4-6[1m]"] != "global.anthropic.claude-sonnet-4-6" {
		t.Errorf("expected [1m] Sonnet override to map to unsuffixed provider ID, got %q", native.ModelOverrides["claude-sonnet-4-6[1m]"])
	}
	if native.ModelOverrides["claude-opus-4-8[1m]"] != "global.anthropic.claude-opus-4-8" {
		t.Errorf("expected [1m] Opus override to map to unsuffixed provider ID, got %q", native.ModelOverrides["claude-opus-4-8[1m]"])
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
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-4-8" {
		t.Errorf("expected Opus env model without [1m], got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-4-6" {
		t.Errorf("expected Sonnet env model without [1m], got %q", block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"] != "1" {
		t.Errorf("expected CLAUDE_CODE_DISABLE_1M_CONTEXT=1, got %q", block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"])
	}
	if _, ok := block.NativeKeys().ModelOverrides["claude-sonnet-4-6[1m]"]; ok {
		t.Error("[1m] model override should be omitted when Use1M=false")
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
	for _, key := range []string{"sonnet", "claude-sonnet-4-6", "anthropic.claude-sonnet-4-6"} {
		if overrides[key] != "global.anthropic.claude-sonnet-4-6" {
			t.Errorf("expected %s to map to global Sonnet profile, got %q", key, overrides[key])
		}
	}
	for _, key := range []string{"opus", "claude-opus-4-8", "anthropic.claude-opus-4-8"} {
		if overrides[key] != "global.anthropic.claude-opus-4-8" {
			t.Errorf("expected %s to map to global Opus profile, got %q", key, overrides[key])
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

func TestBuild_Mantle_StripsGlobalPrefix(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		AuthValidated: true,
		UseMantle:     true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Models.Opus != "anthropic.claude-opus-4-8" {
		t.Errorf("expected opus without global. prefix, got %s", block.Models.Opus)
	}
	if block.Models.Sonnet != "anthropic.claude-sonnet-4-6" {
		t.Errorf("expected sonnet without global. prefix, got %s", block.Models.Sonnet)
	}
	if block.Models.Haiku != "anthropic.claude-haiku-4-5-20251001-v1:0" {
		t.Errorf("expected haiku without global. prefix, got %s", block.Models.Haiku)
	}
}

func TestBuild_Mantle_StripsRegionalInferenceProfilePrefix(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		AuthValidated: true,
		UseMantle:     true,
		SonnetModel:   "us.anthropic.claude-sonnet-4-6",
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Models.Sonnet != "anthropic.claude-sonnet-4-6" {
		t.Errorf("expected sonnet without us. prefix, got %s", block.Models.Sonnet)
	}
}

func TestBuild_NoMantle_KeepsGlobalPrefix(t *testing.T) {
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "xhigh",
		Scope:         "user",
		Version:       "4.0.0",
		AuthValidated: true,
		UseMantle:     false,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Models.Opus != "global.anthropic.claude-opus-4-8" {
		t.Errorf("expected opus with global. prefix preserved, got %s", block.Models.Opus)
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
		// Supported on Bedrock: Opus 4.7 and 4.8 (with any region prefix / [1m]).
		{"global.anthropic.claude-opus-4-8", true},
		{"global.anthropic.claude-opus-4-8[1m]", true},
		{"us.anthropic.claude-opus-4-7", true},
		{"anthropic.claude-opus-4-7", true},
		{"claude-opus-4-8", true},
		// Not supported: Sonnet, Haiku, older Opus, Fable, empty.
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
	// Default config: Sonnet-tier active model => auto mode hidden on Bedrock.
	opts := schema.Options{
		AuthMode: "iam", Region: "us-west-2", Effort: "high",
		Scope: "user", Version: "5.1.6", PermissionMode: "auto", AuthValidated: true,
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
		Scope: "user", Version: "5.1.6", PermissionMode: "auto", AuthValidated: true,
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
		Scope: "user", Version: "5.1.6", PermissionMode: "plan", AuthValidated: true,
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
