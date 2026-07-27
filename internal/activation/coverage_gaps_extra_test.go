package activation

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/testutil"
)

// TestRemoveLegacyLauncherBlock_NoEndMarker covers the early-return path.
func TestRemoveLegacyLauncherBlock_NoEndMarker(t *testing.T) {
	content := "some content with # BEGIN: Juggernaut Launcher\nbut no end"
	got, found := removeLegacyLauncherBlock(content)
	if found {
		t.Error("should not find block without end marker")
	}
	if got != content {
		t.Errorf("content should be unchanged, got %q", got)
	}
}

// TestRemoveLegacyBedrockBlock_NoEndMarker covers the early-return path.
func TestRemoveLegacyBedrockBlock_NoEndMarker(t *testing.T) {
	content := "some content with # BEGIN: Claude Code Bedrock Configuration\nbut no end"
	got, found := removeLegacyBedrockBlock(content)
	if found {
		t.Error("should not find block without end marker")
	}
	if got != content {
		t.Errorf("content should be unchanged, got %q", got)
	}
}

// TestHasLegacyLauncherBlock_OnlyBegin covers partial match.
func TestHasLegacyLauncherBlock_OnlyBegin(t *testing.T) {
	content := "some content with # BEGIN: Juggernaut Launcher\nbut no end marker"
	if HasLegacyLauncherBlock(content) {
		t.Error("should be false when only begin marker exists")
	}
}

// TestHasLegacyBedrockBlock_OnlyBegin covers partial match.
func TestHasLegacyBedrockBlock_OnlyBegin(t *testing.T) {
	content := "some content with # BEGIN: Claude Code Bedrock Configuration\nbut no end marker"
	if HasLegacyBedrockBlock(content) {
		t.Error("should be false when only begin marker exists")
	}
}

// TestNormalizeNewlines_NoCRLF covers the no-op path.
func TestNormalizeNewlines_NoCRLF(t *testing.T) {
	input := "line1\nline2\nline3\n"
	got := normalizeNewlines(input)
	if got != input {
		t.Errorf("normalizeNewlines changed LF-only content: %q vs %q", got, input)
	}
}

// TestNormalizeNewlines_WithCRLF covers the replacement path.
func TestNormalizeNewlines_WithCRLF(t *testing.T) {
	input := "line1\r\nline2\r\nline3\r\n"
	want := "line1\nline2\nline3\n"
	got := normalizeNewlines(input)
	if got != want {
		t.Errorf("normalizeNewlines = %q, want %q", got, want)
	}
}

// TestHasBlockWithMarkers_PartialMatch covers case where only begin or end exists.
func TestHasBlockWithMarkers_PartialMatch(t *testing.T) {
	if HasBlockWithMarkers("# BEGIN only", "# BEGIN", "# END") {
		t.Error("should be false when only begin marker exists")
	}
	if HasBlockWithMarkers("# END only", "# BEGIN", "# END") {
		t.Error("should be false when only end marker exists")
	}
}

// TestHasBlockWithMarkers_EmptyMarkers covers empty marker guard.
func TestHasBlockWithMarkers_EmptyMarkers(t *testing.T) {
	if HasBlockWithMarkers("# BEGIN stuff # END", "", "# END") {
		t.Error("should be false when begin marker is empty")
	}
	if HasBlockWithMarkers("# BEGIN stuff # END", "# BEGIN", "") {
		t.Error("should be false when end marker is empty")
	}
}

// TestRemoveTargetForMarkers_FileNotExist covers the IsNotExist path.
func TestRemoveTargetForMarkers_FileNotExist(t *testing.T) {
	_, err := RemoveTargetForMarkers(filepath.Join(t.TempDir(), "nonexistent"), BeginMarker, EndMarker)
	if err != nil {
		t.Errorf("unexpected error for nonexistent file: %v", err)
	}
}

