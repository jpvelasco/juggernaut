# commands/uninstall.ps1 - Juggernaut v3 uninstall subcommand.

[CmdletBinding(PositionalBinding=$false)]
param(
    [string]$Scope = '',
    [switch]$DryRun,
    [Alias('f')][switch]$Force,
    [switch]$Full,
    [Alias('y')][switch]$Yes,
    [switch]$Help,
    [switch]$Version,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$RemainingArgs
)

$ErrorActionPreference = 'Stop'
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$env:BEDROCK_CONFIG_PATH = if ($env:BEDROCK_CONFIG_PATH) { $env:BEDROCK_CONFIG_PATH } else { Join-Path $RepoRoot 'bedrock-config.json' }

foreach ($arg in $RemainingArgs) {
    switch -Regex ($arg) {
        '^--dry-run$'              { $DryRun = $true }
        '^--force$|^-f$'           { $Force = $true }
        '^--full$'                 { $Full = $true }
        '^--yes$|^-y$'             { $Yes = $true; $Force = $true }
        '^--scope=(user|project)$' { $Scope = $Matches[1] }
        '^--scope='                { Write-Error "uninstall: --scope must be 'user' or 'project' (got: '$($arg.Substring(8))')"; exit 1 }
        '^--help$|^-h$'            { $Help = $true }
        '^--version$|^-v$'         { $Version = $true }
        default                    { Write-Error "uninstall: unknown option '$arg'"; exit 1 }
    }
}

