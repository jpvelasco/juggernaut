// Package activation manages shell activation blocks and Claude Code launch.
package activation

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

// shellCLINameRE is the only identifier shape we embed into generated shell
// profiles (function names, `command X`, Get-Command X). Anything else is
// rejected so a bad CLISpec.Name cannot become shell injection.
var shellCLINameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

const (
	BeginMarker = "# BEGIN: Juggernaut Claude Activation"
	EndMarker   = "# END: Juggernaut Claude Activation"

	// Legacy markers from older Juggernaut versions.
	LegacyLauncherBegin = "# BEGIN: Juggernaut Launcher"
	LegacyLauncherEnd   = "# END: Juggernaut Launcher"
	LegacyBedrockBegin  = "# BEGIN: Claude Code Bedrock Configuration"
	LegacyBedrockEnd    = "# END: Claude Code Bedrock Configuration"

	// Legacy v4.2.6 shim content for exact matching after normalization.
	legacyCmdShimLF   = "@echo off\njuggernaut --launcher %*\n"
	legacyCmdShimCRLF = "@echo off\r\njuggernaut --launcher %*\r\n"
)

// LegacyAction describes one v4.2.6 recovery action.
type LegacyAction struct {
	Path   string
	Action string
}

// Shell identifies the shell syntax used by an activation target.
type Shell string

const (
	ShellPOSIX      Shell = "posix"
	ShellFish       Shell = "fish"
	ShellPowerShell Shell = "powershell"
)

// Target describes one shell profile managed by Juggernaut.
type Target struct {
	Path  string
	Shell Shell
}

// ProfileResolverResult is the shared, authoritative result of PowerShell
// profile discovery. On Windows it contains dynamically discovered profiles.
// On other platforms it returns the same targets as DefaultTargets.
type ProfileResolverResult struct {
	// ActiveTargets are the discovered profiles that PowerShell actually loads.
	// This includes both AllHosts and CurrentHost profiles and is used for
	// health checks and activation scanning.
	ActiveTargets []Target
	// InstallTargets are the profiles that should receive the activation
	// block. This typically contains only AllHosts profiles — CurrentHost
	// profiles load after AllHosts and can override or retain a stale
	// duplicate of the global activation.
	InstallTargets []Target
	// MigrationTargets are all discovered profiles that should be inspected
	// for legacy blocks during apply/migration.
	MigrationTargets []string
	// DiscoveryWarnings lists warnings about the discovery process.
	DiscoveryWarnings []string
	// UsedFallback is true when Known Documents fallback was used.
	UsedFallback bool
	// EditionsDiscovered lists which PowerShell editions were found.
	EditionsDiscovered []string
}

// LaunchOptions carries injectable dependencies for tests.
type LaunchOptions struct {
	Home        string
	Args        []string
	Path        string
	TokenGetter func() (string, error)
	Runner      func(path string, args []string, env []string) error
	// Warner receives non-fatal warnings (e.g. an expired short-term API key).
	// Defaults to printing to stderr.
	Warner func(msg string)

	// Target describes the CLI being launched (binary names + how to route it to
	// Bedrock). A zero Target defaults to Claude Code, so existing callers are
	// unaffected. cmd/ populates it from the resolved Provider's LaunchSpec.
	Target LaunchTarget

	// SelfPaths are additional executable paths that resolveBinary should skip
	// alongside os.Executable(). This is needed on Windows when the npm shim
	// stages the binary to a temp copy: os.Executable() returns the temp path,
	// but PATH candidates hardlinked to the original installed binary must also
	// be skipped to prevent recursive self-invocation.
	SelfPaths []string
}

// LaunchTarget carries the per-CLI launch configuration, mirroring
// provider.LaunchSpec (the activation package stays provider-free; cmd/ maps
// LaunchSpec → LaunchTarget).
type LaunchTarget struct {
	BinaryNames []string          // real exe names to resolve on PATH
	TokenEnvVar string            // env var to inject the bearer token into
	StaticEnv   map[string]string // static enable-flags (Claude: CLAUDE_CODE_USE_BEDROCK=1)
	NeedsToken  bool              // whether a bearer token is required
	// ConfigPath is the provider's config file (e.g. ~/.codex/config.toml).
	// If set, the launch reads the auth mode from the juggernaut block in that
	// file to decide whether to inject the bearer token. Falls back to NeedsToken
	// when the block is absent or has no auth mode.
	ConfigPath string
}

// claudeLaunchTarget is the default Target (back-compat with the historical
// hardcoded Claude launch behavior).
func claudeLaunchTarget() LaunchTarget {
	names := []string{"claude"}
	if runtime.GOOS == "windows" {
		names = []string{"claude.exe", "claude.cmd", "claude.bat"}
	}
	return LaunchTarget{
		BinaryNames: names,
		TokenEnvVar: authmode.BedrockAuthEnvName,
		StaticEnv:   map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"},
		NeedsToken:  false, // determined by auth mode, not the target
	}
}

// DefaultTargets returns the shell profile targets Juggernaut updates.
// PowerShell profiles are NOT included here — they are discovered dynamically
// via ResolvePowerShellProfiles() on Windows, and via the ProfileResolverResult
// on other platforms.
func DefaultTargets(home string) []Target {
	return []Target{
		{Path: filepath.Join(home, ".bashrc"), Shell: ShellPOSIX},
		{Path: filepath.Join(home, ".zshrc"), Shell: ShellPOSIX},
		{Path: filepath.Join(home, ".profile"), Shell: ShellPOSIX},
		{Path: filepath.Join(home, ".config", "fish", "config.fish"), Shell: ShellFish},
	}
}

// statProfile is os.Stat by default; tests replace it to exercise non-NotExist
// Stat failures (Windows maps many path errors to IsNotExist).
var statProfile = os.Stat