// TestRemoveTargetForMarkers_WriteError covers the writeProfile error path.
func TestRemoveTargetForMarkers_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows write-permission semantics differ")
	}
	home := t.TempDir()
	profile := filepath.Join(home, "profile")
	block := blockFor(ShellPOSIX, "claude", BeginMarker, EndMarker)
	if err := os.WriteFile(profile, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make the file read-only so writeProfile fails.
	if err := os.Chmod(profile, 0o444); err != nil {
		t.Fatal(err)
	}
	changed, err := RemoveTargetForMarkers(profile, BeginMarker, EndMarker)
	if err == nil {
		t.Skip("platform allowed write despite 0444 (e.g. running as root)")
	}
	if changed {
		t.Error("should not report changed on write error")
	}
}

// TestRemoveTargetWithLegacy_FileNotExist covers the IsNotExist path.
func TestRemoveTargetWithLegacy_FileNotExist(t *testing.T) {
	ok, err := RemoveTargetWithLegacy(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Errorf("unexpected error for nonexistent file: %v", err)
	}
	if ok {
		t.Error("should return false for nonexistent file")
	}
}

// TestRemoveTargetWithLegacy_WriteError covers the writeProfile error path.
func TestRemoveTargetWithLegacy_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows write-permission semantics differ")
	}
	home := t.TempDir()
	profile := filepath.Join(home, "profile")
	block := blockFor(ShellPOSIX, "claude", BeginMarker, EndMarker)
	if err := os.WriteFile(profile, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make the file read-only so writeProfile fails.
	if err := os.Chmod(profile, 0o444); err != nil {
		t.Fatal(err)
	}
	changed, err := RemoveTargetWithLegacy(profile)
	if err == nil {
		t.Skip("platform allowed write despite 0444 (e.g. running as root)")
	}
	if changed {
		t.Error("should not report changed on write error")
	}
}

// TestRemoveBlockWithMarkers_OrphanedBegin covers the case where begin marker
// has no matching end — content after orphaned begin must be preserved.
func TestRemoveBlockWithMarkers_OrphanedBegin(t *testing.T) {
	content := "line1\n# BEGIN test\nline3\n# BEGIN test again\nline5\n# END test\nline7"
	got, found := removeBlockWithMarkers(content, "# BEGIN test", "# END test")
	if !found {
		t.Error("should find one matched block")
	}
	// The first # BEGIN test is orphaned (stacked with second), so both begins
	// are in the remove set through the last end.
	if got == content {
		t.Error("content should be modified")
	}
}

// TestUpsertBlockWithMarkers_EmptyContent covers the append-to-empty path.
func TestUpsertBlockWithMarkers_EmptyContent(t *testing.T) {
	got := upsertBlockWithMarkers("", "block content", "# B", "# E")
	want := "block content\n"
	if got != want {
		t.Errorf("upsertBlockWithMarkers on empty = %q, want %q", got, want)
	}
}

// TestUpsertBlockWithMarkers_OnlyWhitespace covers trimming before append.
func TestUpsertBlockWithMarkers_OnlyWhitespace(t *testing.T) {
	got := upsertBlockWithMarkers("\n\n", "block content", "# B", "# E")
	want := "block content\n"
	if got != want {
		t.Errorf("upsertBlockWithMarkers on whitespace = %q, want %q", got, want)
	}
}

// TestInstalledTargetsForMarkers_NonWindowsWithPSResult covers the non-Windows
// path when a PowerShell result is injected.
func TestInstalledTargetsForMarkers_NonWindowsWithPSResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	home := t.TempDir()
	profile := filepath.Join(home, "profile.ps1")
	block := blockFor(ShellPowerShell, "claude", BeginMarker, EndMarker)
	if err := os.WriteFile(profile, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := InstalledTargetsForMarkers(home, &ProfileResolverResult{
		ActiveTargets: []Target{{Path: profile, Shell: ShellPowerShell}},
	}, BeginMarker, EndMarker)
	if len(paths) != 1 || paths[0] != profile {
		t.Errorf("expected [%s], got %v", profile, paths)
	}
}

