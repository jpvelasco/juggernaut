// install.go — Install, uninstall, and detection of shell activation blocks.

package activation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
)

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

// specOrClaude returns s if populated, else the Claude default.
func specOrClaude(s CLISpec) CLISpec {
	if s.Name == "" {
		return claudeCLISpec()
	}
	return s
}

// --- Install ---

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

// --- Uninstall ---

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

// --- Installed detection ---

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

	psResolved := resolveOrUse(home, psResult)
	if psResolved != nil {
		for _, target := range psResolved.ActiveTargets {
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

// --- PowerShell install/uninstall/check ---

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

// resolveOrUse returns a concrete ProfileResolverResult, resolving PowerShell
// profiles when psResult is nil and we're on Windows. Returns nil on non-Windows.
func resolveOrUse(home string, psResult *ProfileResolverResult) *ProfileResolverResult {
	if psResult != nil {
		return psResult
	}
	r := ResolvePowerShellProfilesScoped(home)
	return &r
}

func installPowerShellActivationForSpec(home string, psResult *ProfileResolverResult, spec CLISpec) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}

	psResult = resolveOrUse(home, psResult)
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

	psResult = resolveOrUse(home, psResult)
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

	psResult = resolveOrUse(home, psResult)
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
