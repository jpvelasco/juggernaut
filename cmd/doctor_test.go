package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

func TestOpusplanProblem(t *testing.T) {
	if _, ok := opusplanProblem(nil); ok {
		t.Fatal("nil settings should not trigger opusplan warning")
	}

	if _, ok := opusplanProblem(map[string]any{"model": "sonnet"}); ok {
		t.Fatal("non-opusplan model should not trigger warning")
	}

	detail, ok := opusplanProblem(map[string]any{"model": "opusplan"})
	if !ok {
		t.Fatal("opusplan should trigger warning")
	}
	if !strings.Contains(detail, "opusplan") {
		t.Fatalf("warning should mention opusplan, got %q", detail)
	}
}

func TestCheckAutoModeReadiness_SilentWhenNotConfigured(t *testing.T) {
	r := doctor.NewReport()

	checkAutoModeReadiness(r, "user", nil)
	if len(r.String()) != 0 {
		t.Fatalf("expected no check for nil settings, got: %s", r.String())
	}

	checkAutoModeReadiness(r, "user", map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"permissionMode": "default"},
		},
	})
	if len(r.String()) != 0 {
		t.Fatalf("expected no check when permissionMode is not auto, got: %s", r.String())
	}
}

func TestCheckAutoModeReadiness_OKWhenDefaultModelCapable(t *testing.T) {
	r := doctor.NewReport()
	checkAutoModeReadiness(r, "user", map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"permissionMode": "auto"},
			"modelOverrides": map[string]any{
				"opus":   "global.anthropic.claude-opus-4-8",
				"sonnet": "global.anthropic.claude-sonnet-5",
				"haiku":  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			},
		},
	})

	out := r.String()
	if !strings.Contains(out, "[OK]") {
		t.Fatalf("expected OK status, got: %s", out)
	}
	if !strings.Contains(out, "auto-capable") {
		t.Fatalf("expected detail to mention auto-capable, got: %s", out)
	}
}

// TestCheckAutoModeReadiness_ReconcilesNativePermissionsMode: Claude Code's
// Shift+Tab writes the effective mode straight to the native top-level
// permissions.defaultMode WITHOUT touching juggernaut.meta.permissionMode —
// apply's own re-apply path already accounts for this drift (cmd/apply.go's
// "adopt a permission mode set outside Juggernaut" comment). The readiness
// check must reconcile the same way: a user who Shift+Tab'd into auto mode
// without re-running apply is still on effective auto mode, and doctor must
// not stay silent just because the juggernaut block's own copy is stale.
func TestCheckAutoModeReadiness_ReconcilesNativePermissionsMode(t *testing.T) {
	r := doctor.NewReport()
	checkAutoModeReadiness(r, "user", map[string]any{
		"permissions": map[string]any{"defaultMode": "auto"},
		"juggernaut": map[string]any{
			"meta": map[string]any{"permissionMode": "default"}, // stale copy
			"modelOverrides": map[string]any{
				"opus":   "global.anthropic.claude-opus-4-8",
				"sonnet": "global.anthropic.claude-sonnet-5",
				"haiku":  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			},
		},
	})

	out := r.String()
	if !strings.Contains(out, "[OK]") {
		t.Fatalf("expected OK status reconciled from native permissions.defaultMode, got: %s", out)
	}
}

func TestCheckAutoModeReadiness_WarnWhenOnlyOpusCapable(t *testing.T) {
	r := doctor.NewReport()
	checkAutoModeReadiness(r, "user", map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"permissionMode": "auto"},
			"modelOverrides": map[string]any{
				"opus":   "global.anthropic.claude-opus-4-8",
				"sonnet": "global.anthropic.claude-sonnet-4-6", // not auto-capable
				"haiku":  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			},
		},
	})

	out := r.String()
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected WARN status, got: %s", out)
	}
	if !strings.Contains(out, "Opus") {
		t.Fatalf("expected detail to name Opus as the capable tier, got: %s", out)
	}
}

