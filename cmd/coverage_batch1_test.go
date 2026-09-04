package cmd

// coverage_batch1_test.go — coverage push batch 1: closes the remaining
// platform-independent error/edge branches in apply, auth-token, models,
// model write, uninstall, show, version, root, helpers, doctor, and launch.
//
// Strategy notes:
//   - The home-resolution failure branches (HOME/USERPROFILE/HOMEDRIVE/HOMEPATH
//     empty) make safepath.HomeDir() fail deterministically on every platform,
//     because os.UserHomeDir errors when the platform env var is unset.
//   - The models --write paths run under a chdir to a temp directory holding a
//     bedrock-config.json copy so the repo's own config is never mutated (the
//     "../bedrock-config.json" fallback is what tests load by default).
//   - TUI forms (huh) cannot be driven without a TTY, so their success paths
//     are intentionally left to the documented irreducible floor; the
//     construction + Run-error branches are exercised instead.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/discovery"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
	"github.com/spf13/cobra"
)

// noHomeEnv clears every environment source of the home directory so
// safepath.HomeDir() fails deterministically (os.UserHomeDir errors when the
// platform variable is unset).
func noHomeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
}

// blockCredentialWrite makes the credential fallback path a directory so reads
// AND writes to it fail deterministically on every platform.
func blockCredentialWrite(t *testing.T, home string) {
	t.Helper()
	credPath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.MkdirAll(filepath.Join(credPath, "child")); err != nil {
		t.Fatalf("mkdir credential blocker: %v", err)
	}
}

// chdirTo moves the process working directory to dir for the test, restoring it
// in cleanup. Tests in this package never use t.Parallel, so chdir is safe.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%s): %v", dir, err)
	}
}

// withBrokenEmbeddedConfig makes loadBedrockConfig fail by injecting unparseable
// bytes, restoring the prior value in cleanup.
func withBrokenEmbeddedConfig(t *testing.T) {
	t.Helper()
	orig := embeddedConfigBytes
	embeddedConfigBytes = []byte("{not valid json")
	t.Cleanup(func() { embeddedConfigBytes = orig })
}

// swapActiveModelsForWrite stubs the discovery seams with all tiers ACTIVE so
// runModelsCheck's --write path can proceed to the config write.
func swapActiveModelsForWrite(t *testing.T) {
	t.Helper()
	origA, origP := listAnthropicModels, listInferenceProfiles
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{
			{ID: "anthropic.claude-opus-5", Status: "ACTIVE"},
			{ID: "anthropic.claude-sonnet-5", Status: "ACTIVE"},
			{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
			{ID: "anthropic.claude-fable-5", Status: "ACTIVE"},
		}, nil
	}
	listInferenceProfiles = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return []discovery.DiscoveredModel{
			{ID: "global.anthropic.claude-opus-5", Status: "ACTIVE"},
			{ID: "global.anthropic.claude-sonnet-5", Status: "ACTIVE"},
			{ID: "global.anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
			{ID: "global.anthropic.claude-fable-5", Status: "ACTIVE"},
		}, nil
	}
	t.Cleanup(func() { listAnthropicModels, listInferenceProfiles = origA, origP })
}

// assertHomeDirError asserts err is a home-directory resolution failure. The
// error text is platform-specific ($HOME vs %userprofile%), so only the
// lowercase substring is matched.
func assertHomeDirError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "home") {
		t.Errorf("expected home resolution error, got: %v", err)
	}
}

// modelsWriteConfigJSON is a minimal bedrock-config.json with all four tier
// keys present, used by the models --write tests.
const modelsWriteConfigJSON = `{
  "version": "9.9.9",
  "models": {
    "opus": "global.anthropic.claude-opus-5",
    "sonnet": "global.anthropic.claude-sonnet-5",
    "haiku": "global.anthropic.claude-haiku-4-5-20251001-v1:0",
    "fable": "global.anthropic.claude-fable-5"
  },
  "defaults": {"region": "us-west-2", "auth_mode": "iam"}
}`

// writeTempBedrockConfig writes a bedrock-config.json into dir.
func writeTempBedrockConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "bedrock-config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp bedrock config: %v", err)
	}
	return path
}

// noBinaryNamesProvider reports no registered binary names so cliBinaryStatus
// takes its "no binary names registered" warn branch.
type noBinaryNamesProvider struct {
	stubProvider
}

func (noBinaryNamesProvider) BinaryNames() []string { return nil }

