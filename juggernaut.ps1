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

function Convert-GnuStyleArgs {
    param([string[]]$InputArgs)
    $converted = @{}
    for ($i = 0; $i -lt $InputArgs.Count; $i++) {
        $arg = [string]$InputArgs[$i]
        switch -Regex ($arg) {
            '^--([^=]+)=(.*)$' {
                $rawName = $Matches[1]
                $value = $Matches[2]
                $name = $rawName -replace '-', ''
                $converted[$name] = $value
                continue
            }
            '^--(.+)$' {
                $rawName = $Matches[1]
                $name = $rawName -replace '-', ''
                if (($i + 1) -lt $InputArgs.Count -and ([string]$InputArgs[$i + 1]) -notlike '-*') {
                    $converted[$name] = [string]$InputArgs[$i + 1]
                    $i++
                } else {
                    $converted[$name] = $true
                }
                continue
            }
            '^-([^=]+)=(.*)$' {
                $rawName = $Matches[1]
                $value = $Matches[2]
                $name = $rawName -replace '-', ''
                $converted[$name] = $value
                continue
            }
            '^-([^-].*)$' {
                $rawName = $Matches[1]
                if ($rawName -eq 'h') { $name = 'Help' }
                elseif ($rawName -eq 'v') { $name = 'Version' }
                else { $name = $rawName -replace '-', '' }
                if (($i + 1) -lt $InputArgs.Count -and ([string]$InputArgs[$i + 1]) -notlike '-*') {
                    $converted[$name] = [string]$InputArgs[$i + 1]
                    $i++
                } else {
                    $converted[$name] = $true
                }
                continue
            }
            default {
                if (-not $converted.ContainsKey('RemainingArgs')) {
                    $converted['RemainingArgs'] = @()
                }
                $converted['RemainingArgs'] += $arg
            }
        }
    }
    return $converted
}

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
