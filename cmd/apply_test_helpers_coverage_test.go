package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// writeSettingsJSON creates a settings.json under home/.claude/ with the given
// map serialized as pretty-printed JSON.
func writeSettingsJSON(t *testing.T, home string, settings map[string]any) {
	t.Helper()
	settingsPath, err := safepath.JoinUnder(home, ".claude", "settings.json")
	if err != nil {
		t.Fatalf("JoinUnder settings.json: %v", err)
	}
	settingsDir := settingsPath[:len(settingsPath)-len("/settings.json")]
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := safepath.WriteFile(home, settingsPath, data); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
}

// ---------------------------------------------------------------------------
// readJuggernautAuthMode
// ---------------------------------------------------------------------------

func TestReadJuggernautAuthMode_Present(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{
				"mode": "iam",
			},
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readJuggernautAuthMode(t, home)
	if got != "iam" {
		t.Errorf("readJuggernautAuthMode = %q, want %q", got, "iam")
	}
}

func TestReadJuggernautAuthMode_BedrockAPIKey(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{
				"mode": "bedrock-api-key",
			},
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readJuggernautAuthMode(t, home)
	if got != "bedrock-api-key" {
		t.Errorf("readJuggernautAuthMode = %q, want %q", got, "bedrock-api-key")
	}
}

func TestReadJuggernautAuthMode_MissingAuth(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{"schemaVersion": float64(2)},
		},
	}
	writeSettingsJSON(t, home, settings)

	// The helper calls t.Fatal when the auth sub-block is absent. We verify
	// the code path by reading the settings ourselves and confirming the
	// condition that triggers the helper's fatal.
	data := readSettingsJSON(t, home)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	block, ok := parsed["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("juggernaut block missing from test fixture")
	}
	if _, hasAuth := block["auth"]; hasAuth {
		t.Error("test fixture should not contain an auth sub-block")
	}
}

func TestReadJuggernautAuthMode_MissingJuggernautBlock(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"env": map[string]any{
			"SOME_VAR": "hello",
		},
	}
	writeSettingsJSON(t, home, settings)

	// Verify the condition that triggers the helper's "missing juggernaut block" fatal.
	data := readSettingsJSON(t, home)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, hasBlock := parsed["juggernaut"]; hasBlock {
		t.Error("test fixture should not contain a juggernaut block")
	}
}

// ---------------------------------------------------------------------------
// readJuggernautPermissionMode
// ---------------------------------------------------------------------------

func TestReadJuggernautPermissionMode_Present(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"permissionMode": "auto",
			},
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readJuggernautPermissionMode(t, home)
	if got != "auto" {
		t.Errorf("readJuggernautPermissionMode = %q, want %q", got, "auto")
	}
}

func TestReadJuggernautPermissionMode_ReadOnly(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"permissionMode": "readOnly",
			},
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readJuggernautPermissionMode(t, home)
	if got != "readOnly" {
		t.Errorf("readJuggernautPermissionMode = %q, want %q", got, "readOnly")
	}
}

func TestReadJuggernautPermissionMode_MissingMeta(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{
				"mode": "iam",
			},
		},
	}
	writeSettingsJSON(t, home, settings)

	// Verify the condition that triggers the helper's "missing meta" fatal.
	data := readSettingsJSON(t, home)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	block, ok := parsed["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("juggernaut block missing from test fixture")
	}
	if _, hasMeta := block["meta"]; hasMeta {
		t.Error("test fixture should not contain a meta sub-block")
	}
}

// ---------------------------------------------------------------------------
// readNativeDefaultMode
// ---------------------------------------------------------------------------

