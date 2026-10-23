package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func readClaudeSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	settings, err := testutil.ParseJSON(data)
	if err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	return settings
}

func juggernautSection(t *testing.T, settings map[string]any) map[string]any {
	t.Helper()
	jugg, ok := settings["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("settings.json has no juggernaut block")
	}
	return jugg
}

func TestApply_Endpoint_RejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		noWrite string
	}{
		{
			name:    "unknown endpoint",
			args:    []string{"apply", "--cli=claude", "--auth=iam", "--endpoint=bogus"},
			want:    "unknown endpoint",
			noWrite: ".claude/settings.json",
		},
		{
			name:    "Codex mantle rejects CRIS",
			args:    []string{"apply", "--cli=codex", "--auth=iam", "--endpoint=mantle", "--model=global.openai.gpt-6-astra"},
			want:    "cross-region inference profile",
			noWrite: ".codex/config.toml",
		},
		{
			name: "runtime --model pin rejected",
			args: []string{"apply", "--cli=claude", "--auth=iam", "--endpoint=mantle",
				"--model=global.anthropic.claude-sonnet-5"},
			want:    "not a mantle model ID",
			noWrite: ".claude/settings.json",
		},
		{
			name: "runtime --opus-model pin rejected",
			args: []string{"apply", "--cli=claude", "--auth=iam", "--endpoint=mantle",
				"--opus-model=anthropic.claude-sonnet-5-v1:0"},
			want:    "not a mantle model ID",
			noWrite: ".claude/settings.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := setupApplyTestWithReset(t)
			err := ExecuteArgs(tc.args)
			if err == nil {
				t.Fatalf("apply accepted invalid endpoint input %v", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
			if _, statErr := os.Stat(filepath.Join(home, tc.noWrite)); statErr == nil {
				t.Errorf("rejected apply wrote %s", tc.noWrite)
			}
		})
	}
}

func TestApply_Endpoint_MantleWritesDualEndpointConfig(t *testing.T) {
	home := setupApplyTestWithReset(t)
	err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2",
		"--endpoint=mantle", "--model=anthropic.claude-sonnet-5",
	})
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}

	settings := readClaudeSettings(t, home)
	auth, ok := juggernautSection(t, settings)["auth"].(map[string]any)
	if !ok {
		t.Fatal("juggernaut.auth missing")
	}
	if auth["endpoint"] != bedrock.EndpointMantle {
		t.Errorf("auth.endpoint = %v, want mantle", auth["endpoint"])
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatal("env missing")
	}
	if env["CLAUDE_CODE_USE_MANTLE"] != "1" {
		t.Errorf("CLAUDE_CODE_USE_MANTLE = %v, want 1", env["CLAUDE_CODE_USE_MANTLE"])
	}
	if env["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Error("mantle must not turn off CLAUDE_CODE_USE_BEDROCK")
	}
	if got := env["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "anthropic.claude-sonnet-5" {
		t.Errorf("sonnet env = %v, want the bare mantle ID", got)
	}
	if got := env["CLAUDE_CODE_SUBAGENT_MODEL"]; got != "anthropic.claude-sonnet-5" {
		t.Errorf("subagent env = %v, want the bare mantle ID", got)
	}
}

func TestApply_Endpoint_MantlePerTierModels(t *testing.T) {
	home := setupApplyTestWithReset(t)
	err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2", "--endpoint=mantle",
		"--opus-model=anthropic.claude-opus-4-7",
		"--sonnet-model=anthropic.claude-sonnet-5",
		"--haiku-model=anthropic.claude-haiku-4-5",
	})
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}

	settings := readClaudeSettings(t, home)
	models, ok := juggernautSection(t, settings)["modelOverrides"].(map[string]any)
	if !ok {
		t.Fatal("modelOverrides missing")
	}
	for tier, want := range map[string]string{
		"opus":   "anthropic.claude-opus-4-7",
		"sonnet": "anthropic.claude-sonnet-5",
		"haiku":  "anthropic.claude-haiku-4-5",
	} {
		if got := models[tier]; got != want {
			t.Errorf("modelOverrides.%s = %v, want %q", tier, got, want)
		}
	}
}

