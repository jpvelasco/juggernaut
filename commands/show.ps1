# commands/show.ps1 - Juggernaut v3 show subcommand.

[CmdletBinding(PositionalBinding=$false)]
param(
    [string]$Scope = '',
    [switch]$Help,
    [switch]$Version,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$RemainingArgs
)

$ErrorActionPreference = 'Stop'

foreach ($arg in $RemainingArgs) {
    switch -Regex ($arg) {
        '^--scope=(user|project)$' { $Scope = $Matches[1]; break }
        '^--scope=' { throw "show: --scope must be 'user' or 'project' (got: '$($arg.Substring(8))')" }
        '^--help$' { $Help = $true; break }
        '^-h$' { $Help = $true; break }
        '^--version$' { $Version = $true; break }
        '^-v$' { $Version = $true; break }
        default { }
    }
}

if ($Help) {
    @'
juggernaut show - print the current Juggernaut configuration

Usage: juggernaut.ps1 show [-Scope user|project]

Displays the current Juggernaut block and the effective user/project scopes.
'@
    return
}

if ($Version) {
    $repoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
    $vf = Join-Path $repoRoot 'VERSION'
    if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
    return
}

if ($Scope -and $Scope -notin @('user','project')) {
    Write-Error "show: --scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
. (Join-Path $RepoRoot 'lib\config_manager.ps1')

function Show-Value {
    param([AllowNull()]$Value)
    if ($null -eq $Value -or $Value -eq '') { return '-' }
    return [string]$Value
}

function Show-State {
    param([AllowNull()]$Value)
    if ($Value -is [bool]) { return $(if ($Value) { 'enabled' } else { 'disabled' }) }
    if ($Value -eq 'true') { return 'enabled' }
    if ($Value -eq 'false') { return 'disabled' }
    return '-'
}

function Show-Kv {
    param(
        [int]$Indent = 0,
        [Parameter(Mandatory)][string]$Label,
        [AllowNull()]$Value
    )
    $prefix = ' ' * $Indent
    Write-Output ("{0}{1}: {2}" -f $prefix, $Label, (Show-Value $Value))
}

function Show-Auth {
    param([AllowNull()][string]$AuthMode)
    switch ($AuthMode) {
        'api-key' { 'Bedrock API key' }
        'bedrock-api-key' { 'Bedrock API key' }
        'iam' { 'IAM' }
        default { Show-Value $AuthMode }
    }
}

function Show-HomePath {
    param([AllowNull()][string]$Path)
    if (-not $Path) { return '' }
    $homePath = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { '' }
    if ($homePath -and $Path.StartsWith($homePath, [System.StringComparison]::OrdinalIgnoreCase)) {
        return '~' + ($Path.Substring($homePath.Length) -replace '\\', '/')
    }
    return $Path
}

function Get-ShowBlock {
    param([AllowNull()]$Block)
    if (-not $Block) { return $null }
    [ordered]@{
        Scope     = $Block.meta.scope
        AuthMode  = Show-Auth $Block.auth.mode
        Region    = $Block.auth.region
        Model     = $Block.model
        Effort    = $Block.effortLevel
        UseMantle = [bool]$Block.useMantle
        OpusPlan  = $Block.opusplan
    }
}

function Show-ScopeConfig {
    param(
        [Parameter(Mandatory)][string]$ScopeName,
        [Parameter(Mandatory)][string]$Path,
        [AllowNull()]$Block,
        [bool]$Active,
        [bool]$Selected
    )

    $title = (Get-Culture).TextInfo.ToTitleCase($ScopeName) + ' Scope'
    if ($Active -and $Selected) { $title += ' (active, selected)' }
    elseif ($Active) { $title += ' (active)' }
    elseif ($Selected) { $title += ' (selected)' }

    Write-Output $title
    Write-Output ('  ' + (Show-HomePath $Path))

    if (-not $Block) {
        Show-Kv -Indent 4 -Label 'Status' -Value 'No Juggernaut block'
        return
    }

    $view = Get-ShowBlock -Block $Block
    Show-Kv -Indent 4 -Label 'Scope' -Value $view.Scope
    Show-Kv -Indent 4 -Label 'Auth' -Value $view.AuthMode
    Show-Kv -Indent 4 -Label 'Region' -Value $view.Region
    Show-Kv -Indent 4 -Label 'Model' -Value $view.Model
    Show-Kv -Indent 4 -Label 'Effort' -Value $view.Effort
    Show-Kv -Indent 4 -Label 'Opus Plan' -Value (Show-State $view.OpusPlan)
    Show-Kv -Indent 4 -Label 'Mantle' -Value (Show-State $view.UseMantle)
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

$activeScope = ''
if ($projectBlock) {
    $activeScope = 'project'
} elseif ($userBlock) {
    $activeScope = 'user'
}

Write-Output 'Juggernaut show'
Write-Output ''
Write-Output 'Scope Awareness'
if ($Scope) {
    Show-Kv -Indent 2 -Label 'Selected Scope' -Value $Scope
} else {
    Show-Kv -Indent 2 -Label 'Selected Scope' -Value 'not specified'
}
if ($activeScope) {
    Show-Kv -Indent 2 -Label 'Active Scope' -Value "$activeScope takes precedence for this session"
} else {
    Show-Kv -Indent 2 -Label 'Active Scope' -Value 'No Juggernaut block found'
}
Write-Output ''
Show-ScopeConfig -ScopeName 'user' -Path $userPath -Block $userBlock -Active:($activeScope -eq 'user') -Selected:($Scope -eq 'user')
Write-Output ''
$displayProjectPath = if ($projectPath) { $projectPath } else { Join-Path (Get-Location).Path '.claude/settings.json' }
Show-ScopeConfig -ScopeName 'project' -Path $displayProjectPath -Block $projectBlock -Active:($activeScope -eq 'project') -Selected:($Scope -eq 'project')