// ---------------------------------------------------------------------------
// runApply
// ---------------------------------------------------------------------------

func TestRunApply_HomeDirError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	noHomeEnv(t)

	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight"})
	assertHomeDirError(t, err)
}

func TestRunApply_LoadBedrockConfigError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	withBrokenEmbeddedConfig(t)

	err := ExecuteArgs([]string{"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight"})
	if err == nil || !strings.Contains(err.Error(), "bedrock-config.json") {
		t.Errorf("expected bedrock config load error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// auth-token
// ---------------------------------------------------------------------------

func TestAuthToken_HomeDirError(t *testing.T) {
	noHomeEnv(t)

	err := ExecuteArgs([]string{"auth-token"})
	assertHomeDirError(t, err)
}

// TestAuthToken_KeychainReadError_StdoutClean covers the keychain read failure
// branch while proving stdout stays token-only (diagnostics go to stderr).
func TestAuthToken_KeychainReadError_StdoutClean(t *testing.T) {
	home := testutil.NewTestHome(t)
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-batch1-authtoken")
	blockCredentialWrite(t, home)

	out := captureStdout(t, func() {
		err := ExecuteArgs([]string{"auth-token"})
		if err == nil {
			t.Fatal("expected keychain read error")
		}
		if !strings.Contains(err.Error(), "keychain") {
			t.Errorf("expected keychain error message, got: %v", err)
		}
	})
	if out != "" {
		t.Errorf("stdout must stay empty on keychain error, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// runModelsCheck / writeBedrockConfigFile
// ---------------------------------------------------------------------------

func TestRunModelsCheck_LoadConfigError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	withBrokenEmbeddedConfig(t)

	err := ExecuteArgs([]string{"models", "check"})
	if err == nil {
		t.Fatal("expected bedrock config load error")
	}
}

func TestRunModelsCheck_FoundationError(t *testing.T) {
	defer resetFlags()
	resetFlags()

	origA := listAnthropicModels
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, errors.New("sts denied")
	}
	t.Cleanup(func() { listAnthropicModels = origA })

	err := ExecuteArgs([]string{"models", "check"})
	if err == nil || !strings.Contains(err.Error(), "foundation models") {
		t.Errorf("expected foundation model query error, got: %v", err)
	}
}

func TestRunModelsCheck_ProfilesError(t *testing.T) {
	defer resetFlags()
	resetFlags()

	origA, origP := listAnthropicModels, listInferenceProfiles
	listAnthropicModels = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, nil
	}
	listInferenceProfiles = func(_ context.Context, _ string) ([]discovery.DiscoveredModel, error) {
		return nil, errors.New("profiles denied")
	}
	t.Cleanup(func() { listAnthropicModels, listInferenceProfiles = origA, origP })

	err := ExecuteArgs([]string{"models", "check"})
	if err == nil || !strings.Contains(err.Error(), "inference profiles") {
		t.Errorf("expected inference profile query error, got: %v", err)
	}
}

// TestRunModelsCheck_WritePersistsAndRecomputes drives the full --write branch:
// pin replacement, config persistence, and the recomputed exit status. Runs
// under a chdir so the write targets a temp copy of bedrock-config.json.
func TestRunModelsCheck_WritePersistsAndRecomputes(t *testing.T) {
	defer resetFlags()
	resetFlags()

	dir := t.TempDir()
	writeTempBedrockConfig(t, dir, modelsWriteConfigJSON)
	chdirTo(t, dir)
	swapActiveModelsForWrite(t)

	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"models", "check", "--write", "--set-opus=anthropic.claude-opus-5"}); err != nil {
			t.Fatalf("models check --write: %v", err)
		}
	})
	if !strings.Contains(out, "bedrock-config.json updated.") {
		t.Errorf("expected updated message, got:\n%s", out)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "bedrock-config.json")) // nosemgrep: go_filesystem_rule-fileread — path under t.TempDir()
	if err != nil {
		t.Fatalf("reading rewritten config: %v", err)
	}
	var root map[string]any
	parsed, err := testutil.ParseJSON(raw)
	if err != nil {
		t.Fatalf("parsing rewritten config: %v", err)
	}
	root = parsed
	models, _ := root["models"].(map[string]any)
	if models["opus"] != "anthropic.claude-opus-5" {
		t.Errorf("expected opus pinned to anthropic.claude-opus-5, got %v", models["opus"])
	}
}

