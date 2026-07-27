package provider

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestClaude_ConfigPath pins Claude's config location (unchanged from the
// hardcoded cmd/helpers.settingsPath: ~/.claude/settings.json, project scope
// relative).
func TestClaude_ConfigPath(t *testing.T) {
	p, _ := Get("claude")
	home := testutil.NewTestHome(t)

	user, err := p.ConfigPath(home, "user")
	if err != nil {
		t.Fatalf("user scope: %v", err)
	}
	if filepath.Base(user) != "settings.json" || !strings.Contains(user, ".claude") {
		t.Errorf("claude user path = %q, want .../.claude/settings.json", user)
	}

	proj, err := p.ConfigPath(home, "project")
	if err != nil {
		t.Fatalf("project scope: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(proj), ".claude/settings.json") {
		t.Errorf("claude project path = %q, want ./.claude/settings.json", proj)
	}
}

// TestCodex_ConfigPath: ~/.codex/config.toml (user), ./.codex/config.toml (project).
func TestCodex_ConfigPath(t *testing.T) {
	p, _ := Get("codex")
	home := testutil.NewTestHome(t)

	user, err := p.ConfigPath(home, "user")
	if err != nil {
		t.Fatalf("user scope: %v", err)
	}
	if filepath.Base(user) != "config.toml" || !strings.Contains(user, ".codex") {
		t.Errorf("codex user path = %q, want .../.codex/config.toml", user)
	}

	proj, err := p.ConfigPath(home, "project")
	if err != nil {
		t.Fatalf("project scope: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(proj), ".codex/config.toml") {
		t.Errorf("codex project path = %q, want ./.codex/config.toml", proj)
	}
}