// shouldWritePOSIXTarget reports whether install should create/update a POSIX
// or Fish profile. Existing files are always eligible (so re-apply stays
// idempotent). Missing files are only created when the matching shell is on
// PATH — never invent a .zshrc/fish config on a machine that only uses
// PowerShell/Git Bash. Bare ~/.profile is never created from scratch (login
// shells already source .bashrc via .bash_profile on Git for Windows).
//
// Non-NotExist Stat errors (permission, I/O) return true so InstallTargetFor
// surfaces the real error instead of silently skipping a profile that may
// still hold a dead wrapper.
func shouldWritePOSIXTarget(target Target) bool {
	_, err := statProfile(target.Path)
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		return true
	}
	base := filepath.Base(target.Path)
	switch {
	case base == ".bashrc":
		return commandOnPATH("bash")
	case base == ".zshrc":
		return commandOnPATH("zsh")
	case base == "config.fish" || target.Shell == ShellFish:
		return commandOnPATH("fish")
	case base == ".profile":
		return false
	default:
		return false
	}
}

// commandOnPATH reports whether name resolves via exec.LookPath.
func commandOnPATH(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Block returns the Claude activation block. Retained for back-compat; delegates
// to the per-CLI generator with Claude's identity.
func Block(shell Shell) string {
	return blockFor(shell, "claude", BeginMarker, EndMarker)
}

// validateCLISpec rejects names/markers that must not be interpolated into
// shell profiles. CLI names become function identifiers and command tokens;
// markers must stay single-line so block matching cannot be confused.
func validateCLISpec(spec CLISpec) error {
	if err := validateCLIName(spec.Name); err != nil {
		return err
	}
	return validateMarkers(spec.Begin, spec.End)
}

func validateCLIName(cli string) error {
	if !shellCLINameRE.MatchString(cli) {
		return fmt.Errorf("invalid CLI name %q for shell activation (must match %s)", cli, shellCLINameRE.String())
	}
	return nil
}

func validateMarkers(begin, end string) error {
	if begin == "" || end == "" {
		return errors.New("activation markers must be non-empty")
	}
	if begin == end {
		return errors.New("activation begin and end markers must differ")
	}
	for _, m := range []string{begin, end} {
		if strings.ContainsAny(m, "\n\r\x00") {
			return errors.New("activation markers must be single-line")
		}
		if len(m) > 200 {
			return errors.New("activation markers are too long")
		}
	}
	return nil
}

// blockFor generates a shell activation block that defines a function named
// `cli`. Claude uses the historical `juggernaut launch` command; other CLIs
// use `launch-cli <cli>` so a pre-multi-CLI binary fails fast instead of
// silently launching Claude.
//
// Every wrapper falls through to the real CLI binary when `juggernaut` is not
// on PATH. That way an incomplete uninstall (or a PATH without juggernaut)
// does not break `claude`/`codex`/`grok` with "term not recognized".
//
// Callers must pass a name that satisfies validateCLIName. InstallTargetFor
// enforces this; tests use fixed provider names (claude/codex/…).
func blockFor(shell Shell, cli, begin, end string) string {
	// Defense in depth: never interpolate an unvalidated identifier into a
	// profile that will be eval'd by the user's shell.
	if err := validateCLIName(cli); err != nil {
		panic(err)
	}
	if err := validateMarkers(begin, end); err != nil {
		panic(err)
	}

	launchCommand := "juggernaut launch"
	if cli != "claude" {
		launchCommand = "juggernaut launch-cli " + cli
	}
	switch shell {
	case ShellFish:
		return strings.Join([]string{
			begin,
			"function " + cli,
			"    if command -q juggernaut",
			"        " + launchCommand + " -- $argv",
			"    else",
			"        command " + cli + " $argv",
			"    end",
			"end",
			end,
		}, "\n")
	case ShellPowerShell:
		// ApplicationInfo.Path is the executable path. Source is module/command
		// metadata and is not reliable for Application fallbacks — use Path.
		return strings.Join([]string{
			begin,
			"function global:" + cli + " {",
			"  if (Get-Command juggernaut -ErrorAction SilentlyContinue) {",
			"    " + launchCommand + " -- @args",
			"  } else {",
			"    $app = Get-Command " + cli + " -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1",
			"    if ($null -ne $app -and -not [string]::IsNullOrWhiteSpace($app.Path)) { & $app.Path @args } else {",
			"      throw \"juggernaut is not installed and no '" + cli + "' executable was found on PATH\"",
			"    }",
			"  }",
			"}",
			end,
		}, "\n")
	default:
		return strings.Join([]string{
			begin,
			cli + "() {",
			"  if command -v juggernaut >/dev/null 2>&1; then",
			"    " + launchCommand + " -- \"$@\"",
			"  else",
			"    command " + cli + " \"$@\"",
			"  fi",
			"}",
			end,
		}, "\n")
	}
}

// InstallOptions carries optional dependencies for Install.
type InstallOptions struct {
	// PowerShellResult, when set, is used instead of resolving profiles
	// dynamically. This is required for tests to avoid touching real profiles.
	PowerShellResult *ProfileResolverResult
	// Spec selects which CLI's activation block to install. Zero value defaults
	// to Claude (back-compat).
	Spec CLISpec
}

// specOrClaude returns s if populated, else the Claude default.
func specOrClaude(s CLISpec) CLISpec {
	if s.Name == "" {
		return claudeCLISpec()
	}
	return s
}

// Install writes or updates Juggernaut activation blocks in shell profiles.
// On Windows, it uses dynamic PowerShell profile discovery, then installs
// to POSIX targets as well. On other platforms it uses the default target list.
func Install(home string) ([]string, error) {
	return InstallWith(home, InstallOptions{})
}

// InstallWith is like Install but accepts injectable dependencies.
func InstallWith(home string, opts InstallOptions) ([]string, error) {
	var installed []string
	spec := specOrClaude(opts.Spec)

	if runtime.GOOS == "windows" {
		psInstalled, err := installPowerShellActivationForSpec(home, opts.PowerShellResult, spec)
		if err != nil {
			return installed, err
		}
		installed = append(installed, psInstalled...)
	} else if opts.PowerShellResult != nil {
		// On non-Windows, use the injected PowerShell result for profile paths.
		for _, target := range opts.PowerShellResult.ActiveTargets {
			changed, err := InstallTargetFor(target, spec)
			if err != nil {
				return installed, err
			}
			if changed {
				installed = append(installed, target.Path)
			}
		}
	}

	for _, target := range DefaultTargets(home) {
		if !shouldWritePOSIXTarget(target) {
			continue
		}
		changed, err := InstallTargetFor(target, spec)
		if err != nil {
			return installed, err
		}
		if changed {
			installed = append(installed, target.Path)
		}
	}
	return installed, nil
}

// CLISpec identifies a CLI's activation block: its shell-function name and the
// begin/end markers delimiting its block. cmd/ builds this from a Provider.
type CLISpec struct {
	Name  string
	Begin string
	End   string
}

// claudeCLISpec is the default (back-compat).
func claudeCLISpec() CLISpec {
	return CLISpec{Name: "claude", Begin: BeginMarker, End: EndMarker}
}

// InstallTarget writes or updates the Claude activation block for one profile.
func InstallTarget(target Target) (bool, error) {
	return InstallTargetFor(target, claudeCLISpec())
}

// InstallTargetFor writes or updates one CLI's activation block in a profile,
// leaving other CLIs' blocks (matched by their own markers) untouched so they
// coexist.
func InstallTargetFor(target Target, spec CLISpec) (bool, error) {
	if err := validateCLISpec(spec); err != nil {
		return false, err
	}
	data, notFound, err := readProfile(target.Path)
	if notFound {
		data = nil
	} else if err != nil {
		return false, err
	}
	block := blockFor(target.Shell, spec.Name, spec.Begin, spec.End)
	next := upsertBlockWithMarkers(string(data), block, spec.Begin, spec.End)
	if string(data) == next {
		return false, nil
	}
	if err := writeProfile(target.Path, []byte(next)); err != nil {
		return false, err
	}
	return true, nil
}

// UninstallOptions carries optional dependencies for Uninstall.
type UninstallOptions struct {
	// PowerShellResult, when set, is used instead of resolving profiles
	// dynamically. This is required for tests to avoid touching real profiles.
	PowerShellResult *ProfileResolverResult
	// Spec selects which CLI's activation block to remove. Zero value defaults
	// to Claude (which also sweeps legacy launcher/bedrock blocks).
	Spec CLISpec
}

// Uninstall removes Juggernaut activation blocks from shell profiles.
// On Windows, it also removes blocks from discovered PowerShell profiles.
func Uninstall(home string) ([]string, error) {
	return UninstallWith(home, UninstallOptions{})
}

// UninstallWith is like Uninstall but accepts injectable dependencies.
func UninstallWith(home string, opts UninstallOptions) ([]string, error) {
	// Non-Claude specs remove only that CLI's marker-delimited block; the legacy
	// launcher/bedrock sweep is Claude-only (those blocks never existed for other
	// CLIs), so it stays on the default path below.
	if opts.Spec.Name != "" && opts.Spec.Name != "claude" {
		return uninstallCLIBlocks(home, opts)
	}

	var removed []string

	if runtime.GOOS == "windows" {
		psRemoved, err := UninstallPowerShellActivationWith(home, opts.PowerShellResult)
		if err != nil {
			return removed, err
		}
		removed = append(removed, psRemoved...)
	} else if opts.PowerShellResult != nil {
		// On non-Windows, use the injected PowerShell result for profile paths.
		for _, target := range opts.PowerShellResult.ActiveTargets {
			ok, err := RemoveTarget(target.Path)
			if err != nil {
				return removed, err
			}
			if ok {
				removed = append(removed, target.Path)
			}
		}
	}

	for _, target := range DefaultTargets(home) {
		ok, err := RemoveTargetWithLegacy(target.Path)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, target.Path)
		}
	}
	return removed, nil
}

