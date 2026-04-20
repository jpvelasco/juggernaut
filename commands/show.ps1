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
    return
}

if ($Version) {
    $repoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
    $vf = Join-Path $repoRoot 'VERSION'
    if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
    return
}

if (-not $v2Active) {
    Write-Output 'Juggernaut v2 is not active. Use --v2 to enable v2 commands.'
    return
}

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\profile_writer.ps1')

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

function Show-State {
    param([AllowNull()]$Value)
    if ($Value -is [bool]) { return $(if ($Value) { 'enabled' } else { 'disabled' }) }
    if ($Value -eq 'true') { return 'enabled' }
    if ($Value -eq 'false') { return 'disabled' }
    return '—'
}

function Show-Kv {
    param(
        [int]$Indent = 0,
        [Parameter(Mandatory)][string]$Label,
        [AllowNull()]$Value
    )
    $prefix = ' ' * $Indent
    Write-Output ("{0}{1,-20} {2}" -f $prefix, $Label, (Show-Value $Value))
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
    param([AllowNull()]$Block)

    Write-Output 'Current Juggernaut Block'

    if (-not $Block) {
        Show-Kv -Label 'Status' -Value 'no active Juggernaut block'
        return
    }

    $view = Get-ShowBlock -Block $Block
    Show-Kv -Label 'Scope' -Value $view.Scope
    Show-Kv -Label 'Auth' -Value $view.AuthMode
    Show-Kv -Label 'Region' -Value $view.Region
    Show-Kv -Label 'Model' -Value $view.Model
    Show-Kv -Label 'Effort' -Value $view.Effort
    Show-Kv -Label 'Opus Plan' -Value (Show-State $view.OpusPlan)
    Show-Kv -Label 'Mantle' -Value (Show-State $view.UseMantle)
}

function Show-EffectiveConfig {
    param(
        [Parameter(Mandatory)][string]$Path,
        [AllowNull()]$Block
    )

    Write-Output 'Effective Config'
    Write-Output (Show-HomePath $Path)
    if (-not $Block) {
        Show-Kv -Label 'Region' -Value '—'
        Show-Kv -Label 'Model' -Value '—'
        return
    }

    $view = Get-ShowBlock -Block $Block
    Show-Kv -Label 'Region' -Value $view.Region
    Show-Kv -Label 'Model' -Value $view.Model
}

function Show-ShellFallback {
    param([AllowNull()]$Block)
    if (-not $Block) { return }

    $view = Get-ShowBlock -Block $Block
    if (-not $view.ShellEnabled -and $view.ShellProfiles.Count -eq 0) { return }

    Write-Output 'Shell Fallback'
    $shellName = if ($env:SHELL) { Split-Path -Leaf $env:SHELL } else { 'bash' }
    $shellPath = Get-ProfileWriterShellConfigPath -Shell $shellName
    if ($shellPath) {
        Write-Output (Show-HomePath $shellPath)
    }
    Show-Kv -Label 'Present' -Value (Show-Bool $view.ShellEnabled)
    Show-Kv -Label 'Storage' -Value $view.Storage
}

function Show-HomePath {
    param([AllowNull()][string]$Path)
    if (-not $Path) { return '' }
    $homePath = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { '' }
    if ($homePath -and $Path.StartsWith($homePath, [System.StringComparison]::OrdinalIgnoreCase)) {
        return '~' + $Path.Substring($homePath.Length)
    }
    return $Path
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

Write-Output 'Juggernaut show'
Write-Output ''
Show-CurrentBlock -Block $activeBlock
Write-Output ''
Show-EffectiveConfig -Path $activePath -Block $activeBlock
if ($activeBlock) {
    Write-Output ''
    Show-ShellFallback -Block $activeBlock
}
