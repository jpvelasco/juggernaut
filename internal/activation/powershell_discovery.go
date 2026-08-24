//go:build windows

package activation

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"golang.org/x/sys/windows"
)

// psEdition identifies a PowerShell edition and its executable name.
type psEdition struct {
	Name string // e.g. "PowerShell 7", "Windows PowerShell 5.1"
	Exe  string // e.g. "pwsh.exe", "powershell.exe"
}

// psProfilePaths holds the resolved profile paths for one PowerShell edition.
type psProfilePaths struct {
	CurrentUserAllHosts    string `json:"CurrentUserAllHosts"`
	CurrentUserCurrentHost string `json:"CurrentUserCurrentHost"`
}

// psDiscoveryResult holds the result of profile discovery for one edition.
type psDiscoveryResult struct {
	Edition     string
	AllHosts    string
	CurrentHost string
	DiscoveryOK bool
}

// discoveryCommandRunner is the injectable interface for running PowerShell
// discovery commands. The real implementation uses exec.Command; tests inject a
// stub.
type discoveryCommandRunner interface {
	RunContext(ctx context.Context, exe string, args []string) ([]byte, error)
}

// realCommandRunner executes real shell commands.
type realCommandRunner struct{}

func (r *realCommandRunner) RunContext(ctx context.Context, exe string, args []string) ([]byte, error) {
	// exe is a fixed, internally-defined PowerShell binary name ("pwsh.exe"
	// or "powershell.exe") — not user-controlled input.
	cmd := exec.CommandContext(ctx, exe, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command, go_subproc_rule-subproc
	data, err := cmd.CombinedOutput()
	return data, err
}

// psDiscoveryScriptSimple queries $PROFILE and writes compact JSON to stdout.
const psDiscoveryScriptSimple = `
[System.Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$profiles = @{
	CurrentUserAllHosts = $PROFILE.CurrentUserAllHosts
	CurrentUserCurrentHost = $PROFILE.CurrentUserCurrentHost
}
$profiles | ConvertTo-Json -Compress
`

// defaultPSRunner is the default command runner used in production.
var defaultPSRunner discoveryCommandRunner = &realCommandRunner{}

// psRunnerMu protects defaultPSRunner from concurrent access during tests.
var psRunnerMu sync.Mutex

// SetPSRunnerForTesting replaces the default runner (for tests only).
func SetPSRunnerForTesting(r discoveryCommandRunner) {
	psRunnerMu.Lock()
	defer psRunnerMu.Unlock()
	defaultPSRunner = r
}

// ResetPSRunnerForTesting restores the default runner.
func ResetPSRunnerForTesting() {
	psRunnerMu.Lock()
	defer psRunnerMu.Unlock()
	defaultPSRunner = &realCommandRunner{}
}

// discoverPowerShellProfiles queries installed PowerShell editions for their
// profile paths and returns a ProfileResolverResult.
func discoverPowerShellProfiles() ProfileResolverResult {
	return discoverPowerShellProfilesScoped("")
}

// discoverPowerShellProfilesScoped is like discoverPowerShellProfiles but
// uses the supplied home directory for historical candidates. If home is
// empty, it resolves from the environment.
func discoverPowerShellProfilesScoped(home string) ProfileResolverResult {
	if home == "" {
		home = resolveHomeDir()
	}
	result := ProfileResolverResult{}

	// Resolve the trusted containment roots for path validation.
	// This prevents PowerShell from returning paths that escape the user's
	// profile tree (e.g. UNC paths, symlink escapes). Two roots are trusted:
	// $HOME and the OS-resolved Documents Known Folder, which domain folder
	// redirection may place outside $HOME entirely. Fall back to $HOME-only
	// when the Known Folder API is unavailable.
	docsRoot := ""
	primaryDocs := filepath.Join(home, "Documents")
	if docs, err := resolveDocumentsFolder(); err == nil && docs != "" {
		docsRoot = docs
		primaryDocs = docs
	}
	primaryHistorical := resolveHistoricalTargets(primaryDocs)

	// Full historical set (primary + OneDrive/local alternates) for uninstall
	// and legacy-block migration only — never for fresh install targets.
	historical := historicalPowerShellTargetsScoped(home)

	editions := []psEdition{
		{Name: "PowerShell 7", Exe: "pwsh.exe"},
		{Name: "Windows PowerShell 5.1", Exe: "powershell.exe"},
	}

	for _, ed := range editions {
		dr := discoverEdition(ed, defaultPSRunner)

		if dr.DiscoveryOK {
			result.EditionsDiscovered = append(result.EditionsDiscovered, ed.Name)
			allHosts := validateUnderTrustedRoots(dr.AllHosts, home, docsRoot)
			currentHost := validateUnderTrustedRoots(dr.CurrentHost, home, docsRoot)

			if allHosts != "" && !containsTargetPathCI(result.ActiveTargets, allHosts) {
				result.ActiveTargets = append(result.ActiveTargets,
					Target{Path: allHosts, Shell: ShellPowerShell},
				)
				result.InstallTargets = append(result.InstallTargets,
					Target{Path: allHosts, Shell: ShellPowerShell},
				)
				if !containsPathCI(result.MigrationTargets, allHosts) {
					result.MigrationTargets = append(result.MigrationTargets, allHosts)
				}
			}
			// Add currentHost to ActiveTargets for health checks and to
			// MigrationTargets for legacy-block scanning. It is deliberately
			// NOT added to InstallTargets — the host-specific profile loads
			// after the all-hosts profile and can override or retain a stale
			// duplicate of the global activation.
			if currentHost != "" && !containsTargetPathCI(result.ActiveTargets, currentHost) {
				result.ActiveTargets = append(result.ActiveTargets,
					Target{Path: currentHost, Shell: ShellPowerShell},
				)
			}
			if currentHost != "" && !containsPathCI(result.MigrationTargets, currentHost) {
				result.MigrationTargets = append(result.MigrationTargets, currentHost)
			}
		} else {
			result.DiscoveryWarnings = append(result.DiscoveryWarnings,
				fmt.Sprintf("could not query profile paths for %s", ed.Name),
			)
		}
	}

	// If no active targets found, fall back to the primary Known Documents
	// tree only (not every alternate layout). That keeps install focused
	// while MigrationTargets still cover leftovers elsewhere.
	if len(result.ActiveTargets) == 0 {
		result.UsedFallback = true
		for _, p := range primaryHistorical {
			if !containsTargetPathCI(result.ActiveTargets, p) {
				result.ActiveTargets = append(result.ActiveTargets,
					Target{Path: p, Shell: ShellPowerShell},
				)
				// Fallback installs AllHosts profiles only — same policy as
				// live discovery (never write CurrentHost wrappers).
				if strings.EqualFold(filepath.Base(p), "profile.ps1") {
					result.InstallTargets = append(result.InstallTargets,
						Target{Path: p, Shell: ShellPowerShell},
					)
				}
			}
		}
		result.DiscoveryWarnings = append(result.DiscoveryWarnings,
			"PowerShell profile discovery failed; using Known Documents fallback",
		)
	}

	// Merge historical candidates into MigrationTargets (for doctor,
	// uninstall, and legacy-block migration).
	for _, p := range historical {
		if !containsPathCI(result.MigrationTargets, p) {
			result.MigrationTargets = append(result.MigrationTargets, p)
		}
	}

	return result
}

func discoverEdition(ed psEdition, runner discoveryCommandRunner) psDiscoveryResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := runner.RunContext(ctx, ed.Exe, []string{
		"-NoLogo", "-NoProfile", "-NonInteractive",
		"-Command", psDiscoveryScriptSimple,
	})
	// Even when PowerShell exits non-zero, it may have produced valid JSON
	// before the error. Try parsing the output first; fall back to error only
	// if parsing also fails.
	if err == nil {
		return parseDiscoveryOutput(ed.Name, output)
	}
	if dr := parseDiscoveryOutput(ed.Name, output); dr.DiscoveryOK {
		return dr
	}
	return psDiscoveryResult{Edition: ed.Name}
}

