//go:build windows

package activation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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

	// Resolve the base directory for path containment validation.
	// This prevents PowerShell from returning paths that escape the
	// user's home directory (e.g. UNC paths, symlink escapes).
	// Use the Known Documents folder when available (it may be under
	// OneDrive or another redirected location); fall back to $HOME.
	baseDir := home
	docs, err := resolveDocumentsFolder()
	if err == nil {
		// Ensure the Documents folder is under $HOME; if it is, use it
		// as the tighter containment boundary.
		rel, relErr := filepath.Rel(home, docs)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			baseDir = docs
		}
	}

	// Historical candidates: hardcoded paths that may still contain a
	// Juggernaut block even after the user's Documents folder moved to
	// OneDrive. These are always included for uninstall/doctor scanning
	// and legacy-block migration.
	historical := historicalPowerShellTargetsScoped(home)

	editions := []psEdition{
		{Name: "PowerShell 7", Exe: "pwsh.exe"},
		{Name: "Windows PowerShell 5.1", Exe: "powershell.exe"},
	}

	for _, ed := range editions {
		dr := discoverEdition(ed, defaultPSRunner)

		if dr.DiscoveryOK {
			result.EditionsDiscovered = append(result.EditionsDiscovered, ed.Name)
			allHosts := validateAndCanonicalizePath(dr.AllHosts, baseDir)
			currentHost := validateAndCanonicalizePath(dr.CurrentHost, baseDir)

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

	// If no active targets found, fall back to Known Documents paths.
	// This ensures installation can still proceed when PowerShell is
	// missing, timed out, or failed to launch.
	if len(result.ActiveTargets) == 0 {
		result.UsedFallback = true
		for _, p := range historical {
			if !containsTargetPathCI(result.ActiveTargets, p) {
				result.ActiveTargets = append(result.ActiveTargets,
					Target{Path: p, Shell: ShellPowerShell},
				)
				result.InstallTargets = append(result.InstallTargets,
					Target{Path: p, Shell: ShellPowerShell},
				)
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
	if strings.HasPrefix(s, "\xef\xbb\xbf") {
		s = s[3:]
	}

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
	if runtime.GOOS != "windows" {
		return nil
	}
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

	if runtime.GOOS != "windows" {
		paths = append(paths,
			filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"),
		)
	}
	return paths
}

// resolveDocumentsFolder uses the Windows Known Folder API (FOLDERID_Documents)
// to resolve the actual Documents path, which correctly handles OneDrive
// redirection and other custom folder locations.
var resolveDocumentsFolder = func() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
}

// SetResolveDocumentsFolderForTesting replaces the Documents folder resolver
// (for tests only).
func SetResolveDocumentsFolderForTesting(fn func() (string, error)) {
	resolveDocumentsFolder = fn
}

// ResetResolveDocumentsFolderForTesting restores the default resolver.
func ResetResolveDocumentsFolderForTesting() {
	resolveDocumentsFolder = func() (string, error) {
		return windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT)
	}
}

// resolveHistoricalTargets builds the well-known PowerShell profile paths
// under the given Documents folder.
func resolveHistoricalTargets(docs string) []string {
	return []string{
		filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(docs, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}
}

// resolveHomeDir returns the user's home directory from environment or OS.
func resolveHomeDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return home
}

// containsPathCI checks if a path exists in a list, case-insensitive on Windows.
func containsPathCI(paths []string, path string) bool {
	for _, p := range paths {
		if runtime.GOOS == "windows" {
			if strings.EqualFold(p, path) {
				return true
			}
		} else if p == path {
			return true
		}
	}
	return false
}

// deduplicatePathsCI removes duplicate paths, case-insensitive on Windows.
func deduplicatePathsCI(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var result []string
	for _, p := range paths {
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(p)
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, p)
		}
	}
	return result
}

// validateAndCanonicalizePath trims and cleans a path, returning empty string
// for invalid paths. If baseDir is non-empty, the path must be under baseDir
// (checked via filepath.Rel) to prevent path traversal attacks from a
// compromised PowerShell output.
func validateAndCanonicalizePath(p, baseDir string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if p == "." {
		return ""
	}
	if baseDir != "" {
		rel, err := filepath.Rel(baseDir, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
	}
	return p
}
