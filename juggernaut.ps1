# juggernaut.ps1 — v2 subcommand dispatcher (PowerShell).
# Usage: juggernaut.ps1 [<subcommand>] [options]

param([Parameter(Position=0)][string]$Subcommand = 'apply')

$ErrorActionPreference = 'Stop'
$env:JUGGERNAUT_USE_V2 = '1'

$PSScriptRoot_ = $PSScriptRoot

function Show-Help {
    @'
Juggernaut v2 — Claude Code Bedrock configurator

Usage: juggernaut.ps1 <subcommand> [options]

Subcommands:
  apply       Configure Claude Code to use Amazon Bedrock (default)
  migrate     Migrate a v1 profile block to settings.json
  show        Print current Juggernaut configuration (not yet implemented)
  doctor      Verify configuration and connectivity (not yet implemented)
  uninstall   Remove Juggernaut configuration (not yet implemented)

Run 'juggernaut.ps1 apply --help' for apply-specific options.
'@
}

# Shift positional args past the subcommand
$rest = $args

switch ($Subcommand) {
    'apply' {
        $applyScript = Join-Path $PSScriptRoot_ 'commands\apply.ps1'
        & $applyScript @rest
        exit $LASTEXITCODE
    }
    'migrate' {
        $migrateScript = Join-Path $PSScriptRoot_ 'commands\migrate.ps1'
        & $migrateScript @rest
        exit $LASTEXITCODE
    }
    { $_ -in 'show','doctor','uninstall' } {
        Write-Error "juggernaut $Subcommand: not yet implemented in Phase 3"
        exit 1
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