// TestCheckFableDataRetention_SilentWhenFableNotConfigured: no fable override
// present → no check emitted at all.
func TestCheckFableDataRetention_SilentWhenFableNotConfigured(t *testing.T) {
	r := doctor.NewReport()
	checkFableDataRetention(r, "user", map[string]any{
		"juggernaut": map[string]any{
			"modelOverrides": map[string]any{
				"opus":   "global.anthropic.claude-opus-4-8",
				"sonnet": "global.anthropic.claude-sonnet-5",
				"haiku":  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			},
		},
	})
	if len(r.String()) != 0 {
		t.Fatalf("expected no check without a configured Fable model, got: %s", r.String())
	}
}

// TestCheckFableDataRetention_WarnsWhenFableConfigured: doctor cannot verify
// the account's actual provider_data_share opt-in status (no AWS API exposes
// it), so it must always WARN when Fable is configured — this is a re-runnable
// reminder, not a one-time apply-time note that scrolls out of view.
func TestCheckFableDataRetention_WarnsWhenFableConfigured(t *testing.T) {
	r := doctor.NewReport()
	checkFableDataRetention(r, "user", map[string]any{
		"juggernaut": map[string]any{
			"modelOverrides": map[string]any{
				"fable": "global.anthropic.claude-fable-5",
			},
		},
	})
	out := r.String()
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected WARN status, got: %s", out)
	}
	if !strings.Contains(out, "provider_data_share") {
		t.Fatalf("expected detail to mention provider_data_share, got: %s", out)
	}
}

func TestCheckFableDataRetention_AbsentBlockIsNoOp(t *testing.T) {
	r := doctor.NewReport()
	checkFableDataRetention(r, "user", nil)
	if len(r.String()) != 0 {
		t.Fatalf("expected no check for nil settings, got: %s", r.String())
	}
}

// TestAutoModeAvailableDetail_DefaultBranchWhenOpusNotCapable is a direct unit
// test of the function in isolation (not reachable through
// checkAutoModeReadiness's real call pattern, since AutoModeAvailable()
// guarantees Opus is capable whenever Sonnet isn't and Fable never is) — it
// exists so the fallback message stays correct if AutoModeAvailable's
// capability set ever changes.
func TestAutoModeAvailableDetail_DefaultBranchWhenOpusNotCapable(t *testing.T) {
	block := schema.Block{
		Models: schema.ModelOverrides{
			Opus:   "global.anthropic.claude-opus-4-6", // not auto-capable
			Sonnet: "global.anthropic.claude-sonnet-4-6",
		},
	}

	got := autoModeAvailableDetail(block)
	if !strings.Contains(got, "no configured model tier supports it") {
		t.Errorf("expected fallback message, got: %q", got)
	}
	if strings.Contains(got, "Opus") {
		t.Errorf("fallback message must not recommend Opus when it isn't capable, got: %q", got)
	}
}

func TestCheckAutoModeReadiness_WarnWhenNoModelCapable(t *testing.T) {
	r := doctor.NewReport()
	checkAutoModeReadiness(r, "user", map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"permissionMode": "auto"},
			"modelOverrides": map[string]any{
				"opus":   "global.anthropic.claude-opus-4-6", // hand-edited to a non-capable model
				"sonnet": "global.anthropic.claude-sonnet-4-6",
				"haiku":  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			},
		},
	})

	out := r.String()
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected WARN status, got: %s", out)
	}
	if !strings.Contains(out, "no configured model supports auto mode") {
		t.Fatalf("expected detail to say no model supports auto mode, got: %s", out)
	}
}

func TestCheckAutoModeReadiness_AbsentBlockIsNoOp(t *testing.T) {
	r := doctor.NewReport()
	checkAutoModeReadiness(r, "user", map[string]any{
		"juggernaut": "not-a-map", // malformed
	})
	if len(r.String()) != 0 {
		t.Fatalf("expected no-op for malformed juggernaut block, got: %s", r.String())
	}

	checkAutoModeReadiness(r, "user", map[string]any{
		"someOtherKey": "value", // juggernaut key entirely absent
	})
	if len(r.String()) != 0 {
		t.Fatalf("expected no-op for missing juggernaut key, got: %s", r.String())
	}
}

