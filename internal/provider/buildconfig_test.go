package provider

import (
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

// testConfig is a minimal embedded-style bedrock.Config for provider tests.
func testConfig() *bedrock.Config {
	return &bedrock.Config{
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-4-8",
			Sonnet: "global.anthropic.claude-sonnet-4-6",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
		},
		Regions:     []string{"us-west-2", "us-east-1"},
		Environment: map[string]string{},
	}
}

func baseOpts() Options {
	return Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Effort:        "high",
		Scope:         "user",
		Version:       "9.9.9",
		AuthValidated: true,
	}
}

// TestClaude_BuildConfig_ProducesManagedKeys verifies Claude's ConfigPlan carries
// the same top-level native keys commitApply writes today, so rewiring cmd/ to
// use BuildConfig cannot drift the settings.json shape.
func TestClaude_BuildConfig_ProducesManagedKeys(t *testing.T) {
	p, _ := Get("claude")
	plan, err := p.BuildConfig(testConfig(), baseOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	want := []string{"juggernaut", "env", "modelOverrides", "effortLevel", "skipWebFetchPreflight"}
	for _, k := range want {
		if _, ok := plan.Keys[k]; !ok {
			t.Errorf("Claude ConfigPlan.Keys missing %q", k)
		}
	}
}

// TestClaude_BuildConfig_EnvHasBedrockModels spot-checks the env carries the
// Claude-specific model vars (proving BuildConfig wraps schema.Build, not a stub).
func TestClaude_BuildConfig_EnvHasBedrockModels(t *testing.T) {
	p, _ := Get("claude")
	plan, _ := p.BuildConfig(testConfig(), baseOpts())
	env, _ := plan.Keys["env"].(map[string]string)
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] == "" {
		t.Errorf("expected ANTHROPIC_DEFAULT_SONNET_MODEL in env, got: %v", env)
	}
	if env["AWS_REGION"] != "us-west-2" {
		t.Errorf("AWS_REGION = %q, want us-west-2", env["AWS_REGION"])
	}
}

// TestClaude_BuildConfig_AvailableModels verifies that AvailableModels and
// EnforceAvailableModels are threaded through provider.Options to ConfigPlan.Keys.
func TestClaude_BuildConfig_AvailableModels(t *testing.T) {
	p, _ := Get("claude")
	opts := baseOpts()
	opts.AvailableModels = []string{"sonnet", "haiku"}
	opts.EnforceAvailableModels = true
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	got, ok := plan.Keys["availableModels"].([]string)
	if !ok || len(got) != 2 || got[0] != "sonnet" || got[1] != "haiku" {
		t.Errorf("expected availableModels=[sonnet haiku], got %#v", plan.Keys["availableModels"])
	}
	if plan.Keys["enforceAvailableModels"] != true {
		t.Errorf("expected enforceAvailableModels=true, got %#v", plan.Keys["enforceAvailableModels"])
	}
}

// TestClaude_LaunchSpec pins Claude's runtime injection: the use-bedrock flag +
// the bearer token env var (what activation.Launch hardcodes today).
func TestClaude_LaunchSpec(t *testing.T) {
	p, _ := Get("claude")
	ls := p.LaunchSpec()
	if ls.StaticEnv["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Errorf("expected CLAUDE_CODE_USE_BEDROCK=1, got %v", ls.StaticEnv)
	}
	if ls.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("TokenEnvVar = %q, want AWS_BEARER_TOKEN_BEDROCK", ls.TokenEnvVar)
	}
	// NeedsToken MUST be false for Claude: its token requirement is
	// auth-mode-dependent (IAM/SSO need none), decided at launch via
	// needsBearerToken(authModes). Forcing true breaks Claude+IAM launches.
	if ls.NeedsToken {
		t.Error("Claude LaunchSpec.NeedsToken must be false (auth-mode-dependent, not static)")
	}
}

// TestClaude_Supports covers the Claude-only capabilities.
func TestClaude_Supports(t *testing.T) {
	p, _ := Get("claude")
	for _, c := range []Capability{CapAutoMode, Cap1MContext, CapOpusplan, CapThinking, CapServiceTiers, CapEffortLevels, CapNativeAuth} {
		if !p.Supports(c) {
			t.Errorf("Claude should support capability %d", c)
		}
	}
}

// TestMantleOnlyCLIs_NoNativeAuth: OpenCode and Grok route only through Mantle,
// which requires a bearer token, so they must NOT claim CapNativeAuth (IAM/SSO).
// Codex now supports IAM via the AWS SDK credential chain.
func TestMantleOnlyCLIs_NoNativeAuth(t *testing.T) {
	for _, name := range []string{"opencode", "grok"} {
		p, _ := Get(name)
		if p.Supports(CapNativeAuth) {
			t.Errorf("%s is Mantle-only and must NOT support CapNativeAuth", name)
		}
	}
}

// TestCodex_LaunchSpec: no static enable flag (routes via config), NeedsToken is
// false — auth mode (IAM or API key) is stored in the config.toml juggernaut block
// and resolved at launch time.
func TestCodex_LaunchSpec(t *testing.T) {
	p, _ := Get("codex")
	ls := p.LaunchSpec()
	if len(ls.StaticEnv) != 0 {
		t.Errorf("Codex should have no static enable flag, got %v", ls.StaticEnv)
	}
	if ls.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("TokenEnvVar = %q, want AWS_BEARER_TOKEN_BEDROCK", ls.TokenEnvVar)
	}
	// Codex now supports both IAM and API key — NeedsToken=false because the
	// launch wrapper reads auth mode from the config file to decide at runtime.
	if ls.NeedsToken {
		t.Error("Codex NeedsToken must be false — auth mode resolved from config at launch")
	}
}

