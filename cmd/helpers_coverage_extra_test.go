package cmd

import (
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(sub, 0o700); err != nil { //nolint:gosec // test
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bedrock-config.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldWd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldWd) })
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
	defer resetFlags()
	resetFlags()
	applyFlags.preserveKey = false

	home := t.TempDir()
	// Ensure no credential file exists so the TUI prompt path is taken.
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "nonexistent-service-xyz-for-tui")

	// The TUI form will fail because there's no terminal in tests.
	// This exercises the form.Run error path.
	token, err := resolveCredential(authmode.BedrockAPIKey, home)
	if err == nil {
		// If it somehow succeeded (e.g. CI has a pty), token should be empty.
		if token != "" {
			t.Errorf("unexpected token: %q", token)
		}
	} else {
		// The error path from form.Run is exercised.
		if token != "" {
			t.Errorf("expected empty token on error, got %q", token)
		}
	}
}

// ---------------------------------------------------------------------------
// printApplyDryRun — non-Claude provider (no legacy recovery message)
// ---------------------------------------------------------------------------

func TestPrintApplyDryRun_NonClaudeProvider(t *testing.T) {
	defer resetFlags()
	resetFlags()

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
}
