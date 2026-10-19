package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
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
			name:    "mantle is Claude-only",
			args:    []string{"apply", "--cli=codex", "--auth=iam", "--endpoint=mantle"},
			want:    "not supported for Codex",
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