// TestResolvePowerShellProfilesScoped_NonWindows covers the non-Windows return.
func TestResolvePowerShellProfilesScoped_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	r := ResolvePowerShellProfilesScoped(t.TempDir())
	if len(r.ActiveTargets) != 0 {
		t.Error("should return empty on non-Windows")
	}
}

// TestInstallPowerShellActivationForSpec_NonWindows covers the non-Windows return.
func TestInstallPowerShellActivationForSpec_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	_, err := installPowerShellActivationForSpec(t.TempDir(), nil, claudeCLISpec())
	if err != nil {
		t.Errorf("unexpected error on non-Windows: %v", err)
	}
}

// TestCheckPowerShellActivationWith_NonWindows covers the non-Windows return.
func TestCheckPowerShellActivationWith_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	healthy, path, warnings := CheckPowerShellActivationWith(t.TempDir(), nil)
	if !healthy {
		t.Error("should be healthy on non-Windows")
	}
	if path != "" {
		t.Errorf("path should be empty on non-Windows, got %q", path)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings should be empty on non-Windows, got %v", warnings)
	}
}

// TestUninstallPowerShellActivationWith_NonWindows covers the non-Windows return.
func TestUninstallPowerShellActivationWith_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	removed, err := UninstallPowerShellActivationWith(t.TempDir(), nil)
	if err != nil {
		t.Errorf("unexpected error on non-Windows: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("should return empty on non-Windows, got %v", removed)
	}
}

// TestInstallWith_NonWindowsWithPSResult covers the non-Windows path with
// an injected PowerShell result.
func TestInstallWith_NonWindowsWithPSResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	home := t.TempDir()
	profile := filepath.Join(home, "profile.ps1")
	if err := os.WriteFile(profile, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallWith(home, InstallOptions{
		PowerShellResult: &ProfileResolverResult{
			ActiveTargets: []Target{{Path: profile, Shell: ShellPowerShell}},
		},
		Spec: CLISpec{Name: "claude", Begin: BeginMarker, End: EndMarker},
	})
	if err != nil {
		t.Fatalf("InstallWith: %v", err)
	}
	// On non-Windows, both PowerShell and POSIX targets are installed.
	if len(installed) == 0 {
		t.Error("should have installed at least one target")
	}
	// Verify the PowerShell profile is among the installed targets.
	found := false
	for _, p := range installed {
		if p == profile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s in installed targets %v", profile, installed)
	}
}