func parseDiscoveryOutput(edition string, output []byte) psDiscoveryResult {
	// Trim BOM and whitespace, handle both LF and CRLF
	s := strings.TrimSpace(string(output))
	// Strip UTF-8 BOM if present
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")

	var paths psProfilePaths
	if err := json.Unmarshal([]byte(s), &paths); err != nil {
		return psDiscoveryResult{Edition: edition}
	}

	return psDiscoveryResult{
		Edition:     edition,
		AllHosts:    paths.CurrentUserAllHosts,
		CurrentHost: paths.CurrentUserCurrentHost,
		DiscoveryOK: true,
	}
}

// historicalPowerShellTargets returns the profile paths resolved via the
// Windows Known Folder API (FOLDERID_Documents). These paths may still
// contain a Juggernaut block even after the user's Documents folder moved
// to OneDrive. These are used for uninstall/doctor scanning and
// legacy-block migration.
func historicalPowerShellTargets() []string {
	docs, err := resolveDocumentsFolder()
	if err != nil {
		return nil
	}
	return resolveHistoricalTargets(docs)
}

// historicalPowerShellTargetsScoped returns the profile paths resolved via
// the Windows Known Folder API, scoped to the given home directory as a
// fallback when the Known Folder API is unavailable. These are used for
// uninstall/doctor scanning and legacy-block migration.
//
// When Documents is OneDrive-redirected, stale blocks may still live under
// the non-redirected $HOME\Documents tree (and vice versa). Both trees are
// scanned, plus the common OneDrive\Documents layout under $HOME.
func historicalPowerShellTargetsScoped(home string) []string {
	if home == "" {
		return nil
	}
	docs, err := resolveDocumentsFolder()
	if err != nil {
		// Fall back to $HOME/Documents when Known Folders is unavailable.
		docs = filepath.Join(home, "Documents")
	}
	paths := resolveHistoricalTargets(docs)

	// Also scan non-redirected and common OneDrive layouts so uninstall
	// never leaves wrappers behind after a Documents move.
	for _, extra := range []string{
		filepath.Join(home, "Documents"),
		filepath.Join(home, "OneDrive", "Documents"),
	} {
		if pathsEqualCI(extra, docs) {
			continue
		}
		paths = append(paths, resolveHistoricalTargets(extra)...)
	}
	return deduplicatePathsCI(paths)
}