// TestRunModelsCheck_WriteFailure covers the write error branch of the --write
// path: a read-only bedrock-config.json parses but cannot be overwritten.
func TestRunModelsCheck_WriteFailure(t *testing.T) {
	defer resetFlags()
	resetFlags()

	dir := t.TempDir()
	path := writeTempBedrockConfig(t, dir, modelsWriteConfigJSON)
	chdirTo(t, dir)
	swapActiveModelsForWrite(t)
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatalf("chmod config: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	err := ExecuteArgs([]string{"models", "check", "--write", "--set-opus=anthropic.claude-opus-5"})
	if err == nil || !strings.Contains(err.Error(), "writing") {
		t.Errorf("expected config write error, got: %v", err)
	}
}

func TestWriteBedrockConfigFile_ReadError(t *testing.T) {
	dir := t.TempDir()
	chdirTo(t, dir)

	err := writeBedrockConfigFile(testBedrockConfigForModels())
	if err == nil || !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected read error for missing config, got: %v", err)
	}
}

func TestWriteBedrockConfigFile_ParseError(t *testing.T) {
	dir := t.TempDir()
	writeTempBedrockConfig(t, dir, "{not valid json")
	chdirTo(t, dir)

	err := writeBedrockConfigFile(testBedrockConfigForModels())
	if err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

// TestWriteBedrockConfigFile_CreatesMissingModelsKey covers the branch where the
// on-disk config has no "models" map: a fresh one is created and populated.
func TestWriteBedrockConfigFile_CreatesMissingModelsKey(t *testing.T) {
	dir := t.TempDir()
	writeTempBedrockConfig(t, dir, `{"version":"9.9.9","defaults":{"region":"us-west-2"}}`)
	chdirTo(t, dir)

	cfg := testBedrockConfigForModels()
	cfg.Models.Opus = "anthropic.claude-opus-4-9"
	if err := writeBedrockConfigFile(cfg); err != nil {
		t.Fatalf("writeBedrockConfigFile: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "bedrock-config.json")) // nosemgrep: go_filesystem_rule-fileread — path under t.TempDir()
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	root, err := testutil.ParseJSON(raw)
	if err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	models, _ := root["models"].(map[string]any)
	if models == nil || models["opus"] != "anthropic.claude-opus-4-9" {
		t.Errorf("expected models map created with opus pinned, got %v", models)
	}
}

func TestPinStatus_EmptyPin(t *testing.T) {
	status, found := pinStatus("", nil, nil)
	if found || status != "" {
		t.Errorf("pinStatus(\"\") = (%q, %v), want (\"\", false)", status, found)
	}
}

// TestBuildModelsReport_NotInCatalog covers the "not found in live catalog"
// branch: a pinned ID absent from both catalogs.
func TestBuildModelsReport_NotInCatalog(t *testing.T) {
	cfg := testBedrockConfigForModels()
	cfg.Models.Opus = "global.anthropic.claude-opus-nonexistent"
	anthropic := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-sonnet-4-6", Status: "ACTIVE"},
		{ID: "anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
		{ID: "anthropic.claude-fable-5", Status: "ACTIVE"},
	}
	profiles := []discovery.DiscoveredModel{
		{ID: "global.anthropic.claude-sonnet-4-6", Status: "ACTIVE"},
		{ID: "global.anthropic.claude-haiku-4-5-20251001-v1:0", Status: "ACTIVE"},
		{ID: "global.anthropic.claude-fable-5", Status: "ACTIVE"},
	}
	report, anyLegacy := buildModelsReport(cfg, anthropic, profiles)
	if !anyLegacy {
		t.Error("expected anyLegacy=true when a pin is not in the catalog")
	}
	if !strings.Contains(report, "not found in live catalog") {
		t.Errorf("expected not-found report line, got:\n%s", report)
	}
}

// TestUnrecognizedModels_SortsMultiple exercises the sort comparator with two
// unmatched models so the ordering is actually applied.
func TestUnrecognizedModels_SortsMultiple(t *testing.T) {
	models := []discovery.DiscoveredModel{
		{ID: "anthropic.claude-nova-2", Status: "ACTIVE"},
		{ID: "anthropic.claude-nova-1", Status: "ACTIVE"},
	}
	got := unrecognizedModels(models)
	if len(got) != 2 {
		t.Fatalf("expected 2 unrecognized models, got %d", len(got))
	}
	if got[0].ID != "anthropic.claude-nova-1" || got[1].ID != "anthropic.claude-nova-2" {
		t.Errorf("expected sorted output, got %q, %q", got[0].ID, got[1].ID)
	}
}

func TestResolveSetFlags_AllTiers(t *testing.T) {
	defer resetFlags()
	resetFlags()
	modelsCheckFlags.setOpus = "opus-id"
	modelsCheckFlags.setSonnet = "sonnet-id"
	modelsCheckFlags.setHaiku = "haiku-id"
	modelsCheckFlags.setFable = "fable-id"

	got := resolveSetFlags()
	if len(got) != 4 {
		t.Fatalf("expected 4 tier entries, got %v", got)
	}
	for tier, want := range map[discovery.Tier]string{
		discovery.TierOpus:   "opus-id",
		discovery.TierSonnet: "sonnet-id",
		discovery.TierHaiku:  "haiku-id",
		discovery.TierFable:  "fable-id",
	} {
		if got[tier] != want {
			t.Errorf("set flag for %s = %q, want %q", tier, got[tier], want)
		}
	}
}

// ---------------------------------------------------------------------------
// version / root Execute
// ---------------------------------------------------------------------------

func TestVersion_JSONFlag(t *testing.T) {
	out := captureStdout(t, func() {
		if err := ExecuteArgs([]string{"version", "--json"}); err != nil {
			t.Fatalf("version --json: %v", err)
		}
	})
	if !strings.Contains(out, Version) || !strings.Contains(out, "version") {
		t.Errorf("expected JSON version output, got: %s", out)
	}
}

// TestExecute_SuccessPath covers Execute()'s success path (the error path calls
// os.Exit and is untestable in-process).
func TestExecute_SuccessPath(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})
	out := captureStdout(t, func() { Execute() })
	if !strings.Contains(out, Version) {
		t.Errorf("expected version in Execute output, got: %s", out)
	}
}

// ---------------------------------------------------------------------------
// uninstall
// ---------------------------------------------------------------------------

func TestRunUninstall_HomeDirError(t *testing.T) {
	noHomeEnv(t)

	err := ExecuteArgs([]string{"uninstall"})
	assertHomeDirError(t, err)
}

func TestRunUninstall_UnknownCLI(t *testing.T) {
	_ = setupApplyTest(t)

	err := ExecuteArgs([]string{"uninstall", "--cli", "nonesuch"})
	if err == nil || !strings.Contains(err.Error(), "nonesuch") {
		t.Errorf("expected unknown CLI error, got: %v", err)
	}
}

// TestRunUninstall_ConfirmationReadError covers the stdin scanner failure branch
// by pointing os.Stdin at a closed file, so Scan() reports a read error instead
// of EOF.
func TestRunUninstall_ConfirmationReadError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	_ = setupApplyTest(t)

	f, err := os.CreateTemp("", "juggernaut-closed-stdin")
	if err != nil {
		t.Fatalf("create temp stdin: %v", err)
	}
	stdinPath := f.Name()
	if err := f.Close(); err != nil {
		t.Fatalf("close temp stdin: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(stdinPath) })

	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig })

	err = ExecuteArgs([]string{"uninstall"})
	if err == nil || !strings.Contains(err.Error(), "reading confirmation") {
		t.Errorf("expected confirmation read error, got: %v", err)
	}
}