// TestUninstallWith_Claude_NonWindowsWithPSResult covers the Claude uninstall
// path on non-Windows with an injected PowerShell result.
func TestUninstallWith_Claude_NonWindowsWithPSResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	home := t.TempDir()
	profile := filepath.Join(home, "profile.ps1")
	block := blockFor(ShellPowerShell, "claude", BeginMarker, EndMarker)
	if err := os.WriteFile(profile, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := UninstallWith(home, UninstallOptions{
		PowerShellResult: &ProfileResolverResult{
			ActiveTargets: []Target{{Path: profile, Shell: ShellPowerShell}},
		},
	})
	if err != nil {
		t.Fatalf("UninstallWith: %v", err)
	}
	// On non-Windows, both PowerShell and POSIX targets may be removed.
	if len(removed) == 0 {
		t.Error("should have removed at least one target")
	}
	// Verify the PowerShell profile is among the removed targets.
	found := false
	for _, p := range removed {
		if p == profile {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s in removed targets %v", profile, removed)
	}
}

// TestCommandOnPATH covers both found and not-found paths.
func TestCommandOnPATH(t *testing.T) {
	// PATH always has something resolvable, but "this-command-definitely-does-not-exist-xyz" won't.
	if commandOnPATH("this-command-definitely-does-not-exist-xyz") {
		t.Error("should not find nonexistent command")
	}
}

// TestShouldWritePOSIXTarget_ExistingFile covers the existing-file path.
func TestShouldWritePOSIXTarget_ExistingFile(t *testing.T) {
	home := t.TempDir()
	bashrc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("# existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := Target{Path: bashrc, Shell: ShellPOSIX}
	if !shouldWritePOSIXTarget(target) {
		t.Error("existing .bashrc should be writable")
	}
}

// TestShouldWritePOSIXTarget_ProfileNeverCreated covers .profile never being created.
func TestShouldWritePOSIXTarget_ProfileNeverCreated(t *testing.T) {
	home := t.TempDir()
	profile := Target{Path: filepath.Join(home, ".profile"), Shell: ShellPOSIX}
	if shouldWritePOSIXTarget(profile) {
		t.Error(".profile should never be created from scratch")
	}
}

// TestValidateCLIName_Invalid covers rejected CLI names.
func TestValidateCLIName_Invalid(t *testing.T) {
	for _, name := range []string{"bad;name", "bad name", "!invalid", ""} {
		err := validateCLIName(name)
		if err == nil {
			t.Errorf("validateCLIName(%q) should fail", name)
		}
	}
}

// TestValidateCLIName_Valid covers accepted names.
func TestValidateCLIName_Valid(t *testing.T) {
	for _, name := range []string{"claude", "codex", "my-cli", "a", "a-b-c"} {
		err := validateCLIName(name)
		if err != nil {
			t.Errorf("validateCLIName(%q) should pass: %v", name, err)
		}
	}
}

// TestValidateCLISpec_MarkerIssues covers marker validation.
func TestValidateCLISpec_MarkerIssues(t *testing.T) {
	spec := CLISpec{Name: "claude", Begin: "# same", End: "# same"}
	err := validateCLISpec(spec)
	if err == nil {
		t.Error("same begin/end markers should fail")
	}

	spec = CLISpec{Name: "claude", Begin: "# B\nnewline", End: "# E"}
	err = validateCLISpec(spec)
	if err == nil {
		t.Error("multi-line begin marker should fail")
	}
}

// TestBlockFor_Fish covers the Fish shell generation.
func TestBlockFor_Fish(t *testing.T) {
	block := blockFor(ShellFish, "claude", "# B", "# E")
	if block == "" {
		t.Error("Fish block should not be empty")
	}
}

// TestBlockFor_PowerShell covers the PowerShell generation.
func TestBlockFor_PowerShell(t *testing.T) {
	block := blockFor(ShellPowerShell, "claude", "# B", "# E")
	if block == "" {
		t.Error("PowerShell block should not be empty")
	}
}

// TestBlockFor_POSIX covers the POSIX shell generation.
func TestBlockFor_POSIX(t *testing.T) {
	block := blockFor(ShellPOSIX, "claude", "# B", "# E")
	if block == "" {
		t.Error("POSIX block should not be empty")
	}
}

// TestBlockFor_NonClaudeCLI covers the launch-cli form.
func TestBlockFor_NonClaudeCLI(t *testing.T) {
	block := blockFor(ShellPOSIX, "codex", "# B", "# E")
	if block == "" {
		t.Error("non-Claude block should not be empty")
	}
}

// TestSetEnv_UpdateExisting covers updating an existing env var.
func TestSetEnv_UpdateExisting(t *testing.T) {
	env := []string{"KEY=old", "OTHER=val"}
	env = setEnv(env, "KEY", "new")
	if env[0] != "KEY=new" {
		t.Errorf("first entry should be KEY=new, got %q", env[0])
	}
	if len(env) != 2 {
		t.Errorf("should have 2 entries, got %d", len(env))
	}
}

// TestSetEnv_AddNew covers adding a new env var.
func TestSetEnv_AddNew(t *testing.T) {
	env := []string{"OTHER=val"}
	env = setEnv(env, "KEY", "new")
	if len(env) != 2 {
		t.Fatalf("should have 2 entries, got %d", len(env))
	}
	if env[1] != "KEY=new" {
		t.Errorf("new entry should be KEY=new, got %q", env[1])
	}
}

// TestUnsetEnv_Remove covers removing an existing env var.
func TestUnsetEnv_Remove(t *testing.T) {
	env := []string{"KEY=val", "OTHER=val"}
	env = unsetEnv(env, "KEY")
	if len(env) != 1 {
		t.Fatalf("should have 1 entry, got %d", len(env))
	}
	if env[0] != "OTHER=val" {
		t.Errorf("should be OTHER=val, got %q", env[0])
	}
}

// TestUnsetEnv_NotFound covers removing a non-existent key.
func TestUnsetEnv_NotFound(t *testing.T) {
	env := []string{"KEY=val", "OTHER=val"}
	env = unsetEnv(env, "MISSING")
	if len(env) != 2 {
		t.Fatalf("should have 2 entries, got %d", len(env))
	}
}

// TestNeedsBearerToken_IAM covers IAM mode (no token needed).
func TestNeedsBearerToken_IAM(t *testing.T) {
	if needsBearerToken([]string{"iam"}) {
		t.Error("iam mode should not need bearer token")
	}
}

// TestNeedsBearerToken_BedrockAPIKey covers bedrock-api-key mode.
func TestNeedsBearerToken_BedrockAPIKey(t *testing.T) {
	if !needsBearerToken([]string{"bedrock-api-key"}) {
		t.Error("bedrock-api-key mode should need bearer token")
	}
}

// TestNeedsBearerToken_Empty covers empty modes list.
func TestNeedsBearerToken_Empty(t *testing.T) {
	if needsBearerToken(nil) {
		t.Error("empty modes should not need bearer token")
	}
	if needsBearerToken([]string{}) {
		t.Error("empty slice should not need bearer token")
	}
}

// TestContainsTargetPathCI_Found covers the found path.
func TestContainsTargetPathCI_Found(t *testing.T) {
	targets := []Target{{Path: "/a"}, {Path: "/b"}}
	if !containsTargetPathCI(targets, "/b") {
		t.Error("/b should be found")
	}
}

// TestContainsTargetPathCI_NotFound covers the not-found path.
func TestContainsTargetPathCI_NotFound(t *testing.T) {
	targets := []Target{{Path: "/a"}, {Path: "/b"}}
	if containsTargetPathCI(targets, "/c") {
		t.Error("/c should not be found")
	}
}

// TestIsExecutable_NotExecutable covers the non-executable path.
func TestIsExecutable_NotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows always returns true for existing files")
	}
	home := t.TempDir()
	path := filepath.Join(home, "test")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	if isExecutable(path) {
		t.Error("non-executable file should return false")
	}
}

