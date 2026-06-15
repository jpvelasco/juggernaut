package schema_test

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v4/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v4/internal/schema"
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
		AuthMode: "iam", Region: "us-west-2", Effort: "max",
		Scope: "user", Version: "4.1.0", AuthValidated: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if native.EffortLevel != "max" {
		t.Errorf("expected effortLevel=max in NativeKeys, got %q", native.EffortLevel)
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