// TestCheckAutoModeReadiness_WarnsOnDeserializationError: a juggernaut block
// that IS a map but has a field whose type doesn't match schema.Block (e.g.
// corrupted by a hand-edit or a future schema-version drift) must emit a WARN
// naming the parse failure, not silently disappear — a corrupted block is not
// the same as "auto mode not configured", and the user needs to know the
// check couldn't run at all.
func TestCheckAutoModeReadiness_WarnsOnDeserializationError(t *testing.T) {
	r := doctor.NewReport()
	checkAutoModeReadiness(r, "user", map[string]any{
		"juggernaut": map[string]any{
			// schemaVersion is declared as int in schema.Meta; a string here
			// makes the JSON round-trip in fromMap fail.
			"meta": map[string]any{"schemaVersion": "not-a-number", "permissionMode": "auto"},
		},
	})

	out := r.String()
	if !strings.Contains(out, "[WARN]") {
		t.Fatalf("expected WARN status on deserialization failure, got: %s", out)
	}
	if !strings.Contains(out, "could not parse juggernaut block") {
		t.Fatalf("expected detail to explain the parse failure, got: %s", out)
	}
	if !strings.Contains(out, "juggernaut apply") {
		t.Fatalf("expected detail to guide the user to re-run apply, got: %s", out)
	}
}

func TestClaudeCommandStatus_OKWhenRealClaudeFound(t *testing.T) {
	dir := t.TempDir()
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.cmd"
	}
	claude := filepath.Join(dir, name)
	writeExecutableStub(t, claude)
	t.Setenv("PATH", dir)

	status, detail := claudeCommandStatus()
	if status != doctor.OK {
		t.Fatalf("expected OK, got %s (%s)", status, detail)
	}
	if detail != claude {
		t.Fatalf("expected claude path %s, got %s", claude, detail)
	}
}

// TestClaudeCommandStatus_WarnWhenMissing covers the not-found branch: an empty
// PATH yields a Warn with guidance instead of a path.
func TestClaudeCommandStatus_WarnWhenMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // a dir containing no claude binary

	status, detail := claudeCommandStatus()
	if status != doctor.Warn {
		t.Fatalf("expected Warn when no claude on PATH, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, "not found on PATH") {
		t.Errorf("expected 'not found on PATH' guidance, got %q", detail)
	}
}

func TestCheckSettingsScope_DefaultProjectMissingIsOK(t *testing.T) {
	home := testutil.NewTestHome(t)

	status, detail := checkSettingsScope(home, "project", false)
	if status != doctor.OK {
		t.Fatalf("expected OK for missing project scope, got %s (%s)", status, detail)
	}
	if detail != "not configured" {
		t.Fatalf("expected not configured detail, got %q", detail)
	}
}

func TestCheckSettingsScope_RequiredScopeMissingFails(t *testing.T) {
	home := testutil.NewTestHome(t)

	status, detail := checkSettingsScope(home, "user", true)
	if status != doctor.Fail {
		t.Fatalf("expected FAIL for missing required scope, got %s (%s)", status, detail)
	}
}

