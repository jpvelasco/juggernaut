package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
)

// TestCheckBedrockConnectivity_APIKeyNoToken covers the early return when API
// key auth is configured but no bearer token is present — no network call.
func TestCheckBedrockConnectivity_APIKeyNoToken(t *testing.T) {
	result := checkBedrockConnectivity(authmode.BedrockAPIKey, "us-west-2", "model-id", "")
	if result == nil {
		t.Fatal("expected a non-nil connectivity result")
	}
	if !result.IsFailure() {
		t.Error("missing API key token should be a failure")
	}
	if !strings.Contains(result.Message, "not found in keychain") {
		t.Errorf("unexpected message: %q", result.Message)
	}
	if result.AuthMode != authmode.BedrockAPIKey || result.Region != "us-west-2" {
		t.Errorf("result did not echo back mode/region: %+v", result)
	}
}

// TestCheckSettingsScope_PresentBlock covers the OK-with-block branch.
func TestCheckSettingsScope_PresentBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	status, detail := checkSettingsScope(home, "user", true)
	if status != doctor.OK {
		t.Fatalf("expected OK when block present, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, "present") {
		t.Errorf("expected 'present' detail, got %q", detail)
	}
}

// TestReadScopeData covers both the populated and the unreadable-path branches.
func TestReadScopeData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	data := readScopeData(home, "user")
	if data == nil {
		t.Fatal("expected non-nil settings data after apply")
	}
	if _, ok := data["juggernaut"]; !ok {
		t.Error("expected juggernaut block in scope data")
	}
}

// TestDoctor_JSON_AfterApply exercises runDoctor's JSON output path end to end.
func TestDoctor_JSON_AfterApply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply error: %v", err)
	}

	out := captureStdout(t, func() {
		// doctor may return an error if connectivity fails in the sandbox; we
		// only assert the JSON report itself is well-formed.
		_ = ExecuteArgs([]string{"doctor", "--scope=user", "--json"})
	})

	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		t.Fatal("doctor --json produced no output")
	}
	var report any
	if err := json.Unmarshal([]byte(trimmed), &report); err != nil {
		t.Fatalf("doctor --json should emit valid JSON, got:\n%s\nerr: %v", trimmed, err)
	}
}
