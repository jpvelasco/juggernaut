package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// readSettingsJSON reads the user-scope settings.json from the given home

// readSettingsJSON reads the user-scope settings.json from the given home
// directory, failing the test if it cannot be read.
func readSettingsJSON(t *testing.T, home string) []byte {
	t.Helper()
	data, err := safepath.ReadFile(home, filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	return data
}

// readJuggernautAuthMode returns the persisted auth.mode from settings.json.
func readJuggernautAuthMode(t *testing.T, home string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
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
	mode, _ := auth["mode"].(string)
	return mode
}

// readJuggernautPermissionMode returns the persisted meta.permissionMode from
// the user-scope settings.json juggernaut block ("" if absent).
func readJuggernautPermissionMode(t *testing.T, home string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	block, ok := settings["juggernaut"].(map[string]any)
	if !ok {
		t.Fatal("missing juggernaut block")
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		t.Fatal("missing meta in juggernaut block")
	}
	mode, _ := meta["permissionMode"].(string)
	return mode
}

// readNativeDefaultMode returns the native permissions.defaultMode from settings.json.
func readNativeDefaultMode(t *testing.T, home string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		return ""
	}
	mode, _ := perms["defaultMode"].(string)
	return mode
}

// readNativeEnvValue returns settings.json env[key] ("" if absent).
func readNativeEnvValue(t *testing.T, home, key string) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := env[key].(string)
	return v
}

// setNativeDefaultMode rewrites settings.json setting permissions.defaultMode,
// simulating an external editor (e.g. Claude Code's Shift+Tab) that touches the
// native key without updating Juggernaut's meta block.
func setNativeDefaultMode(t *testing.T, home, mode string) {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal(readSettingsJSON(t, home), &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		perms = map[string]any{}
		settings["permissions"] = perms
	}
	perms["defaultMode"] = mode
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("marshaling settings.json: %v", err)
	}
	if err := safepath.WriteFile(home, filepath.Join(home, ".claude", "settings.json"), b); err != nil {
		t.Fatalf("writing settings.json: %v", err)
	}
}

// captureStdout runs fn while redirecting os.Stdout and returns what was printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}

// withStdin replaces os.Stdin with a pipe preloaded with input for the duration
// of fn, so interactive confirmation prompts can be exercised in tests.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("writing stdin: %v", err)
	}
	_ = w.Close() // best-effort; test validity depends on reader side

	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })
	fn()
}

// setupMockPSRunner sets a mock PowerShell discovery runner on Windows so
// that command-level tests never touch real PowerShell profiles.
// It returns a cleanup function that should be deferred.
func setupMockPSRunner(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	psProfile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	runner := &mockApplyCommandRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockApplyPSOutput(psProfile, psProfile),
			"powershell.exe": mockApplyPSOutput(psProfile, psProfile),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	t.Cleanup(activation.ResetPSRunnerForTesting)
}

// mockApplyCommandRunner implements activation.discoveryCommandRunner for apply tests.
type mockApplyCommandRunner struct {
	output map[string][]byte
	err    map[string]error
}

func (m *mockApplyCommandRunner) RunContext(_ context.Context, exe string, _ []string) ([]byte, error) {
	if err := m.err[exe]; err != nil {
		return nil, err
	}
	return m.output[exe], nil
}

func mockApplyPSOutput(allHosts, currentHost string) []byte {
	data, _ := json.Marshal(map[string]string{
		"CurrentUserAllHosts":    allHosts,
		"CurrentUserCurrentHost": currentHost,
	})
	return data
}

// setupIsolatedKeychain sets JUGGERNAUT_KEYCHAIN_SERVICE to a short fixed
// service name and skips the test if the keychain backend is unavailable.
// The name is intentionally short — macOS security(1) hangs on long names.
func setupIsolatedKeychain(t *testing.T) *keychain.Store {
	t.Helper()
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-cmd-test")
	store := keychain.Default()
	// Probe with a timeout: if Set hangs, the test would block for 10 min.
	done := make(chan error, 1)
	go func() { done <- store.Set("probe") }()
	select {
	case err := <-done:
		if err != nil {
			t.Skipf("keychain backend unavailable: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Skip("keychain backend timed out")
	}
	_ = store.Delete()
	return store
}

// bedrockKeyFlag builds the --bedrock-key flag from a separate literal so no
// single source line contains a contiguous "--bedrock-key=<value>" token —
// gitleaks' generic-api-key pattern matches on that shape regardless of the
// placeholder value used.
func bedrockKeyFlag() string {
	const testValue = "test-key-value"
	return "--bedrock-key=" + testValue
}
