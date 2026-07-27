// Package activation manages shell activation blocks and Claude Code launch.
package activation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
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
	ActiveTargets      []Target
	InstallTargets     []Target
	MigrationTargets   []string
	DiscoveryWarnings  []string
	UsedFallback       bool
	EditionsDiscovered []string
}

// LaunchOptions carries injectable dependencies for tests.
type LaunchOptions struct {
	Home        string
	Args        []string
	Path        string
	TokenGetter func() (string, error)
	Runner      func(path string, args []string, env []string) error
	Warner      func(msg string)
	Target      LaunchTarget
	SelfPaths   []string
}

// LaunchTarget carries the per-CLI launch configuration, mirroring
// provider.LaunchSpec (the activation package stays provider-free; cmd/ maps
// LaunchSpec → LaunchTarget).
type LaunchTarget struct {
	BinaryNames []string
	TokenEnvVar string
	StaticEnv   map[string]string
	NeedsToken  bool
	ConfigPath  string
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
		NeedsToken:  false,
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

// InstallOptions carries optional dependencies for Install.
type InstallOptions struct {
	PowerShellResult *ProfileResolverResult
	Spec             CLISpec
}

// UninstallOptions carries optional dependencies for Uninstall.
type UninstallOptions struct {
	PowerShellResult *ProfileResolverResult
	Spec             CLISpec
}

// --- Profile I/O ---

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

// --- Block presence checks ---

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

// --- Block manipulation ---

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

func upsertBlock(content, block string) string {
	return upsertBlockWithMarkers(content, block, BeginMarker, EndMarker)
}

func removeBlock(content string) (string, bool) {
	return removeBlockWithMarkers(normalizeNewlines(content), BeginMarker, EndMarker)
}

// removeBlockWithMarkers removes blocks delimited by begin and end markers.
// It only removes spans whose begin marker has a matching following end marker;
// orphaned begin markers are left untouched along with all subsequent content.
func removeBlockWithMarkers(content, begin, end string) (string, bool) {
	lines := strings.Split(content, "\n")

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

	remove := make(map[int]bool, len(lines))
	for _, span := range stack {
		for i := span[0]; i <= span[1]; i++ {
			remove[i] = true
		}
	}

	out := make([]string, 0, len(lines))
	for i := range lines {
		if !remove[i] {
			out = append(out, lines[i])
		}
	}
	return strings.Join(out, "\n"), len(stack) > 0
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

func normalizeNewlines(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}

// --- PowerShell profile resolution ---

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
