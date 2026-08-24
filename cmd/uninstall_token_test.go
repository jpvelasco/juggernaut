package cmd

import (
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
