# commands/doctor.ps1 - Juggernaut v2 doctor subcommand.

[CmdletBinding(PositionalBinding=$false)]
param(
    [string]$Scope = '',
    [Alias('v2')][switch]$UseV2,
    [switch]$Help,
    [switch]$Version,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$RemainingArgs
)

$ErrorActionPreference = 'Stop'

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$env:BEDROCK_CONFIG_PATH = if ($env:BEDROCK_CONFIG_PATH) { $env:BEDROCK_CONFIG_PATH } else { Join-Path $RepoRoot 'bedrock-config.json' }

$v2Active = if ($env:JUGGERNAUT_USE_V2 -eq '0') { $false } else { $true }
if ($UseV2) { $v2Active = $true }
foreach ($arg in $RemainingArgs) {
    switch -Regex ($arg) {
        '^--v2$' { $v2Active = $true; break }
        '^--scope=(user|project)$' { $Scope = $Matches[1]; break }
        '^--scope=' { throw "doctor: --scope must be 'user' or 'project' (got: '$($arg.Substring(8))')" }
        '^--help$' { $Help = $true; break }
        '^-h$' { $Help = $true; break }
        '^--version$' { $Version = $true; break }
        '^-v$' { $Version = $true; break }
    }
}

if ($Help) {
    @'
juggernaut doctor - check Juggernaut v2 configuration

Usage: juggernaut.ps1 doctor [-Scope user|project]

Checks user and project settings.json files, credentials, model/region settings,
Mantle status, and drift between settings.json and the shell fallback.
'@
    return
}

if ($Version) {
    $vf = Join-Path $RepoRoot 'VERSION'
    if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
    return
}

if (-not $v2Active) {
    Write-Error "juggernaut: invoke via the 'juggernaut' dispatcher (or set JUGGERNAUT_USE_V2=1)."
    exit 2
}

if ($Scope -and $Scope -notin @('user','project')) {
    Write-Error "doctor: --scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\schema.ps1')
. (Join-Path $RepoRoot 'lib\profile_writer.ps1')
. (Join-Path $RepoRoot 'lib\profile_paths.ps1')
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
    Write-Output 'none (no Juggernaut v2 block found)'
}

# -- v1 Artifacts --------------------------------------------------------------
Write-Output ''
Write-Output 'v1 Artifacts'
Write-DoctorV1Artifacts -Settings $userSettings -HasV2Block ([bool]$userHasBlock)

# Resolve which block to use for the detailed checks below.
$checkScope = if ($Scope) { $Scope } else { $activeScope }
$checkSettings = $null
if ($checkScope -eq 'user') { $checkSettings = $userSettings }
elseif ($checkScope -eq 'project') { $checkSettings = $projectSettings }

if ($checkSettings -and (Test-HasJuggernautBlock -Settings $checkSettings)) {
    $checkBlock = Get-JuggernautBlockFromSettings -Settings $checkSettings
    $profilePath = Get-DoctorProfilePath -Block $checkBlock

    # -- Credentials ------------------------------------------------------------
    Write-Output ''
    Write-Output 'Credentials'
    Write-DoctorCredentials -Block $checkBlock -ProfilePath $profilePath

    # -- Region & Models --------------------------------------------------------
    Write-Output ''
    Write-Output 'Region & Models'
    Write-DoctorRegionModels -Block $checkBlock

    # -- Mantle -----------------------------------------------------------------
    Write-Output ''
    Write-Output 'Mantle'
    Write-DoctorMantle -Block $checkBlock

    # -- Drift ------------------------------------------------------------------
    Write-Output ''
    Write-Output 'Drift'
    Write-DoctorDrift -Settings $checkSettings -Block $checkBlock -ProfilePath $profilePath
}

Write-DoctorSummary
if ($script:DoctorFails -gt 0) { exit 1 }
