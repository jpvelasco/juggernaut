package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/activation"
	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/doctor"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/jpvelasco/juggernaut/v5/internal/schema"
	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
	"github.com/spf13/cobra"
)

// mockDoctorPSRunner sets up a fresh temp home with a mock PowerShell discovery
// runner returning a profile that has NO Juggernaut activation block, so
// runDoctor's activation checks are hermetic and deterministic on every OS.
func mockDoctorPSRunner(t *testing.T) string {
	t.Helper()
	home := testutil.NewTestHome(t)
	profile := filepath.Join(home, "OneDrive", "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	if err := safepath.MkdirAll(filepath.Dir(profile)); err != nil {
		t.Fatalf("creating profile dir: %v", err)
	}
	if err := safepath.WriteFile(filepath.Dir(profile), profile, []byte("export FOO=bar")); err != nil {
		t.Fatalf("writing profile: %v", err)
	}
	runner := &mockPSRunner{
		output: map[string][]byte{
			"pwsh.exe":       mockPSOutputJSON(profile, profile),
			"powershell.exe": mockPSOutputJSON(profile, profile),
		},
	}
	activation.SetPSRunnerForTesting(runner)
	t.Cleanup(activation.ResetPSRunnerForTesting)
	return home
}

// isolateDoctorKeychain points the shared keychain at a throwaway service so
// doctor tests never read real stored credentials.
func isolateDoctorKeychain(t *testing.T) {
	t.Helper()
	t.Setenv("JUGGERNAUT_KEYCHAIN_SERVICE", "jug-doctor-coverage")
}

// blockSettingsLock replaces the settings.json lock file with a DIRECTORY so
// the next config Write fails deterministically at flock acquisition.
func blockSettingsLock(t *testing.T, home string) {
	t.Helper()
	lockPath := filepath.Join(home, ".claude", "settings.json.lock")
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("removing stale lock: %v", err)
	}
	if err := safepath.MkdirAll(lockPath); err != nil {
		t.Fatalf("mkdir lock path: %v", err)
	}
}

// ---------------------------------------------------------------------------
// runDoctor
// ---------------------------------------------------------------------------

// TestRunDoctor_HomeDirError covers the homeDir() failure branch: with HOME,
// USERPROFILE, and (on Windows) HOMEDRIVE/HOMEPATH all empty, the home
// directory cannot be resolved and runDoctor fails immediately.
func TestRunDoctor_HomeDirError(t *testing.T) {
	defer resetFlags()
	resetFlags()

	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	err := runDoctor(&cobra.Command{}, []string{})
	if err == nil {
		t.Fatal("expected error when no home directory can be resolved")
	}
	if !strings.Contains(err.Error(), "home") && !strings.Contains(err.Error(), "HOME") {
		t.Errorf("expected home resolution error, got: %v", err)
	}
}

