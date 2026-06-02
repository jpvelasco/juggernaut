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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
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
	if err := os.WriteFile(bashrc, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

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

func TestDetect_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := migrate.Detect(dir)
	if err == nil {
		t.Error("expected error for corrupted settings.json")
	}
}

func TestDetect_V322_TooOld(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 1,
				"version":       "3.2.2",
			},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !state.TooOld {
		t.Error("expected TooOld=true for v3.2.2 (minimum is v3.2.3)")
	}
}

func TestDetect_V323_NotTooOld(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 1,
				"version":       "3.2.3",
			},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if state.TooOld {
		t.Error("expected TooOld=false for v3.2.3 (exact minimum)")
	}
}

func TestVersionBoundaries(t *testing.T) {
	// Test the exact boundary: 3.2.2 is too old, 3.2.3 is the minimum.
	tests := []struct {
		version    string
		shouldPass bool
	}{
		{"3.2.3", true},
		{"3.2.2", false},
		{"3.2.4", true},
		{"3.3.0", true},
		{"4.0.0", true},
		{"3.1.9", false},
		{"2.9.9", false},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		writeSettings(t, dir, map[string]any{
			"juggernaut": map[string]any{
				"meta": map[string]any{
					"managedBy":     "juggernaut",
					"schemaVersion": 1,
					"version":       tt.version,
				},
			},
		})
		state, err := migrate.Detect(dir)
		if err != nil {
			t.Fatalf("v%s: Detect() error: %v", tt.version, err)
		}
		tooOld := state.TooOld
		if tt.shouldPass && tooOld {
			t.Errorf("v%s should NOT be too old, but was flagged as too old", tt.version)
		}
		if !tt.shouldPass && !tooOld {
			t.Errorf("v%s SHOULD be too old, but was not flagged", tt.version)
		}
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