func TestApply_Endpoint_MantleReapplyPreservesPinsThenRuntimeResets(t *testing.T) {
	home := setupApplyTestWithReset(t)
	apply := func(extra ...string) {
		t.Helper()
		args := append([]string{"apply", "--cli=claude", "--auth=iam", "--region=us-west-2"}, extra...)
		if err := ExecuteArgs(args); err != nil {
			t.Fatalf("apply %v error: %v", extra, err)
		}
	}

	apply("--endpoint=mantle", "--model=anthropic.claude-sonnet-5")
	// A plain re-apply must keep the account-allowlisted mantle pins rather
	// than resetting them to runtime defaults.
	apply()

	settings := readClaudeSettings(t, home)
	jugg := juggernautSection(t, settings)
	auth := jugg["auth"].(map[string]any)
	if auth["endpoint"] != bedrock.EndpointMantle {
		t.Errorf("re-apply lost the mantle endpoint: %v", auth["endpoint"])
	}
	models := jugg["modelOverrides"].(map[string]any)
	if models["sonnet"] != "anthropic.claude-sonnet-5" {
		t.Errorf("re-apply reset the mantle pin to %v", models["sonnet"])
	}
	env := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_USE_MANTLE"] != "1" {
		t.Error("re-apply lost CLAUDE_CODE_USE_MANTLE")
	}

	// An explicit runtime endpoint removes both the persisted choice and the
	// mantle env, and restores runtime model IDs.
	apply("--endpoint=runtime")
	settings = readClaudeSettings(t, home)
	jugg = juggernautSection(t, settings)
	auth = jugg["auth"].(map[string]any)
	if _, ok := auth["endpoint"]; ok {
		t.Errorf("runtime apply kept auth.endpoint: %v", auth["endpoint"])
	}
	env = settings["env"].(map[string]any)
	if _, ok := env["CLAUDE_CODE_USE_MANTLE"]; ok {
		t.Error("runtime apply kept CLAUDE_CODE_USE_MANTLE")
	}
	models = jugg["modelOverrides"].(map[string]any)
	if bedrock.IsMantleModelID(models["sonnet"].(string)) {
		t.Errorf("runtime apply kept mantle model pin %v", models["sonnet"])
	}
}

func TestApply_Endpoint_MantleRegionSwitch(t *testing.T) {
	home := setupApplyTestWithReset(t)
	var applyErr error
	out := captureStdout(t, func() {
		applyErr = ExecuteArgs([]string{
			"apply", "--cli=claude", "--auth=iam", "--region=us-west-1",
			"--endpoint=mantle", "--model=anthropic.claude-sonnet-5",
		})
	})
	if applyErr != nil {
		t.Fatalf("apply error: %v", applyErr)
	}
	if !strings.Contains(out, "mantle endpoint") || !strings.Contains(out, "us-east-1") {
		t.Errorf("expected a mantle region-switch warning, got: %s", out)
	}

	settings := readClaudeSettings(t, home)
	auth := juggernautSection(t, settings)["auth"].(map[string]any)
	if auth["region"] != "us-east-1" {
		t.Errorf("auth.region = %v, want us-east-1", auth["region"])
	}
}

func TestApply_Endpoint_MantleWarnsWithoutMantleModel(t *testing.T) {
	home := setupApplyTestWithReset(t)
	var applyErr error
	out := captureStdout(t, func() {
		applyErr = ExecuteArgs([]string{
			"apply", "--cli=claude", "--auth=iam", "--region=us-west-2", "--endpoint=mantle",
		})
	})
	if applyErr != nil {
		t.Fatalf("apply error: %v", applyErr)
	}
	if !strings.Contains(out, "no mantle-format model is pinned") {
		t.Errorf("expected a no-mantle-model warning, got: %s", out)
	}
	settings := readClaudeSettings(t, home)
	env := settings["env"].(map[string]any)
	if env["CLAUDE_CODE_USE_MANTLE"] != "1" {
		t.Error("dual-endpoint config should still enable mantle")
	}
}

