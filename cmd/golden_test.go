package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Golden test: the "serial-numbered artifacts" inventory for the provider
// refactor. It captures Claude's real apply output (settings.json + dry-run) and
// diffs byte-for-byte against a committed snapshot. Any drift fails.
//
// Volatile fields that legitimately change per run are normalized before compare:
//   - juggernaut.meta.appliedAt (time.Now)
//   - the temp HOME path embedded in some values
//
// Regenerate after an INTENTIONAL change with: UPDATE_GOLDEN=1 go test ./cmd/ -run Golden
const goldenSettingsFile = "testdata/golden/claude_settings.json"

var appliedAtRE = regexp.MustCompile(`"appliedAt"\s*:\s*"[^"]*"`)

func normalizeSettings(t *testing.T, raw []byte, home string) string {
	t.Helper()
	// Canonicalize JSON (stable key order) so the golden is order-independent,
	// then normalize the volatile timestamp and the temp-home path.
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("golden: settings.json not valid JSON: %v", err)
	}
	canon, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("golden: re-marshal: %v", err)
	}
	s := string(canon)
	s = appliedAtRE.ReplaceAllString(s, `"appliedAt":"<NORMALIZED>"`)
	s = strings.ReplaceAll(s, home, "<HOME>")
	// Windows path separators in embedded paths → forward slash for stability.
	s = strings.ReplaceAll(s, "\\\\", "/")
	return s
}

func TestGolden_ClaudeApply_SettingsUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setupMockPSRunner(t, home)

	// A representative apply exercising many managed keys/paths.
	if err := ExecuteArgs([]string{
		"apply", "--cli=claude", "--auth=iam", "--region=us-west-2",
		"--effort=high", "--mode=plan", "--fallback-model=a,b", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	got := normalizeSettings(t, raw, home)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenSettingsFile), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(goldenSettingsFile, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden updated: %s", goldenSettingsFile)
		return
	}

	want, err := os.ReadFile(goldenSettingsFile)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("Claude settings.json drifted from golden.\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}
