package cmd

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/redact"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

const logsTestToken = "super-secret-token-value"
const logsTestAccount = "123456789012"

func TestLogsExport_DefaultRedactsSecrets(t *testing.T) {
	home := setupLogsExportHome(t)
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", logsTestToken)

	out := filepath.Join(t.TempDir(), "bundle.zip")
	printed := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"logs", "export", "--out", out}); err != nil {
			t.Fatalf("logs export: %v", err)
		}
	})
	if !strings.Contains(printed, "redacted diagnostic bundle") {
		t.Fatalf("expected redacted confirmation, got %q", printed)
	}

	files := readLogsZip(t, out)
	show := files["show.txt"]
	env := files["env.txt"]
	if showContainsPath(show, home) {
		t.Errorf("show.txt still contains home path %q:\n%s", home, show)
	}
	for _, secret := range []string{logsTestToken, logsTestAccount} {
		if strings.Contains(show, secret) {
			t.Errorf("show.txt still contains %q:\n%s", secret, show)
		}
		if strings.Contains(env, secret) {
			t.Errorf("env.txt still contains %q:\n%s", secret, env)
		}
	}
	if !strings.Contains(show, redact.Token) {
		t.Errorf("show.txt missing %s:\n%s", redact.Token, show)
	}
	if !strings.Contains(show, redact.AccountID) {
		t.Errorf("show.txt missing %s:\n%s", redact.AccountID, show)
	}
	if !strings.Contains(show, redact.Home) {
		t.Errorf("show.txt missing %s:\n%s", redact.Home, show)
	}
	if !strings.Contains(env, "AWS_BEARER_TOKEN_BEDROCK="+redact.Token) {
		t.Errorf("env.txt should redact bearer token value, got:\n%s", env)
	}
	assertBundleMinimum(t, files)
	for _, leaked := range []string{"settings.json", "juggernaut-credential", "credential-fallback.json"} {
		if _, ok := files[leaked]; ok {
			t.Errorf("default zip must not copy %s", leaked)
		}
	}
}

func TestLogsExport_RawIncludesSecrets(t *testing.T) {
	home := setupLogsExportHome(t)
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", logsTestToken)

	dir := t.TempDir()
	out := filepath.Join(dir, "raw.zip")
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := ExecuteArgs([]string{"logs", "export", "--raw", "--out", out}); err != nil {
				t.Fatalf("logs export --raw: %v", err)
			}
		})
	})
	if !strings.Contains(stderr, "unredacted") {
		t.Fatalf("expected one-line raw warning on stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "unredacted diagnostic bundle") {
		t.Fatalf("expected unredacted confirmation, got %q", stdout)
	}

	files := readLogsZip(t, out)
	show := files["show.txt"]
	env := files["env.txt"]
	if !strings.Contains(show, logsTestToken) {
		t.Errorf("raw show.txt should include token, got:\n%s", show)
	}
	if !strings.Contains(show, logsTestAccount) {
		t.Errorf("raw show.txt should include account id, got:\n%s", show)
	}
	if !showContainsPath(show, home) {
		t.Errorf("raw show.txt should include home path, got:\n%s", show)
	}
	if !strings.Contains(env, logsTestToken) {
		t.Errorf("raw env.txt should include token, got:\n%s", env)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("raw export must not also write a redacted sibling, got %d files", len(entries))
	}
}

func TestLogsExport_IncludeSecretsAlias(t *testing.T) {
	_ = setupLogsExportHome(t)
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", logsTestToken)

	out := filepath.Join(t.TempDir(), "alias.zip")
	if err := ExecuteArgs([]string{"logs", "export", "--include-secrets", "--out", out}); err != nil {
		t.Fatalf("logs export --include-secrets: %v", err)
	}
	env := readLogsZip(t, out)["env.txt"]
	if !strings.Contains(env, logsTestToken) {
		t.Errorf("--include-secrets should keep env secrets, got:\n%s", env)
	}
}

func TestLogsExport_SucceedsWhenDoctorWouldFail(t *testing.T) {
	_ = setupApplyTest(t)
	out := filepath.Join(t.TempDir(), "broken.zip")
	if err := ExecuteArgs([]string{"logs", "export", "--out", out}); err != nil {
		t.Fatalf("export should succeed even when doctor reports failures: %v", err)
	}
	files := readLogsZip(t, out)
	if !strings.Contains(files["doctor.txt"], "FAIL") && !strings.Contains(files["doctor.txt"], "not found") {
		t.Errorf("expected doctor failure details in bundle, got:\n%s", files["doctor.txt"])
	}
}

