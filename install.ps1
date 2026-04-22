#Requires -Version 5.1
# install.ps1 — Juggernaut installer
#
# Usage:
#   irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.0.0
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Latest
#
# Or after downloading:
#   .\install.ps1 -Version 2.0.0
#   .\install.ps1 -Latest

param(
    [string]$Version = '',
    [switch]$Latest,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$SetupArgs
)

$ErrorActionPreference = 'Stop'

if ($Latest) { $Version = '' }

$RepoUrl    = 'https://github.com/jpvelasco/juggernaut.git'
$InstallDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } else { Join-Path $HOME '.juggernaut' }

if ($Version) {
    Write-Host "Installing Juggernaut $Version..."
} else {
    Write-Host 'Installing Juggernaut (latest)...'
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Error 'git is required but not installed'
    exit 1
}

if (Test-Path $InstallDir) {
    Write-Host "Updating existing installation in $InstallDir"
    git -C $InstallDir fetch --tags --quiet
    if ($LASTEXITCODE -ne 0) { throw 'git fetch failed' }
    if ($Version) {
        git -C $InstallDir checkout --quiet $Version
    } else {
        git -C $InstallDir checkout --quiet main
        git -C $InstallDir pull --ff-only --quiet
    }
    if ($LASTEXITCODE -ne 0) { throw 'git checkout/pull failed' }
} else {
    if ($Version) {
        git clone --branch $Version --depth 1 --quiet $RepoUrl $InstallDir
    } else {
        git clone --quiet $RepoUrl $InstallDir
    }
    if ($LASTEXITCODE -ne 0) { throw 'git clone failed' }
}

Write-Host "Installed to $InstallDir"
Set-Location $InstallDir
& .\setup-claude-bedrock.ps1 @SetupArgs