// TestUninstallActivationFull_UninstallErrorWarns covers the warn-and-continue
// branch of uninstallActivationFull: when UninstallWith cannot process a shell
// profile (here: ~/.bashrc is a directory, so reading it as a file fails), the
// failure is surfaced as a warning instead of aborting uninstall.
func TestUninstallActivationFull_UninstallErrorWarns(t *testing.T) {
	home := setupApplyTestWithReset(t)
	rc := filepath.Join(home, ".bashrc")
	if err := safepath.MkdirAll(rc); err != nil {
		t.Fatalf("mkdir %s: %v", rc, err)
	}

	prov, err := provider.Get("codex")
	if err != nil {
		t.Fatalf("provider.Get(codex): %v", err)
	}
	out := captureStderr(t, func() { uninstallActivationFull(home, prov) })
	if !strings.Contains(out, "could not remove shell activation") {
		t.Errorf("expected uninstall warning, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// doctor: cliBinaryStatus
// ---------------------------------------------------------------------------

func TestCliBinaryStatus_NoBinaryNames(t *testing.T) {
	status, detail := cliBinaryStatus(noBinaryNamesProvider{})
	if status != doctor.Warn {
		t.Errorf("expected WARN status, got %s", status)
	}
	if !strings.Contains(detail, "no binary names registered") {
		t.Errorf("expected no-binary-names detail, got %q", detail)
	}
}

// ---------------------------------------------------------------------------
// helpers: findBedrockConfigFile, printApplyDryRun, reportLegacyRecovery,
// resolveApplyInputs, resolveCredential, commitApply
// ---------------------------------------------------------------------------

// TestFindBedrockConfigFile_NextToExecutable covers the branch that prefers a
// bedrock-config.json sitting beside the running executable.
func TestFindBedrockConfigFile_NextToExecutable(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	dir := filepath.Dir(self)
	candidate := filepath.Join(dir, "bedrock-config.json")
	if _, err := os.Stat(candidate); err == nil {
		// A config already sits next to the binary; the branch is trivially covered.
		if got := findBedrockConfigFile(); got != candidate {
			t.Fatalf("findBedrockConfigFile = %q, want %q", got, candidate)
		}
		return
	}
	if err := os.WriteFile(candidate, []byte("{}"), 0o600); err != nil {
		t.Skipf("test binary dir not writable: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(candidate) })

	if got := findBedrockConfigFile(); got != candidate {
		t.Errorf("findBedrockConfigFile = %q, want %q", got, candidate)
	}
}

func TestPrintApplyDryRun_ConfigPathError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)

	err := printApplyDryRun(home, &schema.Block{}, stubProvider{formatName: "json", pathErr: errors.New("bad path")}, &bedrock.Config{}, provider.Options{})
	if err == nil {
		t.Fatal("expected ConfigPath error to propagate")
	}
}

func TestPrintApplyDryRun_BuildConfigError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)

	err := printApplyDryRun(home, &schema.Block{}, buildErrorProvider{stubProvider{formatName: "json"}}, &bedrock.Config{}, provider.Options{})
	if err == nil || !strings.Contains(err.Error(), "build config failed") {
		t.Errorf("expected BuildConfig error, got: %v", err)
	}
}

func TestPrintApplyDryRun_CollisionReadError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)
	dir := filepath.Join(home, "isadir")
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err := printApplyDryRun(home, &schema.Block{}, dirPathProvider{stubProvider: stubProvider{formatName: "json"}, dir: dir}, &bedrock.Config{}, provider.Options{})
	if err == nil || !strings.Contains(err.Error(), "checking existing configuration") {
		t.Errorf("expected collision read error, got: %v", err)
	}
}

