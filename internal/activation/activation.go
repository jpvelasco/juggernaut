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

	"github.com/jpvelasco/juggernaut/v5/internal/authmode"
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

	legacyCmdShimLF   = "@echo off\njuggernaut --launcher %*\n"
	legacyCmdShimCRLF = "@echo off\r\njuggernaut --launcher %*\r\n"
)

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

// LegacyAction describes one v4.2.6 recovery action.
type LegacyAction struct {
	Path   string
	Action string
}

// LaunchOptions carries injectable dependencies for tests.
type LaunchOptions struct {
	Home        string
	Args        []string
	Path        string
	TokenGetter func() (string, error)
	Runner      func(path string, args []string, env []string) error
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

// DefaultTargets returns the shell profile targets Juggernaut updates.
func DefaultTargets(home string) []Target {
	targets := []Target{
		{Path: filepath.Join(home, ".bashrc"), Shell: ShellPOSIX},
		{Path: filepath.Join(home, ".zshrc"), Shell: ShellPOSIX},
		{Path: filepath.Join(home, ".profile"), Shell: ShellPOSIX},
		{Path: filepath.Join(home, ".config", "fish", "config.fish"), Shell: ShellFish},
	}
	if runtime.GOOS == "windows" {
		targets = append(targets,
			Target{Path: filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"), Shell: ShellPowerShell},
			Target{Path: filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"), Shell: ShellPowerShell},
		)
	} else {
		targets = append(targets,
			Target{Path: filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), Shell: ShellPowerShell},
		)
	}
	return targets
}

// Block returns the activation block for a shell.
func Block(shell Shell) string {
	switch shell {
	case ShellFish:
		return strings.Join([]string{
			BeginMarker,
			"function claude",
			"    juggernaut launch -- $argv",
			"end",
			EndMarker,
		}, "\n")
	case ShellPowerShell:
		return strings.Join([]string{
			BeginMarker,
			"function global:claude {",
			"  juggernaut launch -- @args",
			"}",
			EndMarker,
		}, "\n")
	default:
		return strings.Join([]string{
			BeginMarker,
			"claude() {",
			"  juggernaut launch -- \"$@\"",
			"}",
			EndMarker,
		}, "\n")
	}
}

// Install writes or updates Juggernaut activation blocks in shell profiles.
// On Windows, it uses dynamic PowerShell profile discovery and migrates
// legacy blocks, then installs to POSIX targets as well. On other platforms
// it uses the default target list.
func Install(home string) ([]string, error) {
	var installed []string

	if runtime.GOOS == "windows" {
		psInstalled, err := InstallPowerShellActivation()
		if err != nil {
			return installed, err
		}
		installed = append(installed, psInstalled...)
	}

	for _, target := range DefaultTargets(home) {
		// On Windows, skip PowerShell targets (handled by InstallPowerShellActivation).
		if runtime.GOOS == "windows" && target.Shell == ShellPowerShell {
			continue
		}
		changed, err := InstallTarget(target)
		if err != nil {
			return installed, err
		}
		if changed {
			installed = append(installed, target.Path)
		}
	}
	return installed, nil
}

// InstallTarget writes or updates the activation block for one profile.
func InstallTarget(target Target) (bool, error) {
	base := filepath.Dir(target.Path)
	data, err := safepath.ReadFile(base, target.Path)
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return false, fmt.Errorf("reading %s: %w", target.Path, err)
	}
	next := upsertBlock(string(data), Block(target.Shell))
	if string(data) == next {
		return false, nil
	}
	if err := safepath.WriteFile(base, target.Path, []byte(next)); err != nil {
		return false, fmt.Errorf("writing %s: %w", target.Path, err)
	}
	return true, nil
}

// Uninstall removes Juggernaut activation blocks from shell profiles.
// On Windows, it also removes legacy blocks from discovered and historical paths.
func Uninstall(home string) ([]string, error) {
	var removed []string

	if runtime.GOOS == "windows" {
		psRemoved, err := UninstallPowerShellActivation()
		if err != nil {
			return removed, err
		}
		removed = append(removed, psRemoved...)
	}

	for _, target := range DefaultTargets(home) {
		// On Windows, skip PowerShell targets (handled by UninstallPowerShellActivation).
		if runtime.GOOS == "windows" && target.Shell == ShellPowerShell {
			continue
		}
		ok, err := RemoveTarget(target.Path)
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

// InstalledTargets returns profile paths currently containing the activation block.
// On Windows it checks both discovered active targets, historical candidates,
// and POSIX targets.
func InstalledTargets(home string) []string {
	var paths []string

	if runtime.GOOS == "windows" {
		result := ResolvePowerShellProfiles()
		allPaths := make([]string, 0, len(result.MigrationTargets)+len(result.HistoricalCandidates))
		allPaths = append(allPaths, result.MigrationTargets...)
		allPaths = append(allPaths, result.HistoricalCandidates...)
		for _, path := range allPaths {
			data, err := safepath.ReadFile(filepath.Dir(path), path)
			if err == nil && HasBlock(string(data)) {
				paths = append(paths, path)
			}
		}
	}

	for _, target := range DefaultTargets(home) {
		// On Windows, skip PowerShell targets (handled above via discovered paths).
		if runtime.GOOS == "windows" && target.Shell == ShellPowerShell {
			continue
		}
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

// Launch runs Claude Code with Juggernaut Bedrock activation.
func Launch(home string, args []string) error {
	return LaunchWithOptions(LaunchOptions{
		Home:        home,
		Args:        args,
		Path:        os.Getenv("PATH"),
		TokenGetter: keychain.Default().Get,
		Runner:      runClaudeBinary,
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
		opts.TokenGetter = keychain.Default().Get
	}
	if opts.Runner == nil {
		opts.Runner = runClaudeBinary
	}

	env := os.Environ()
	modes, err := authModes(opts.Home)
	if err != nil {
		return err
	}
	if len(modes) > 0 {
		env = setEnv(env, "CLAUDE_CODE_USE_BEDROCK", "1")
	}
	needsToken := needsBearerToken(modes)
	if needsToken {
		token, err := opts.TokenGetter()
		if err != nil {
			return fmt.Errorf("reading Bedrock API key from keychain: %w", err)
		}
		if token == "" {
			return fmt.Errorf("bedrock API key not found in keychain; run `juggernaut apply --auth=%s`", authmode.BedrockAPIKey)
		}
		env = setEnv(env, "AWS_BEARER_TOKEN_BEDROCK", token)
	} else if len(modes) > 0 {
		env = unsetEnv(env, "AWS_BEARER_TOKEN_BEDROCK")
	}

	claudePath, err := ResolveClaudeBinary(opts.Path)
	if err != nil {
		return errors.New("real claude binary not found on PATH; install Claude Code with `curl -fsSL https://claude.ai/install.sh | bash`")
	}
	return opts.Runner(claudePath, opts.Args, env)
}

// ResolveClaudeBinary finds the real Anthropic claude command while avoiding
// Juggernaut recursion through old shims or symlinks.
func ResolveClaudeBinary(pathList string) (string, error) {
	self, _ := os.Executable()
	return resolveClaudeBinary(pathList, self)
}

func upsertBlock(content, block string) string {
	content = normalizeNewlines(content)
	without, _ := removeBlock(content)
	without = strings.TrimRight(without, "\n")
	if without == "" {
		return block + "\n"
	}
	return without + "\n\n" + block + "\n"
}

func removeBlock(content string) (string, bool) {
	content = normalizeNewlines(content)
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inBlock := false
	found := false

	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case BeginMarker:
			inBlock = true
			found = true
			continue
		case EndMarker:
			if inBlock {
				inBlock = false
				continue
			}
		}
		if !inBlock {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), found
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

// HasAnyLegacyBlock reports whether content contains any positively
// identified legacy Juggernaut block.
func HasAnyLegacyBlock(content string) bool {
	return HasLegacyLauncherBlock(content) || HasLegacyBedrockBlock(content)
}

// removeLegacyLauncherBlock removes a legacy "# BEGIN: Juggernaut Launcher"
// block from content.
func removeLegacyLauncherBlock(content string) (string, bool) {
	content = normalizeNewlines(content)
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inBlock := false
	found := false

	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case LegacyLauncherBegin:
			inBlock = true
			found = true
			continue
		case LegacyLauncherEnd:
			if inBlock {
				inBlock = false
				continue
			}
		}
		if !inBlock {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), found
}

// removeLegacyBedrockBlock removes a legacy "# BEGIN: Claude Code Bedrock
// Configuration" block from content.
func removeLegacyBedrockBlock(content string) (string, bool) {
	content = normalizeNewlines(content)
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	inBlock := false
	found := false

	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case LegacyBedrockBegin:
			inBlock = true
			found = true
			continue
		case LegacyBedrockEnd:
			if inBlock {
				inBlock = false
				continue
			}
		}
		if !inBlock {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), found
}

// removeLegacyBlocks removes all positively identified legacy Juggernaut
// blocks from content. Returns the cleaned content and a list of which legacy
// blocks were removed.
func removeLegacyBlocks(content string) (string, []string) {
	var removed []string

	cleaned, found := removeLegacyLauncherBlock(content)
	if found {
		removed = append(removed, "Juggernaut Launcher")
		content = cleaned
	}

	cleaned, found = removeLegacyBedrockBlock(content)
	if found {
		removed = append(removed, "Claude Code Bedrock Configuration")
		content = cleaned
	}

	return content, removed
}

// ResolvePowerShellProfiles returns the shared profile resolver result.
// On Windows it dynamically queries PowerShell. On other platforms it
// returns the default targets.
func ResolvePowerShellProfiles() ProfileResolverResult {
	if runtime.GOOS != "windows" {
		return ProfileResolverResult{}
	}
	return discoverPowerShellProfiles()
}

// InstallPowerShellActivation installs the activation block in the
// authoritative PowerShell profile and migrates any legacy blocks from
// discovered profiles. Returns paths that were modified.
func InstallPowerShellActivation() ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}

	result := ResolvePowerShellProfiles()
	var installed []string

	// First pass: migrate legacy blocks from all discovered profiles.
	for _, path := range result.MigrationTargets {
		modified, err := migrateLegacyAndInstall(path)
		if err != nil {
			return installed, err
		}
		if modified {
			installed = append(installed, path)
		}
	}

	// Also check historical candidates for legacy block cleanup.
	for _, path := range result.HistoricalCandidates {
		modified, err := migrateLegacyOnly(path)
		if err != nil {
			return installed, err
		}
		if modified {
			installed = append(installed, path)
		}
	}

	// Second pass: ensure the install target has the current block.
	if result.InstallTarget.Path != "" {
		changed, err := InstallTarget(result.InstallTarget)
		if err != nil {
			return installed, err
		}
		if changed {
			installed = append(installed, result.InstallTarget.Path)
		}
	}

	return installed, nil
}

// migrateLegacyAndInstall removes legacy blocks and installs the current
// block if no current block exists. Returns true if the file was modified.
func migrateLegacyAndInstall(path string) (bool, error) {
	base := filepath.Dir(path)
	data, err := safepath.ReadFile(base, path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	original := content

	// Remove legacy blocks.
	content, _ = removeLegacyBlocks(content)

	// If no current block, install one.
	if !HasBlock(content) {
		content = upsertBlock(content, Block(ShellPowerShell))
	}

	if content == original {
		return false, nil
	}

	return true, safepath.WriteFile(base, path, []byte(content))
}

// migrateLegacyOnly removes legacy blocks without installing the current block.
// Used for historical cleanup candidates. Returns true if the file was modified.
func migrateLegacyOnly(path string) (bool, error) {
	base := filepath.Dir(path)
	data, err := safepath.ReadFile(base, path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	original := content

	// Remove legacy blocks.
	content, _ = removeLegacyBlocks(content)

	if content == original {
		return false, nil
	}

	return true, safepath.WriteFile(base, path, []byte(content))
}

// UninstallPowerShellActivation removes activation blocks from all discovered
// and historical PowerShell profiles. Returns paths that were modified.
func UninstallPowerShellActivation() ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}

	result := ResolvePowerShellProfiles()
	var removed []string

	allPaths := make([]string, 0, len(result.MigrationTargets)+len(result.HistoricalCandidates))
	allPaths = append(allPaths, result.MigrationTargets...)
	allPaths = append(allPaths, result.HistoricalCandidates...)

	for _, path := range allPaths {
		ok, err := removeActivationAndLegacy(path)
		if err != nil {
			return removed, err
		}
		if ok {
			removed = append(removed, path)
		}
	}

	return removed, nil
}

// removeActivationAndLegacy removes both the current activation block and any
// legacy blocks from a profile. Returns true if any block was removed.
func removeActivationAndLegacy(path string) (bool, error) {
	base := filepath.Dir(path)
	data, err := safepath.ReadFile(base, path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	original := content

	// Remove current block.
	content, _ = removeBlock(content)

	// Remove legacy blocks.
	content, _ = removeLegacyBlocks(content)

	if content == original {
		return false, nil
	}

	return true, safepath.WriteFile(base, path, []byte(content))
}

// CheckPowerShellActivation checks whether the current activation block is
// present in an effective (discovered) profile. Returns whether activation is
// healthy, the path where it was found (if healthy), and any warnings.
func CheckPowerShellActivation() (healthy bool, path string, warnings []string) {
	if runtime.GOOS != "windows" {
		return true, "", nil
	}

	result := ResolvePowerShellProfiles()

	// Check effective profiles for the current block.
	for _, target := range result.ActiveTargets {
		data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
		if err != nil {
			continue
		}
		if HasBlock(string(data)) {
			return true, target.Path, nil
		}
	}

	// Check if legacy block exists in an effective profile.
	for _, target := range result.ActiveTargets {
		data, err := safepath.ReadFile(filepath.Dir(target.Path), target.Path)
		if err != nil {
			continue
		}
		if HasAnyLegacyBlock(string(data)) {
			warnings = append(warnings,
				fmt.Sprintf("legacy Juggernaut launcher block found in effective profile %s", target.Path))
		}
	}

	// Check if activation exists only in a historical (non-loaded) path.
	for _, path := range result.HistoricalCandidates {
		data, err := safepath.ReadFile(filepath.Dir(path), path)
		if err != nil {
			continue
		}
		if HasBlock(string(data)) {
			warnings = append(warnings,
				fmt.Sprintf("activation block found in non-loaded historical path %s — PowerShell does not load this profile", path))
		}
	}

	// Warn about host-specific override risk.
	if len(result.ActiveTargets) > 1 {
		// Multiple discovered profiles — check if a later-loading host-specific
		// profile has a legacy block that could override the all-hosts activation.
		for _, path := range result.MigrationTargets {
			data, err := safepath.ReadFile(filepath.Dir(path), path)
			if err != nil {
				continue
			}
			if HasLegacyLauncherBlock(string(data)) {
				warnings = append(warnings,
					fmt.Sprintf("host-specific profile %s contains legacy launcher that may override all-hosts activation", path))
			}
		}
	}

	return false, "", warnings
}

func normalizeNewlines(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
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

func isKnownJuggernautArtifact(path, self string) bool {
	if runtime.GOOS == "windows" {
		data, err := os.ReadFile(path) // nosemgrep: gosec.G304-1, go_filesystem_rule-fileread
		if err != nil {
			return false
		}
		content := string(data)
		return content == legacyCmdShimLF || content == legacyCmdShimCRLF
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

func resolveClaudeBinary(pathList, self string) (string, error) {
	names := platformNames()
	for _, dir := range filepath.SplitList(pathList) {
		for _, name := range names.commandNames {
			candidate := filepath.Join(dir, name)
			if isKnownJuggernautArtifact(candidate, self) || sameExecutable(candidate, self) {
				continue
			}
			if isExecutable(candidate) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
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

func runClaudeBinary(path string, args []string, env []string) error {
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