func TestApply_Endpoint_InheritedUnknownEndpointRejected(t *testing.T) {
	home := setupApplyTestWithReset(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2",
		"--endpoint=mantle", "--model=anthropic.claude-sonnet-5",
	}); err != nil {
		t.Fatalf("initial apply error: %v", err)
	}
	rewriteClaudeSettings(t, home, func(settings map[string]any) {
		settings["juggernaut"].(map[string]any)["auth"].(map[string]any)["endpoint"] = "bogus"
	})

	err := ExecuteArgs([]string{"apply", "--cli=claude", "--auth=iam", "--region=us-west-2"})
	if err == nil {
		t.Fatal("apply accepted an unknown inherited endpoint")
	}
	if !strings.Contains(err.Error(), "unknown endpoint") {
		t.Errorf("error = %v, want an unknown-endpoint message", err)
	}
}

func TestApply_Endpoint_MalformedMantleOverridesRejected(t *testing.T) {
	home := setupApplyTestWithReset(t)
	if err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2",
		"--endpoint=mantle", "--model=anthropic.claude-sonnet-5",
	}); err != nil {
		t.Fatalf("initial apply error: %v", err)
	}
	rewriteClaudeSettings(t, home, func(settings map[string]any) {
		settings["juggernaut"].(map[string]any)["modelOverrides"].(map[string]any)["sonnet"] = 123
	})

	err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2", "--endpoint=mantle",
	})
	if err == nil {
		t.Fatal("apply accepted malformed mantle model overrides")
	}
	if !strings.Contains(err.Error(), "reading existing mantle model overrides") {
		t.Errorf("error = %v, want a model-override read error", err)
	}
}

// readGrokTOML parses the applied Grok config through the same TOML format
// manager apply wrote it with.
func readGrokTOML(t *testing.T, home string) map[string]any {
	t.Helper()
	tomlFmt, err := config.FormatByName("toml")
	if err != nil {
		t.Fatalf("toml format: %v", err)
	}
	mgr := config.NewManagerWithFormat(filepath.Join(home, ".grok", "config.toml"), tomlFmt)
	got, err := mgr.Read()
	if err != nil {
		t.Fatalf("parse grok config.toml: %v", err)
	}
	return got
}

func grokTOMLModelBlock(t *testing.T, cfg map[string]any) map[string]any {
	t.Helper()
	modelTbl, ok := cfg["model"].(map[string]any)
	if !ok {
		t.Fatalf("[model] table missing: %v", cfg)
	}
	block, ok := modelTbl["bedrock-grok"].(map[string]any)
	if !ok {
		t.Fatalf("[model.bedrock-grok] missing: %v", modelTbl)
	}
	return block
}

// TestApply_Endpoint_GrokMantleWritesConfig: the Grok mantle opt-in writes the
// bare foundation model on the mantle base URL, the Responses wire API, the
// API-key auth-provider block, and the endpoint record launch/re-apply reads.
func TestApply_Endpoint_GrokMantleWritesConfig(t *testing.T) {
	home := setupApplyTestWithReset(t)
	setupIsolatedKeychain(t)

	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
		"--region=us-west-2", "--endpoint=mantle", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	cfg := readGrokTOML(t, home)
	block := grokTOMLModelBlock(t, cfg)
	if block["model"] != "xai.grok-4.6" {
		t.Errorf("model = %v, want the bare mantle default xai.grok-4.6", block["model"])
	}
	if block["base_url"] != "https://bedrock-mantle.us-west-2.api.aws/openai/v1" {
		t.Errorf("base_url = %v, want the mantle base URL", block["base_url"])
	}
	if block["api_backend"] != "responses" {
		t.Errorf("api_backend = %v, want responses", block["api_backend"])
	}
	if block["reasoning_summary"] != "none" {
		t.Errorf("reasoning_summary = %v, want none (mantle rejects reasoning.summary)", block["reasoning_summary"])
	}
	if got := fmt.Sprint(block["context_window"]); got != "500000" {
		t.Errorf("context_window = %v, want 500000", got)
	}

	authTbl, ok := cfg["auth"].(map[string]any)
	if !ok {
		t.Fatal("[auth] table missing")
	}
	if cmd, _ := authTbl["auth_provider_command"].(string); !strings.Contains(cmd, "auth-token") {
		t.Errorf("auth_provider_command = %v, want `juggernaut auth-token`", authTbl["auth_provider_command"])
	}

	jAuth, ok := testutil.NestedMapChain(cfg, "juggernaut", "auth")
	if !ok {
		t.Fatalf("juggernaut.auth block missing: %v", cfg)
	}
	jMap := jAuth.(map[string]any)
	if jMap["endpoint"] != bedrock.EndpointMantle {
		t.Errorf("juggernaut.auth.endpoint = %v, want mantle", jMap["endpoint"])
	}
	if jMap["mode"] != authmode.BedrockAPIKey {
		t.Errorf("juggernaut.auth.mode = %v, want %s", jMap["mode"], authmode.BedrockAPIKey)
	}
	if jMap["region"] != "us-west-2" {
		t.Errorf("juggernaut.auth.region = %v, want us-west-2", jMap["region"])
	}
}

