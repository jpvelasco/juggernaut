# commands/uninstall.ps1 - Juggernaut v3 uninstall subcommand.

[CmdletBinding(PositionalBinding=$false)]
param(
    [string]$Scope = '',
    [switch]$DryRun,
    [Alias('f')][switch]$Force,
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

Usage: juggernaut.ps1 uninstall [-Scope user|project] [-DryRun] [-Force]

Options:
  -Scope user|project  Limit removal to one scope (default: all scopes with a block)
  -DryRun              Preview changes without writing files
  -Force               Skip confirmation prompt

Removes the Juggernaut block from settings.json and the keychain entry.
Shell-profile blocks are not touched here; in v3 Juggernaut does not write
to shell profiles. Run the installer (install.ps1) for a full wipe that
includes legacy profile blocks from earlier versions.
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

if (-not ($hasUser -or $hasProject -or $hasKeychain)) {
    Write-Output 'Nothing to uninstall.'
    exit 0
}

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------
if (-not $Force -and -not $DryRun) {
    Write-Output 'The following will be removed:'
    if ($hasUser)    { Write-Output "  - Juggernaut block from $userPath" }
    if ($hasProject) { Write-Output "  - Juggernaut block from $projectPath" }
    if ($hasKeychain) { Write-Output "  - Keychain entry: $($script:KeychainService)/$($script:KeychainAccount)" }
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

if ($DryRun) {
    Write-Output 'No files were changed.'
} else {
    Write-Output ''
    Write-Output 'Uninstall complete.'
}
