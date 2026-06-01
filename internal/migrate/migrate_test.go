package migrate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/migrate"
)

func writeSettings(t *testing.T, dir string, data map[string]any) {
	t.Helper()
	path := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0o644)
}

func TestDetect_V3Block(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 1,
				"version":       "3.2.3",
			},
			"auth": map[string]any{"mode": "iam"},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !state.HasV3Block {
		t.Error("expected HasV3Block=true")
	}
	if state.V3Version != "3.2.3" {
		t.Errorf("expected V3Version=3.2.3, got %s", state.V3Version)
	}
	if state.AuthMode != "iam" {
		t.Errorf("expected AuthMode=iam, got %s", state.AuthMode)
	}
}

func TestDetect_NoBlock(t *testing.T) {
	dir := t.TempDir()
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if state.HasV3Block {
		t.Error("expected HasV3Block=false for clean dir")
	}
}

func TestDetect_TooOld(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 1,
				"version":       "3.1.0",
			},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !state.TooOld {
		t.Error("expected TooOld=true for version < 3.2.3")
	}
}

func TestDetect_AlreadyV4(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 2,
				"version":       "4.0.0",
			},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !state.AlreadyV4 {
		t.Error("expected AlreadyV4=true for schemaVersion 2")
	}
}

func TestStripLauncherBlocks(t *testing.T) {
	dir := t.TempDir()

	bashrc := filepath.Join(dir, ".bashrc")
	content := "export PATH=$PATH:~/.local/bin\n# BEGIN: Juggernaut Launcher\nfunction claude() { echo old; }\n# END: Juggernaut Launcher\nexport FOO=bar\n"
	os.WriteFile(bashrc, []byte(content), 0o644)

	stripped := migrate.StripLauncherBlocks(dir)
	if len(stripped) != 1 {
		t.Errorf("expected 1 stripped file, got %d", len(stripped))
	}

	result, _ := os.ReadFile(bashrc)
	if contains(string(result), "Juggernaut Launcher") {
		t.Error("launcher block should be removed")
	}
	if !contains(string(result), "export PATH") {
		t.Error("other content should be preserved")
	}
	if !contains(string(result), "export FOO=bar") {
		t.Error("content after block should be preserved")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