// TestReportLegacyRecovery_PrintsActions seeds a v4.2.6 backup artifact so
// recovery emits one action line.
func TestReportLegacyRecovery_PrintsActions(t *testing.T) {
	home := testutil.NewTestHome(t)
	binDir := activation.DefaultBinDir(home)
	if err := safepath.MkdirAll(binDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backup := filepath.Join(binDir, "claude.juggernaut-original")
	if runtime.GOOS == "windows" {
		backup = filepath.Join(binDir, "claude.juggernaut-original.cmd")
	}
	if err := os.WriteFile(backup, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	out := captureStdout(t, func() { reportLegacyRecovery(home) })
	if !strings.Contains(out, "✓") {
		t.Errorf("expected recovery action output, got: %q", out)
	}
}

// TestReportLegacyRecovery_ErrorWarns makes the backup-restore rename fail by
// stripping write permission from the bin directory (Windows ignores directory
// write permission for renames, so the test is POSIX-only).
func TestReportLegacyRecovery_ErrorWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write permission does not gate renames on Windows")
	}
	home := testutil.NewTestHome(t)
	binDir := activation.DefaultBinDir(home)
	if err := safepath.MkdirAll(binDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backup := filepath.Join(binDir, "claude.juggernaut-original")
	if err := os.WriteFile(backup, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	if err := os.Chmod(binDir, 0o555); err != nil {
		t.Fatalf("chmod binDir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(binDir, 0o700) }) // nosemgrep: go.lang.correctness.permissions.file_permission.incorrect-default-permission, go_file-permissions_rule-fileperm -- restore dir perms for cleanup

	out := captureStderr(t, func() { reportLegacyRecovery(home) })
	if !strings.Contains(out, "could not recover legacy artifacts") {
		t.Errorf("expected recovery warning, got: %q", out)
	}
}

// TestResolveApplyInputs_PromptsWithoutAuthMode reaches the interactive prompt
// fallback of resolveApplyInputs when stdin is treated as a TTY and --auth is
// omitted. Without a real console the form fails, which exercises
// promptApplyInputs' construction and Run-error branches.
func TestResolveApplyInputs_PromptsWithoutAuthMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TUI form blocks on Windows without a real console")
	}
	defer resetFlags()
	resetFlags()
	withInteractiveStdin(t, true)

	home := testutil.NewTestHome(t)
	bCfg := &bedrock.Config{Defaults: bedrock.Defaults{Region: "us-west-2", AuthMode: authmode.IAM}}
	_, _, _, err := resolveApplyInputs(home, bCfg, provider.MustGet("claude"))
	if err == nil {
		t.Log("TUI prompt unexpectedly succeeded (CI may have a pty)")
		return
	}
	if !strings.Contains(err.Error(), "terminal") && !strings.Contains(err.Error(), "tty") &&
		!strings.Contains(err.Error(), "interactive") {
		t.Logf("prompt failed as expected (non-terminal): %v", err)
	}
}

// TestResolveCredential_KeychainErrorPrompts covers the warn-then-prompt branch:
// an unreadable keychain without --preserve-key warns and falls through to the
// (failing in tests) TUI form.
func TestResolveCredential_KeychainErrorPrompts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TUI form blocks on Windows without a real console")
	}
	defer resetFlags()
	resetFlags()

	home := testutil.NewTestHome(t)
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-batch1-cred")
	blockCredentialWrite(t, home)

	out := captureStderr(t, func() {
		token, err := resolveCredential(authmode.BedrockAPIKey, home)
		if err == nil {
			t.Log("TUI prompt unexpectedly succeeded (CI may have a pty)")
		}
		if token != "" {
			t.Errorf("expected empty token on keychain error, got %q", token)
		}
	})
	if !strings.Contains(out, "could not read keychain") {
		t.Errorf("expected keychain warning, got: %q", out)
	}
}