// uninstallCLIBlocks removes one non-Claude CLI's activation block (by its
// markers) from all managed profiles, leaving other CLIs' blocks intact.
// Scans ActiveTargets, MigrationTargets (historical OneDrive/Documents paths),
// and POSIX defaults so host-specific or redirected leftovers are cleaned.
func uninstallCLIBlocks(home string, opts UninstallOptions) ([]string, error) {
	var removed []string
	begin, end := opts.Spec.Begin, opts.Spec.End

	targets := DefaultTargets(home)
	var migration []string
	if opts.PowerShellResult != nil {
		targets = append(targets, opts.PowerShellResult.ActiveTargets...)
		migration = opts.PowerShellResult.MigrationTargets
	} else if runtime.GOOS == "windows" {
		r := ResolvePowerShellProfilesScoped(home)
		targets = append(targets, r.ActiveTargets...)
		migration = r.MigrationTargets
	}
	for _, path := range migration {
		targets = append(targets, Target{Path: path, Shell: ShellPowerShell})
	}

	seen := map[string]bool{}
	for _, target := range targets {
		key := pathKey(target.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		ok, err := RemoveTargetForMarkers(target.Path, begin, end)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, target.Path)
		}
	}
	return removed, nil
}

// readProfile reads a shell profile file. Returns nil data and a notFound flag
// when the file does not exist (so callers can decide to create or skip).
func readProfile(path string) (data []byte, notFound bool, err error) {
	base := filepath.Dir(path)
	data, err = safepath.ReadFile(base, path)
	if os.IsNotExist(err) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", path, err)
	}
	return data, false, nil
}

