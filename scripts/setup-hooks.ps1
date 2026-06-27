# Install git hooks from .githooks/
$ErrorActionPreference = "Stop"

$hooksDir = Join-Path (git rev-parse --git-dir) "hooks"
New-Item -ItemType Directory -Force -Path $hooksDir | Out-Null

Get-ChildItem .githooks/* -File | ForEach-Object {
    Copy-Item $_.FullName -Destination $hooksDir -Force
    Write-Output "Installed $($_.Name)"
}

Write-Output "Git hooks installed successfully."