// TestRunDoctor_MissingBedrockConfig_Fails covers the loadBedrockConfig failure
// branch: an unloadable config produces a FAIL check instead of a hard crash.
func TestRunDoctor_MissingBedrockConfig_Fails(t *testing.T) {
	defer resetFlags()
	resetFlags()

	orig := embeddedConfigBytes
	embeddedConfigBytes = []byte("{not valid json")
	t.Cleanup(func() { embeddedConfigBytes = orig })

	_ = mockDoctorPSRunner(t)
	isolateDoctorKeychain(t)

	out := captureStdout(t, func() {
		err := runDoctor(&cobra.Command{}, []string{})
		if err == nil {
			t.Error("expected doctor to report failures for a broken bedrock config")
		}
	})
	if !strings.Contains(out, "bedrock-config.json") {
		t.Errorf("expected bedrock-config.json check in report, got:\n%s", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("expected a FAIL status for the broken config, got:\n%s", out)
	}
}

// TestRunDoctor_GrokProjectScope_Errors covers the grok project-scope guard,
// while the user-scope run proves grok still falls through to the normal
// user-scope flow (scopes = []string{"user"}).
func TestRunDoctor_GrokProjectScope_Errors(t *testing.T) {
	defer resetFlags()
	resetFlags()

	t.Run("project scope rejected", func(t *testing.T) {
		doctorFlags.cli = "grok"
		doctorFlags.scope = "project"
		err := runDoctor(&cobra.Command{}, []string{})
		if err == nil {
			t.Fatal("expected project scope to be rejected for grok")
		}
		if !strings.Contains(err.Error(), "grok has no project-scope config") {
			t.Errorf("expected grok project-scope error, got: %v", err)
		}
	})

	t.Run("user scope proceeds", func(t *testing.T) {
		_ = mockDoctorPSRunner(t)
		isolateDoctorKeychain(t)
		doctorFlags.cli = "grok"
		doctorFlags.scope = "user"
		out := captureStdout(t, func() {
			err := runDoctor(&cobra.Command{}, []string{})
			if err == nil {
				t.Error("expected doctor to report failures for an unconfigured grok")
			}
			if strings.Contains(err.Error(), "project-scope") {
				t.Errorf("user scope must not hit the grok project guard: %v", err)
			}
		})
		if !strings.Contains(out, "grok") {
			t.Errorf("expected grok checks in report, got:\n%s", out)
		}
	})
}

// TestRunDoctor_OpusplanWarn covers the top-level "opusplan" model warning
// emitted for a hand-edited settings.json.
func TestRunDoctor_OpusplanWarn(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := mockDoctorPSRunner(t)
	isolateDoctorKeychain(t)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := safepath.MkdirAll(filepath.Dir(settingsPath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"model":"opusplan"}`), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	out := captureStdout(t, func() {
		_ = runDoctor(&cobra.Command{}, []string{})
	})
	if !strings.Contains(out, "top-level model") || !strings.Contains(out, "opusplan") {
		t.Errorf("expected opusplan warning, got:\n%s", out)
	}
}

// TestRunDoctor_KeychainErrorWarns covers the keychain error branch: an
// unreadable credential store is reported as a WARN, not a failure.
func TestRunDoctor_KeychainErrorWarns(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := mockDoctorPSRunner(t)
	isolateDoctorKeychain(t)

	// Make the credential path a DIRECTORY so reading it fails deterministically
	// on every platform (not ErrNotExist).
	credPath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.MkdirAll(filepath.Join(credPath, "child")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	out := captureStdout(t, func() {
		_ = runDoctor(&cobra.Command{}, []string{})
	})
	if !strings.Contains(out, "keychain") || !strings.Contains(out, "error reading") {
		t.Errorf("expected keychain read warning, got:\n%s", out)
	}
}

// TestRunDoctor_KeychainTokenFound covers the bearer-token-found branch, which
// routes into checkKeyExpiry. The seeded versioned credential makes the read
// deterministic without touching the OS keychain.
func TestRunDoctor_KeychainTokenFound(t *testing.T) {
	defer resetFlags()
	resetFlags()

	home := mockDoctorPSRunner(t)
	isolateDoctorKeychain(t)

	credPath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.WriteFile(home, credPath, []byte("juggernaut-credential-v1\n"+shortTermKey("20990101T000000Z", "43200"))); err != nil {
		t.Fatalf("seeding credential: %v", err)
	}

	out := captureStdout(t, func() {
		_ = runDoctor(&cobra.Command{}, []string{})
	})
	if !strings.Contains(out, "bearer token found") {
		t.Errorf("expected bearer token found check, got:\n%s", out)
	}
}

// TestRunDoctor_NonClaudeActivationWarns covers the Windows non-Claude
// activation branch (InstalledTargetsForMarkers returns zero) and, on other
// platforms, the equivalent not-installed branch. Both must produce a WARN.
func TestRunDoctor_NonClaudeActivationWarns(t *testing.T) {
	defer resetFlags()
	resetFlags()

	_ = mockDoctorPSRunner(t)
	isolateDoctorKeychain(t)
	doctorFlags.cli = "codex"

	out := captureStdout(t, func() {
		_ = runDoctor(&cobra.Command{}, []string{})
	})
	if !strings.Contains(out, "codex activation") || !strings.Contains(out, "[WARN]") {
		t.Errorf("expected codex activation warning, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// providerConfigLabel / legacyArtifactStatus
// ---------------------------------------------------------------------------

// TestProviderConfigLabel_FallbackAndResolved covers both the ConfigPath-error
// fallback and the normal basename label.
func TestProviderConfigLabel_FallbackAndResolved(t *testing.T) {
	label := providerConfigLabel(stubProvider{pathErr: errors.New("boom")}, "user")
	if label != "stub config (user)" {
		t.Errorf("expected fallback label for path error, got %q", label)
	}

	label = providerConfigLabel(provider.MustGet("claude"), "user")
	if !strings.Contains(label, "settings.json") || !strings.Contains(label, "(user)") {
		t.Errorf("expected settings.json (user) label, got %q", label)
	}
}

// TestLegacyArtifactStatus_DetectsBackup covers the v4.2.6 artifact warning by
// seeding a restoreable backup file (no content-hash match required).
func TestLegacyArtifactStatus_DetectsBackup(t *testing.T) {
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

	r := doctor.NewReport()
	legacyArtifactStatus(home, r)
	out := r.String()
	if !strings.Contains(out, "v4.2.6 artifact") || !strings.Contains(out, "[WARN]") {
		t.Errorf("expected v4.2.6 artifact warning, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// checkConnectivity
// ---------------------------------------------------------------------------

// writeClaudeSettings writes settings.json under home for connectivity tests.
func writeClaudeSettings(t *testing.T, home string, data map[string]any) {
	t.Helper()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := safepath.MkdirAll(filepath.Dir(settingsPath)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	enc, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(settingsPath, enc, 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// TestCheckConnectivity_MissingAuthConfig covers the "no auth config" and
// partial-auth branches of checkConnectivity.
func TestCheckConnectivity_MissingAuthConfig(t *testing.T) {
	cases := []struct {
		name   string
		block  map[string]any
		want   string
		status doctor.Status
	}{
		{
			name:   "no auth config",
			block:  map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
			want:   "no auth config in juggernaut block",
			status: doctor.Warn,
		},
		{
			name:   "missing auth mode",
			block:  map[string]any{"auth": map[string]any{"region": "us-west-2"}},
			want:   "missing auth mode",
			status: doctor.Warn,
		},
		{
			name:   "missing auth region",
			block:  map[string]any{"auth": map[string]any{"mode": "iam"}},
			want:   "missing auth region",
			status: doctor.Warn,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := testutil.NewTestHome(t)
			writeClaudeSettings(t, home, map[string]any{"juggernaut": tc.block})

			r := doctor.NewReport()
			checkConnectivity(r, home, "", []string{"user"})
			out := r.String()
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q, got:\n%s", tc.want, out)
			}
			if !strings.Contains(out, "["+string(tc.status)+"]") {
				t.Errorf("expected %s status, got:\n%s", tc.status, out)
			}
		})
	}
}

// TestCheckConnectivity_APIAKeyWithoutToken_Fails covers the FAIL branch: a
// bedrock-api-key auth block with no token fails without any network call.
func TestCheckConnectivity_APIAKeyWithoutToken_Fails(t *testing.T) {
	home := testutil.NewTestHome(t)
	writeClaudeSettings(t, home, map[string]any{
		"juggernaut": map[string]any{
			"auth": map[string]any{"mode": authmode.BedrockAPIKey, "region": "us-west-2"},
		},
	})

	r := doctor.NewReport()
	checkConnectivity(r, home, "", []string{"user"})
	out := r.String()
	if !strings.Contains(out, "bedrock connectivity") || !strings.Contains(out, "[FAIL]") {
		t.Errorf("expected connectivity FAIL, got:\n%s", out)
	}
	if !strings.Contains(out, "not found in keychain") {
		t.Errorf("expected no-keychain message, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// checkFableDataRetention / checkProviderConfigScope
// ---------------------------------------------------------------------------

// TestCheckFableDataRetention_NoModelOverrides covers the early return when the
// juggernaut block has no modelOverrides table.
func TestCheckFableDataRetention_NoModelOverrides(t *testing.T) {
	r := doctor.NewReport()
	checkFableDataRetention(r, "user", map[string]any{
		"juggernaut": map[string]any{"meta": map[string]any{"managedBy": "juggernaut"}},
	})
	if out := r.String(); out != "" {
		t.Errorf("expected no check without modelOverrides, got:\n%s", out)
	}
}

// TestCheckProviderConfigScope_ReadError_Fails covers the read-error branch of
// checkProviderConfigScope using a config path that is a directory.
func TestCheckProviderConfigScope_ReadError_Fails(t *testing.T) {
	home := testutil.NewTestHome(t)
	dir := filepath.Join(home, "isadir")
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	prov := dirPathProvider{stubProvider: stubProvider{formatName: "json"}, dir: dir}

	status, detail := checkProviderConfigScope(prov, home, "user", true)
	if status != doctor.Fail {
		t.Fatalf("expected FAIL for unreadable config, got %s", status)
	}
	if detail == "" {
		t.Error("expected a detail message for the read failure")
	}
}

// ---------------------------------------------------------------------------
// resolveCredential
// ---------------------------------------------------------------------------

// TestResolveCredential_PreserveKey_KeychainReadError covers the
// --preserve-key keychain error branch deterministically (no backend
// dependency): the credential path is a directory, so GetWithFallback fails.
func TestResolveCredential_PreserveKey_KeychainReadError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	applyFlags.preserveKey = true

	home := testutil.NewTestHome(t)
	credPath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.MkdirAll(filepath.Join(credPath, "child")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	token, err := resolveCredential(authmode.BedrockAPIKey, home)
	if err == nil {
		t.Fatal("expected error when keychain read fails with --preserve-key")
	}
	if !strings.Contains(err.Error(), "reading existing key") {
		t.Errorf("expected 'reading existing key' error, got: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token on error, got %q", token)
	}
}

// ---------------------------------------------------------------------------
// commitApply
// ---------------------------------------------------------------------------

// buildErrorProvider fails during BuildConfig.
type buildErrorProvider struct {
	stubProvider
}

func (buildErrorProvider) BuildConfig(*bedrock.Config, provider.Options) (provider.ConfigPlan, error) {
	return provider.ConfigPlan{}, errors.New("build config failed")
}

// invalidPlanProvider returns a plan that fails Validate (empty managed key).
type invalidPlanProvider struct {
	stubProvider
}

func (invalidPlanProvider) BuildConfig(*bedrock.Config, provider.Options) (provider.ConfigPlan, error) {
	return provider.ConfigPlan{ManagedKeys: []string{""}}, nil
}

func TestCommitApply_BuildConfigError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)

	err := commitApply(home, "iam", "", &schema.Block{}, buildErrorProvider{stubProvider{formatName: "json"}}, &bedrock.Config{}, provider.Options{})
	if err == nil || !strings.Contains(err.Error(), "build config failed") {
		t.Errorf("expected BuildConfig error to propagate, got: %v", err)
	}
}

func TestCommitApply_InvalidPlan_ValidationError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)

	err := commitApply(home, "iam", "", &schema.Block{}, invalidPlanProvider{stubProvider{formatName: "json"}}, &bedrock.Config{}, provider.Options{})
	if err == nil || !strings.Contains(err.Error(), "invalid config plan for stub") {
		t.Errorf("expected plan validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "empty managed key") {
		t.Errorf("expected empty-managed-key detail, got: %v", err)
	}
}

func TestCommitApply_ConfigPathError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)

	sentinel := errors.New("bad path")
	err := commitApply(home, "iam", "", &schema.Block{}, stubProvider{formatName: "json", pathErr: sentinel}, &bedrock.Config{}, provider.Options{})
	if err == nil || !errors.Is(err, sentinel) {
		t.Errorf("expected ConfigPath error to propagate, got: %v", err)
	}
}

func TestCommitApply_CollisionReadError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)
	dir := filepath.Join(home, "isadir")
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err := commitApply(home, "iam", "", &schema.Block{}, dirPathProvider{stubProvider: stubProvider{formatName: "json"}, dir: dir}, &bedrock.Config{}, provider.Options{})
	if err == nil || !strings.Contains(err.Error(), "checking existing configuration") {
		t.Errorf("expected collision-detection read error, got: %v", err)
	}
}

// TestCommitApply_MergeError_WhenLockBlocked covers the config-write failure
// branch: a settings.json.lock path that is a DIRECTORY makes flock acquisition
// fail deterministically after collision detection passes.
func TestCommitApply_MergeError_WhenLockBlocked(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := setupApplyTest(t)

	bCfg, err := loadBedrockConfig()
	if err != nil {
		t.Skipf("no bedrock config: %v", err)
	}
	prov := provider.MustGet("claude")
	opts := provider.Options{
		SchemaOpts: schema.Options{
			AuthMode:      "iam",
			Region:        "us-west-2",
			Scope:         "user",
			Version:       "5.4.0",
			Effort:        "high",
			AuthValidated: true,
		},
		AuthMode: "iam",
		Region:   "us-west-2",
		Scope:    "user",
		Version:  "5.4.0",
	}
	block, err := schema.Build(bCfg, opts.SchemaOpts)
	if err != nil {
		t.Fatalf("schema.Build: %v", err)
	}

	blockSettingsLock(t, home)

	err = commitApply(home, "iam", "", block, prov, bCfg, opts)
	if err == nil || !strings.Contains(err.Error(), "lock") {
		t.Errorf("expected lock acquisition error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// uninstallSettingsBlock / removeKeychainToken
// ---------------------------------------------------------------------------

// TestUninstallSettingsBlock_NewManagerError covers the newProviderManager
// warnf branch via a ConfigPath error.
func TestUninstallSettingsBlock_NewManagerError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)

	out := captureStderr(t, func() {
		uninstallSettingsBlock(home, "user", stubProvider{pathErr: errors.New("bad path")})
	})
	if !strings.Contains(out, "user config: bad path") {
		t.Errorf("expected manager error warning, got: %q", out)
	}
}

// TestUninstallSettingsBlock_HasManagedKeysError covers the warnf when the
// config cannot be read to check for managed keys.
func TestUninstallSettingsBlock_HasManagedKeysError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)
	dir := filepath.Join(home, "isadir")
	if err := safepath.MkdirAll(dir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	out := captureStderr(t, func() {
		uninstallSettingsBlock(home, "user", dirPathProvider{stubProvider: stubProvider{formatName: "json"}, dir: dir})
	})
	if !strings.Contains(out, "could not check user scope") {
		t.Errorf("expected could-not-check warning, got: %q", out)
	}
}

// TestUninstallSettingsBlock_RemoveError covers the warnf when removing managed
// keys fails (flock blocked by a directory at the lock path).
func TestUninstallSettingsBlock_RemoveError(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	blockSettingsLock(t, home)

	out := captureStderr(t, func() {
		uninstallSettingsBlock(home, "user", provider.MustGet("claude"))
	})
	if !strings.Contains(out, "could not remove user block") {
		t.Errorf("expected could-not-remove warning, got: %q", out)
	}
}

// TestUninstallSettingsBlock_Success covers the success line after removing a
// managed juggernaut block.
func TestUninstallSettingsBlock_Success(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := setupApplyTest(t)
	if err := ExecuteArgs([]string{
		"apply", "--auth=iam", "--region=us-west-2", "--skip-preflight",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out := captureStdout(t, func() {
		uninstallSettingsBlock(home, "user", provider.MustGet("claude"))
	})
	if !strings.Contains(out, "✓ Removed juggernaut block from user claude config") {
		t.Errorf("expected removal confirmation, got: %q", out)
	}
}

// TestRemoveKeychainToken_ErrorWarns covers the warnf when the shared bearer
// token cannot be deleted.
func TestRemoveKeychainToken_ErrorWarns(t *testing.T) {
	defer resetFlags()
	resetFlags()
	home := testutil.NewTestHome(t)
	isolateDoctorKeychain(t)
	// A non-empty directory at the credential path makes the file removal fail.
	credPath := filepath.Join(home, ".claude", "juggernaut-credential")
	if err := safepath.MkdirAll(filepath.Join(credPath, "child")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	out := captureStderr(t, func() {
		removeKeychainToken(home)
	})
	if !strings.Contains(out, "could not remove keychain entry") {
		t.Errorf("expected keychain removal warning, got: %q", out)
	}
}