// writeProfile atomically writes the updated content to the profile path.
func writeProfile(path string, content []byte) error {
	base := filepath.Dir(path)
	if err := safepath.WriteFile(base, path, content); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// RemoveTarget removes the Claude activation block from a profile.
func RemoveTarget(path string) (bool, error) {
	return RemoveTargetForMarkers(path, BeginMarker, EndMarker)
}

// RemoveTargetForMarkers removes the block delimited by the given markers from a
// profile, leaving other CLIs' blocks untouched (uses the matched-span remover
// so an orphaned marker never deletes trailing content).
func RemoveTargetForMarkers(path, begin, end string) (bool, error) {
	data, notFound, err := readProfile(path)
	if notFound || err != nil {
		return false, err
	}
	next, found := removeBlockWithMarkers(normalizeNewlines(string(data)), begin, end)
	if !found {
		return false, nil
	}
	if err := writeProfile(path, []byte(next)); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveTargetWithLegacy removes the activation block AND any legacy launcher
// or Bedrock blocks from a profile. This is used by full uninstall to ensure
// no Juggernaut-related blocks remain.
func RemoveTargetWithLegacy(path string) (bool, error) {
	data, notFound, err := readProfile(path)
	if notFound || err != nil {
		return false, err
	}
	content := string(data)
	changed := false

	// Remove current activation block.
	next, found := removeBlock(content)
	if found {
		content = next
		changed = true
	}

	// Remove legacy launcher block.
	next, found = removeLegacyLauncherBlock(content)
	if found {
		content = next
		changed = true
	}

	// Remove legacy Bedrock block.
	next, found = removeLegacyBedrockBlock(content)
	if found {
		content = next
		changed = true
	}

	if !changed {
		return false, nil
	}
	if err := writeProfile(path, []byte(content)); err != nil {
		return false, err
	}
	return true, nil
}

// InstalledTargets returns profile paths currently containing the Claude activation block.
// On Windows it checks discovered active targets and POSIX targets.
func InstalledTargets(home string) []string {
	return InstalledTargetsWith(home, nil)
}

// InstalledTargetsWith is like InstalledTargets but accepts a pre-resolved
// ProfileResolverResult to avoid launching PowerShell.
func InstalledTargetsWith(home string, psResult *ProfileResolverResult) []string {
	return InstalledTargetsForMarkers(home, psResult, BeginMarker, EndMarker)
}

// InstalledTargetsForMarkers returns profile paths containing the given
// begin/end activation markers (any managed CLI).
func InstalledTargetsForMarkers(home string, psResult *ProfileResolverResult, begin, end string) []string {
	var paths []string

	if runtime.GOOS == "windows" {
		result := psResult
		if result == nil {
			r := ResolvePowerShellProfilesScoped(home)
			result = &r
		}
		for _, target := range result.ActiveTargets {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err == nil && HasBlockWithMarkers(string(data), begin, end) {
				paths = append(paths, target.Path)
			}
		}
	} else if psResult != nil {
		for _, target := range psResult.ActiveTargets {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err == nil && HasBlockWithMarkers(string(data), begin, end) {
				paths = append(paths, target.Path)
			}
		}
	}

	for _, target := range DefaultTargets(home) {
		data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
		if err == nil && HasBlockWithMarkers(string(data), begin, end) {
			paths = append(paths, target.Path)
		}
	}
	return paths
}

// HasBlock reports whether content contains a Juggernaut activation block.
func HasBlock(content string) bool {
	return HasBlockWithMarkers(content, BeginMarker, EndMarker)
}

// HasBlockWithMarkers reports whether content contains the given begin/end
// activation markers (used for multi-CLI doctor/activation checks).
func HasBlockWithMarkers(content, begin, end string) bool {
	if begin == "" || end == "" {
		return false
	}
	return strings.Contains(content, begin) && strings.Contains(content, end)
}

// HasLegacyBlock reports whether content contains both begin and end markers.
func HasLegacyBlock(content, begin, end string) bool {
	return strings.Contains(content, begin) && strings.Contains(content, end)
}

// HasLegacyLauncherBlock reports whether content contains a legacy
// "# BEGIN: Juggernaut Launcher" block.
func HasLegacyLauncherBlock(content string) bool {
	return HasLegacyBlock(content, LegacyLauncherBegin, LegacyLauncherEnd)
}

// HasLegacyBedrockBlock reports whether content contains a legacy
// "# BEGIN: Claude Code Bedrock Configuration" block.
func HasLegacyBedrockBlock(content string) bool {
	return HasLegacyBlock(content, LegacyBedrockBegin, LegacyBedrockEnd)
}

// removeLegacyBlock removes a legacy block from content. It only removes the
// block if both the begin and end markers are present; an orphaned begin marker
// is left untouched.
func removeLegacyBlock(content, begin, end string) (string, bool) {
	content = normalizeNewlines(content)
	if !strings.Contains(content, end) {
		return content, false
	}
	return removeBlockWithMarkers(content, begin, end)
}

// removeLegacyLauncherBlock removes a legacy "# BEGIN: Juggernaut Launcher"
// block from content.
func removeLegacyLauncherBlock(content string) (string, bool) {
	return removeLegacyBlock(content, LegacyLauncherBegin, LegacyLauncherEnd)
}

// removeLegacyBedrockBlock removes a legacy "# BEGIN: Claude Code Bedrock
// Configuration" block from content.
func removeLegacyBedrockBlock(content string) (string, bool) {
	return removeLegacyBlock(content, LegacyBedrockBegin, LegacyBedrockEnd)
}

// removeBlockWithMarkers removes blocks delimited by begin and end markers.
// It only removes spans whose begin marker has a matching following end marker;
// orphaned begin markers are left untouched along with all subsequent content.
func removeBlockWithMarkers(content, begin, end string) (string, bool) {
	lines := strings.Split(content, "\n")

	// First pass: find all valid begin/end span pairs (indices to remove).
	// Use a stack so nested or multiple blocks are handled correctly.
	var stack [][]int // each entry: [beginIndex, endIndex]
	beginStack := []int{}
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case begin:
			beginStack = append(beginStack, i)
		case end:
			if len(beginStack) > 0 {
				bi := beginStack[len(beginStack)-1]
				beginStack = beginStack[:len(beginStack)-1]
				stack = append(stack, []int{bi, i})
			}
		}
	}

	// Build a set of line indices to remove.
	remove := make(map[int]bool, len(lines))
	for _, span := range stack {
		for i := span[0]; i <= span[1]; i++ {
			remove[i] = true
		}
	}

	// Second pass: emit lines not in the remove set.
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if !remove[i] {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), len(stack) > 0
}

