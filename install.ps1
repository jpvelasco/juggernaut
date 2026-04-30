#Requires -Version 5.1
# install.ps1 - Juggernaut installer
#
# Usage:
#   irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.1.2
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Ref fix-branch
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Latest
#
# Or after downloading:
#   .\install.ps1 -Version 2.1.2
#   .\install.ps1 -Ref fix-branch
#   .\install.ps1 -Latest

param(
    [string]$Version = '',
    [string]$Ref = '',
    [switch]$Latest,
    [switch]$Configure,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$SetupArgs
)

$ErrorActionPreference = 'Stop'

if (-not $Ref -and $env:JUGGERNAUT_REF) { $Ref = $env:JUGGERNAUT_REF }
if ($Latest) { $Version = ''; $Ref = '' }
if ($Ref) { $Version = '' }

# Normalize version: accept "2.1.2" or "v2.1.2" - tags are always v-prefixed.
if ($Version -and -not $Version.StartsWith('v')) { $Version = "v$Version" }

$RepoUrl    = if ($env:JUGGERNAUT_REPO_URL) { $env:JUGGERNAUT_REPO_URL } else { 'https://github.com/jpvelasco/juggernaut.git' }
$InstallDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } else { Join-Path $HOME '.juggernaut' }

if ($Ref) {
    Write-Host "Installing Juggernaut $Ref..."
} elseif ($Version) {
    Write-Host "Installing Juggernaut $Version..."
} else {
    Write-Host 'Installing Juggernaut (latest)...'
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Error 'git is required but not installed'
    exit 1
}

function Clone-Install {
    param([string]$Target = $InstallDir)
    if ($Ref) {
        git clone --branch $Ref --depth 1 --quiet $RepoUrl $Target
    } elseif ($Version) {
        git clone --branch $Version --depth 1 --quiet $RepoUrl $Target
    } else {
        git clone --quiet $RepoUrl $Target
    }
    if ($LASTEXITCODE -ne 0) { throw 'git clone failed' }
}

function Backup-ExistingInstall {
    $timestamp = Get-Date -Format 'yyyyMMdd_HHmmss'
    $backup = "$InstallDir.backup.$timestamp"
    $n = 1
    while (Test-Path $backup) {
        $backup = "$InstallDir.backup.$timestamp.$n"
        $n++
    }
    Write-Host "Backup created: $backup"
    Move-Item -LiteralPath $InstallDir -Destination $backup
}

function Test-InstallTreeDirty {
    git -C $InstallDir rev-parse --git-dir *> $null
    if ($LASTEXITCODE -ne 0) { return $true }

    git -C $InstallDir diff --quiet --ignore-submodules --
    if ($LASTEXITCODE -ne 0) { return $true }

    git -C $InstallDir diff --cached --quiet --ignore-submodules --
    if ($LASTEXITCODE -ne 0) { return $true }

    $untracked = git -C $InstallDir ls-files --others --exclude-standard
    return [bool]$untracked
}

function Convert-InstallerApplyArgs {
    param([string[]]$InputArgs)
    $converted = @{}
    for ($i = 0; $i -lt $InputArgs.Count; $i++) {
        $arg = [string]$InputArgs[$i]
        switch -Regex ($arg) {
            '^--([^=]+)=(.*)$' {
                $name = ($Matches[1] -replace '-', '')
                $converted[$name] = $Matches[2]
                continue
            }
            '^--(.+)$' {
                $name = ($Matches[1] -replace '-', '')
                if (($i + 1) -lt $InputArgs.Count -and ([string]$InputArgs[$i + 1]) -notlike '-*') {
                    $converted[$name] = [string]$InputArgs[$i + 1]
                    $i++
                } else {
                    $converted[$name] = $true
                }
                continue
            }
            '^-([^=]+)=(.*)$' {
                $name = ($Matches[1] -replace '-', '')
                $converted[$name] = $Matches[2]
                continue
            }
            '^-([^-].*)$' {
                $name = ($Matches[1] -replace '-', '')
                if (($i + 1) -lt $InputArgs.Count -and ([string]$InputArgs[$i + 1]) -notlike '-*') {
                    $converted[$name] = [string]$InputArgs[$i + 1]
                    $i++
                } else {
                    $converted[$name] = $true
                }
                continue
            }
            default {
                if (-not $converted.ContainsKey('RemainingArgs')) { $converted['RemainingArgs'] = @() }
                $converted['RemainingArgs'] += $arg
            }
        }
    }
    return $converted
}

