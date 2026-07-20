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
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
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
	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockPSOutputJSON(psProfile, psProfile),
			"powershell.exe": mockPSOutputJSON(psProfile, psProfile),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	t.Cleanup(activation.ResetPSRunnerForTesting)
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

// ---------------------------------------------------------------------------
// Shared test setup helpers
// ---------------------------------------------------------------------------

// setupApplyTest creates a temp home directory, sets HOME/USERPROFILE env vars,
// and installs a mock PS runner on Windows. Returns the temp home path.
// This is the standard first step for any test that calls ExecuteArgs for apply.
func setupApplyTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)
	return home
}

// setupApplyTestWithReset is like setupApplyTest but also resets and defers
// Cobra flag state. Use for tests that directly manipulate applyFlags or call
// commitApply/printApplyDryRun.
func setupApplyTestWithReset(t *testing.T) string {
	t.Helper()
	defer resetFlags()
	resetFlags()
	return setupApplyTest(t)
}

// buildClaudeBlockAndOpts creates a schema.Block and matching provider.Options
// from the embedded bedrock config. Returns (block, provOpts, bCfg) or skips
// the test if the embedded config is unavailable. Use the optional optsFn to
// override default schema.Options fields.
func buildClaudeBlockAndOpts(t *testing.T, optsFn func(*schema.Options)) (*schema.Block, provider.Options, *bedrock.Config) {
	t.Helper()
	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	opts := schema.Options{
		AuthMode:      "iam",
		Region:        "us-west-2",
		Scope:         "user",
		Version:       "5.4.0",
		Effort:        "high",
		AuthValidated: true,
	}
	if optsFn != nil {
		optsFn(&opts)
	}
	block, err := schema.Build(bCfg, opts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}
	provOpts := provider.Options{
		AuthMode:       opts.AuthMode,
		Region:         opts.Region,
		Scope:          opts.Scope,
		Version:        opts.Version,
		Effort:         opts.Effort,
		AuthValidated:  opts.AuthValidated,
		PermissionMode: opts.PermissionMode,
	}
	return block, provOpts, bCfg
}

// defaultClaudeBlockAndOpts returns a standard IAM block, provider options, and
// bedrock config for claude tests. Convenience wrapper around
// buildClaudeBlockAndOpts with no overrides.
func defaultClaudeBlockAndOpts(t *testing.T) (*schema.Block, provider.Options, *bedrock.Config) {
	t.Helper()
	return buildClaudeBlockAndOpts(t, nil)
}

// writeSettingsFile creates the .claude/settings.json directory and writes the
// given JSON string. Convenience wrapper to avoid the mkdir + writeFile boilerplate.
func writeSettingsFile(t *testing.T, home, jsonStr string) {
	t.Helper()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil { //nolint:gosec // test-only temp dir
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(jsonStr), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// writeOwnedClaudeConfig pre-writes a settings.json that Juggernaut owns
// (has a juggernaut block with auth and meta). Optional extra top-level keys
// can be merged in via extraKeys.
func writeOwnedClaudeConfig(t *testing.T, home string, extraKeys ...map[string]any) {
	t.Helper()
	base := `{"juggernaut":{"auth":{"mode":"iam","region":"us-west-2"},"meta":{"managedBy":"juggernaut"}}}`
	var settings map[string]any
	if err := json.Unmarshal([]byte(base), &settings); err != nil {
		t.Fatalf("unmarshal base config: %v", err)
	}
	for _, extra := range extraKeys {
		for k, v := range extra {
			settings[k] = v
		}
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	writeSettingsFile(t, home, string(data))
}

// ---------------------------------------------------------------------------
// mockPSRunner (unified) — replaces mockApplyCommandRunner and mockDoctorCommandRunner
// ---------------------------------------------------------------------------

// mockPSRunner is a unified activation.discoveryCommandRunner used across all
// test files. It replaces the separate mockApplyCommandRunner and
// mockDoctorCommandRunner types.
type mockPSRunner struct {
	output map[string][]byte
	err    map[string]error
	counts map[string]int // optional: tracks invocation counts
}

func (m *mockPSRunner) RunContext(_ context.Context, exe string, _ []string) ([]byte, error) {
	if m.counts != nil {
		m.counts[exe]++
	}
	if err := m.err[exe]; err != nil {
		return nil, err
	}
	return m.output[exe], nil
}

// mockPSOutputJSON builds the JSON response for a PowerShell profile discovery
// call. Replaces both mockApplyPSOutput and mockPSOutput.
func mockPSOutputJSON(allHosts, currentHost string) []byte {
	data, _ := json.Marshal(map[string]string{
		"CurrentUserAllHosts":    allHosts,
		"CurrentUserCurrentHost": currentHost,
	})
	return data
}