// Launch runs Claude Code with Juggernaut Bedrock activation.
func Launch(home string, args []string) error {
	return LaunchCLI(home, args, LaunchTarget{})
}

// LaunchCLI runs the CLI described by target (a zero target defaults to Claude),
// injecting Bedrock env from the keychain. It is the target-aware entry point
// used by the hidden launch commands.
func LaunchCLI(home string, args []string, target LaunchTarget) error {
	return LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        args,
		Path:        os.Getenv("PATH"),
		TokenGetter: func() (string, error) { return keychain.Default().GetWithFallback(home) },
		Runner:      RunBinary,
		Target:      target,
	})
}

// LaunchWithOptions runs Claude Code with injectable dependencies.
func LaunchWithOptions(opts LaunchOptions) error {
	if opts.Home == "" {
		return errors.New("home directory is required")
	}
	if opts.Path == "" {
		opts.Path = os.Getenv("PATH")
	}
	if opts.TokenGetter == nil {
		opts.TokenGetter = func() (string, error) { return keychain.Default().GetWithFallback(opts.Home) }
	}
	if opts.Runner == nil {
		opts.Runner = RunBinary
	}
	if opts.Warner == nil {
		opts.Warner = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	}
	target := opts.Target
	if len(target.BinaryNames) == 0 {
		target = claudeLaunchTarget() // default: Claude (back-compat)
	}
	tokenEnvVar := target.TokenEnvVar
	if tokenEnvVar == "" {
		tokenEnvVar = authmode.BedrockAuthEnvName
	}

	env := os.Environ()
	modes, err := authModes(opts.Home)
	if err != nil {
		return err
	}

	// Determine whether this launch is Juggernaut-managed and whether it needs a
	// bearer token. Claude declares its auth mode in ~/.claude/settings.json
	// (scanned by authModes). Non-Claude CLIs (Codex) store the auth mode in their
	// own config file's juggernaut block — read it from ConfigPath if set.
	// The bearer token is SHARED, so injecting it for a token-needing target
	// is correct regardless of authModes.
	managed := len(modes) > 0 || target.NeedsToken
	wantToken := needsBearerToken(modes) || target.NeedsToken

	// If the target has a config path (non-Claude provider), read the auth mode
	// from its juggernaut block to decide token injection.
	if target.ConfigPath != "" {
		if mode := readAuthModeFromConfig(target.ConfigPath); mode != "" {
			managed = true
			wantToken = authmode.IsBedrockAPIKey(mode)
		}
	}

	if managed {
		for k, v := range target.StaticEnv {
			env = setEnv(env, k, v)
		}
	}
	if wantToken {
		token, err := opts.TokenGetter()
		if err != nil {
			return fmt.Errorf("reading Bedrock API key from keychain: %w", err)
		}
		if token == "" {
			return fmt.Errorf("bedrock API key not found in keychain; run `juggernaut apply --auth=%s`", authmode.BedrockAPIKey)
		}
		// Warn (non-fatally) if a short-term key has already expired — the token
		// is still injected, but Bedrock will reject it, so surface why up front.
		// Juggernaut can't refresh short-term keys (it holds no AWS creds).
		if bedrock.IsAPIKeyExpired(token, time.Now().UTC()) {
			opts.Warner("Warning: your short-term Bedrock API key has expired; regenerate it and run " +
				"`juggernaut apply --auth=" + authmode.BedrockAPIKey + "` (the CLI will fail to authenticate until then)")
		}
		env = setEnv(env, tokenEnvVar, token)
	} else if managed {
		env = unsetEnv(env, tokenEnvVar)
	}

	self, _ := os.Executable()
	binPath, err := resolveBinaryFrom(opts.Path, target.BinaryNames, self, opts.SelfPaths)
	if err != nil {
		return fmt.Errorf("real %s binary not found on PATH", target.BinaryNames[0])
	}
	return opts.Runner(binPath, opts.Args, env)
}

// ResolveClaudeBinary finds the real Anthropic claude command while avoiding
// Juggernaut recursion through old shims or symlinks.
func ResolveClaudeBinary(pathList string) (string, error) {
	self, _ := os.Executable()
	return resolveClaudeBinary(pathList, self)
}

// ResolveBinary resolves the first real executable among names on PATH,
// skipping this process's own executable (wrapper recursion guard).
func ResolveBinary(pathList string, names []string) (string, error) {
	self, _ := os.Executable()
	return resolveBinaryFrom(pathList, names, self, nil)
}

