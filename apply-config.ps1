# Claude Code - Amazon Bedrock Configuration (Current Session)
# Usage: . .\apply-config.ps1 [-Region <region>] [-Help]
#
# NOTE: This script must be dot-sourced (. .\apply-config.ps1) for environment variables to persist.

param(
    [string]$Region = "us-west-2",
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
    Write-Host "  -Region <region>    AWS region (default: us-west-2)"
    Write-Host "  -Help               Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  . .\apply-config.ps1"
    Write-Host "  . .\apply-config.ps1 -Region us-east-1"
    return
}

Write-Host "Applying Claude Code Bedrock configuration..." -ForegroundColor Cyan
Write-Host ""

# Apply configuration
$env:CLAUDE_CODE_USE_BEDROCK = "1"
$env:AWS_REGION = $Region
$env:CLAUDE_CODE_MAX_OUTPUT_TOKENS = "16384"
$env:MAX_THINKING_TOKENS = "1024"
$env:ANTHROPIC_MODEL = "global.anthropic.claude-opus-4-5-20251101-v1:0"
$env:ANTHROPIC_SMALL_FAST_MODEL = "global.anthropic.claude-sonnet-4-5-20250929-v1:0"
$env:DISABLE_ERROR_REPORTING = "1"
$env:DISABLE_TELEMETRY = "1"
$env:DISABLE_AUTOUPDATE = "1"
$env:DISABLE_BUG_COMMAND = "1"

Write-Host "Configuration applied:" -ForegroundColor Green
Write-Host "  CLAUDE_CODE_USE_BEDROCK=$env:CLAUDE_CODE_USE_BEDROCK"
Write-Host "  AWS_REGION=$env:AWS_REGION"
Write-Host "  CLAUDE_CODE_MAX_OUTPUT_TOKENS=$env:CLAUDE_CODE_MAX_OUTPUT_TOKENS"
Write-Host "  MAX_THINKING_TOKENS=$env:MAX_THINKING_TOKENS"
Write-Host "  ANTHROPIC_MODEL=$env:ANTHROPIC_MODEL"
Write-Host "  ANTHROPIC_SMALL_FAST_MODEL=$env:ANTHROPIC_SMALL_FAST_MODEL"
Write-Host "  DISABLE_ERROR_REPORTING=$env:DISABLE_ERROR_REPORTING"
Write-Host "  DISABLE_TELEMETRY=$env:DISABLE_TELEMETRY"
Write-Host "  DISABLE_AUTOUPDATE=$env:DISABLE_AUTOUPDATE"
Write-Host "  DISABLE_BUG_COMMAND=$env:DISABLE_BUG_COMMAND"
Write-Host ""
Write-Host "This configuration is active for the current PowerShell session only." -ForegroundColor Yellow
Write-Host "To make it permanent, run: .\setup-claude-bedrock.ps1"