// TestApply_Endpoint_GrokMantleModelFlag: Claude's anthropic.* mantle-ID
// validation must not run for Grok; the Grok provider validates the bare
// foundation ID and pins its own context window.
func TestApply_Endpoint_GrokMantleModelFlag(t *testing.T) {
	home := setupApplyTestWithReset(t)
	setupIsolatedKeychain(t)

	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
		"--region=us-east-1", "--endpoint=mantle", "--model=xai.grok-4.3", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	cfg := readGrokTOML(t, home)
	block := grokTOMLModelBlock(t, cfg)
	if block["model"] != "xai.grok-4.3" {
		t.Errorf("model = %v, want xai.grok-4.3", block["model"])
	}
	if got := fmt.Sprint(block["context_window"]); got != "1000000" {
		t.Errorf("context_window = %v, want 1000000", got)
	}
	if block["base_url"] != "https://bedrock-mantle.us-east-1.api.aws/openai/v1" {
		t.Errorf("base_url = %v, want the us-east-1 mantle URL (served by 4.3)", block["base_url"])
	}
}

// TestApply_Endpoint_GrokMantleRegionSwitchSingleWarning: a region that serves
// mantle but not the selected model is switched straight to the model's region
// with ONE warning (the endpoint-level list must not correct it first).
func TestApply_Endpoint_GrokMantleRegionSwitchSingleWarning(t *testing.T) {
	home := setupApplyTestWithReset(t)
	setupIsolatedKeychain(t)

	var applyErr error
	out := captureStdout(t, func() {
		applyErr = ExecuteArgs([]string{
			"apply", "--cli=grok", "--auth=" + authmode.BedrockAPIKey, bedrockKeyFlag(),
			"--region=eu-west-1", "--endpoint=mantle", "--skip-preflight",
		})
	})
	if applyErr != nil {
		t.Fatalf("apply error: %v", applyErr)
	}
	if !strings.Contains(out, "using us-west-2 instead") {
		t.Errorf("expected a switch to the model's first mantle region, got:\n%s", out)
	}
	if n := strings.Count(out, "⚠ mantle endpoint:"); n != 1 {
		t.Errorf("expected exactly one mantle region warning, got %d:\n%s", n, out)
	}

	cfg := readGrokTOML(t, home)
	block := grokTOMLModelBlock(t, cfg)
	if block["base_url"] != "https://bedrock-mantle.us-west-2.api.aws/openai/v1" {
		t.Errorf("base_url = %v, want the us-west-2 mantle URL", block["base_url"])
	}
	jAuth := cfg["juggernaut"].(map[string]any)["auth"].(map[string]any)
	if jAuth["region"] != "us-west-2" {
		t.Errorf("juggernaut.auth.region = %v, want us-west-2", jAuth["region"])
	}
}