// DefaultBinDir returns the user-local bin directory where broken v4.2.6
// artifacts may exist.
func DefaultBinDir(home string) string {
	if home == "" {
		home = safepath.HomeDirOrEmpty()
	}
	path, err := safepath.JoinUnder(home, ".local", "bin")
	if err != nil {
		return filepath.Join(home, ".local", "bin")
	}
	return path
}

// RecoverLegacyArtifacts removes or restores only positively identified
// v4.2.6 launcher artifacts.
func RecoverLegacyArtifacts(binDir string) ([]LegacyAction, error) {
	self, _ := os.Executable()
	actions, err := recoverPlatformArtifacts(binDir, self, platformNames())
	if err != nil {
		return actions, err
	}
	return actions, nil
}

// DetectLegacyArtifacts reports recoverable or removable v4.2.6 artifacts.
func DetectLegacyArtifacts(binDir string) []LegacyAction {
	self, _ := os.Executable()
	return detectPlatformArtifacts(binDir, self, platformNames())
}

func upsertBlock(content, block string) string {
	return upsertBlockWithMarkers(content, block, BeginMarker, EndMarker)
}

// upsertBlockWithMarkers replaces the block delimited by begin/end (if present)
// with the given block, or appends it. Only the block for THESE markers is
// touched, so different CLIs' blocks (Claude, Codex, …) coexist in one profile.
func upsertBlockWithMarkers(content, block, begin, end string) string {
	content = normalizeNewlines(content)
	without, _ := removeBlockWithMarkers(content, begin, end)
	without = strings.TrimRight(without, "\n")
	if without == "" {
		return block + "\n"
	}
	return without + "\n\n" + block + "\n"
}

func removeBlock(content string) (string, bool) {
	// Delegate to the shared matched-span remover so an orphaned begin marker
	// (a BEGIN with no following END) leaves all subsequent content untouched
	// instead of silently deleting it.
	return removeBlockWithMarkers(normalizeNewlines(content), BeginMarker, EndMarker)
}

// ResolvePowerShellProfiles returns the shared profile resolver result.
// On Windows it dynamically queries PowerShell. On other platforms it
// returns the default targets.
func ResolvePowerShellProfiles() ProfileResolverResult {
	return ResolvePowerShellProfilesScoped("")
}

// ResolvePowerShellProfilesScoped is like ResolvePowerShellProfiles but
// scopes historical candidates to the supplied home directory.
func ResolvePowerShellProfilesScoped(home string) ProfileResolverResult {
	if runtime.GOOS != "windows" {
		return ProfileResolverResult{}
	}
	return discoverPowerShellProfilesScoped(home)
}

// InstallPowerShellActivation installs the activation block in the
// authoritative PowerShell profile. Returns paths that were modified.
// The home parameter scopes historical candidates to the supplied directory.
func InstallPowerShellActivation(home string) ([]string, error) {
	return InstallPowerShellActivationWith(home, nil)
}

// InstallPowerShellActivationWith is like InstallPowerShellActivation but
// accepts a pre-resolved ProfileResolverResult to avoid launching PowerShell.
func InstallPowerShellActivationWith(home string, psResult *ProfileResolverResult) ([]string, error) {
	return installPowerShellActivationForSpec(home, psResult, claudeCLISpec())
}

func installPowerShellActivationForSpec(home string, psResult *ProfileResolverResult, spec CLISpec) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}

	if psResult == nil {
		r := ResolvePowerShellProfilesScoped(home)
		psResult = &r
	}
	result := *psResult
	var installed []string

	// First pass: migrate legacy blocks in all discovered profiles (ActiveTargets
	// plus MigrationTargets) before installing the current block.
	for _, p := range result.MigrationTargets {
		base := filepath.Dir(p)
		data, err := safepath.ReadFile(base, p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return installed, fmt.Errorf("reading %s for legacy migration: %w", p, err)
		}
		content := string(data)
		changed := false
		if HasLegacyLauncherBlock(content) {
			content, _ = removeLegacyLauncherBlock(content)
			changed = true
		}
		if HasLegacyBedrockBlock(content) {
			content, _ = removeLegacyBedrockBlock(content)
			changed = true
		}
		if changed {
			if err := writeProfile(p, []byte(content)); err != nil {
				return installed, fmt.Errorf("migrating legacy block in %s: %w", p, err)
			}
		}
	}

	// Second pass: install the current block into InstallTargets only
	// (AllHosts profiles — CurrentHost profiles load after AllHosts and
	// can override or retain a stale duplicate of the global activation).
	installSet := map[string]bool{}
	for _, target := range result.InstallTargets {
		installSet[pathKey(target.Path)] = true
		changed, err := InstallTargetFor(target, spec)
		if err != nil {
			return installed, err
		}
		if changed {
			installed = append(installed, target.Path)
		}
	}

	// Third pass: strip this CLI's markers from every non-install path
	// (CurrentHost + historical Documents trees). Older Juggernaut versions
	// wrote host-specific blocks; those load AFTER AllHosts and override the
	// correct wrapper (or break the CLI when juggernaut is gone).
	stalePaths := make([]string, 0, len(result.ActiveTargets)+len(result.MigrationTargets))
	for _, target := range result.ActiveTargets {
		stalePaths = append(stalePaths, target.Path)
	}
	stalePaths = append(stalePaths, result.MigrationTargets...)
	seenStale := map[string]bool{}
	for _, path := range stalePaths {
		key := pathKey(path)
		if installSet[key] || seenStale[key] {
			continue
		}
		seenStale[key] = true
		_, err := RemoveTargetForMarkers(path, spec.Begin, spec.End)
		if err != nil {
			return installed, err
		}
		// Do not append stripped paths to installed — callers treat that list
		// as "profiles that received activation" (apply messaging). Cleanup is
		// intentional side work; counting it as an install misleads users.
	}

	return installed, nil
}