// TestCodex_Supports: only effort levels; the Claude-only caps are absent.
func TestCodex_Supports(t *testing.T) {
	p, _ := Get("codex")
	if !p.Supports(CapEffortLevels) {
		t.Error("Codex should support effort levels")
	}
	for _, c := range []Capability{CapAutoMode, Cap1MContext, CapOpusplan, CapServiceTiers} {
		if p.Supports(c) {
			t.Errorf("Codex should NOT support Claude-only capability %d", c)
		}
	}
}

// TestCodex_BuildConfig_AmazonBedrockProvider verifies the Codex plan writes
// the built-in amazon-bedrock provider shape: model, model_provider, and a
// nested [model_providers.amazon-bedrock.aws] table with region. This provider
// ships a model catalog (eliminates "Model metadata not found" warnings and
// /model 404s that occurred with a custom bedrock-mantle provider).
func TestCodex_BuildConfig_AmazonBedrockProvider(t *testing.T) {
	p, _ := Get("codex")
	plan, err := p.BuildConfig(testConfig(), baseOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if plan.Keys["model_provider"] != "amazon-bedrock" {
		t.Errorf("model_provider = %v, want amazon-bedrock", plan.Keys["model_provider"])
	}
	if plan.Keys["model"] != "openai.gpt-5.5" {
		t.Errorf("model = %v, want openai.gpt-5.5", plan.Keys["model"])
	}
	mp, ok := plan.Keys["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers missing/wrong type: %T", plan.Keys["model_providers"])
	}
	ab, ok := mp["amazon-bedrock"].(map[string]any)
	if !ok {
		t.Fatalf("amazon-bedrock block missing: %v", mp)
	}
	aws, ok := ab["aws"].(map[string]any)
	if !ok {
		t.Fatalf("aws sub-table missing: %v", ab)
	}
	// baseOpts uses the default region us-west-2 (non-explicit). gpt-5.5 is only
	// in us-east-1/2, so the region auto-switches to us-east-1.
	if got := aws["region"].(string); got != "us-east-1" {
		t.Errorf("region = %q, want us-east-1 (auto-switched from default us-west-2)", got)
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected an auto-switch heads-up when the default region can't serve gpt-5.5")
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("Codex plan should validate: %v", err)
	}
}

// TestCodex_BuildConfig_UnknownModel errors on an unlisted model.
func TestCodex_BuildConfig_UnknownModel(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Model = "gpt-nonesuch"
	if _, err := p.BuildConfig(testConfig(), opts); err == nil {
		t.Error("expected error for unknown Codex model")
	}
}

// TestCodex_BuildConfig_RegionIronFist: gpt-5.5 is us-east-1/2 only, so ANY
// non-serving region — even one the user passed explicitly — is overridden to a
// known-good region. Juggernaut never writes a config that can't reach the model.
func TestCodex_BuildConfig_RegionIronFist(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Model = "gpt-5.5"
	opts.Region = "eu-west-1" // not in gpt-5.5's known regions
	opts.RegionExplicit = true
	plan, err := p.BuildConfig(testConfig(), opts)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	mp := plan.Keys["model_providers"].(map[string]any)
	ab := mp["amazon-bedrock"].(map[string]any)
	aws := ab["aws"].(map[string]any)
	if got := aws["region"].(string); got != "us-east-1" {
		t.Errorf("region = %q, want us-east-1 (overridden from eu-west-1)", got)
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected a message noting the region override")
	}
}

// TestCodex_BuildConfig_GptOssRejected: gpt-oss is no longer a valid Codex model
// (Codex is Responses-only; gpt-oss on Mantle is Chat-only), so BuildConfig must
// error rather than emit a config Codex would reject at load.
func TestCodex_BuildConfig_GptOssRejected(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Model = "gpt-oss-120b"
	if _, err := p.BuildConfig(testConfig(), opts); err == nil {
		t.Error("expected gpt-oss-120b to be rejected for Codex (Responses-only)")
	}
}

// TestCodex_BuildConfig_Region uses the requested region in the aws sub-table.
func TestCodex_BuildConfig_Region(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Region = "us-east-1"
	plan, _ := p.BuildConfig(testConfig(), opts)
	mp := plan.Keys["model_providers"].(map[string]any)
	ab := mp["amazon-bedrock"].(map[string]any)
	aws := ab["aws"].(map[string]any)
	if got := aws["region"].(string); got != "us-east-1" {
		t.Errorf("region = %q, want us-east-1", got)
	}
}

// TestCodex_BuildConfig_ExplicitServingRegion: when the user explicitly
// requests a region that serves the model, no warning is emitted and the
// region is kept as-is. This covers the "happy path" that resolveMantleRegion
// takes when the explicit region is already in the known set.
func TestCodex_BuildConfig_ExplicitServingRegion(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Region = "us-east-1"
	opts.RegionExplicit = true
	opts.Model = "gpt-5.5"
	plan, _ := p.BuildConfig(testConfig(), opts)
	mp := plan.Keys["model_providers"].(map[string]any)
	ab := mp["amazon-bedrock"].(map[string]any)
	aws := ab["aws"].(map[string]any)
	if got := aws["region"].(string); got != "us-east-1" {
		t.Errorf("region = %q, want us-east-1 (explicit serving region)", got)
	}
	if len(plan.Warnings) > 0 {
		t.Errorf("explicit serving region must not warn, got: %v", plan.Warnings)
	}
}
