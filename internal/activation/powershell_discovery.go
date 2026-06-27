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
	"time"
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
	Edition      string
	AllHosts     string
	CurrentHost  string
	DiscoveryOK  bool
	UsedFallback bool
}

// ProfileResolverResult is the shared, authoritative result of PowerShell
// profile discovery.
type ProfileResolverResult struct {
	// ActiveTargets are the discovered profiles that PowerShell actually loads.
	ActiveTargets []Target
	// InstallTarget is the authoritative profile for installing the new
	// activation block. Typically the first AllHosts path.
	InstallTarget Target
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

// discoveryCommandRunner is the injectable interface for running PowerShell
// discovery commands. The real implementation uses exec.Command; tests inject a
// stub.
type discoveryCommandRunner interface {
	RunContext(ctx context.Context, exe string, args []string) ([]byte, error)
}

// realCommandRunner executes real shell commands.
type realCommandRunner struct{}

func (r *realCommandRunner) RunContext(ctx context.Context, exe string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
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

// SetPSRunnerForTesting replaces the default runner (for tests only).
func SetPSRunnerForTesting(r discoveryCommandRunner) {
	defaultPSRunner = r
}

// ResetPSRunnerForTesting restores the default runner.
func ResetPSRunnerForTesting() {
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

	editions := []psEdition{
		{Name: "PowerShell 7", Exe: "pwsh.exe"},
		{Name: "Windows PowerShell 5.1", Exe: "powershell.exe"},
	}

	for _, ed := range editions {
		dr := discoverEdition(ed, defaultPSRunner)

		if dr.DiscoveryOK {
			result.EditionsDiscovered = append(result.EditionsDiscovered, ed.Name)
			allHosts := validateAndCanonicalizePath(dr.AllHosts)
			currentHost := validateAndCanonicalizePath(dr.CurrentHost)

			if allHosts != "" {
				result.ActiveTargets = append(result.ActiveTargets,
					Target{Path: allHosts, Shell: ShellPowerShell},
				)
				if !containsPathCI(result.MigrationTargets, allHosts) {
					result.MigrationTargets = append(result.MigrationTargets, allHosts)
				}
			}
			// Add currentHost to ActiveTargets when it differs from allHosts.
			// This is the host-specific profile that PowerShell loads after the
			// all-hosts profile, and activation may exist only there.
			if currentHost != "" && !containsPathCI(result.MigrationTargets, currentHost) {
				result.MigrationTargets = append(result.MigrationTargets, currentHost)
				if !containsTargetPathCI(result.ActiveTargets, currentHost) {
					result.ActiveTargets = append(result.ActiveTargets,
						Target{Path: currentHost, Shell: ShellPowerShell},
					)
				}
			}
			if result.InstallTarget.Path == "" && allHosts != "" {
				result.InstallTarget = Target{Path: allHosts, Shell: ShellPowerShell}
			}
		} else {
			result.DiscoveryWarnings = append(result.DiscoveryWarnings,
				fmt.Sprintf("could not query profile paths for %s", ed.Name),
			)
		}
	}

	// If no active targets found, report a discovery failure.
	if len(result.ActiveTargets) == 0 {
		result.DiscoveryWarnings = append(result.DiscoveryWarnings,
			"PowerShell profile discovery failed; no PowerShell editions found",
		)
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
	if err != nil {
		return psDiscoveryResult{Edition: ed.Name}
	}

	return parseDiscoveryOutput(ed.Name, output)
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

// historicalPowerShellTargets returns nil. Hardcoded historical paths have
// been removed — PowerShell profile locations are discovered dynamically.
func historicalPowerShellTargets() []string {
	return nil
}

// historicalPowerShellTargetsScoped returns nil. Hardcoded historical paths
// have been removed — PowerShell profile locations are discovered dynamically.
func historicalPowerShellTargetsScoped(home string) []string {
	return nil
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
// for invalid paths.
func validateAndCanonicalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if p == "." || p == "\"\"" {
		return ""
	}
	return p
}
