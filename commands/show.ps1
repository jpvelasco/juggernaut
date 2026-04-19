# commands/show.ps1 — Juggernaut v2 show subcommand.

param(
    [switch]$Help,
    [switch]$Version
)

$ErrorActionPreference = 'Stop'

$v2Active = $env:JUGGERNAUT_USE_V2 -eq '1'
$remaining = @()
foreach ($arg in $args) {
    switch ($arg) {
        '--v2'      { $v2Active = $true }
        '--help'    { $Help = $true }
        '-h'        { $Help = $true }
        '--version' { $Version = $true }
        '-v'        { $Version = $true }
        default     { $remaining += $arg }
    }
}
$args = $remaining

if ($Help) {
    @'
juggernaut show — print the current Juggernaut configuration

Usage: juggernaut.ps1 show

Displays the active Juggernaut block plus a side-by-side summary of user and
project scope settings when present.
'@
    exit 0
}

if ($Version) {
    $repoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
    $vf = Join-Path $repoRoot 'VERSION'
    if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
    exit 0
}

if (-not $v2Active) {
    Write-Host "show: v2 is not active. Set `$env:JUGGERNAUT_USE_V2 = '1' or pass --v2 to ./setup."
    exit 0
}

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
. (Join-Path $RepoRoot 'lib\config_manager.ps1')

function Show-Value {
    param([AllowNull()]$Value)
    if ($null -eq $Value -or $Value -eq '') { return '—' }
    return [string]$Value
}

function Show-Bool {
    param([AllowNull()]$Value)
    if ($Value -is [bool]) { return $(if ($Value) { 'yes' } else { 'no' }) }
    if ($Value -eq 'true') { return 'yes' }
    if ($Value -eq 'false') { return 'no' }
    return '—'
}

function Get-ShowBlock {
    param([AllowNull()]$Block)
    if (-not $Block) { return $null }
    [ordered]@{
        AuthMode   = $Block.auth.mode
        Region     = $Block.auth.region
        Storage    = $Block.auth.storage
        Model      = $Block.model
        Effort     = $Block.effortLevel
        UseMantle  = [bool]$Block.useMantle
        MantleUrl  = $Block.mantle.baseUrl
        LastUpdated = $Block.meta.lastUpdated
        OpusPlan   = $Block.opusplan
    }
}

function Show-Block {
    param(
        [Parameter(Mandatory)][string]$Title,
        [Parameter(Mandatory)][string]$Path,
        [AllowNull()]$Block
    )

    Write-Host $Title
    if (-not $Block) {
        Write-Host '  not present'
        return
    }

    $view = Get-ShowBlock -Block $Block
    Write-Host "  File: $Path"
    Write-Host ("  Auth: {0}" -f (Show-Value $view.AuthMode))
    Write-Host ("  Region: {0}" -f (Show-Value $view.Region))
    Write-Host ("  Storage: {0}" -f (Show-Value $view.Storage))
    Write-Host ("  Model: {0}" -f (Show-Value $view.Model))
    Write-Host ("  Effort: {0}" -f (Show-Value $view.Effort))
    Write-Host ("  Opus plan: {0}" -f (Show-Value $view.OpusPlan))
    Write-Host ("  Mantle: {0}" -f (Show-Bool $view.UseMantle))
    if ($view.MantleUrl)  { Write-Host ("  Mantle URL: {0}" -f $view.MantleUrl) }
    if ($view.LastUpdated){ Write-Host ("  Last updated: {0}" -f $view.LastUpdated) }
}

function Show-SummaryRow {
    param(
        [Parameter(Mandatory)][string]$Scope,
        [AllowNull()]$Block
    )

    if (-not $Block) {
        Write-Host ("  {0,-8} {1,-7} {2,-12} {3,-9} {4,-36} {5,-7} {6,-7}" -f $Scope, '—', '—', '—', '—', '—', '—')
        return
    }

    $view = Get-ShowBlock -Block $Block
    Write-Host ("  {0,-8} {1,-7} {2,-12} {3,-9} {4,-36} {5,-7} {6,-7}" -f `
        $Scope,
        (Show-Value $view.AuthMode),
        (Show-Value $view.Region),
        (Show-Value $view.Storage),
        (Show-Value $view.Model),
        (Show-Value $view.Effort),
        (Show-Bool $view.UseMantle))
}

$effective = Get-EffectiveSettings
$userPath = $effective.user.path
$userBlock = if ($effective.user.settings -and (Test-HasJuggernautBlock -Settings $effective.user.settings)) {
    $effective.user.settings['juggernaut']
} else { $null }

$projectPath = ''
$projectBlock = $null
if ($effective.project) {
    $projectPath = $effective.project.path
    if ($effective.project.settings -and (Test-HasJuggernautBlock -Settings $effective.project.settings)) {
        $projectBlock = $effective.project.settings['juggernaut']
    }
}

$activePath = '—'
$activeBlock = $null
if ($projectBlock) {
    $activePath = $projectPath
    $activeBlock = $projectBlock
} elseif ($userBlock) {
    $activePath = $userPath
    $activeBlock = $userBlock
}

Write-Host 'Juggernaut show'
Write-Host ''
Show-Block -Title 'Current block' -Path $activePath -Block $activeBlock
Write-Host ''
Write-Host 'Effective config'
Write-Host ("  {0,-8} {1,-7} {2,-12} {3,-9} {4,-36} {5,-7} {6,-7}" -f 'Scope', 'Auth', 'Region', 'Storage', 'Model', 'Effort', 'Mantle')
Show-SummaryRow -Scope 'User' -Block $userBlock
if ($effective.project) {
    Show-SummaryRow -Scope 'Project' -Block $projectBlock
}
