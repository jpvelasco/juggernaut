package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// validateProfileCalls records stub invocations of validateAWSProfile; each
// apply test restores the real implementation and resets the log.
var validateProfileCalls []string

func stubValidateProfileOK(t *testing.T) {
	t.Helper()
	orig := validateAWSProfile
	validateAWSProfile = func(_ context.Context, _ string, profile string) error {
		validateProfileCalls = append(validateProfileCalls, profile)
		return nil
	}
	t.Cleanup(func() {
		validateAWSProfile = orig
		validateProfileCalls = nil
	})
}

func stubValidateProfileErr(t *testing.T, err error) {
	t.Helper()
	orig := validateAWSProfile
	validateAWSProfile = func(_ context.Context, _ string, profile string) error {
		validateProfileCalls = append(validateProfileCalls, profile)
		return err
	}
	t.Cleanup(func() {
		validateAWSProfile = orig
		validateProfileCalls = nil
	})
}

func juggernautAuthValue(t *testing.T, home, key string) string {
	t.Helper()
	settings, err := testutil.ParseJSON(readSettingsJSON(t, home))
	if err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	block, ok := settings["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("missing juggernaut block")
	}
	auth, ok := block["auth"].(map[string]any)
	if !ok {
		t.Fatal("missing auth in juggernaut block")
	}
	v, _ := auth[key].(string)
	return v
}

func settingsEnvValue(t *testing.T, home, key string) string {
	t.Helper()
	settings, err := testutil.ParseJSON(readSettingsJSON(t, home))
	if err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := env[key].(string)
	return v
}

// TestApply_WritesProfile_IAM pins the full --profile write path for Claude:
// the profile is persisted in both places consumers read it — the managed env (the CLI's
// own environment injection) and juggernaut.auth.profile (read by launch and
// doctor).
func TestApply_WritesProfile_IAM(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=developer",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	if got := juggernautAuthValue(t, home, "profile"); got != "developer" {
		t.Errorf("juggernaut.auth.profile = %q, want developer", got)
	}
	if got := settingsEnvValue(t, home, "AWS_PROFILE"); got != "developer" {
		t.Errorf("env AWS_PROFILE = %q, want developer", got)
	}
	if len(validateProfileCalls) != 1 || validateProfileCalls[0] != "developer" {
		t.Errorf("validateAWSProfile calls = %v, want exactly [developer]", validateProfileCalls)
	}
}

// TestApply_Profile_NoValidationWithoutProfile: without --profile the
// validator is never invoked and no profile is persisted (ambient chain).
func TestApply_Profile_NoValidationWithoutProfile(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	if len(validateProfileCalls) != 0 {
		t.Errorf("validateAWSProfile must not run without --profile; calls = %v", validateProfileCalls)
	}
	if got := juggernautAuthValue(t, home, "profile"); got != "" {
		t.Errorf("juggernaut.auth.profile = %q, want absent for default chain", got)
	}
}

// TestApply_ProfileRejectedWithBearerAuth: bearer-token auth never reads an
// AWS profile, so combining the flags is a command error before any write.
func TestApply_ProfileRejectedWithBearerAuth(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=k",
		"--region=us-west-2",
		"--profile=developer",
		"--scope=user",
		"--skip-preflight",
	})
	if err == nil {
		t.Fatal("apply accepted --profile with bearer auth")
	}
	if !strings.Contains(err.Error(), "--profile requires --auth=iam") {
		t.Errorf("error = %v, want a --profile-requires-iam message", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(statErr) {
		t.Error("nothing must be written when --profile is rejected")
	}
}

// TestApply_ProfileInvalidName: names outside the shell-safe grammar are
// rejected up front (they can be embedded in shell commands later).
func TestApply_ProfileInvalidName(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=a b",
		"--scope=user",
		"--skip-preflight",
	})
	if err == nil {
		t.Fatal("apply accepted an unsafe --profile name")
	}
	if !strings.Contains(err.Error(), "invalid --profile") {
		t.Errorf("error = %v, want an invalid --profile message", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(statErr) {
		t.Error("nothing must be written when the profile name is rejected")
	}
}

// TestApply_ProfileValidatedBeforeWrite: when the profile cannot resolve
// (swap-able stub simulates a missing shared-config profile), apply must fail
// BEFORE writing, so a typo'd profile never produces a "successful" broken
// config — the same fail-fast contract as --auth.
func TestApply_ProfileValidatedBeforeWrite(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileErr(t, context.Canceled)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=developer",
		"--scope=user",
		"--skip-preflight",
	})
	if err == nil {
		t.Fatal("apply must fail when the profile cannot resolve")
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(statErr) {
		t.Error("nothing must be written when profile validation fails")
	}
}

// TestApply_ReapplyPreservesProfile: re-applying without --profile keeps the
// previously persisted profile — matches the region/auth-mode preservation
// contract so a plain `juggernaut apply` cannot silently drop it.
func TestApply_ReapplyPreservesProfile(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--profile=developer",
		"--region=us-west-2",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	validateProfileCalls = nil // the second run's validation is not the point
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply error: %v", err)
	}

	if got := juggernautAuthValue(t, home, "profile"); got != "developer" {
		t.Errorf("profile after re-apply = %q, want preserved developer", got)
	}
	if got := settingsEnvValue(t, home, "AWS_PROFILE"); got != "developer" {
		t.Errorf("env AWS_PROFILE after re-apply = %q, want developer", got)
	}
}

