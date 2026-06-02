package schema_test

import (
	"testing"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/schema"
)

func testConfig() *bedrock.Config {
	return &bedrock.Config{
		Version: "4.0.0",
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-4-7",
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
