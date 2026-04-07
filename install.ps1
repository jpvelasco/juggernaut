#Requires -Version 5.1
$ErrorActionPreference = "Stop"

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Error "git is required but not installed"
    exit 1
}

$RepoUrl = "https://github.com/jpvelasco/juggernaut.git"
$InstallDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } else { Join-Path $HOME ".juggernaut" }

Write-Host "Installing Juggernaut..."

if (Test-Path $InstallDir) {
    Write-Host "Updating existing installation in $InstallDir"
    git -C $InstallDir pull --ff-only
    if ($LASTEXITCODE -ne 0) { throw "git pull failed" }
} else {
    git clone $RepoUrl $InstallDir
    if ($LASTEXITCODE -ne 0) { throw "git clone failed" }
}

Set-Location $InstallDir
& .\setup-claude-bedrock.ps1 @args