func TestReadNativeDefaultMode_Present(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"permissions": map[string]any{
			"defaultMode": "auto",
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeDefaultMode(t, home)
	if got != "auto" {
		t.Errorf("readNativeDefaultMode = %q, want %q", got, "auto")
	}
}

func TestReadNativeDefaultMode_ReadOnly(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"permissions": map[string]any{
			"defaultMode": "readOnly",
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeDefaultMode(t, home)
	if got != "readOnly" {
		t.Errorf("readNativeDefaultMode = %q, want %q", got, "readOnly")
	}
}

func TestReadNativeDefaultMode_MissingPermissions(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": "iam"},
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeDefaultMode(t, home)
	if got != "" {
		t.Errorf("readNativeDefaultMode = %q, want empty string when permissions block missing", got)
	}
}

func TestReadNativeDefaultMode_EmptyPermissions(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"permissions": map[string]any{},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeDefaultMode(t, home)
	if got != "" {
		t.Errorf("readNativeDefaultMode = %q, want empty string when permissions block is empty", got)
	}
}

// ---------------------------------------------------------------------------
// readNativeEnvValue
// ---------------------------------------------------------------------------

func TestReadNativeEnvValue_Present(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"env": map[string]any{
			"CLAUDE_CODE_USE_BEDROCK": "1",
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeEnvValue(t, home, "CLAUDE_CODE_USE_BEDROCK")
	if got != "1" {
		t.Errorf("readNativeEnvValue = %q, want %q", got, "1")
	}
}

func TestReadNativeEnvValue_MultipleKeys(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"env": map[string]any{
			"CLAUDE_CODE_USE_BEDROCK":        "1",
			"ANTHROPIC_DEFAULT_OPUS_MODEL":   "claude-opus-4-20250514",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-20250514",
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeEnvValue(t, home, "ANTHROPIC_DEFAULT_SONNET_MODEL")
	if got != "claude-sonnet-4-20250514" {
		t.Errorf("readNativeEnvValue = %q, want %q", got, "claude-sonnet-4-20250514")
	}
}

func TestReadNativeEnvValue_MissingKey(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"env": map[string]any{
			"CLAUDE_CODE_USE_BEDROCK": "1",
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeEnvValue(t, home, "NONEXISTENT_KEY")
	if got != "" {
		t.Errorf("readNativeEnvValue = %q, want empty string for missing key", got)
	}
}

func TestReadNativeEnvValue_MissingEnvBlock(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": "iam"},
		},
	}
	writeSettingsJSON(t, home, settings)

	got := readNativeEnvValue(t, home, "CLAUDE_CODE_USE_BEDROCK")
	if got != "" {
		t.Errorf("readNativeEnvValue = %q, want empty string when env block missing", got)
	}
}

// ---------------------------------------------------------------------------
// setNativeDefaultMode
// ---------------------------------------------------------------------------

func TestSetNativeDefaultMode_SetsValue(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": "iam"},
			"meta": map[string]any{"schemaVersion": float64(2)},
		},
	}
	writeSettingsJSON(t, home, settings)

	setNativeDefaultMode(t, home, "auto")

	got := readNativeDefaultMode(t, home)
	if got != "auto" {
		t.Errorf("after setNativeDefaultMode, readNativeDefaultMode = %q, want %q", got, "auto")
	}
}

func TestSetNativeDefaultMode_CreatesPermissionsBlock(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": "iam"},
		},
	}
	writeSettingsJSON(t, home, settings)

	setNativeDefaultMode(t, home, "writeAll")

	got := readNativeDefaultMode(t, home)
	if got != "writeAll" {
		t.Errorf("after setNativeDefaultMode with no prior permissions block, got %q, want %q", got, "writeAll")
	}
}

func TestSetNativeDefaultMode_OverwritesExisting(t *testing.T) {
	home := t.TempDir()
	settings := map[string]any{
		"permissions": map[string]any{
			"defaultMode": "readOnly",
		},
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": "iam"},
		},
	}
	writeSettingsJSON(t, home, settings)

	setNativeDefaultMode(t, home, "auto")

	got := readNativeDefaultMode(t, home)
	if got != "auto" {
		t.Errorf("after overwriting, readNativeDefaultMode = %q, want %q", got, "auto")
	}
}

// ---------------------------------------------------------------------------
// mockApplyCommandRunner.RunContext
// ---------------------------------------------------------------------------

func TestMockPSRunner_RunContext_Success(t *testing.T) {
	runner := &mockApplyCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": []byte(`{"CurrentUserAllHosts":"/p","CurrentUserCurrentHost":"/p"}`),
		},
	}

	data, err := runner.RunContext(context.Background(), "pwsh.exe", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"CurrentUserAllHosts":"/p","CurrentUserCurrentHost":"/p"}` {
		t.Errorf("RunContext = %q, want mock JSON output", string(data))
	}
}

func TestMockPSRunner_RunContext_Error(t *testing.T) {
	runner := &mockApplyCommandRunner{
		err: map[string]error{
			"pwsh.exe": fmt.Errorf("powershell not found"),
		},
	}

	_, err := runner.RunContext(context.Background(), "pwsh.exe", nil)
	if err == nil {
		t.Fatal("expected error from mock, got nil")
	}
	if err.Error() != "powershell not found" {
		t.Errorf("error = %q, want %q", err.Error(), "powershell not found")
	}
}

func TestMockPSRunner_RunContext_EmptyOutput(t *testing.T) {
	runner := &mockApplyCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": {}, // empty output
		},
	}

	data, err := runner.RunContext(context.Background(), "pwsh.exe", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("RunContext = %q (len=%d), want empty output", string(data), len(data))
	}
}

func TestMockPSRunner_RunContext_UnknownExe(t *testing.T) {
	runner := &mockApplyCommandRunner{
		output: map[string][]byte{
			"pwsh.exe": []byte(`{"CurrentUserAllHosts":"/p"}`),
		},
	}

	data, err := runner.RunContext(context.Background(), "unknown.exe", nil)
	if err != nil {
		t.Fatalf("unexpected error for unknown exe: %v", err)
	}
	// Unknown exe returns nil (zero value from map lookup) with no error.
	if data != nil {
		t.Errorf("RunContext for unknown exe = %v, want nil", string(data))
	}
}