// TestApply_Endpoint_GrokMantleIAM: IAM works on mantle too — Grok's [auth]
// provider mints a short-term Bedrock API key from the AWS credential chain,
// and the config still carries the Responses wire API and reasoning override.
func TestApply_Endpoint_GrokMantleIAM(t *testing.T) {
	home := setupApplyTestWithReset(t)

	if err := ExecuteArgs([]string{
		"apply", "--cli=grok", "--auth=iam", "--region=us-west-2", "--endpoint=mantle", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --auth=iam --endpoint=mantle: %v", err)
	}

	cfg := readGrokTOML(t, home)
	block := grokTOMLModelBlock(t, cfg)
	if block["base_url"] != "https://bedrock-mantle.us-west-2.api.aws/openai/v1" {
		t.Errorf("base_url = %v, want the mantle URL", block["base_url"])
	}
	if block["reasoning_summary"] != "none" {
		t.Errorf("reasoning_summary = %v, want none", block["reasoning_summary"])
	}
	authTbl, ok := cfg["auth"].(map[string]any)
	if !ok {
		t.Fatal("[auth] table missing")
	}
	wantCmd := "juggernaut auth-token --iam --region=us-west-2"
	if cmd, _ := authTbl["auth_provider_command"].(string); cmd != wantCmd {
		t.Errorf("auth_provider_command = %q, want %q", cmd, wantCmd)
	}
	jAuth := cfg["juggernaut"].(map[string]any)["auth"].(map[string]any)
	if jAuth["mode"] != authmode.IAM {
		t.Errorf("juggernaut.auth.mode = %v, want iam", jAuth["mode"])
	}
}

// TestApply_Endpoint_GrokMantleReapplyKeepsEndpoint: a plain re-apply keeps the
// mantle endpoint and is NOT mistaken for the v5 Mantle-era config (which the
// migration path would rewrite to bedrock-runtime with a migration notice).
func TestApply_Endpoint_GrokMantleReapplyKeepsEndpoint(t *testing.T) {
	home := setupApplyTestWithReset(t)
	setupIsolatedKeychain(t)

	applyCLI(t, "grok", "--endpoint=mantle")

	out := applyCLI(t, "grok")
	if strings.Contains(out, "written by Juggernaut v5") {
		t.Errorf("re-apply over a mantle config must NOT trigger the v5 migration notice, got:\n%s", out)
	}

	cfg := readGrokTOML(t, home)
	block := grokTOMLModelBlock(t, cfg)
	if block["base_url"] != "https://bedrock-mantle.us-west-2.api.aws/openai/v1" {
		t.Errorf("base_url = %v, want the mantle URL to survive re-apply", block["base_url"])
	}
	if block["model"] != "xai.grok-4.6" {
		t.Errorf("model = %v, want the mantle default to survive re-apply", block["model"])
	}
	jAuth := cfg["juggernaut"].(map[string]any)["auth"].(map[string]any)
	if jAuth["endpoint"] != bedrock.EndpointMantle {
		t.Errorf("juggernaut.auth.endpoint = %v, want mantle after re-apply", jAuth["endpoint"])
	}
}

// TestApply_Endpoint_GrokMantleToRuntimeClearsMantleOnlyKeys: switching back to
// runtime deep-merges the same [model.bedrock-grok] table, so the mantle-only
// api_backend must be actively cleared and the runtime default restored.
func TestApply_Endpoint_GrokMantleToRuntimeClearsMantleOnlyKeys(t *testing.T) {
	home := setupApplyTestWithReset(t)
	setupIsolatedKeychain(t)

	applyCLI(t, "grok", "--endpoint=mantle")
	applyCLI(t, "grok", "--endpoint=runtime")

	cfg := readGrokTOML(t, home)
	block := grokTOMLModelBlock(t, cfg)
	if got, ok := block["api_backend"]; ok {
		t.Errorf("api_backend = %v, want it cleared when switching to runtime", got)
	}
	if block["base_url"] != "https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1" {
		t.Errorf("base_url = %v, want the runtime URL", block["base_url"])
	}
	if block["model"] != "global.xai.grok-4.7" {
		t.Errorf("model = %v, want the runtime default", block["model"])
	}
	if got := fmt.Sprint(block["context_window"]); got != "524288" {
		t.Errorf("context_window = %v, want 524288", got)
	}
	jAuth := cfg["juggernaut"].(map[string]any)["auth"].(map[string]any)
	if _, ok := jAuth["endpoint"]; ok {
		t.Errorf("juggernaut.auth.endpoint = %v, want it cleared for runtime", jAuth["endpoint"])
	}
}
