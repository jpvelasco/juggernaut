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

Displays the current Juggernaut block, the effective user/project scopes, and
shell fallback details when present.
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
    Write-Host 'show: v2 is not active yet. Run ./setup --v2 or set JUGGERNAUT_USE_V2=1 to continue.'
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

function Show-Kv {
    param(
        [int]$Indent = 0,
        [Parameter(Mandatory)][string]$Label,
        [AllowNull()]$Value
    )
    $prefix = ' ' * $Indent
    Write-Host ("{0}{1,-20} {2}" -f $prefix, $Label, (Show-Value $Value))
}

function Get-ShowBlock {
    param([AllowNull()]$Block)
    if (-not $Block) { return $null }

    $profiles = @()
    if ($Block.shellFallback.lastWrittenProfiles) {
        $profiles = @($Block.shellFallback.lastWrittenProfiles)
    }

    [ordered]@{
        Scope         = $Block.meta.scope
        Version       = $Block.meta.version
        AuthMode      = $Block.auth.mode
        Region        = $Block.auth.region
        Storage       = $Block.auth.storage
        Model         = $Block.model
        Effort        = $Block.effortLevel
        UseMantle     = [bool]$Block.useMantle
        MantleUrl     = $Block.mantle.baseUrl
        LastUpdated   = $Block.meta.lastUpdated
        OpusPlan      = $Block.opusplan
        ShellEnabled  = [bool]$Block.shellFallback.enabled
        ShellMode     = $Block.shellFallback.mode
        ShellProfiles = $profiles
    }
}

function Show-CurrentBlock {
    param(
        [Parameter(Mandatory)][string]$Path,
        [AllowNull()]$Block
    )

    if (-not $Block) {
        Show-Kv -Indent 2 -Label 'Status' -Value 'no active Juggernaut block'
        return
    }

    $view = Get-ShowBlock -Block $Block
    Show-Kv -Indent 2 -Label 'File' -Value $Path
    Show-Kv -Indent 2 -Label 'Scope' -Value $view.Scope
    Show-Kv -Indent 2 -Label 'Version' -Value $view.Version
    Show-Kv -Indent 2 -Label 'Auth mode' -Value $view.AuthMode
    Show-Kv -Indent 2 -Label 'Region' -Value $view.Region
    Show-Kv -Indent 2 -Label 'Storage' -Value $view.Storage
    Show-Kv -Indent 2 -Label 'Model' -Value $view.Model
    Show-Kv -Indent 2 -Label 'Effort level' -Value $view.Effort
    Show-Kv -Indent 2 -Label 'Opus plan' -Value (Show-Bool $view.OpusPlan)
    Show-Kv -Indent 2 -Label 'Mantle' -Value (Show-Bool $view.UseMantle)
    if ($view.MantleUrl)   { Show-Kv -Indent 2 -Label 'Mantle URL' -Value $view.MantleUrl }
    if ($view.LastUpdated) { Show-Kv -Indent 2 -Label 'Last updated' -Value $view.LastUpdated }
}

function Show-ScopeSection {
    param(
        [Parameter(Mandatory)][string]$Scope,
        [Parameter(Mandatory)][string]$Path,
        [AllowNull()]$Block
    )

    Write-Host "  $Scope"
    if (-not $Block) {
        Show-Kv -Indent 4 -Label 'Status' -Value 'not configured'
        return
    }

    $view = Get-ShowBlock -Block $Block
    Show-Kv -Indent 4 -Label 'File' -Value $Path
    Show-Kv -Indent 4 -Label 'Auth mode' -Value $view.AuthMode
    Show-Kv -Indent 4 -Label 'Region' -Value $view.Region
    Show-Kv -Indent 4 -Label 'Storage' -Value $view.Storage
    Show-Kv -Indent 4 -Label 'Model' -Value $view.Model
    Show-Kv -Indent 4 -Label 'Effort level' -Value $view.Effort
    Show-Kv -Indent 4 -Label 'Mantle' -Value (Show-Bool $view.UseMantle)
}

function Show-ShellFallback {
    param([AllowNull()]$Block)
    if (-not $Block) { return }

    $view = Get-ShowBlock -Block $Block
    if (-not $view.ShellEnabled -and $view.ShellProfiles.Count -eq 0) { return }

    Write-Host 'Shell fallback'
    Show-Kv -Indent 2 -Label 'Enabled' -Value (Show-Bool $view.ShellEnabled)
    Show-Kv -Indent 2 -Label 'Mode' -Value $view.ShellMode

    if ($view.ShellProfiles.Count -eq 0) {
        Show-Kv -Indent 2 -Label 'Last written profiles' -Value 'none recorded'
        return
    }

    Show-Kv -Indent 2 -Label 'Last written profiles' -Value ("{0} item(s)" -f $view.ShellProfiles.Count)
    foreach ($profile in $view.ShellProfiles) {
        Show-Kv -Indent 4 -Label '-' -Value $profile
    }
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
Write-Host 'Current Juggernaut block'
Show-CurrentBlock -Path $activePath -Block $activeBlock
Write-Host ''
Write-Host 'Effective config'
Show-ScopeSection -Scope 'User scope' -Path $userPath -Block $userBlock
if ($effective.project) {
    Show-ScopeSection -Scope 'Project scope' -Path $projectPath -Block $projectBlock
}
if ($activeBlock) {
    Write-Host ''
    Show-ShellFallback -Block $activeBlock
}