func TestCheckProviderConfigScope_CodexMissingRequiredFails(t *testing.T) {
	home := testutil.NewTestHome(t)
	prov, err := provider.Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	status, detail := checkProviderConfigScope(prov, home, "user", true)
	if status != doctor.Fail {
		t.Fatalf("expected FAIL for missing codex config, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, "--cli=codex") {
		t.Errorf("expected apply --cli=codex guidance, got %q", detail)
	}
}

func TestCheckProviderConfigScope_CodexOwnedOK(t *testing.T) {
	home := testutil.NewTestHome(t)
	prov, err := provider.Get("codex")
	if err != nil {
		t.Fatal(err)
	}
	path, err := prov.ConfigPath(home, "user")
	if err != nil {
		t.Fatal(err)
	}
	if err := safepath.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	// Minimal OwnsConfig marker for Codex.
	if err := os.WriteFile(path, []byte("model_provider = \"amazon-bedrock\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, detail := checkProviderConfigScope(prov, home, "user", true)
	if status != doctor.OK {
		t.Fatalf("expected OK for owned codex config, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, "present") {
		t.Errorf("expected present detail, got %q", detail)
	}
}

// TestDoctor_Codex_MissingConfig_FailsWithGuidance: doctor --cli=codex on a
// fresh home fails the config check with actionable apply guidance — the
// end-to-end wiring of checkProviderConfigScope into runDoctor for a
// non-Claude CLI.
func TestDoctor_Codex_MissingConfig_FailsWithGuidance(t *testing.T) {
	_ = setupApplyTest(t)

	out := captureStdout(t, func() {
		err := ExecuteArgs([]string{"doctor", "--cli=codex"})
		if err == nil {
			t.Fatal("expected doctor to fail when codex is not configured")
		}
	})
	if !strings.Contains(out, "config.toml (user)") {
		t.Errorf("expected the codex config check line, got:\n%s", out)
	}
	if !strings.Contains(out, "run `juggernaut apply --cli=codex`") {
		t.Errorf("expected apply --cli=codex guidance, got:\n%s", out)
	}
}

// TestDoctor_Codex_HealthyAfterApply: after a real codex apply, doctor
// --cli=codex reports the user config as juggernaut-managed and finds no
// failures — the mirror image of the missing-config case.
func TestDoctor_Codex_HealthyAfterApply(t *testing.T) {
	_ = setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real credential; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=codex", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-east-1", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=codex: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"doctor", "--cli=codex"}); err != nil {
			t.Fatalf("doctor --cli=codex should find no failures after apply: %v", err)
		}
	})
	if !strings.Contains(out, "juggernaut-managed Bedrock config present") {
		t.Errorf("expected managed-config OK check, got:\n%s", out)
	}
}

// TestDoctor_OpenCode_MissingConfig_FailsWithGuidance: doctor --cli=opencode
// on a fresh home fails the config check with actionable apply guidance.
func TestDoctor_OpenCode_MissingConfig_FailsWithGuidance(t *testing.T) {
	_ = setupApplyTest(t)

	out := captureStdout(t, func() {
		err := ExecuteArgs([]string{"doctor", "--cli=opencode"})
		if err == nil {
			t.Fatal("expected doctor to fail when opencode is not configured")
		}
	})
	if !strings.Contains(out, "opencode.json (user)") {
		t.Errorf("expected the opencode config check line, got:\n%s", out)
	}
	if !strings.Contains(out, "run `juggernaut apply --cli=opencode`") {
		t.Errorf("expected apply --cli=opencode guidance, got:\n%s", out)
	}
}

// TestDoctor_OpenCode_HealthyAfterApply: after a real opencode apply, doctor
// --cli=opencode reports the user config as juggernaut-managed and finds no
// failures.
func TestDoctor_OpenCode_HealthyAfterApply(t *testing.T) {
	_ = setupApplyTest(t)
	setupIsolatedKeychain(t) // stores a real credential; skip if keychain backend hangs (macOS CI)

	if err := ExecuteArgs([]string{
		"apply", "--cli=opencode", "--auth=" + authmode.BedrockAPIKey, "--bedrock-key=test-key-value", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=opencode: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"doctor", "--cli=opencode"}); err != nil {
			t.Fatalf("doctor --cli=opencode should find no failures after apply: %v", err)
		}
	})
	if !strings.Contains(out, "juggernaut-managed Bedrock config present") {
		t.Errorf("expected managed-config OK check, got:\n%s", out)
	}
}

