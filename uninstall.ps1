# Claude Code - Amazon Bedrock Uninstall Script for Windows
# Removes Juggernaut configuration from PowerShell profile
# Usage: .\uninstall.ps1

$ErrorActionPreference = "Stop"

Write-Host "Removing Claude Code Bedrock configuration..." -ForegroundColor Cyan

# Determine PowerShell profile path
$ProfilePath = $PROFILE.CurrentUserAllHosts

if (-not (Test-Path $ProfilePath)) {
    Write-Host "PowerShell profile not found ($ProfilePath)" -ForegroundColor Yellow
    exit 0
}

$ProfileContent = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue

if ($ProfileContent -match "CLAUDE_CODE_USE_BEDROCK") {
    # Clean up keychain credentials before removing config (needs the markers to detect storage mode)
    if ($ProfileContent -match "# Storage: keychain") {
        $null = cmdkey /delete:juggernaut-bedrock 2>$null
        Write-Host "Removed API key from Windows Credential Manager" -ForegroundColor Gray
    }

    # Remove configuration (supports both old and new marker formats)
    $ProfileContent = $ProfileContent -replace "(?ms)\r?\n?# BEGIN: Claude Code Bedrock Configuration.*?# END: Claude Code Bedrock Configuration\r?\n?", "`n"
    $ProfileContent = $ProfileContent -replace "(?ms)\r?\n?# Claude Code - Amazon Bedrock Configuration.*?`$env:ANTHROPIC_(?:SMALL_FAST_)?MODEL = `"[^`"]+`"\r?\n?", "`n"
    # Remove multiple consecutive blank lines
    $ProfileContent = $ProfileContent -replace "(\r?\n){3,}", "`n`n"
    Set-Content -Path $ProfilePath -Value $ProfileContent.TrimEnd() -NoNewline
    Add-Content -Path $ProfilePath -Value ""
    Write-Host "Configuration removed from $ProfilePath" -ForegroundColor Green
} else {
    Write-Host "No configuration found in $ProfilePath" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "1. Restart PowerShell or run: . `$PROFILE"
Write-Host "2. Claude Code will now use Anthropic's direct API (requires login)"
Write-Host ""
Write-Host "Uninstall complete!" -ForegroundColor Green
