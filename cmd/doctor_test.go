package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v4/internal/doctor"
	"github.com/jpvelasco/juggernaut/v4/internal/launcher"
)

func TestOpusplanProblem(t *testing.T) {
	if _, ok := opusplanProblem(nil); ok {
		t.Fatal("nil settings should not trigger opusplan warning")
	}

	if _, ok := opusplanProblem(map[string]any{"model": "sonnet"}); ok {
		t.Fatal("non-opusplan model should not trigger warning")
	}

	detail, ok := opusplanProblem(map[string]any{"model": "opusplan"})
	if !ok {
		t.Fatal("opusplan should trigger warning")
	}
	if !strings.Contains(detail, "opusplan") {
		t.Fatalf("warning should mention opusplan, got %q", detail)
	}
}

func TestClaudeCommandStatus_OKWhenShimFirst(t *testing.T) {
	dir := t.TempDir()
	if err := launcher.Install(dir); err != nil {
		t.Fatalf("installing claude shim: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	status, detail := claudeCommandStatus(dir)
	if status != doctor.OK {
		t.Fatalf("expected OK, got %s (%s)", status, detail)
	}
}

func TestClaudeCommandStatus_WarnsWhenDifferentClaudeIsFound(t *testing.T) {
	expectedDir := t.TempDir()
	otherDir := t.TempDir()
	if err := launcher.Install(expectedDir); err != nil {
		t.Fatalf("installing expected claude shim: %v", err)
	}
	if err := launcher.Install(otherDir); err != nil {
		t.Fatalf("installing other claude shim: %v", err)
	}
	t.Setenv("PATH", otherDir+string(os.PathListSeparator)+expectedDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	status, detail := claudeCommandStatus(expectedDir)
	if status != doctor.Warn {
		t.Fatalf("expected WARN, got %s (%s)", status, detail)
	}
	if !strings.Contains(detail, otherDir) {
		t.Fatalf("warning should mention the other claude path, got %q", detail)
	}
}

func TestCheckSettingsScope_DefaultProjectMissingIsOK(t *testing.T) {
	home := t.TempDir()

	status, detail := checkSettingsScope(home, "project", false)
	if status != doctor.OK {
		t.Fatalf("expected OK for missing project scope, got %s (%s)", status, detail)
	}
	if detail != "not configured" {
		t.Fatalf("expected not configured detail, got %q", detail)
	}
}

func TestCheckSettingsScope_RequiredScopeMissingFails(t *testing.T) {
	home := t.TempDir()

	status, detail := checkSettingsScope(home, "user", true)
	if status != doctor.Fail {
		t.Fatalf("expected FAIL for missing required scope, got %s (%s)", status, detail)
	}
}