// UninstallPowerShellActivation removes activation blocks from all discovered
// and historical PowerShell profiles. Returns paths that were modified.
// The home parameter scopes historical candidates to the supplied directory.
func UninstallPowerShellActivation(home string) ([]string, error) {
	return UninstallPowerShellActivationWith(home, nil)
}

// UninstallPowerShellActivationWith is like UninstallPowerShellActivation but
// accepts a pre-resolved ProfileResolverResult to avoid launching PowerShell.
func UninstallPowerShellActivationWith(home string, psResult *ProfileResolverResult) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}

	if psResult == nil {
		r := ResolvePowerShellProfilesScoped(home)
		psResult = &r
	}
	result := *psResult
	var removed []string

	// Remove activation from all active targets.
	for _, target := range result.ActiveTargets {
		ok, err := RemoveTargetWithLegacy(target.Path)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, target.Path)
		}
	}

	// Also remove from historical profiles (MigrationTargets) — when
	// PowerShell discovers a redirected profile such as a OneDrive path,
	// old Documents profiles are placed in MigrationTargets but not
	// ActiveTargets, so iterating only active targets would leave
	// Juggernaut blocks in those historical files.
	for _, path := range result.MigrationTargets {
		if containsTargetPathCI(result.ActiveTargets, path) {
			continue // already handled above
		}
		ok, err := RemoveTargetWithLegacy(path)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, path)
		}
	}

	return removed, nil
}

// CheckPowerShellActivation checks whether the current activation block is
// present in an effective (discovered) profile. Returns whether activation is
// healthy, the path where it was found (if healthy), and any warnings.
// The home parameter scopes historical candidates to the supplied directory.
func CheckPowerShellActivation(home string) (healthy bool, path string, warnings []string) {
	return CheckPowerShellActivationWith(home, nil)
}

// CheckPowerShellActivationWith is like CheckPowerShellActivation but
// accepts a pre-resolved ProfileResolverResult to avoid launching PowerShell.
func CheckPowerShellActivationWith(home string, psResult *ProfileResolverResult) (healthy bool, path string, warnings []string) {
	if runtime.GOOS != "windows" {
		return true, "", nil
	}

	if psResult == nil {
		r := ResolvePowerShellProfilesScoped(home)
		psResult = &r
	}
	result := *psResult

	// Check effective profiles for the current block (first match wins).
	for _, target := range result.ActiveTargets {
		data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
		if err != nil {
			continue
		}
		if HasBlock(string(data)) {
			path = target.Path
			break
		}
	}

	healthy = path != ""

	// After finding activation, check ALL remaining effective profiles
	// (especially later-loading CurrentHost profiles) for legacy launcher
	// or Bedrock blocks that could override the activation.
	if healthy {
		for _, target := range result.ActiveTargets {
			if target.Path == path {
				continue // already checked
			}
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err != nil {
				continue
			}
			content := string(data)
			if HasLegacyLauncherBlock(content) || HasLegacyBedrockBlock(content) {
				warnings = append(warnings,
					fmt.Sprintf("legacy block in %s may override activation in %s", target.Path, path),
				)
			}
		}
	}

	return healthy, path, warnings
}