func TestCheckRuntimeFallback_ReportsConfigDrift(t *testing.T) {
	home := testutil.NewTestHome(t)
	prov := provider.MustGet("claude")
	if err := activation.SaveRuntimeState(home, "claude", activation.RuntimeState{
		AuthMode: authmode.IAM,
		Env:      map[string]string{"AWS_REGION": "us-west-2"},
	}); err != nil {
		t.Fatal(err)
	}

	r := doctor.NewReport()
	checkRuntimeFallback(r, prov, home)
	out := r.String()
	if !strings.Contains(out, "[WARN]") || !strings.Contains(out, "managed user config is missing") {
		t.Fatalf("expected runtime fallback drift warning, got:\n%s", out)
	}
}

func TestCheckRuntimeFallback_OKWithManagedConfig(t *testing.T) {
	home := setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatal(err)
	}

	r := doctor.NewReport()
	checkRuntimeFallback(r, provider.MustGet("claude"), home)
	out := r.String()
	if !strings.Contains(out, "[OK]") || !strings.Contains(out, "saved for iam auth") {
		t.Fatalf("expected healthy runtime fallback, got:\n%s", out)
	}
}

func TestCheckRuntimeFallbackEdgeCases(t *testing.T) {
	t.Run("provider without persistence", func(t *testing.T) {
		r := doctor.NewReport()
		checkRuntimeFallback(r, provider.MustGet("codex"), testutil.NewTestHome(t))
		if out := r.String(); out != "" {
			t.Fatalf("non-persistent provider reported runtime fallback:\n%s", out)
		}
	})

	t.Run("missing state", func(t *testing.T) {
		r := doctor.NewReport()
		checkRuntimeFallback(r, provider.MustGet("claude"), testutil.NewTestHome(t))
		if out := r.String(); out != "" {
			t.Fatalf("missing runtime fallback produced a report:\n%s", out)
		}
	})

	t.Run("invalid state", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		path, err := activation.RuntimeStatePath(home, "claude")
		if err != nil {
			t.Fatal(err)
		}
		if err := safepath.WriteFile(home, path, []byte(`{`)); err != nil {
			t.Fatal(err)
		}

		r := doctor.NewReport()
		checkRuntimeFallback(r, provider.MustGet("claude"), home)
		out := r.String()
		if !strings.Contains(out, "[WARN]") || !strings.Contains(out, "invalid:") ||
			!strings.Contains(out, "juggernaut apply") {
			t.Fatalf("invalid fallback report:\n%s", out)
		}
	})

	t.Run("unreadable user config", func(t *testing.T) {
		home := testutil.NewTestHome(t)
		if err := activation.SaveRuntimeState(home, "claude", activation.RuntimeState{
			AuthMode: authmode.IAM,
		}); err != nil {
			t.Fatal(err)
		}
		configPath, err := provider.MustGet("claude").ConfigPath(home, "user")
		if err != nil {
			t.Fatal(err)
		}
		if err := safepath.MkdirAll(configPath); err != nil {
			t.Fatal(err)
		}

		r := doctor.NewReport()
		checkRuntimeFallback(r, provider.MustGet("claude"), home)
		out := r.String()
		if !strings.Contains(out, "[WARN]") || !strings.Contains(out, "could not be read") {
			t.Fatalf("unreadable config report:\n%s", out)
		}
	})
}

func TestDoctorProjectScopeSkipsUserRuntimeFallback(t *testing.T) {
	home := setupApplyTest(t)
	if err := activation.SaveRuntimeState(home, "claude", activation.RuntimeState{
		AuthMode: authmode.IAM,
	}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		_ = ExecuteArgs([]string{"doctor", "--scope=project"})
	})
	if strings.Contains(out, "runtime fallback") {
		t.Fatalf("project-only doctor reported user runtime fallback:\n%s", out)
	}
}

func TestCliBinaryStatus_WarnWhenMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	prov, err := provider.Get("opencode")
	if err != nil {
		t.Fatal(err)
	}
	status, detail := cliBinaryStatus(prov)
	if status != doctor.Warn {
		t.Fatalf("expected Warn when opencode missing, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, "not found on PATH") {
		t.Errorf("expected PATH guidance, got %q", detail)
	}
}