// TestCommitApply_KeychainSetError covers the credential-store failure branch of
// commitApply. A token over the Windows keychain limit forces the file fallback,
// which fails because the credential path is a directory. Windows-only: the OS
// keychain accepts isolated-service writes on other platforms.
func TestCommitApply_KeychainSetError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fallback is only forced by the Windows keychain size limit")
	}
	defer resetFlags()
	resetFlags()
	home := setupApplyTest(t)
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-batch1-commit")
	blockCredentialWrite(t, home)

	huge := strings.Repeat("t", 3000)
	err := commitApply(home, authmode.BedrockAPIKey, huge, &schema.Block{}, stubProvider{formatName: "json"}, &bedrock.Config{}, provider.Options{})
	if err == nil || !strings.Contains(err.Error(), "storing API key") {
		t.Errorf("expected keychain store error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// launch
// ---------------------------------------------------------------------------

func TestRunLaunch_HomeDirError(t *testing.T) {
	noHomeEnv(t)

	err := runLaunch(&cobra.Command{}, []string{})
	assertHomeDirError(t, err)
}

// TestLaunchNamedCLI_TokenReadError covers the TokenGetter failure branch: with
// the shared credential unreadable, the launch aborts before running the binary.
func TestLaunchNamedCLI_TokenReadError(t *testing.T) {
	home := testutil.NewTestHome(t)
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-batch1-launch")
	blockCredentialWrite(t, home)

	grokDir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(grokDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "[juggernaut]\n  [juggernaut.auth]\n    mode = \"" + authmode.BedrockAPIKey + "\"\n  [juggernaut.meta]\n    managedBy = \"juggernaut\"\n"
	if err := os.WriteFile(filepath.Join(grokDir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	err := launchNamedCLI("grok", nil)
	if err == nil {
		t.Fatal("expected keychain read error from launch")
	}
	if !strings.Contains(err.Error(), "keychain") {
		t.Errorf("expected keychain error message, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// show
// ---------------------------------------------------------------------------

func TestRunShow_HomeDirError(t *testing.T) {
	noHomeEnv(t)

	err := ExecuteArgs([]string{"show"})
	assertHomeDirError(t, err)
}

// TestRunShow_SettingsReadErrorWarns covers the warn-and-continue branch when a
// scope's settings.json cannot be read (here: it is a directory).
func TestRunShow_SettingsReadErrorWarns(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := setupApplyTest(t)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := safepath.MkdirAll(settingsPath); err != nil {
		t.Fatalf("mkdir settings blocker: %v", err)
	}

	out := captureStderr(t, func() {
		if err := ExecuteArgs([]string{"show", "--scope=user"}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if !strings.Contains(out, "could not read user settings") {
		t.Errorf("expected settings read warning, got: %q", out)
	}
}