func normalizeNewlines(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

func resolveBinaryFrom(pathList string, names []string, self string, selfPaths []string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if sameExecutable(candidate, self) {
				continue
			}
			// On Windows staged launches, os.Executable() returns the temp copy
			// path. A stale claude.exe/codex.exe hardlinked to the installed
			// binary won't match the temp copy's inode, so also check against
			// any additional self paths passed by the caller.
			skip := false
			for _, sp := range selfPaths {
				if sameExecutable(candidate, sp) {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			// Reject legacy v4.2.6 claude.cmd/claude.bat shims that invoke
			// the removed juggernaut --launcher path. These shims are not
			// the same file as the juggernaut binary, so sameExecutable
			// alone does not catch them.
			if isKnownJuggernautArtifact(candidate, self) {
				continue
			}
			if isExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func resolveClaudeBinary(pathList, self string) (string, error) {
	names := []string{"claude"}
	if runtime.GOOS == "windows" {
		names = []string{"claude.exe", "claude.cmd", "claude.bat"}
	}
	return resolveBinaryFrom(pathList, names, self, nil)
}

// isLegacyClaudeShim returns true if the file at path is a legacy v4.2.6
// claude.cmd/claude.bat shim that invokes the removed juggernaut --launcher
// path. These shims must be rejected so they are not selected as the real
// Claude Code binary.
//
//nolint:unused // retained for future v4.2.6 artifact recovery
func isLegacyClaudeShim(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	data, err := os.ReadFile(path) // #nosec G703,G501 -- path is resolved from known config paths, not user input // nosemgrep: go_filesystem_rule-fileread -- path derived from known config paths
	if err != nil {
		return false
	}
	content := string(data)
	// Normalize line endings for comparison — legacy shims may have LF or
	// CRLF endings, and may have trailing whitespace or extra blank lines.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	return strings.Contains(content, "juggernaut --launcher")
}

func sameExecutable(candidate, self string) bool {
	if self == "" {
		return false
	}
	candidateInfo, err := os.Stat(candidate) // #nosec G703 -- candidate is resolved from PATH, not user input
	if err != nil {
		return false
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return false
	}
	return os.SameFile(candidateInfo, selfInfo)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path) // #nosec G703 -- path comes from known config paths, not user input
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func needsBearerToken(modes []string) bool {
	for _, mode := range modes {
		if authmode.IsBedrockAPIKey(mode) {
			return true
		}
	}
	return false
}

// readAuthModeFromConfig reads the auth mode from a provider config file's
// juggernaut block. It handles both JSON and TOML formats based on file
// extension. Returns empty string if the file doesn't exist, has no juggernaut
// block, or can't be parsed.
func readAuthModeFromConfig(path string) string {
	var mgr *config.Manager
	if strings.HasSuffix(path, ".toml") {
		f, err := config.FormatByName("toml")
		if err != nil {
			return ""
		}
		mgr = config.NewManagerWithFormat(path, f)
	} else {
		mgr = config.NewManager(path)
	}
	data, err := mgr.Read()
	if err != nil {
		return ""
	}
	if jb, ok := config.ParseJuggernautBlock(data); ok {
		return jb.AuthMode
	}
	return ""
}

func authModes(home string) ([]string, error) {
	paths := []string{
		filepath.Join(".", ".claude", "settings.json"),
		filepath.Join(home, ".claude", "settings.json"),
	}
	var modes []string
	for _, path := range paths {
		mgr := config.NewManager(path)
		data, err := mgr.Read()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		if jb, ok := config.ParseJuggernautBlock(data); ok && jb.AuthMode != "" {
			modes = append(modes, jb.AuthMode)
		}
	}
	return modes, nil
}

// RunBinary executes the resolved CLI binary with the given args and environment,
// inheriting stdin/stdout/stderr. Exported for cmd/launch.go which constructs
// LaunchOptions directly instead of using LaunchCLI.
func RunBinary(path string, args []string, env []string) error {
	// path is the resolved real Claude Code executable from ResolveClaudeBinary;
	// exec.Command is intentionally used without a shell to preserve arguments.
	cmd := exec.Command(path, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command, go_subproc_rule-subproc
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	return cmd.Run()
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func unsetEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0]
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

// legacyNames holds platform-specific file names for v4.2.6 artifacts.
type legacyNames struct {
	claude          string
	backup          string
	legacyLaunchers []string
	commandNames    []string
}

func platformNames() legacyNames {
	if runtime.GOOS == "windows" {
		return legacyNames{
			claude:          "claude.cmd",
			backup:          "claude.juggernaut-original.cmd",
			legacyLaunchers: []string{"juggernaut-claude.cmd"},
			commandNames:    []string{"claude.exe", "claude.cmd", "claude.bat"},
		}
	}
	return legacyNames{
		claude:          "claude",
		backup:          "claude.juggernaut-original",
		legacyLaunchers: []string{"juggernaut-claude"},
		commandNames:    []string{"claude"},
	}
}

// isKnownJuggernautArtifact returns true if the file at path is a known
// v4.2.6 Juggernaut artifact (legacy shim or symlink to the juggernaut binary).
func isKnownJuggernautArtifact(path, self string) bool {
	if runtime.GOOS == "windows" {
		data, err := os.ReadFile(path) // #nosec G703,G501 -- path is resolved from known config paths, not user input // nosemgrep: go_filesystem_rule-fileread -- path derived from known config paths
		if err != nil {
			return false
		}
		content := string(data)
		// Normalize line endings for comparison — legacy shims may have LF or
		// CRLF endings, and may have trailing whitespace or extra blank lines.
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.TrimSpace(content)
		// Normalize the constants the same way for a fair comparison.
		normalizedLF := strings.TrimSpace(strings.ReplaceAll(legacyCmdShimLF, "\r\n", "\n"))
		normalizedCRLF := strings.TrimSpace(strings.ReplaceAll(legacyCmdShimCRLF, "\r\n", "\n"))
		return content == normalizedLF || content == normalizedCRLF
	}

	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	if self == "" {
		return false
	}
	return samePath(target, self)
}

func recoverPlatformArtifacts(binDir, self string, names legacyNames) ([]LegacyAction, error) {
	var actions []LegacyAction
	for _, name := range names.legacyLaunchers {
		path := filepath.Join(binDir, name)
		if isKnownJuggernautArtifact(path, self) {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return actions, fmt.Errorf("removing %s: %w", path, err)
			}
			actions = append(actions, LegacyAction{Path: path, Action: "removed legacy Juggernaut launcher"})
		}
	}

	claudePath := filepath.Join(binDir, names.claude)
	backupPath := filepath.Join(binDir, names.backup)
	if isKnownJuggernautArtifact(claudePath, self) {
		if err := os.Remove(claudePath); err != nil && !os.IsNotExist(err) {
			return actions, fmt.Errorf("removing %s: %w", claudePath, err)
		}
		actions = append(actions, LegacyAction{Path: claudePath, Action: "removed legacy Juggernaut claude shim"})
	}
	if fileExists(backupPath) && !fileExists(claudePath) {
		if err := os.Rename(backupPath, claudePath); err != nil {
			return actions, fmt.Errorf("restoring %s: %w", claudePath, err)
		}
		actions = append(actions, LegacyAction{Path: claudePath, Action: "restored Claude Code binary from v4.2.6 backup"})
	}
	return actions, nil
}

func detectPlatformArtifacts(binDir, self string, names legacyNames) []LegacyAction {
	var actions []LegacyAction
	for _, name := range names.legacyLaunchers {
		path := filepath.Join(binDir, name)
		if isKnownJuggernautArtifact(path, self) {
			actions = append(actions, LegacyAction{Path: path, Action: "legacy Juggernaut launcher detected"})
		}
	}

	claudePath := filepath.Join(binDir, names.claude)
	backupPath := filepath.Join(binDir, names.backup)
	if isKnownJuggernautArtifact(claudePath, self) {
		actions = append(actions, LegacyAction{Path: claudePath, Action: "legacy Juggernaut claude shim detected"})
	}
	if fileExists(backupPath) && !fileExists(claudePath) {
		actions = append(actions, LegacyAction{Path: claudePath, Action: "Claude Code backup can be restored"})
	}
	return actions
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