func TestDoctor_UnknownCLIRejected(t *testing.T) {
	resetFlags()
	err := ExecuteArgs([]string{"doctor", "--cli=nonesuch"})
	if err == nil {
		t.Fatal("expected error for unknown --cli")
	}
	if !strings.Contains(err.Error(), "unknown CLI") {
		t.Errorf("expected unknown CLI error, got %v", err)
	}
}

func TestMigrateCommandIsNotRegistered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "migrate" {
			t.Fatal("migrate command should not be registered in v5")
		}
	}
}

func TestLaunchCommandIsHidden(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "launch" {
			found = true
			if !cmd.Hidden {
				t.Fatal("launch command should be hidden")
			}
		}
	}
	if !found {
		t.Fatal("launch command should be registered")
	}
}

func writeExecutableStub(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("real claude"), 0o600); err != nil {
		t.Fatalf("creating executable stub: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission, go_file-permissions_rule-fileperm
			t.Fatalf("making executable stub runnable: %v", err)
		}
	}
}

func TestDoctor_ActivationUnhealthy_NoActivationInDiscoveredProfile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	home := setupApplyTest(t)

	// Real discovered profile has no activation block
	realProfile := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(realProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(realProfile), realProfile, []byte(
		"export FOO=bar",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	// Set up mock discovery
	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe": mockPSOutputJSON(realProfile, realProfile),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	activation.SetPSRunnerForTesting(runner)
	defer activation.ResetPSRunnerForTesting()

	// Check activation using the shared resolver
	healthy, _, _ := activation.CheckPowerShellActivation(home)
	if healthy {
		t.Error("doctor should NOT report activation healthy when discovered profile has no activation")
	}
}

func TestDoctor_ActivationHealthy_AfterInstall(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	home := setupApplyTest(t)

	realProfile := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(realProfile)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(realProfile), realProfile, []byte(
		"export FOO=bar",
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe": mockPSOutputJSON(realProfile, realProfile),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	activation.SetPSRunnerForTesting(runner)
	defer activation.ResetPSRunnerForTesting()

	// Apply install
	_, err := activation.InstallPowerShellActivation(home)
	if err != nil {
		t.Fatalf("InstallPowerShellActivation: %v", err)
	}

	// After install, should be healthy
	healthy, path, warnings := activation.CheckPowerShellActivation(home)
	if !healthy {
		t.Error("doctor should report activation healthy after install")
	}
	if path != realProfile {
		t.Errorf("expected path %s, got %s", realProfile, path)
	}
	if len(warnings) > 0 {
		t.Errorf("expected no warnings after install, got %v", warnings)
	}
}

func TestDoctor_DiscoveryCalledOncePerEdition(t *testing.T) {
	// Regression: doctor previously resolved profiles twice — once in doctor.go
	// and again inside CheckPowerShellActivation. With the fix, the resolver
	// result is passed in, so each edition is called exactly once.
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	home := setupApplyTest(t)

	allHosts := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(allHosts)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(allHosts), allHosts, []byte(activation.Block(activation.ShellPowerShell))); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	runner := &mockPSRunner{
		counts: make(map[string]int),
		output: map[string][]byte{
			"pwsh.exe": mockPSOutputJSON(allHosts, allHosts),
		},
		err: map[string]error{
			"powershell.exe": os.ErrNotExist,
		},
	}
	activation.SetPSRunnerForTesting(runner)
	defer activation.ResetPSRunnerForTesting()

	// Run doctor
	resetFlags()
	rootCmd.SetArgs([]string{"doctor"})
	rootCmd.SetOut(&strings.Builder{})
	rootCmd.SetErr(&strings.Builder{})
	_ = rootCmd.Execute()

	// Each edition should be called at most once
	if runner.counts["pwsh.exe"] > 1 {
		t.Errorf("pwsh.exe was called %d times, expected at most 1", runner.counts["pwsh.exe"])
	}
	if runner.counts["powershell.exe"] > 1 {
		t.Errorf("powershell.exe was called %d times, expected at most 1", runner.counts["powershell.exe"])
	}
}

func TestDoctor_MultiProfileLegacyOverride_DetectsWarning(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}

	home := setupApplyTest(t)

	// PS7 AllHosts: has the current activation block (healthy)
	ps7All := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(ps7All)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(ps7All), ps7All, []byte(
		activation.BeginMarker+"\n$env.TEST='1'\n"+activation.EndMarker,
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	// PS5.1 CurrentHost: has a legacy launcher block (overrides activation)
	ps5Host := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile_5host.ps1")
	if err := safepath.MkdirAll(filepath.Dir(ps5Host)); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(ps5Host), ps5Host, []byte(
		activation.LegacyLauncherBegin+"\n$env.LEGACY='1'\n"+activation.LegacyLauncherEnd,
	)); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	// Mock discovery: PS7 returns AllHosts, PS5.1 returns CurrentHost
	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockPSOutputJSON(ps7All, ps7All),
			"powershell.exe": mockPSOutputJSON(ps5Host, ps5Host),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	defer activation.ResetPSRunnerForTesting()

	// Check activation — should be healthy but with a warning
	healthy, _, warnings := activation.CheckPowerShellActivation(home)
	if !healthy {
		t.Fatal("doctor should report activation healthy (AllHosts has the block)")
	}
	if len(warnings) == 0 {
		t.Fatal("doctor should emit a warning about legacy block in later profile")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, ps5Host) && strings.Contains(w, "legacy") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about legacy block in %s, got: %v", ps5Host, warnings)
	}
}

