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

$v2Active = ($env:JUGGERNAUT_USE_V2 -eq '1') -or $UseV2
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
    Write-Output 'Juggernaut v2 is not active. Use --v2 to enable v2 commands.'
    return
}

if ($Scope -and $Scope -notin @('user','project')) {
    Write-Error "doctor: --scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\schema.ps1')
. (Join-Path $RepoRoot 'lib\profile_writer.ps1')
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
if (-not $projectPath) { $projectPath = Join-Path (Get-Location).Path '.claude/settings.json' }

$userSettings = Read-DoctorSettingsOrNull -Path $userPath
$projectSettings = Read-DoctorSettingsOrNull -Path $projectPath

$userHasBlock = $userSettings -and (Test-HasJuggernautBlock -Settings $userSettings)
$projectHasBlock = $projectSettings -and (Test-HasJuggernautBlock -Settings $projectSettings)

$activeScope = ''
if ($projectHasBlock) { $activeScope = 'project' }
elseif ($userHasBlock) { $activeScope = 'user' }

$profilePath = Get-DoctorProfilePath

Write-Output 'Juggernaut doctor'
Write-Output ''
if ($activeScope) {
    Write-Output "Active scope: $activeScope"
} else {
    Write-Output 'Active scope: none  (no Juggernaut v2 block found)'
    $script:DoctorFails += 1
}
if ($Scope) {
    Write-Output "Showing:      $Scope scope (explicitly selected)"
}

Write-Output ''
Invoke-DoctorScopeCheck -Scope 'user' -Path $userPath -Settings $userSettings -Active:($activeScope -eq 'user') -Selected:($Scope -eq 'user') -ProfilePath $profilePath
Write-Output ''
Invoke-DoctorScopeCheck -Scope 'project' -Path $projectPath -Settings $projectSettings -Active:($activeScope -eq 'project') -Selected:($Scope -eq 'project') -ProfilePath $profilePath
Write-DoctorSummary
if ($script:DoctorFails -gt 0) { exit 1 }
