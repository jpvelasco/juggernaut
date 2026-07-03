// Package activation manages shell activation blocks and Claude Code launch.
package activation

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
	"github.com/jpvelasco/juggernaut/v5/internal/config"
	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

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
}

// LaunchTarget carries the per-CLI launch configuration, mirroring
// provider.LaunchSpec (the activation package stays provider-free; cmd/ maps
// LaunchSpec → LaunchTarget).
type LaunchTarget struct {
	BinaryNames []string          // real exe names to resolve on PATH
	TokenEnvVar string            // env var to inject the bearer token into
	StaticEnv   map[string]string // static enable-flags (Claude: CLAUDE_CODE_USE_BEDROCK=1)
	NeedsToken  bool              // whether a bearer token is required
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
		TokenEnvVar: "AWS_BEARER_TOKEN_BEDROCK",
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

// Block returns the activation block for a shell.
// Block returns the Claude activation block. Retained for back-compat; delegates
// to the per-CLI generator with Claude's identity.
func Block(shell Shell) string {
	return blockFor(shell, "claude", BeginMarker, EndMarker)
}

// blockFor generates a shell activation block that defines a function named
// `cli` delegating to `juggernaut launch [cli] -- ...`. For the default CLI
// (claude) the launch verb takes no CLI argument, so the emitted block is
// byte-identical to the historical Claude block; other CLIs pass their name
// (`juggernaut launch codex --`).
func blockFor(shell Shell, cli, begin, end string) string {
	// Default CLI (claude) uses the bare `launch`; others name the CLI.
	launchArg := ""
	if cli != "claude" {
		launchArg = " " + cli
	}
	switch shell {
	case ShellFish:
		return strings.Join([]string{
			begin,
			"function " + cli,
			"    juggernaut launch" + launchArg + " -- $argv",
			"end",
			end,
		}, "\n")
	case ShellPowerShell:
		return strings.Join([]string{
			begin,
			"function global:" + cli + " {",
			"  juggernaut launch" + launchArg + " -- @args",
			"}",
			end,
		}, "\n")
	default:
		return strings.Join([]string{
			begin,
			cli + "() {",
			"  juggernaut launch" + launchArg + " -- \"$@\"",
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
	base := filepath.Dir(target.Path)
	data, err := safepath.ReadFile(base, target.Path)
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return false, fmt.Errorf("reading %s: %w", target.Path, err)
	}
	block := blockFor(target.Shell, spec.Name, spec.Begin, spec.End)
	next := upsertBlockWithMarkers(string(data), block, spec.Begin, spec.End)
	if string(data) == next {
		return false, nil
	}
	if err := safepath.WriteFile(base, target.Path, []byte(next)); err != nil {
		return false, fmt.Errorf("writing %s: %w", target.Path, err)
	}
	return true, nil
}

// UninstallOptions carries optional dependencies for Uninstall.
type UninstallOptions struct {
	// PowerShellResult, when set, is used instead of resolving profiles
	// dynamically. This is required for tests to avoid touching real profiles.
	PowerShellResult *ProfileResolverResult
}

// Uninstall removes Juggernaut activation blocks from shell profiles.
// On Windows, it also removes blocks from discovered PowerShell profiles.
func Uninstall(home string) ([]string, error) {
	return UninstallWith(home, UninstallOptions{})
}

// UninstallWith is like Uninstall but accepts injectable dependencies.
func UninstallWith(home string, opts UninstallOptions) ([]string, error) {
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

// RemoveTarget removes the activation block from a profile.
func RemoveTarget(path string) (bool, error) {
	base := filepath.Dir(path)
	data, err := safepath.ReadFile(base, path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	next, found := removeBlock(string(data))
	if !found {
		return false, nil
	}
	if err := safepath.WriteFile(base, path, []byte(next)); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

// RemoveTargetWithLegacy removes the activation block AND any legacy launcher
// or Bedrock blocks from a profile. This is used by full uninstall to ensure
// no Juggernaut-related blocks remain.
func RemoveTargetWithLegacy(path string) (bool, error) {
	base := filepath.Dir(path)
	data, err := safepath.ReadFile(base, path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
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
	if err := safepath.WriteFile(base, path, []byte(content)); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

// InstalledTargets returns profile paths currently containing the activation block.
// On Windows it checks discovered active targets and POSIX targets.
func InstalledTargets(home string) []string {
	return InstalledTargetsWith(home, nil)
}

// InstalledTargetsWith is like InstalledTargets but accepts a pre-resolved
// ProfileResolverResult to avoid launching PowerShell.
func InstalledTargetsWith(home string, psResult *ProfileResolverResult) []string {
	var paths []string

	if runtime.GOOS == "windows" {
		result := psResult
		if result == nil {
			r := ResolvePowerShellProfilesScoped(home)
			result = &r
		}
		for _, target := range result.ActiveTargets {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err == nil && HasBlock(string(data)) {
				paths = append(paths, target.Path)
			}
		}
	} else if psResult != nil {
		for _, target := range psResult.ActiveTargets {
			data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
			if err == nil && HasBlock(string(data)) {
				paths = append(paths, target.Path)
			}
		}
	}

	for _, target := range DefaultTargets(home) {
		data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
		if err == nil && HasBlock(string(data)) {
			paths = append(paths, target.Path)
		}
	}
	return paths
}

// HasBlock reports whether content contains a Juggernaut activation block.
func HasBlock(content string) bool {
	return strings.Contains(content, BeginMarker) && strings.Contains(content, EndMarker)
}

// HasLegacyLauncherBlock reports whether content contains a legacy
// "# BEGIN: Juggernaut Launcher" block.
func HasLegacyLauncherBlock(content string) bool {
	return strings.Contains(content, LegacyLauncherBegin) && strings.Contains(content, LegacyLauncherEnd)
}

// HasLegacyBedrockBlock reports whether content contains a legacy
// "# BEGIN: Claude Code Bedrock Configuration" block.
func HasLegacyBedrockBlock(content string) bool {
	return strings.Contains(content, LegacyBedrockBegin) && strings.Contains(content, LegacyBedrockEnd)
}

// removeLegacyLauncherBlock removes a legacy "# BEGIN: Juggernaut Launcher"
// block from content. It only removes the block if both the begin and end
// markers are present; an orphaned begin marker is left untouched.
func removeLegacyLauncherBlock(content string) (string, bool) {
	content = normalizeNewlines(content)
	if !strings.Contains(content, LegacyLauncherEnd) {
		return content, false
	}
	return removeBlockWithMarkers(content, LegacyLauncherBegin, LegacyLauncherEnd)
}

// removeLegacyBedrockBlock removes a legacy "# BEGIN: Claude Code Bedrock
// Configuration" block from content. It only removes the block if both the
// begin and end markers are present; an orphaned begin marker is left untouched.
func removeLegacyBedrockBlock(content string) (string, bool) {
	content = normalizeNewlines(content)
	if !strings.Contains(content, LegacyBedrockEnd) {
		return content, false
	}
	return removeBlockWithMarkers(content, LegacyBedrockBegin, LegacyBedrockEnd)
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
// used by `juggernaut launch [cli] -- ...`.
func LaunchCLI(home string, args []string, target LaunchTarget) error {
	return LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        args,
		Path:        os.Getenv("PATH"),
		TokenGetter: func() (string, error) { return keychain.Default().GetWithFallback(home) },
		Runner:      runBinary,
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
		opts.Runner = runBinary
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
		tokenEnvVar = "AWS_BEARER_TOKEN_BEDROCK"
	}

	env := os.Environ()
	modes, err := authModes(opts.Home)
	if err != nil {
		return err
	}
	if len(modes) > 0 {
		for k, v := range target.StaticEnv {
			env = setEnv(env, k, v)
		}
	}
	if needsBearerToken(modes) {
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
	} else if len(modes) > 0 {
		env = unsetEnv(env, tokenEnvVar)
	}

	binPath, err := resolveBinary(opts.Path, target.BinaryNames)
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

// DefaultBinDir returns the user-local bin directory where broken v4.2.6
// artifacts may exist.
func DefaultBinDir(home string) string {
	if home == "" {
		home = os.Getenv("HOME")
	}
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		home, _ = os.UserHomeDir()
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
			continue
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
			if err := safepath.WriteFile(base, p, []byte(content)); err != nil {
				return installed, fmt.Errorf("migrating legacy block in %s: %w", p, err)
			}
		}
	}

	// Second pass: install the current block into InstallTargets only
	// (AllHosts profiles — CurrentHost profiles load after AllHosts and
	// can override or retain a stale duplicate of the global activation).
	for _, target := range result.InstallTargets {
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

// resolveBinary finds a real CLI executable on pathList by trying each of names,
// skipping this juggernaut binary and known v4.2.6 launcher artifacts. Used by
// the launcher for any CLI (claude, codex, …).
func resolveBinary(pathList string, names []string) (string, error) {
	self, _ := os.Executable()
	return resolveBinaryFrom(pathList, names, self)
}

func resolveBinaryFrom(pathList string, names []string, self string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		for _, name := range names {
			candidate := filepath.Join(dir, name)
			if sameExecutable(candidate, self) {
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
	return resolveBinaryFrom(pathList, names, self)
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
	data, err := os.ReadFile(path)
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
	candidateInfo, err := os.Stat(candidate)
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
	info, err := os.Stat(path)
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
		block, ok := data["juggernaut"].(map[string]any)
		if !ok {
			continue
		}
		meta, ok := block["meta"].(map[string]any)
		if !ok || meta["managedBy"] != "juggernaut" {
			continue
		}
		auth, ok := block["auth"].(map[string]any)
		if !ok {
			continue
		}
		if mode, ok := auth["mode"].(string); ok {
			modes = append(modes, mode)
		}
	}
	return modes, nil
}

func runBinary(path string, args []string, env []string) error {
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

// containsTargetPathCI checks if a path exists in a []Target,
// case-insensitive on Windows.
func containsTargetPathCI(targets []Target, path string) bool {
	for _, t := range targets {
		if runtime.GOOS == "windows" {
			if strings.EqualFold(t.Path, path) {
				return true
			}
		} else if t.Path == path {
			return true
		}
	}
	return false
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
		data, err := os.ReadFile(path) // nosemgrep: gosec.G304-1, go_filesystem_rule-fileread
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