// TestApply_NoProfile_ClearsStoredProfile: --no-profile on a re-apply drops
// the stored profile from every consumer (juggernaut.auth.profile and the
// managed env) and returns to the ambient chain. This is the escape hatch for
// a profile deleted from the shared config, where every plain re-apply fails
// on re-validation.
func TestApply_NoProfile_ClearsStoredProfile(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=developer",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}

	// Once the stored profile stops resolving, a plain re-apply must stay
	// stuck on validation — the trap --no-profile exists to escape.
	validateProfileCalls = nil
	stubValidateProfileErr(t, context.Canceled)
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--scope=user",
		"--skip-preflight",
	}); err == nil {
		t.Fatal("plain re-apply must fail once the stored profile stops resolving")
	}

	// The exit hatch clears the profile without validating it (there is
	// nothing left to validate) and still writes a managed config.
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--no-profile",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("clearing re-apply error: %v", err)
	}
	if got := juggernautAuthValue(t, home, "profile"); got != "" {
		t.Errorf("juggernaut.auth.profile after --no-profile = %q, want cleared", got)
	}
	if got := settingsEnvValue(t, home, "AWS_PROFILE"); got != "" {
		t.Errorf("env AWS_PROFILE after --no-profile = %q, want cleared", got)
	}
}

// TestApply_NoProfile_ConflictWithProfile: the precedence chain only holds
// when the explicit pin and the explicit clear cannot both be set — the
// contradiction is a command error before anything is written.
func TestApply_NoProfile_ConflictWithProfile(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=developer",
		"--no-profile",
		"--scope=user",
		"--skip-preflight",
	})
	if err == nil {
		t.Fatal("apply accepted --profile together with --no-profile")
	}
	if !strings.Contains(err.Error(), "--no-profile cannot be combined with --profile") {
		t.Errorf("error = %v, want the conflicting-flags message", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(statErr) {
		t.Error("nothing must be written when the flags conflict")
	}
}

// TestApply_NoProfile_InertWithBearerAuth: bearer auth never stores a profile,
// so --no-profile is accepted (not a command error) and changes nothing —
// pinned so a regression can neither break bearer applies nor route a
// profile through them.
func TestApply_NoProfile_InertWithBearerAuth(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=" + authmode.BedrockAPIKey,
		"--bedrock-key=k",
		"--region=us-west-2",
		"--no-profile",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("bearer apply with --no-profile error: %v", err)
	}
	if len(validateProfileCalls) != 0 {
		t.Errorf("validator must not run for bearer auth; calls = %v", validateProfileCalls)
	}
	if got := juggernautAuthValue(t, home, "profile"); got != "" {
		t.Errorf("juggernaut.auth.profile = %q, want absent for bearer auth", got)
	}
}

// TestApply_ExplicitProfileOverridesOnReapply: --profile on a re-apply always
// wins, matching the documented --auth/--region precedence.
func TestApply_ExplicitProfileOverridesOnReapply(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=old",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("first apply error: %v", err)
	}
	validateProfileCalls = nil
	if err := ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=developer",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("re-apply error: %v", err)
	}

	if got := juggernautAuthValue(t, home, "profile"); got != "developer" {
		t.Errorf("profile after override = %q, want developer", got)
	}
}

// TestApply_GrokIAMAuthCommandCarriesProfile: Grok minted-key mode must embed
// the named profile in auth_provider_command so the short-term key is minted
// from the profile's credentials.
func TestApply_GrokIAMAuthCommandCarriesProfile(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	grokApply := []string{
		"apply",
		"--cli=grok",
		"--auth=iam",
		"--region=us-west-2",
		"--profile=developer",
		"--scope=user",
		"--skip-preflight",
	}
	if err := ExecuteArgs(grokApply); err != nil {
		t.Fatalf("apply --cli=grok error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".grok", "config.toml"))
	if err != nil {
		t.Fatalf("reading grok config.toml: %v", err)
	}
	cfg := string(data)
	if !strings.Contains(cfg, "auth_provider_command") || !strings.Contains(cfg, "--profile=developer") {
		t.Errorf("auth_provider_command does not carry --profile=developer:\n%s", cfg)
	}
	if !strings.Contains(cfg, `profile = "developer"`) {
		t.Errorf("grok config juggernaut.auth.profile not written:\n%s", cfg)
	}
}

// TestApply_GrokIAMAuthCommandUnchangedWithoutProfile: existing Grok IAM
// configs must not change shape when no profile is requested.
func TestApply_GrokIAMAuthCommandUnchangedWithoutProfile(t *testing.T) {
	home := setupApplyTest(t)
	stubValidateProfileOK(t)

	if err := ExecuteArgs([]string{
		"apply",
		"--cli=grok",
		"--auth=iam",
		"--region=us-west-2",
		"--scope=user",
		"--skip-preflight",
	}); err != nil {
		t.Fatalf("apply --cli=grok error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".grok", "config.toml"))
	if err != nil {
		t.Fatalf("reading grok config.toml: %v", err)
	}
	if strings.Contains(string(data), "--profile=") {
		t.Errorf("auth_provider_command must not contain a profile without --profile:\n%s", data)
	}
}
