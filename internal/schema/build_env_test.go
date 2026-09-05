package schema_test

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

func TestBuildEnv_DefaultOptions(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region: "us-west-2",
		Effort: "high",
		Use1M:  true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if block.Env["AWS_REGION"] != "us-west-2" {
		t.Errorf("expected AWS_REGION=us-west-2, got %q", block.Env["AWS_REGION"])
	}
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-5[1m]" {
		t.Errorf("expected annotated opus model, got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "global.anthropic.claude-sonnet-5[1m]" {
		t.Errorf("expected annotated sonnet model, got %q", block.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
		t.Errorf("expected haiku model (no 1m), got %q", block.Env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if block.Env["CLAUDE_CODE_SUBAGENT_MODEL"] != "global.anthropic.claude-haiku-4-5-20251001-v1:0" {
		t.Errorf("expected subagent model, got %q", block.Env["CLAUDE_CODE_SUBAGENT_MODEL"])
	}
	if block.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "high" {
		t.Errorf("expected effort=high, got %q", block.Env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
	// Config-level env vars should be present
	if block.Env["CLAUDE_CODE_MAX_OUTPUT_TOKENS"] != "32768" {
		t.Errorf("expected config env var, got %q", block.Env["CLAUDE_CODE_MAX_OUTPUT_TOKENS"])
	}
}

func TestBuildEnv_AuthValidated(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Error("expected CLAUDE_CODE_USE_BEDROCK=1 when AuthValidated=true")
	}
}

func TestBuildEnv_AuthNotValidated(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: false,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if _, ok := block.Env["CLAUDE_CODE_USE_BEDROCK"]; ok {
		t.Error("CLAUDE_CODE_USE_BEDROCK should NOT be set when AuthValidated=false")
	}
}

func TestBuildEnv_Opusplan(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		Opusplan:      true,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_MODEL"] != "opusplan[1m]" {
		t.Errorf("expected ANTHROPIC_MODEL=opusplan[1m], got %q", block.Env["ANTHROPIC_MODEL"])
	}
}

func TestBuildEnv_Opusplan_No1M(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         false,
		Opusplan:      true,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_MODEL"] != "opusplan" {
		t.Errorf("expected ANTHROPIC_MODEL=opusplan (no 1m), got %q", block.Env["ANTHROPIC_MODEL"])
	}
}

func TestBuildEnv_Use1M_False(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         false,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"] != "1" {
		t.Errorf("expected CLAUDE_CODE_DISABLE_1M_CONTEXT=1, got %q", block.Env["CLAUDE_CODE_DISABLE_1M_CONTEXT"])
	}
	// Models should not have [1m] suffix
	if block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "global.anthropic.claude-opus-5" {
		t.Errorf("expected opus without [1m], got %q", block.Env["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
}

func TestBuildEnv_FableModel(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "global.anthropic.claude-fable-5[1m]" {
		t.Errorf("expected fable model with [1m], got %q", block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"])
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_NAME"] != "Fable 5.1" {
		t.Errorf("expected Fable display name, got %q", block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_NAME"])
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_DESCRIPTION"] == "" {
		t.Error("expected Fable description to be set")
	}
	if block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL_SUPPORTED_CAPABILITIES"] == "" {
		t.Error("expected Fable capabilities to be set")
	}
}

func TestBuildEnv_FableModel_Empty(t *testing.T) {
	cfg := testConfig()
	cfg.Models.Fable = ""
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if _, ok := block.Env["ANTHROPIC_DEFAULT_FABLE_MODEL"]; ok {
		t.Error("Fable env vars should not be set when fable model is empty")
	}
}

func TestBuildEnv_AutoMode_CapableModel(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:         "us-west-2",
		Effort:         "high",
		Use1M:          true,
		PermissionMode: "auto",
		AuthValidated:  true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["CLAUDE_CODE_ENABLE_AUTO_MODE"] != "1" {
		t.Error("expected CLAUDE_CODE_ENABLE_AUTO_MODE=1 with capable models")
	}
}

func TestBuildEnv_AutoMode_IncapableModel(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:         "us-west-2",
		Effort:         "high",
		Use1M:          true,
		PermissionMode: "auto",
		AuthValidated:  true,
		OpusModel:      "global.anthropic.claude-sonnet-4-6",
		SonnetModel:    "global.anthropic.claude-sonnet-4-6",
		HaikuModel:     "global.anthropic.claude-sonnet-4-6",
		FableModel:     "global.anthropic.claude-sonnet-4-6",
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if _, ok := block.Env["CLAUDE_CODE_ENABLE_AUTO_MODE"]; ok {
		t.Error("auto mode env should not be set when no model is capable")
	}
}

func TestBuildEnv_ServiceTier(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		ServiceTier:   "flex",
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"] != "flex" {
		t.Errorf("expected ANTHROPIC_BEDROCK_SERVICE_TIER=flex, got %q", block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"])
	}
}

func TestBuildEnv_ServiceTier_Priority(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		ServiceTier:   "priority",
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"] != "priority" {
		t.Errorf("expected ANTHROPIC_BEDROCK_SERVICE_TIER=priority, got %q", block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"])
	}
}

func TestBuildEnv_ServiceTier_Empty(t *testing.T) {
	cfg := testConfig()
	opts := schema.Options{
		Region:        "us-west-2",
		Effort:        "high",
		Use1M:         true,
		ServiceTier:   "",
		AuthValidated: true,
	}
	block, err := schema.Build(cfg, opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if _, ok := block.Env["ANTHROPIC_BEDROCK_SERVICE_TIER"]; ok {
		t.Error("service tier env should not be set when ServiceTier is empty")
	}
}