if (Test-Path $InstallDir) {
    if (Test-InstallTreeDirty) {
        Write-Host 'Existing installation has local changes or is not a clean Git checkout.'
        # Clone to a sibling directory first so a failed clone cannot destroy the
        # existing install. Only if the clone succeeds do we swap directories.
        $NewDir = "$InstallDir.new"
        if (Test-Path $NewDir) { Remove-Item -LiteralPath $NewDir -Recurse -Force }
        try {
            Clone-Install -Target $NewDir
            Backup-ExistingInstall
            Move-Item -LiteralPath $NewDir -Destination $InstallDir
        } catch {
            if (Test-Path $NewDir) { Remove-Item -LiteralPath $NewDir -Recurse -Force -ErrorAction SilentlyContinue }
            throw
        }
    } else {
        Write-Host "Updating existing installation in $InstallDir"
        git -C $InstallDir fetch --tags --quiet
        if ($LASTEXITCODE -ne 0) { throw 'git fetch failed' }
        if ($Ref) {
            git -C $InstallDir fetch --quiet origin $Ref
            if ($LASTEXITCODE -ne 0) { throw "git fetch $Ref failed" }
            git -C $InstallDir checkout --quiet FETCH_HEAD
            if ($LASTEXITCODE -ne 0) { throw "git checkout $Ref failed" }
        } elseif ($Version) {
            git -C $InstallDir checkout --quiet $Version
            if ($LASTEXITCODE -ne 0) { throw "git checkout $Version failed" }
        } else {
            git -C $InstallDir checkout --quiet main
            if ($LASTEXITCODE -ne 0) { throw 'git checkout main failed' }
            git -C $InstallDir pull --ff-only --quiet
            if ($LASTEXITCODE -ne 0) { throw 'git pull failed' }
        }
    }
} else {
    Clone-Install
}

Write-Host "Installed to $InstallDir"

$ShimDir = Join-Path $HOME '.local\bin'
New-Item -ItemType Directory -Path $ShimDir -Force | Out-Null

$ShimPs1 = Join-Path $ShimDir 'juggernaut.ps1'
$ShimCmd = Join-Path $ShimDir 'juggernaut.cmd'
$TargetPs1 = Join-Path $InstallDir 'juggernaut.ps1'

@"
param([Parameter(ValueFromRemainingArguments=`$true)][string[]]`$Args)
& '$TargetPs1' @Args
if (`$?) { exit 0 }
exit 1
"@ | Set-Content -Path $ShimPs1 -Encoding utf8

@"
@echo off
where pwsh.exe >nul 2>nul
if %ERRORLEVEL% EQU 0 (
  pwsh.exe -NoProfile -ExecutionPolicy Bypass -File "$ShimPs1" %*
  exit /b %ERRORLEVEL%
) else (
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$ShimPs1" %*
  exit /b %ERRORLEVEL%
)
"@ | Set-Content -Path $ShimCmd -Encoding ascii

Write-Host "Launcher written to $ShimCmd"
if (-not (($env:PATH -split ';') -contains $ShimDir)) {
    Write-Host "Note: add $ShimDir to PATH to run 'juggernaut' from any directory."
}
Write-Host 'If PowerShell blocks first run scripts, run:'
Write-Host '  Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'
Write-Host 'Verify after install with: juggernaut doctor --v2'
Write-Host 'Configure with one of:'
Write-Host '  juggernaut apply --v2 --auth=bedrock-api-key'
Write-Host '  juggernaut apply --v2 --auth=iam'

if ($Configure) {
    $oldLocation = (Get-Location).Path
    try {
        Set-Location $InstallDir
        $applyArgs = Convert-InstallerApplyArgs -InputArgs $SetupArgs
        & (Join-Path $InstallDir 'commands\apply.ps1') @applyArgs
    } finally {
        Set-Location $oldLocation
    }
    return
} elseif ($SetupArgs.Count -gt 0) {
    Write-Warning 'Install arguments after -Version were ignored. Use -Configure to run apply during install.'
}
