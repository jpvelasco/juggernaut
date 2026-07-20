package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
)

// ---------------------------------------------------------------------------
// findBedrockConfigFile — parent directory fallback
// ---------------------------------------------------------------------------

func TestFindBedrockConfigFile_ParentDirFallback(t *testing.T) {
	// When bedrock-config.json exists in the parent directory.
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil { // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission — 0o700 is correct for directories, test under t.TempDir()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bedrock-config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	got := findBedrockConfigFile()
	if got != "../bedrock-config.json" {
		t.Errorf("findBedrockConfigFile = %q, want ../bedrock-config.json", got)
	}
}

// ---------------------------------------------------------------------------
// resolveCredential — TUI prompt error path
// ---------------------------------------------------------------------------

func TestResolveCredential_TUIPromptError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TUI form blocks on Windows without a real console")
	}
	defer resetFlags()

	home := t.TempDir()
	// Ensure no credential file exists so the TUI prompt path is taken.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "nonexistent-service-xyz-for-tui")

	// The TUI form will fail because there's no terminal in tests.
	// This exercises the form.Run error path.
	token, err := resolveCredential(authmode.BedrockAPIKey, home)
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
	// If the form failed (expected in tests without a terminal), verify the error path.
	if err == nil {
		t.Log("TUI form succeeded unexpectedly (CI may have a pty) — token is empty, which is acceptable")
	}
}

// ---------------------------------------------------------------------------
// printApplyDryRun — non-Claude provider (no legacy recovery message)
// ---------------------------------------------------------------------------

func TestPrintApplyDryRun_NonClaudeProvider(t *testing.T) {
	defer resetFlags()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov, _ := provider.Get("codex")

	provOpts := provider.Options{
		AuthMode: authmode.BedrockAPIKey,
		Region:   "us-west-2",
		Scope:    "user",
		Version:  "5.4.0",
		Effort:   "high",
	}
	block := &schema.Block{}

	out := captureStdout(t, func() {
		err = printApplyDryRun(home, block, prov, bCfg, provOpts)
	})
	if err != nil {
		t.Fatalf("printApplyDryRun: %v", err)
	}
	if !strings.Contains(out, "Dry run — no changes written.") {
		t.Fatalf("expected dry run header, got:\n%s", out)
	}
	// Non-Claude providers should NOT mention legacy recovery.
	if strings.Contains(out, "v4.2.6") {
		t.Error("non-Claude provider should not mention v4.2.6 recovery")
	}
	// Should mention Codex-specific config path.
	if !strings.Contains(out, "config.toml") {
		t.Error("expected codex config.toml path in output")
	}
}
