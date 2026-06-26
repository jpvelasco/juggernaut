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
	"unsafe"

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
	Edition      string
	AllHosts     string
	CurrentHost  string
	DiscoveryOK  bool
	UsedFallback bool
}

// ProfileResolverResult is the shared, authoritative result of PowerShell
// profile discovery. It separates active installation targets from historical
// cleanup candidates.
type ProfileResolverResult struct {
	// ActiveTargets are the discovered profiles that PowerShell actually loads.
	ActiveTargets []Target
	// InstallTarget is the authoritative profile for installing the new
	// activation block. Typically the first AllHosts path.
	InstallTarget Target
	// MigrationTargets are all discovered profiles that should be inspected
	// for legacy blocks during apply/migration.
	MigrationTargets []string
	// HistoricalCandidates are hardcoded paths from older Juggernaut versions
	// that PowerShell may no longer load. These are cleanup-only targets.
	HistoricalCandidates []string
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
	result := ProfileResolverResult{}

	editions := []psEdition{
		{Name: "PowerShell 7", Exe: "pwsh.exe"},
		{Name: "Windows PowerShell 5.1", Exe: "powershell.exe"},
	}

	for _, ed := range editions {
		dr := discoverEdition(ed, defaultPSRunner)
		result.EditionsDiscovered = append(result.EditionsDiscovered, ed.Name)

		if dr.DiscoveryOK {
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
			if currentHost != "" && !containsPathCI(result.MigrationTargets, currentHost) {
				result.MigrationTargets = append(result.MigrationTargets, currentHost)
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

	// Add historical hardcoded paths as cleanup candidates.
	result.HistoricalCandidates = historicalPowerShellTargets()

	// If no active targets found, fall back to Known Documents folder.
	if len(result.ActiveTargets) == 0 {
		fallback := fallbackKnownDocumentsPowerShell()
		if fallback != "" {
			result.InstallTarget = Target{Path: fallback, Shell: ShellPowerShell}
			result.ActiveTargets = append(result.ActiveTargets, Target{Path: fallback, Shell: ShellPowerShell})
			result.MigrationTargets = append(result.MigrationTargets, fallback)
			result.UsedFallback = true
			result.DiscoveryWarnings = append(result.DiscoveryWarnings,
				"PowerShell profile discovery failed; using Known Documents fallback",
			)
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

// historicalPowerShellTargets returns the hardcoded PowerShell profile paths
// used by older Juggernaut versions. These are cleanup candidates only.
func historicalPowerShellTargets() []string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}
}

// fallbackKnownDocumentsPowerShell returns the PowerShell profile path using
// the Windows Known Documents folder API (SHGetKnownFolderPath).
func fallbackKnownDocumentsPowerShell() string {
	documentsPath, err := getKnownDocumentsPath()
	if err != nil {
		return ""
	}
	return filepath.Join(documentsPath, "PowerShell", "Microsoft.PowerShell_profile.ps1")
}

// getKnownDocumentsPath uses the Windows Known Folder API to resolve the
// actual Documents folder, which handles OneDrive redirection correctly.
func getKnownDocumentsPath() (string, error) {
	// FOLDERID_Documents = {FDD39AD0-238F-46AF-ADB4-6C85480369C7}
	var folderID windows.GUID = windows.GUID{
		Data1: 0xFDD39AD0,
		Data2: 0x238F,
		Data3: 0x46AF,
		Data4: [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7},
	}

	shell32 := windows.MustLoadDLL("shell32.dll")
	proc := shell32.MustFindProc("SHGetKnownFolderPath")

	var pathPtr *uint16
	ret, _, _ := proc.Call(
		uintptr(unsafe.Pointer(&folderID)),
		0, // KNOWN_FOLDER_FLAG
		0, // hToken
		uintptr(unsafe.Pointer(&pathPtr)),
	)
	if ret != 0 {
		return "", fmt.Errorf("SHGetKnownFolderPath failed with code 0x%x", ret)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(pathPtr))

	path := windows.UTF16PtrToString(pathPtr)
	return path, nil
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
