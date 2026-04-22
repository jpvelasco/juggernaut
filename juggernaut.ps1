# juggernaut.ps1 — v2 subcommand dispatcher (PowerShell).
# Usage: juggernaut.ps1 [<subcommand>] [options]

param(
    [Parameter(Position=0)][string]$Subcommand = 'apply',
    [Alias('v2')][switch]$UseV2,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$RemainingArgs
)

$ErrorActionPreference = 'Stop'
$v2Active = ($env:JUGGERNAUT_USE_V2 -eq '1') -or $UseV2
$filteredArgs = @()
foreach ($arg in $RemainingArgs) {
    if ($arg -eq '--v2') {
        $v2Active = $true
    } else {
        $filteredArgs += $arg
    }
}
$subcommandArgs = $filteredArgs

$PSScriptRoot_ = $PSScriptRoot

function Show-Help {
    @'
Juggernaut v2 — Claude Code Bedrock configurator

Usage: juggernaut.ps1 <subcommand> [options]

Subcommands:
  apply       Configure Claude Code to use Amazon Bedrock (default)
  migrate     Migrate a v1 profile block to settings.json
  show        Print current Juggernaut configuration
  doctor      Verify configuration and connectivity (not yet implemented)
  uninstall   Remove Juggernaut configuration

Run 'juggernaut.ps1 apply --help' for apply-specific options.
'@
}

if (-not $v2Active) {
    switch ($Subcommand) {
        { $_ -in '--version','-v' } {
            $vf = Join-Path $PSScriptRoot_ 'VERSION'
            if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
            exit 0
        }
        { $_ -in '--help','-h','help' } {
            Show-Help
            exit 0
        }
        default {
            Write-Host 'Juggernaut v2 is not active. Use --v2 to enable v2 commands.'
            exit 0
        }
    }
}

$env:JUGGERNAUT_USE_V2 = '1'

switch ($Subcommand) {
    'apply' {
        $applyScript = Join-Path $PSScriptRoot_ 'commands\apply.ps1'
        & $applyScript @subcommandArgs
        exit $LASTEXITCODE
    }
    'migrate' {
        $migrateScript = Join-Path $PSScriptRoot_ 'commands\migrate.ps1'
        & $migrateScript @subcommandArgs
        exit $LASTEXITCODE
    }
    'show' {
        $showScript = Join-Path $PSScriptRoot_ 'commands\show.ps1'
        & $showScript @subcommandArgs
        exit $LASTEXITCODE
    }
    'doctor' {
        $doctorScript = Join-Path $PSScriptRoot_ 'commands\doctor.ps1'
        & $doctorScript @subcommandArgs
        exit $LASTEXITCODE
    }
    'uninstall' {
        $uninstallScript = Join-Path $PSScriptRoot_ 'commands\uninstall.ps1'
        & $uninstallScript @subcommandArgs
        exit $LASTEXITCODE
    }
    { $_ -in '--version','-v' } {
        $vf = Join-Path $PSScriptRoot_ 'VERSION'
        if (Test-Path $vf) { Get-Content $vf -Raw } else { 'unknown' }
        exit 0
    }
    { $_ -in '--help','-h','help' } {
        Show-Help
        exit 0
    }
    default {
        Write-Error "juggernaut: unknown subcommand '$Subcommand'. Run 'juggernaut.ps1 --help' for usage."
        exit 1
    }
}
