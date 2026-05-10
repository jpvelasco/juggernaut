# juggernaut.ps1 - v3 subcommand dispatcher (PowerShell).
# Usage: juggernaut.ps1 [<subcommand>] [options]

$ErrorActionPreference = 'Stop'
$rawArgs = @($args)
$Subcommand = 'apply'
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

$subcommandArgs = $RemainingArgs

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
Juggernaut v3 - Claude Code Bedrock configurator

Usage: juggernaut.ps1 <subcommand> [options]

Subcommands:
  apply       Configure Claude Code to use Amazon Bedrock (default)
  show        Print current Juggernaut configuration
  doctor      Verify configuration and credentials
  uninstall   Remove Juggernaut configuration
  version     Print the installed Juggernaut version

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

$subcommandArgs = Convert-GnuStyleArgs -InputArgs $subcommandArgs

switch ($Subcommand) {
    'apply' {
        $applyScript = Join-Path $PSScriptRoot_ 'commands\apply.ps1'
        Invoke-JuggernautPowerShellScript -Path $applyScript -Arguments $subcommandArgs
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
    { $_ -in 'version','--version','-v' } {
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