func TestLogsExport_HomeDirError(t *testing.T) {
	noHomeEnv(t)
	err := ExecuteArgs([]string{"logs", "export", "--out", filepath.Join(t.TempDir(), "x.zip")})
	assertHomeDirError(t, err)
}

func TestLogsExport_DefaultOutPath(t *testing.T) {
	_ = setupLogsExportHome(t)
	dir := t.TempDir()
	chdirTo(t, dir)
	logsExportNow = func() time.Time {
		return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() { logsExportNow = time.Now })

	if err := ExecuteArgs([]string{"logs", "export"}); err != nil {
		t.Fatalf("logs export: %v", err)
	}
	want := filepath.Join(dir, "juggernaut-logs-20260904T120000Z.zip")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected default zip at %s: %v", want, err)
	}
}

func TestRedactedEnvValue_StablePlaceholders(t *testing.T) {
	if got := redactedEnvValue("AWS_BEARER_TOKEN_BEDROCK"); got != redact.Token {
		t.Errorf("token env = %q, want %s", got, redact.Token)
	}
	if got := redactedEnvValue("AWS_ACCESS_KEY_ID"); got != redact.Token {
		t.Errorf("access key env = %q, want %s", got, redact.Token)
	}
	if got := redactedEnvValue("HOME"); got != redact.Home {
		t.Errorf("HOME = %q, want %s", got, redact.Home)
	}
	if got := redactedEnvValue("AWS_REGION"); got != "<redacted>" {
		t.Errorf("region env = %q, want <redacted>", got)
	}
}

func TestIsDiagnosticEnv(t *testing.T) {
	for _, name := range []string{"AWS_REGION", "ANTHROPIC_MODEL", "CLAUDE_CODE_USE_BEDROCK", "HOME", "PATH"} {
		if !isDiagnosticEnv(name) {
			t.Errorf("%s should be diagnostic", name)
		}
	}
	if isDiagnosticEnv("EDITOR") {
		t.Error("EDITOR should not be treated as a diagnostic env var")
	}
}

func TestLogsExport_RejectsDirectoryOut(t *testing.T) {
	_ = setupApplyTest(t)
	dir := t.TempDir()
	err := ExecuteArgs([]string{"logs", "export", "--out", dir})
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected directory error, got %v", err)
	}
}

func setupLogsExportHome(t *testing.T) string {
	t.Helper()
	home := setupApplyTest(t)
	claudeDir := filepath.Join(home, ".claude")
	if err := safepath.MkdirAll(claudeDir); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy": "juggernaut",
			},
			"auth": map[string]any{
				"token":   "Bearer " + logsTestToken,
				"account": logsTestAccount,
			},
			"home": home,
		},
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	path := filepath.Join(claudeDir, "settings.json")
	if err := safepath.WriteFile(claudeDir, path, payload); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	return home
}

func readLogsZip(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer func() { _ = r.Close() }()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = string(body)
	}
	return out
}

func showContainsPath(show, home string) bool {
	if strings.Contains(show, home) {
		return true
	}
	escaped := strings.ReplaceAll(home, `\`, `\\`)
	if escaped != home && strings.Contains(show, escaped) {
		return true
	}
	return strings.Contains(show, filepath.ToSlash(home))
}

func assertBundleMinimum(t *testing.T, files map[string]string) {
	t.Helper()
	for _, name := range []string{
		"version.txt", "runtime.txt", "env.txt", "providers.txt",
		"doctor.txt", "show.txt", "manifest.txt",
	} {
		if strings.TrimSpace(files[name]) == "" {
			t.Errorf("bundle missing %s", name)
		}
	}
	if !strings.Contains(files["version.txt"], Version) {
		t.Errorf("version.txt should contain %s, got %q", Version, files["version.txt"])
	}
	if !strings.Contains(files["runtime.txt"], "os=") || !strings.Contains(files["runtime.txt"], "arch=") {
		t.Errorf("runtime.txt missing os/arch:\n%s", files["runtime.txt"])
	}
	if !strings.Contains(files["manifest.txt"], "privacy=redacted") {
		t.Errorf("manifest should record redacted privacy, got %q", files["manifest.txt"])
	}
	if !strings.Contains(files["providers.txt"], "supported:") {
		t.Errorf("providers.txt missing supported list:\n%s", files["providers.txt"])
	}
}