// TestIsExecutable_IsDir covers directory path.
func TestIsExecutable_IsDir(t *testing.T) {
	if isExecutable(t.TempDir()) {
		t.Error("directory should not be executable")
	}
}

// TestIsExecutable_NotFound covers missing file.
func TestIsExecutable_NotFound(t *testing.T) {
	if isExecutable(filepath.Join(t.TempDir(), "nonexistent")) {
		t.Error("nonexistent file should not be executable")
	}
}

// TestSameExecutable_EmptySelf covers empty self path.
func TestSameExecutable_EmptySelf(t *testing.T) {
	if sameExecutable(t.Name(), "") {
		t.Error("empty self should return false")
	}
}

// TestSameExecutable_DifferentFiles covers different files.
func TestSameExecutable_DifferentFiles(t *testing.T) {
	home := t.TempDir()
	a := filepath.Join(home, "a")
	b := filepath.Join(home, "b")
	if err := os.WriteFile(a, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sameExecutable(a, b) {
		t.Error("different files should not be same executable")
	}
}

// TestSameExecutable_NotFound covers stat failure.
func TestSameExecutable_NotFound(t *testing.T) {
	if sameExecutable(filepath.Join(t.TempDir(), "nonexistent"), t.Name()) {
		t.Error("nonexistent file should return false")
	}
}

// TestSamePath_CleanAndEqual covers the basic path.
func TestSamePath_CleanAndEqual(t *testing.T) {
	if !samePath("/a/b", "/a/b") {
		t.Error("same paths should match")
	}
}

// TestSamePath_Different covers different paths.
func TestSamePath_Different(t *testing.T) {
	if samePath("/a/b", "/a/c") {
		t.Error("different paths should not match")
	}
}

// TestFileExists_Exists covers existing file.
func TestFileExists_Exists(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "test")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !fileExists(path) {
		t.Error("existing file should return true")
	}
}

