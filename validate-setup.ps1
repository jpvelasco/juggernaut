# Juggernaut - Claude Code Bedrock Configuration Validator for Windows
# Validates that Claude Code is properly configured for Bedrock with Global CRIS
# Usage: .\validate-setup.ps1 [-Help]

param(
    [switch]$Help
)

if ($Help) {
    Write-Host "Juggernaut - Configuration Validator"
    Write-Host ""
    Write-Host "Usage: .\validate-setup.ps1"
    Write-Host ""
    Write-Host "Validates that Claude Code is properly configured for Amazon Bedrock."
    Write-Host "Checks environment variables, AWS credentials, Bedrock access, and Claude Code installation."
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Help    Show this help message"
    exit 0
}

#───────────────────────────────────────────────────────────────────────────────
# Configuration
#───────────────────────────────────────────────────────────────────────────────

# Expected environment variable values
$ExpectedEnvVars = @{
    "CLAUDE_CODE_USE_BEDROCK" = "1"
    "CLAUDE_CODE_MAX_OUTPUT_TOKENS" = "16384"
    "MAX_THINKING_TOKENS" = "1024"
    "ANTHROPIC_MODEL" = "global.anthropic.claude-opus-4-5-20251101-v1:0"
    "ANTHROPIC_SMALL_FAST_MODEL" = "global.anthropic.claude-sonnet-4-5-20250929-v1:0"
    "DISABLE_ERROR_REPORTING" = "1"
    "DISABLE_TELEMETRY" = "1"
    "DISABLE_AUTOUPDATE" = "1"
    "DISABLE_BUG_COMMAND" = "1"
}

# Variables that just need to be set (any value)
$RequiredEnvVars = @("AWS_REGION")

# Counters
$script:Errors = 0
$script:Warnings = 0

#───────────────────────────────────────────────────────────────────────────────
# Validation Functions
#───────────────────────────────────────────────────────────────────────────────

function Test-EnvVarExact {
    param([string]$VarName, [string]$Expected)

    $Current = [Environment]::GetEnvironmentVariable($VarName)

    if ([string]::IsNullOrEmpty($Current)) {
        Write-Host "FAIL" -ForegroundColor Red -NoNewline
        Write-Host " $VarName is not set"
        $script:Errors++
    }
    elseif ($Current -ne $Expected) {
        Write-Host "WARN" -ForegroundColor Yellow -NoNewline
        Write-Host " $VarName=$Current (expected: $Expected)"
        $script:Warnings++
    }
    else {
        Write-Host "PASS" -ForegroundColor Green -NoNewline
        Write-Host " $VarName=$Current"
    }
}

function Test-EnvVarExists {
    param([string]$VarName)

    $Current = [Environment]::GetEnvironmentVariable($VarName)

    if ([string]::IsNullOrEmpty($Current)) {
        Write-Host "FAIL" -ForegroundColor Red -NoNewline
        Write-Host " $VarName is not set"
        $script:Errors++
    }
    else {
        Write-Host "PASS" -ForegroundColor Green -NoNewline
        Write-Host " $VarName=$Current"
    }
}

function Test-AwsCredentials {
    try {
        $result = aws sts get-caller-identity 2>$null | ConvertFrom-Json
        if ($result) {
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " AWS credentials valid"
            Write-Host "     Account: $($result.Account)"
            Write-Host "     Identity: $($result.Arn)"
        }
        else {
            throw "No result"
        }
    }
    catch {
        Write-Host "FAIL" -ForegroundColor Red -NoNewline
        Write-Host " AWS credentials not configured or expired"
        Write-Host "     Run: aws configure or aws sso login"
        $script:Errors++
    }
}

function Test-BedrockAccess {
    $region = $env:AWS_REGION
    if ([string]::IsNullOrEmpty($region)) {
        $region = "us-west-2"
    }

    try {
        $result = aws bedrock list-foundation-models --region $region --by-provider anthropic 2>$null | ConvertFrom-Json
        if ($result -and $result.modelSummaries) {
            $modelCount = $result.modelSummaries.Count
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " Bedrock access confirmed"
            Write-Host "     Available Anthropic models: $modelCount"
        }
        else {
            throw "No models found"
        }
    }
    catch {
        Write-Host "FAIL" -ForegroundColor Red -NoNewline
        Write-Host " Cannot access Bedrock models"
        Write-Host "     Check IAM permissions and region availability"
        $script:Errors++
    }
}

function Test-ClaudeCode {
    try {
        $version = claude --version 2>$null
        if ($version) {
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " Claude Code installed"
            Write-Host "     Version: $version"
        }
        else {
            throw "Not found"
        }
    }
    catch {
        Write-Host "FAIL" -ForegroundColor Red -NoNewline
        Write-Host " Claude Code not found"
        Write-Host "     Install: npm install -g @anthropic-ai/claude-code"
        $script:Errors++
    }
}

#───────────────────────────────────────────────────────────────────────────────
# Main
#───────────────────────────────────────────────────────────────────────────────

Write-Host "Validating Claude Code Bedrock Configuration..." -ForegroundColor Cyan
Write-Host ""

# System Info
Write-Host "System" -ForegroundColor Cyan
Write-Host "  OS:    Windows"
Write-Host "  Shell: PowerShell $($PSVersionTable.PSVersion)"
Write-Host ""

# Environment Variables
Write-Host "Environment Variables" -ForegroundColor Cyan
foreach ($var in $ExpectedEnvVars.Keys) {
    Test-EnvVarExact -VarName $var -Expected $ExpectedEnvVars[$var]
}
foreach ($var in $RequiredEnvVars) {
    Test-EnvVarExists -VarName $var
}
Write-Host ""

# AWS Credentials
Write-Host "AWS Credentials" -ForegroundColor Cyan
Test-AwsCredentials
Write-Host ""

# Bedrock Access
Write-Host "Bedrock Access" -ForegroundColor Cyan
Test-BedrockAccess
Write-Host ""

# Claude Code
Write-Host "Claude Code" -ForegroundColor Cyan
Test-ClaudeCode
Write-Host ""

# Summary
Write-Host "Summary" -ForegroundColor Cyan
if ($script:Errors -eq 0 -and $script:Warnings -eq 0) {
    Write-Host "All checks passed! Claude Code is ready for Bedrock." -ForegroundColor Green
    Write-Host ""
    Write-Host "Next steps:"
    Write-Host "  1. Launch Claude Code: claude"
    Write-Host "  2. Verify it connects to Bedrock (should not prompt for login)"
}
elseif ($script:Errors -eq 0) {
    Write-Host "Configuration mostly correct with $($script:Warnings) warning(s)" -ForegroundColor Yellow
    Write-Host "Claude Code should work, but consider addressing warnings above."
}
else {
    Write-Host "Found $($script:Errors) error(s) and $($script:Warnings) warning(s)" -ForegroundColor Red
    Write-Host "Please fix the errors above before using Claude Code with Bedrock."
    exit 1
}
