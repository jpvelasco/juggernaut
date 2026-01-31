# Claude Code - Amazon Bedrock Configuration (Current Session)
# Usage: . .\apply-config.ps1 [-Region <region>] [-Help]
#
# NOTE: This script must be dot-sourced (. .\apply-config.ps1) for environment variables to persist.

param(
    [string]$Region,
    [switch]$Help
)

if ($Help) {
    Write-Host "Apply Claude Code Bedrock Configuration"
    Write-Host ""
    Write-Host "Usage: . .\apply-config.ps1 [-Region <region>]"
    Write-Host ""
    Write-Host "Applies Claude Code Bedrock configuration to the current PowerShell session."
    Write-Host "This script must be dot-sourced (note the leading dot) for environment variables to persist."
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Region <region>    AWS region (default: from bedrock-config.json)"
    Write-Host "  -Help               Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  . .\apply-config.ps1"
    Write-Host "  . .\apply-config.ps1 -Region us-east-1"
    return
}

#───────────────────────────────────────────────────────────────────────────────
# Load Configuration from JSON
#───────────────────────────────────────────────────────────────────────────────

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ConfigFile = Join-Path $ScriptDir "bedrock-config.json"

if (-not (Test-Path $ConfigFile)) {
    Write-Host "Error: Config file not found: $ConfigFile" -ForegroundColor Red
    return
}

$config = Get-Content $ConfigFile -Raw | ConvertFrom-Json

Write-Host "Applying Claude Code Bedrock configuration..." -ForegroundColor Cyan
Write-Host ""

# Apply configuration from JSON
foreach ($property in $config.environment.PSObject.Properties) {
    [Environment]::SetEnvironmentVariable($property.Name, $property.Value, "Process")
}

# Region: command line overrides config default
if ($Region) {
    $env:AWS_REGION = $Region
} else {
    $env:AWS_REGION = $config.defaults.region
}

Write-Host "Configuration applied:" -ForegroundColor Green
Write-Host "  AWS_REGION=$env:AWS_REGION"
foreach ($property in $config.environment.PSObject.Properties) {
    Write-Host "  $($property.Name)=$($property.Value)"
}
Write-Host ""
Write-Host "This configuration is active for the current PowerShell session only." -ForegroundColor Yellow
Write-Host "To make it permanent, run: .\setup-claude-bedrock.ps1"