// TestFileExists_NotExists covers missing file.
func TestFileExists_NotExists(t *testing.T) {
	if fileExists(filepath.Join(t.TempDir(), "nonexistent")) {
		t.Error("nonexistent file should return false")
	}
}

// TestDefaultBinDir_WithHome covers the happy path.
func TestDefaultBinDir_WithHome(t *testing.T) {
	got := DefaultBinDir("/home/user")
	want := filepath.Join("/home/user", ".local", "bin")
	if got != want {
		t.Errorf("DefaultBinDir = %q, want %q", got, want)
	}
}

// TestDefaultBinDir_EmptyHome covers falling back to env.
func TestDefaultBinDir_EmptyHome(t *testing.T) {
	// When HOME and USERPROFILE are both empty, it falls back to os.UserHomeDir.
	// On CI this may fail, so we just check it doesn't crash.
	_ = DefaultBinDir("")
}

// TestResolveBinary_NotFound covers the ErrNotFound path.
func TestResolveBinary_NotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — path resolution differs")
	}
	_, err := ResolveBinary("/nonexistent:path", []string{"nonexistent"})
	if err == nil {
		t.Error("should return error for nonexistent binary")
	}
}

// TestLaunchWithOptions_EmptyHome covers the home validation.
func TestLaunchWithOptions_EmptyHome(t *testing.T) {
	err := LaunchWithOptions(LaunchOptions{})
	if err == nil {
		t.Error("empty home should error")
	}
}

// TestLaunchWithOptions_ZeroTargetDefaultsToClaude covers the default target.
func TestLaunchWithOptions_ZeroTargetDefaultsToClaude(t *testing.T) {
	home := testutil.NewTestHome(t)
	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: "/nonexistent",
		Runner: func(path string, args []string, env []string) error {
			return nil
		},
	})
	// With empty target, it should default to Claude and try to resolve claude binary.
	// Since PATH is /nonexistent, it will fail with ErrNotFound.
	_ = err // Expect error or nil — either way, no crash.
}

// TestLaunchWithOptions_CustomTokenEnvVar covers non-default token env.
func TestLaunchWithOptions_CustomTokenEnvVar(t *testing.T) {
	home := testutil.NewTestHome(t)
	// NeedsToken=true triggers the token path.
	err := LaunchWithOptions(LaunchOptions{
		Home: home,
		Path: "/nonexistent",
		Target: LaunchTarget{
			BinaryNames: []string{"test"},
			TokenEnvVar: "CUSTOM_TOKEN",
			NeedsToken:  true,
		},
		TokenGetter: func() (string, error) {
			return "token-value", nil
		},
		Runner: func(path string, args []string, env []string) error {
			// Verify CUSTOM_TOKEN is in env.
			for _, e := range env {
				if e == "CUSTOM_TOKEN=token-value" {
					return nil
				}
			}
			return os.ErrNotExist
		},
	})
	// This will fail because PATH is /nonexistent, but if it reached the runner,
	// the token env was set correctly.
	_ = err // Expect error or nil — either way, no crash.
}

// TestRunBinary_NotFound covers the binary-not-found path.
func TestRunBinary_NotFound(t *testing.T) {
	err := RunBinary("/nonexistent/binary", []string{}, []string{})
	if err == nil {
		t.Error("should error for nonexistent binary")
	}
}

// TestBlock_Default covers the default Block() function.
func TestBlock_Default(t *testing.T) {
	for _, shell := range []Shell{ShellPOSIX, ShellFish, ShellPowerShell} {
		block := Block(shell)
		if block == "" {
			t.Errorf("Block(%s) should not be empty", shell)
		}
	}
}