// pathsEqualCI reports path equality (case-insensitive). This file is
// Windows-only (//go:build windows), so EqualFold is always correct.
func pathsEqualCI(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// defaultResolveDocumentsFolder uses the Windows Known Folder API
// (FOLDERID_Documents) so OneDrive redirects and custom Documents locations
// resolve correctly.
func defaultResolveDocumentsFolder() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
}

// resolveDocumentsFolder is the active Documents resolver (swappable in tests).
var resolveDocumentsFolder = defaultResolveDocumentsFolder

// SetResolveDocumentsFolderForTesting replaces the Documents folder resolver
// (for tests only).
func SetResolveDocumentsFolderForTesting(fn func() (string, error)) {
	resolveDocumentsFolder = fn
}

// ResetResolveDocumentsFolderForTesting restores the default resolver.
func ResetResolveDocumentsFolderForTesting() {
	resolveDocumentsFolder = defaultResolveDocumentsFolder
}

// resolveHistoricalTargets builds the well-known PowerShell profile paths
// under the given Documents folder. Includes both AllHosts (profile.ps1) and
// CurrentHost (Microsoft.PowerShell_profile.ps1) for PS 7 and Windows
// PowerShell 5.1 — older installs wrote CurrentHost blocks that must still
// be found on uninstall/migrate.
func resolveHistoricalTargets(docs string) []string {
	return []string{
		filepath.Join(docs, "PowerShell", "profile.ps1"),
		filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(docs, "WindowsPowerShell", "profile.ps1"),
		filepath.Join(docs, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}
}

// resolveHomeDir returns the user's home directory from environment or OS.
func resolveHomeDir() string {
	return safepath.HomeDirOrEmpty()
}
