# juggernaut.ps1 - v2 subcommand dispatcher (PowerShell).
# Usage: juggernaut.ps1 [<subcommand>] [options]

$ErrorActionPreference = 'Stop'
$rawArgs = @($args)
$Subcommand = 'apply'
$UseV2 = $false
$RemainingArgs = @()

if ($rawArgs.Count -gt 0) {
    $firstArg = [string]$rawArgs[0]
    if (($firstArg -notlike '-*') -or ($firstArg -in @('--version','-v','--help','-h','help'))) {
        $Subcommand = $firstArg
        if ($rawArgs.Count -gt 1) {
            $RemainingArgs = @($rawArgs[1..($rawArgs.Count - 1)])
        }
    } else {
        $RemainingArgs = $rawArgs
    }
}

# v2 is the default since 2.3.0. Set JUGGERNAUT_USE_V2=0 or pass --legacy-v1 to use v1.
$v2Active = if ($env:JUGGERNAUT_USE_V2 -eq '0') { $false } elseif ($UseV2) { $true } else { $true }
if ($env:JUGGERNAUT_USE_V1 -eq '1') { $v2Active = $false }
$legacyV1Requested = $false
$filteredArgs = @()
foreach ($arg in $RemainingArgs) {
    switch ($arg) {
        '--v2'                     { $v2Active = $true }
        { $_ -in '--legacy-v1', '-v1' } { $v2Active = $false; $legacyV1Requested = $true }
        default                    { $filteredArgs += $arg }
    }
}
$subcommandArgs = $filteredArgs

function Resolve-JuggernautScriptRoot {
    param([string]$Path)

    $current = Get-Item -LiteralPath $Path
    while ($current.Target) {
        $target = [string]$current.Target
        if (-not [System.IO.Path]::IsPathRooted($target)) {
            $target = Join-Path $current.DirectoryName $target
        }
        $current = Get-Item -LiteralPath $target
    }
    return $current.DirectoryName
}

$PSScriptRoot_ = Resolve-JuggernautScriptRoot -Path $PSCommandPath

. (Join-Path $PSScriptRoot_ 'lib\arg_parsing.ps1')

function Show-Help {
    @'
Juggernaut v2 - Claude Code Bedrock configurator

Usage: juggernaut.ps1 <subcommand> [options]

Subcommands:
  apply       Configure Claude Code to use Amazon Bedrock (default)
  migrate     Migrate a v1 profile block to settings.json
  show        Print current Juggernaut configuration
  doctor      Verify configuration and credentials
  uninstall   Remove Juggernaut configuration

Run 'juggernaut.ps1 apply --help' for apply-specific options.
'@
}

function Invoke-JuggernautPowerShellScript {
    param(
        [Parameter(Mandatory)][string]$Path,
        [hashtable]$Arguments = @{}
    )
    & $Path @Arguments
    if ($?) { exit 0 }
    exit 1
}

if (-not $v2Active) {
    if ($env:JUGGERNAUT_SUPPRESS_DEPRECATION -ne '1') {
        [Console]::Error.WriteLine('Juggernaut v1 is deprecated and will be removed in v3.0. Run without --legacy-v1 for v2.')
    }
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
            $setupScript = Join-Path $PSScriptRoot_ 'setup-claude-bedrock.ps1'
            if (Test-Path $setupScript) {
                & $setupScript @subcommandArgs
                exit $LASTEXITCODE
            }
            Write-Error 'juggernaut: --legacy-v1 requested but setup-claude-bedrock.ps1 was not found.'
            exit 1
        }
    }
}

$env:JUGGERNAUT_USE_V2 = '1'
$subcommandArgs = Convert-GnuStyleArgs -InputArgs $subcommandArgs

switch ($Subcommand) {
    'apply' {
        $applyScript = Join-Path $PSScriptRoot_ 'commands\apply.ps1'
        Invoke-JuggernautPowerShellScript -Path $applyScript -Arguments $subcommandArgs
    }
    'migrate' {
        $migrateScript = Join-Path $PSScriptRoot_ 'commands\migrate.ps1'
        Invoke-JuggernautPowerShellScript -Path $migrateScript -Arguments $subcommandArgs
    }
    'show' {
        $showScript = Join-Path $PSScriptRoot_ 'commands\show.ps1'
        Invoke-JuggernautPowerShellScript -Path $showScript -Arguments $subcommandArgs
    }
    'doctor' {
        $doctorScript = Join-Path $PSScriptRoot_ 'commands\doctor.ps1'
        Invoke-JuggernautPowerShellScript -Path $doctorScript -Arguments $subcommandArgs
    }
    'uninstall' {
        $uninstallScript = Join-Path $PSScriptRoot_ 'commands\uninstall.ps1'
        Invoke-JuggernautPowerShellScript -Path $uninstallScript -Arguments $subcommandArgs
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
