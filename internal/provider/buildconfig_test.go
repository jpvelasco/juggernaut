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

// TestMantleOnlyCLIs_NoNativeAuth: Codex/OpenCode/Grok route only through Mantle,
// which requires a bearer token, so none may claim CapNativeAuth (IAM/SSO). apply
// relies on this to reject --auth=iam for them.
func TestMantleOnlyCLIs_NoNativeAuth(t *testing.T) {
	for _, name := range []string{"codex", "opencode", "grok"} {
		p, _ := Get(name)
		if p.Supports(CapNativeAuth) {
			t.Errorf("%s is Mantle-only and must NOT support CapNativeAuth", name)
		}
	}
}

// TestCodex_LaunchSpec: no static enable flag (routes via config), bearer token
// still injected (Mantle requires it).
func TestCodex_LaunchSpec(t *testing.T) {
	p, _ := Get("codex")
	ls := p.LaunchSpec()
	if len(ls.StaticEnv) != 0 {
		t.Errorf("Codex should have no static enable flag, got %v", ls.StaticEnv)
	}
	if ls.TokenEnvVar != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Errorf("TokenEnvVar = %q, want AWS_BEARER_TOKEN_BEDROCK", ls.TokenEnvVar)
	}
	if !ls.NeedsToken {
		t.Error("Codex via Mantle needs a token")
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

// TestCodex_BuildConfig_MantleBlock verifies the Codex plan writes a
// [model_providers] block pointing at the correct per-model Mantle base_url +
// wire_api for the default model (gpt-5.5 → /openai/v1 + responses).
func TestCodex_BuildConfig_MantleBlock(t *testing.T) {
	p, _ := Get("codex")
	plan, err := p.BuildConfig(testConfig(), baseOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if plan.Keys["model_provider"] != "bedrock-mantle" {
		t.Errorf("model_provider = %v, want bedrock-mantle", plan.Keys["model_provider"])
	}
	mp, ok := plan.Keys["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers missing/wrong type: %T", plan.Keys["model_providers"])
	}
	bm, ok := mp["bedrock-mantle"].(map[string]any)
	if !ok {
		t.Fatalf("bedrock-mantle block missing: %v", mp)
	}
	if bm["wire_api"] != "responses" {
		t.Errorf("wire_api = %v, want responses (gpt-5.5 default)", bm["wire_api"])
	}
	// baseOpts uses the default region us-west-2 (non-explicit). gpt-5.5 is only
	// in us-east-1/2, so the region auto-switches to us-east-1.
	if got := bm["base_url"].(string); got != "https://bedrock-mantle.us-east-1.api.aws/openai/v1" {
		t.Errorf("base_url = %q, want us-east-1 /openai/v1 (auto-switched from default us-west-2)", got)
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected an auto-switch heads-up when the default region can't serve gpt-5.5")
	}
	if err := plan.Validate(); err != nil {
		t.Errorf("Codex plan should validate: %v", err)
	}
}

// TestCodex_BuildConfig_AuthCommand: the Mantle provider block uses a command-
// backed auth provider (reads the keychain via `juggernaut auth-token
// --format=token`) instead of env_key. env_key requires the launch wrapper to
// inject AWS_BEARER_TOKEN_BEDROCK; running codex directly then fails with
// "Missing environment variable". The auth.command is self-contained (verified
// in openai/codex external_bearer.rs: it execs the command and uses trimmed
// stdout as the bearer token, refreshing after a 401).
func TestCodex_BuildConfig_AuthCommand(t *testing.T) {
	p, _ := Get("codex")
	plan, err := p.BuildConfig(testConfig(), baseOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	bm := plan.Keys["model_providers"].(map[string]any)["bedrock-mantle"].(map[string]any)
	if _, hasEnvKey := bm["env_key"]; hasEnvKey {
		t.Errorf("env_key must NOT be set (it needs the launch wrapper); got %v", bm["env_key"])
	}
	auth, ok := bm["auth"].(map[string]any)
	if !ok {
		t.Fatalf("bedrock-mantle.auth block missing: %v", bm)
	}
	if auth["command"] != "juggernaut" {
		t.Errorf("auth.command = %v, want juggernaut", auth["command"])
	}
	args, ok := auth["args"].([]string)
	if !ok || len(args) < 2 || args[0] != "auth-token" || args[1] != "--format=token" {
		t.Errorf("auth.args = %v, want [auth-token --format=token]", auth["args"])
	}
}

// TestCodex_BuildConfig_SkipsOpenAILogin verifies the Mantle provider block sets
// requires_openai_auth = false. Without it, the Codex CLI shows its ChatGPT
// login screen on launch even with a valid custom provider
// (verified in openai/codex tui/src/lib.rs should_show_login_screen: login is
// skipped ONLY when the active provider's requires_openai_auth is false).
func TestCodex_BuildConfig_SkipsOpenAILogin(t *testing.T) {
	p, _ := Get("codex")
	plan, err := p.BuildConfig(testConfig(), baseOpts())
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	mp := plan.Keys["model_providers"].(map[string]any)
	bm := mp["bedrock-mantle"].(map[string]any)
	got, ok := bm["requires_openai_auth"]
	if !ok {
		t.Fatal("bedrock-mantle block must set requires_openai_auth to skip the ChatGPT login screen")
	}
	if got != false {
		t.Errorf("requires_openai_auth = %v, want false", got)
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
// known-good region and the base_url reflects the override. Juggernaut never
// writes a config that can't reach the model.
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
	bm := mp["bedrock-mantle"].(map[string]any)
	if got := bm["base_url"].(string); got != "https://bedrock-mantle.us-east-1.api.aws/openai/v1" {
		t.Errorf("base_url = %q, want us-east-1 (overridden from eu-west-1)", got)
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

// TestCodex_BuildConfig_Region uses the requested region in the base_url.
func TestCodex_BuildConfig_Region(t *testing.T) {
	p, _ := Get("codex")
	opts := baseOpts()
	opts.Region = "us-east-1"
	plan, _ := p.BuildConfig(testConfig(), opts)
	mp := plan.Keys["model_providers"].(map[string]any)
	bm := mp["bedrock-mantle"].(map[string]any)
	if got := bm["base_url"].(string); got != "https://bedrock-mantle.us-east-1.api.aws/openai/v1" {
		t.Errorf("base_url = %q, want us-east-1 /openai/v1", got)
	}
}