// TestDoctor_ReportsAutoModeReadiness_EndToEnd verifies checkAutoModeReadiness
// is actually wired into runDoctor's per-scope loop: a real `apply --mode=auto`
// followed by `doctor` must surface the auto-mode readiness line.
func TestDoctor_ReportsAutoModeReadiness_EndToEnd(t *testing.T) {
	_ = setupApplyTest(t)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--mode=auto", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out := captureStdout(t, func() {
		// doctor may return an error if connectivity fails in the sandbox; we
		// only assert the auto-mode readiness line is present in the report.
		_ = ExecuteArgs([]string{"doctor"})
	})

	if !strings.Contains(out, "auto-mode readiness") {
		t.Fatalf("expected doctor output to include auto-mode readiness check, got:\n%s", out)
	}
}

// TestCheckBedrockConnectivity_IAMMode: IAM mode always calls
// CheckIAMConnectivity regardless of token presence.
func TestCheckBedrockConnectivity_IAMMode(t *testing.T) {
	result := checkBedrockConnectivity("iam", "us-west-2", "anthropic.claude-opus-4-8", "")
	// The result depends on whether AWS credentials are available in the
	// test environment; we only assert the call doesn't panic and returns
	// a result with the expected auth mode.
	if result.AuthMode != "iam" {
		t.Errorf("auth mode = %q, want iam", result.AuthMode)
	}
}

// TestCheckConnectivity_MissingBedrockConfig: when bedrock-config.json
// cannot be loaded, checkConnectivity emits a Warn rather than failing.
func TestCheckConnectivity_MissingBedrockConfig(t *testing.T) {
	r := doctor.NewReport()
	// Use a home directory that has no bedrock-config.json.
	home := testutil.NewTestHome(t)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(home); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "cannot load bedrock-config.json") {
		t.Errorf("expected bedrock-config load warning, got: %s", out)
	}
}

// TestCheckConnectivity_MissingJuggernautBlock: when settings.json has no
// juggernaut block, checkConnectivity emits a Warn.
func TestCheckConnectivity_MissingJuggernautBlock(t *testing.T) {
	r := doctor.NewReport()
	home := setupApplyTest(t)
	// Ensure the .claude directory exists.
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission -- directory
		t.Fatal(err)
	}
	// Write a settings.json with no juggernaut block.
	settings := map[string]any{"other": "value"}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	checkConnectivity(r, home, "some-token", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "no juggernaut block in settings.json") {
		t.Errorf("expected missing juggernaut block warning, got: %s", out)
	}
}
