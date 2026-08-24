package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCorruptClaudeSettings(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("<<<<<<< merge conflict"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Given a corrupt (but present) .claude/settings.json,
// When the launch pipeline probes auth modes,
// Then the probe degrades to a warning naming the file instead of a fatal
// error — the wrapped CLI must never be hard-blocked by advisory config.
func TestAuthModes_CorruptConfigDegradesToWarning(t *testing.T) {
	home := t.TempDir()
	writeCorruptClaudeSettings(t, home)

	var warns []string
	modes := authModes(home, func(msg string) { warns = append(warns, msg) })

	if len(modes) != 0 {
		t.Errorf("modes = %v, want empty on unreadable config", modes)
	}
	if len(warns) == 0 {
		t.Fatal("expected a degradation warning for the corrupt settings file")
	}
	for _, w := range warns {
		if strings.Contains(w, "settings.json") {
			return
		}
	}
	t.Errorf("warning should name the offending file, got: %v", warns)
}

// Given a corrupt user settings.json but valid saved runtime state,
// When the wrapper launches claude,
// Then it falls back to the runtime configuration with a warning and still
// execs the real binary.
func TestLaunchWithOptions_CorruptSettingsFallsBackToRuntimeState(t *testing.T) {
	home := t.TempDir()
	writeCorruptClaudeSettings(t, home)
	if err := SaveRuntimeState(home, "claude", RuntimeState{AuthMode: "iam"}); err != nil {
		t.Fatalf("seeding runtime state: %v", err)
	}

	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(binDir, "claude")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil { // #nosec G306 -- stub must be executable
		t.Fatal(err)
	}

	var launched [][]string
	var warns []string
	err := LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        []string{"--version"},
		Path:        binDir,
		TokenGetter: func() (string, error) { return "", nil },
		Runner: func(path string, args []string, _ []string) error {
			launched = append(launched, append([]string{path}, args...))
			return nil
		},
		Warner: func(msg string) { warns = append(warns, msg) },
	})
	if err != nil {
		t.Fatalf("launch should degrade past corrupt settings, got: %v", err)
	}
	if len(launched) == 0 {
		t.Error("wrapped binary was never exec'd")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "settings.json") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning about the corrupt settings file, got: %v", warns)
	}
}