if ($Help) {
    @'
juggernaut uninstall - remove Juggernaut configuration

Usage:
  juggernaut.ps1 uninstall [-Scope user|project] [-DryRun] [-Force]
  juggernaut.ps1 uninstall -Full [-DryRun] [-Yes]

Options:
  -Scope user|project  Limit removal to one scope (default: all scopes with a block)
  -DryRun              Preview changes without writing files
  -Force               Skip confirmation prompt
  -Full                Also remove the Juggernaut command shims and install tree
  -Yes                 Confirm full removal without prompting

Removes the Juggernaut block from settings.json, the keychain entry, and
the launcher profile block from `$PROFILE.CurrentUserCurrentHost` (plus
the sibling host's profile when present). The installer-wipe regex is
the only place Juggernaut strips legacy profile blocks from earlier
versions - run `install.ps1` for that.

Examples:
  juggernaut uninstall --dry-run
  juggernaut uninstall --force
  juggernaut uninstall --full --dry-run
  juggernaut uninstall --full --yes
'@
    exit 0
}

if ($Version) {
    $vf = Join-Path $RepoRoot 'VERSION'
    if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
    exit 0
}

if ($Scope -and $Scope -notin @('user', 'project')) {
    Write-Error "uninstall: --scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\keychain.ps1')

# ---------------------------------------------------------------------------
# Detect what's installed
# ---------------------------------------------------------------------------
$userPath    = Get-UserSettingsPath
$projectPath = Join-Path (Get-Location).Path '.claude\settings.json'

$hasUser = $false; $hasProject = $false

if (Test-Path $userPath) {
    $j = try { Read-Settings -Path $userPath } catch { $null }
    if ($j -and (Test-HasJuggernautBlock -Settings $j)) { $hasUser = $true }
}
if (Test-Path $projectPath) {
    $j = try { Read-Settings -Path $projectPath } catch { $null }
    if ($j -and (Test-HasJuggernautBlock -Settings $j)) { $hasProject = $true }
}

if ($Scope -eq 'user')    { $hasProject = $false }
if ($Scope -eq 'project') { $hasUser    = $false }

$hasKeychain = $false
if (($hasUser -or $hasProject) -and (Test-KeychainAvailable)) {
    $kv = try { Get-KeychainEntry } catch { $null }
    if ($kv) { $hasKeychain = $true }
}

$hasDpapi = $false
$dpapiPath = ''
if ((Get-KeychainOS) -eq 'windows') {
    $dpapiPath = Get-DPAPIEntryPath
    if (Test-Path $dpapiPath) { $hasDpapi = $true }
}

$hasProfileToken = $false
$profileTokenPath = Get-ProfileTokenPath
if (Test-Path $profileTokenPath) { $hasProfileToken = $true }

function Get-LauncherProfileTargets {
    $targets = @()
    try {
        if ($PROFILE -and $PROFILE.CurrentUserCurrentHost) {
            $targets += [string]$PROFILE.CurrentUserCurrentHost
            $curr = [string]$PROFILE.CurrentUserCurrentHost
            if ($curr -match '\\WindowsPowerShell\\') {
                $targets += ($curr -replace '\\WindowsPowerShell\\', '\PowerShell\')
            } elseif ($curr -match '\\PowerShell\\') {
                $targets += ($curr -replace '\\PowerShell\\', '\WindowsPowerShell\')
            }
        }
    } catch {}
    return @($targets | Where-Object { $_ } | Select-Object -Unique)
}

function Test-ProfileHasLauncherBlock([string]$Path) {
    if (-not (Test-Path $Path)) { return $false }
    try {
        $content = Get-Content -Path $Path -Raw -ErrorAction Stop
    } catch { return $false }
    return ($content -match '(?m)^# BEGIN: Juggernaut Launcher')
}

$launcherProfiles = @(Get-LauncherProfileTargets | Where-Object { Test-ProfileHasLauncherBlock $_ })
$hasLauncher = $launcherProfiles.Count -gt 0

$fullInstallDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } else { Join-Path $HOME '.juggernaut' }
$fullShimDir = Join-Path $HOME '.local\bin'
$fullShimPaths = @(
    (Join-Path $fullShimDir 'juggernaut.cmd'),
    (Join-Path $fullShimDir 'juggernaut.ps1'),
    (Join-Path $fullShimDir 'juggernaut-install-dir.txt')
)
$fullExistingShimPaths = @()
$hasFullInstallDir = $false
if ($Full) {
    $fullExistingShimPaths = @($fullShimPaths | Where-Object { Test-Path $_ })
    $hasFullInstallDir = Test-Path $fullInstallDir
}

if (-not ($hasUser -or $hasProject -or $hasKeychain -or $hasDpapi -or $hasProfileToken -or $hasLauncher -or $fullExistingShimPaths.Count -gt 0 -or $hasFullInstallDir)) {
    Write-Output 'Nothing to uninstall.'
    exit 0
}

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------
if (-not $DryRun -and ((-not $Force) -or ($Full -and -not $Yes))) {
    if ($Full) {
        Write-Output 'Warning: full uninstall permanently deletes your Juggernaut installation, stored tokens, and configuration.'
        Write-Output 'This action cannot be undone.'
        Write-Output ''
    }
    Write-Output 'The following will be removed:'
    if ($hasUser)        { Write-Output "  - Juggernaut block from $userPath" }
    if ($hasProject)     { Write-Output "  - Juggernaut block from $projectPath" }
    if ($hasKeychain)    { Write-Output "  - Keychain entry: $($script:KeychainService)/$($script:KeychainAccount)" }
    if ($hasDpapi)       { Write-Output "  - DPAPI file: $dpapiPath" }
    if ($hasProfileToken) { Write-Output "  - Profile token file: $profileTokenPath" }
    foreach ($lp in $launcherProfiles) { Write-Output "  - Launcher block from $lp" }
    foreach ($shim in $fullExistingShimPaths) { Write-Output "  - Juggernaut command shim: $shim" }
    if ($hasFullInstallDir) { Write-Output "  - Juggernaut install directory: $fullInstallDir" }
    Write-Output ''
    $answer = Read-Host 'Proceed? [y/N]'
    if ($answer -notmatch '^[Yy]') { Write-Output 'Aborted.'; exit 0 }
}

# ---------------------------------------------------------------------------
# Execute
# ---------------------------------------------------------------------------
function Remove-SettingsBlock([string]$Path) {
    $json    = Read-Settings -Path $Path
    $cleaned = Remove-JuggernautBlockFromSettings -Existing $json
    Write-SettingsAtomic -Path $Path -Content $cleaned
    Write-Output "Removed Juggernaut block from $($Path.Replace('\', '/'))"
}

if ($hasUser) {
    if ($DryRun) { Write-Output "[dry-run] Would remove Juggernaut block from $($userPath.Replace('\', '/'))" }
    else         { Remove-SettingsBlock $userPath }
}
if ($hasProject) {
    if ($DryRun) { Write-Output "[dry-run] Would remove Juggernaut block from $($projectPath.Replace('\', '/'))" }
    else         { Remove-SettingsBlock $projectPath }
}
if ($hasKeychain) {
    if ($DryRun) { Write-Output "[dry-run] Would remove keychain entry: $($script:KeychainService)/$($script:KeychainAccount)" }
    else         { Remove-KeychainEntry; Write-Output "Removed keychain entry: $($script:KeychainService)/$($script:KeychainAccount)" }
}
if ($hasDpapi) {
    if ($DryRun) { Write-Output "[dry-run] Would remove DPAPI file: $dpapiPath" }
    else         { Remove-DPAPIEntry; Write-Output "Removed DPAPI file: $dpapiPath" }
}
if ($hasProfileToken) {
    if ($DryRun) { Write-Output "[dry-run] Would remove profile token file: $profileTokenPath" }
    else         { Remove-ProfileTokenEntry; Write-Output "Removed profile token file: $profileTokenPath" }
}

function Remove-LauncherProfileBlock([string]$Path) {
    if (-not (Test-Path $Path)) { return }
    try {
        $lines = Get-Content -Path $Path -ErrorAction Stop
    } catch {
        Write-Warning "Could not read $Path - $($_.Exception.Message). Skipping (may require elevation)."
        return
    }
    $out = New-Object System.Collections.Generic.List[string]
    $skip = $false
    foreach ($line in $lines) {
        if ($line -match '^# BEGIN: Juggernaut Launcher') { $skip = $true; continue }
        if ($line -match '^# END: Juggernaut Launcher')   { $skip = $false; continue }
        if (-not $skip) { $out.Add($line) }
    }
    # Trim a trailing blank line that the install step added as a separator.
    while ($out.Count -gt 0 -and $out[$out.Count - 1] -eq '') {
        $out.RemoveAt($out.Count - 1)
    }
    try {
        Set-Content -Path $Path -Value $out -Encoding utf8 -ErrorAction Stop
        Write-Output "Removed launcher block from $Path"
    } catch {
        Write-Warning "Could not write $Path - $($_.Exception.Message). Skipping (may require elevation)."
    }
}

foreach ($lp in $launcherProfiles) {
    if ($DryRun) { Write-Output "[dry-run] Would remove launcher block from $lp" }
    else         { Remove-LauncherProfileBlock $lp }
}

if ($Full) {
    foreach ($shim in $fullExistingShimPaths) {
        if ($DryRun) {
            Write-Output "[dry-run] Would remove Juggernaut command shim: $shim"
        } else {
            Remove-Item -LiteralPath $shim -Force -ErrorAction SilentlyContinue
            Write-Output "Removed Juggernaut command shim: $shim"
        }
    }

    if ($hasFullInstallDir) {
        if ($DryRun) {
            Write-Output "[dry-run] Would remove Juggernaut install directory: $fullInstallDir"
        } else {
            Remove-Item -LiteralPath $fullInstallDir -Recurse -Force -ErrorAction SilentlyContinue
            Write-Output "Removed Juggernaut install directory: $fullInstallDir"
        }
    }
}

if ($DryRun) {
    Write-Output 'No files were changed.'
} else {
    Write-Output ''
    if ($Full) {
        Write-Output 'Full uninstall complete.'
    } else {
        Write-Output 'Uninstall complete.'
    }
}
