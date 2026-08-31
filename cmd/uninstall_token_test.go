package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Given a Bedrock bearer token shared by every configured provider,
// When the user removes ONLY Claude's project-scope block,
// Then the shared token must survive: user-scope Claude and any non-Claude
// provider configs still reference it.
func TestUninstall_ProjectScope_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--scope=project", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after project-scope uninstall: %v", err)
	}
	if got != "shared-token" {
		t.Errorf("project-scope uninstall deleted the shared bearer token (got %q)\noutput:\n%s", got, out)
	}
}

// Given Claude is uninstalled outright (no scope restriction),
// When the uninstall completes,
// Then the primary's bearer token is removed as before.
func TestUninstall_FullClaude_RemovesToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("doomed-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after full uninstall: %v", err)
	}
	if got != "" {
		t.Errorf("full Claude uninstall should remove the bearer token, got %q", got)
	}
}

// Given a second CLI (Codex) is configured for Bedrock with a Juggernaut-owned
// config, and the user uninstalls Claude in full,
// Then the shared bearer token must survive — Codex's config still references
// it — and the output must name the surviving provider so the user knows the
// token is being kept on purpose.
func TestUninstall_FullClaude_WithCodexConfigured_KeepsSharedToken(t *testing.T) {
	home := setupApplyTest(t)
	store := setupIsolatedKeychain(t)
	if err := store.SetWithFallback("shared-token", home); err != nil {
		t.Fatalf("seeding keychain: %v", err)
	}

	// Seed a Juggernaut-owned Codex config (user scope).
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("creating codex dir: %v", err)
	}
	codexConfig := "model = \"gpt-5.6-sol\"\nmodel_provider = \"amazon-bedrock\"\n[aws]\nregion = \"us-east-1\"\n"
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(codexConfig), 0o644); err != nil {
		t.Fatalf("seeding codex config: %v", err)
	}

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"uninstall", "--cli=claude", "--force"}); err != nil {
			t.Fatalf("uninstall error: %v", err)
		}
	})

	got, err := store.GetWithFallback(home)
	if err != nil {
		t.Fatalf("reading token after uninstall: %v", err)
	}
	if got != "shared-token" {
		t.Errorf("shared bearer token was removed while a Codex config still references it (got %q)\noutput:\n%s", got, out)
	}
	if !strings.Contains(out, "Shared Bedrock bearer token retained") {
		t.Errorf("expected a retain warning in output, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "codex") {
		t.Errorf("retain warning should name the surviving provider (codex), got:\n%s", out)
	}
}
