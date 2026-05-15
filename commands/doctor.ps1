# commands/doctor.ps1 - Juggernaut v3 doctor subcommand.

[CmdletBinding(PositionalBinding=$false)]
param(
    [string]$Scope = '',
    [switch]$Help,
    [switch]$Version,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$RemainingArgs
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$env:BEDROCK_CONFIG_PATH = if ($env:BEDROCK_CONFIG_PATH) { $env:BEDROCK_CONFIG_PATH } else { Join-Path $RepoRoot 'bedrock-config.json' }

foreach ($arg in $RemainingArgs) {
    switch -Regex ($arg) {
        '^--scope=(user|project)$' { $Scope = $Matches[1]; break }
        '^--scope=' { Write-Error "doctor: --scope must be 'user' or 'project' (got: '$($arg.Substring(8))')"; exit 1 }
        '^--help$' { $Help = $true; break }
        '^-h$' { $Help = $true; break }
        '^--version$' { $Version = $true; break }
        '^-v$' { $Version = $true; break }
        default { Write-Error "doctor: unknown option '$arg'`nRun 'juggernaut.ps1 doctor --help' for usage."; exit 1 }
    }
}

if ($Help) {
    @'
juggernaut doctor - check Juggernaut v3 configuration

Usage: juggernaut.ps1 doctor [-Scope user|project]

Checks user and project settings.json files, credentials, model/region
settings, Mantle status, and opusplan wiring.
'@
    return
}

if ($Version) {
    $vf = Join-Path $RepoRoot 'VERSION'
    if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
    return
}

if ($Scope -and $Scope -notin @('user','project')) {
    Write-Error "doctor: --scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\schema.ps1')
. (Join-Path $RepoRoot 'lib\keychain.ps1')
. (Join-Path $RepoRoot 'lib\doctor.ps1')

function Read-DoctorSettingsOrNull {
    param([Parameter(Mandatory)][string]$Path)
    if (-not (Test-Path $Path)) { return $null }
    try { return Read-Settings -Path $Path }
    catch { return $null }
}

$userPath = Get-UserSettingsPath
$projectPath = Get-ProjectSettingsPath
$displayProjectPath = if ($projectPath) { $projectPath } else { Join-Path (Get-Location).Path '.claude/settings.json' }

$userSettings = Read-DoctorSettingsOrNull -Path $userPath
$projectSettings = if ($projectPath) { Read-DoctorSettingsOrNull -Path $projectPath } else { $null }

$userHasBlock = $userSettings -and (Test-HasJuggernautBlock -Settings $userSettings)
$projectHasBlock = $projectSettings -and (Test-HasJuggernautBlock -Settings $projectSettings)

$activeScope = ''
if ($projectHasBlock) { $activeScope = 'project' }
elseif ($userHasBlock) { $activeScope = 'user' }

Write-Output 'Juggernaut doctor'

# -- User Scope ----------------------------------------------------------------
Write-Output ''
Write-Output 'User Scope'
Write-DoctorScopeBlock -Path $userPath -Settings $userSettings

# -- Project Scope -------------------------------------------------------------
Write-Output ''
Write-Output 'Project Scope'
if ($projectPath) {
    Write-DoctorScopeBlock -Path $displayProjectPath -Settings $projectSettings
} else {
    Write-Output (Show-DoctorHomePath $displayProjectPath)
    Write-Output 'Status: not found'
}

# -- Active Scope --------------------------------------------------------------
Write-Output ''
Write-Output 'Active Scope'
if ($activeScope) {
    Write-Output $activeScope
} else {
    $script:DoctorFails += 1
    Write-Output 'none (no Juggernaut block found)'
}

# Resolve which block to use for the detailed checks below.
$checkScope = if ($Scope) { $Scope } else { $activeScope }
$checkSettings = $null
if ($checkScope -eq 'user') { $checkSettings = $userSettings }
elseif ($checkScope -eq 'project') { $checkSettings = $projectSettings }

if ($checkSettings -and (Test-HasJuggernautBlock -Settings $checkSettings)) {
    $checkBlock = Get-JuggernautBlockFromSettings -Settings $checkSettings

    # -- Credentials ------------------------------------------------------------
    Write-Output ''
    Write-Output 'Credentials'
    Write-DoctorCredentials -Block $checkBlock

    # -- Region & Models --------------------------------------------------------
    Write-Output ''
    Write-Output 'Region & Models'
    Write-DoctorRegionModels -Block $checkBlock
    Write-DoctorTopLevelModel -Settings $checkSettings

    # -- Mantle -----------------------------------------------------------------
    Write-Output ''
    Write-Output 'Mantle'
    Write-DoctorMantle -Block $checkBlock

    # -- Opusplan ---------------------------------------------------------------
    Write-Output ''
    Write-Output 'Opusplan'
    Write-DoctorOpusplan -Settings $checkSettings -Block $checkBlock

    # -- Launcher ---------------------------------------------------------------
    Write-Output ''
    Write-Output 'Launcher'
    Write-DoctorLauncher -Block $checkBlock
} elseif ($checkSettings) {
    # Settings.json exists but has no valid juggernaut block — check for drift.
    $driftPath = if ($checkScope -eq 'user') { $userPath } elseif ($checkScope -eq 'project') { $projectPath } else { '' }
    if ($driftPath) {
        Write-Output ''
        Write-Output 'Settings Drift'
        Write-DoctorSettingsDrift -Scope $checkScope -Settings $checkSettings -Path $driftPath
    }
}

Write-DoctorSummary
if ($script:DoctorFails -gt 0) { exit 1 }