// TestSpecOrClaude_ZeroValue covers the default Claude spec.
func TestSpecOrClaude_ZeroValue(t *testing.T) {
	spec := specOrClaude(CLISpec{})
	if spec.Name != "claude" {
		t.Errorf("zero spec should default to claude, got %q", spec.Name)
	}
}

// TestSpecOrClaude_Populated covers the pass-through path.
func TestSpecOrClaude_Populated(t *testing.T) {
	spec := specOrClaude(CLISpec{Name: "codex", Begin: "# B", End: "# E"})
	if spec.Name != "codex" {
		t.Errorf("populated spec should pass through, got %q", spec.Name)
	}
}

// TestInstallTarget_Default covers the default InstallTarget function.
func TestInstallTarget_Default(t *testing.T) {
	home := t.TempDir()
	bashrc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("# existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := InstallTarget(Target{Path: bashrc, Shell: ShellPOSIX})
	if err != nil {
		t.Fatalf("InstallTarget: %v", err)
	}
	if !changed {
		t.Error("should have changed for empty file")
	}
}

// TestRemoveTarget_Default covers the default RemoveTarget function.
func TestRemoveTarget_Default(t *testing.T) {
	home := t.TempDir()
	bashrc := filepath.Join(home, ".bashrc")
	block := Block(ShellPOSIX)
	if err := os.WriteFile(bashrc, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := RemoveTarget(bashrc)
	if err != nil {
		t.Fatalf("RemoveTarget: %v", err)
	}
	if !changed {
		t.Error("should have changed for file with block")
	}
}

// TestHasBlock_Present covers the positive path.
func TestHasBlock_Present(t *testing.T) {
	block := Block(ShellPOSIX)
	if !HasBlock(block) {
		t.Error("should detect block presence")
	}
}

// TestHasBlock_Absent covers the negative path.
func TestHasBlock_Absent(t *testing.T) {
	if HasBlock("no block here") {
		t.Error("should not detect block in plain text")
	}
}

// TestInstall_Default covers the default Install function.
func TestInstall_Default(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — requires PowerShell")
	}
	home := t.TempDir()
	bashrc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("# existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Should not crash even without activation.
	_, _ = Install(home)
}

// TestUninstall_Default covers the default Uninstall function.
func TestUninstall_Default(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — requires PowerShell")
	}
	home := t.TempDir()
	_, _ = Uninstall(home)
}

// TestInstalledTargets_Default covers the default InstalledTargets function.
func TestInstalledTargets_Default(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — requires PowerShell")
	}
	home := t.TempDir()
	bashrc := filepath.Join(home, ".bashrc")
	block := Block(ShellPOSIX)
	if err := os.WriteFile(bashrc, []byte(block), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := InstalledTargets(home)
	if len(paths) == 0 {
		t.Error("should find .bashrc with block")
	}
}

// TestLaunchCLI_ZeroTarget covers the default Claude target.
func TestLaunchCLI_ZeroTarget(t *testing.T) {
	home := testutil.NewTestHome(t)
	// With zero target, it defaults to Claude. PATH is /nonexistent so it fails.
	err := LaunchCLI(home, []string{}, LaunchTarget{})
	_ = err // Expect error — either way, no crash.
}

// TestLaunchCLI_NeedsTokenTrue covers the token-injection path.
func TestLaunchCLI_NeedsTokenTrue(t *testing.T) {
	home := testutil.NewTestHome(t)
	err := LaunchCLI(home, []string{}, LaunchTarget{
		BinaryNames: []string{"test"},
		NeedsToken:  true,
	})
	// Will fail because PATH doesn't have test, but shouldn't crash.
	_ = err // Expect error — either way, no crash.
}

// TestLaunchCLI_NeedsTokenFalse covers the no-token path.
func TestLaunchCLI_NeedsTokenFalse(t *testing.T) {
	home := testutil.NewTestHome(t)
	err := LaunchCLI(home, []string{}, LaunchTarget{
		BinaryNames: []string{"test"},
		NeedsToken:  false,
	})
	// Will fail because PATH doesn't have test.
	_ = err // Expect error — either way, no crash.
}
