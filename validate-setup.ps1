# Juggernaut - Claude Code Bedrock Configuration Validator for Windows
# Validates that Claude Code is properly configured for Bedrock with Global CRIS
# Usage: .\validate-setup.ps1 [-Help]

param(
    [switch]$Help,
    [Alias("v")]
    [switch]$Version
)

if ($Version) {
    $versionFile = Join-Path (Split-Path -Parent $MyInvocation.MyCommand.Path) "VERSION"
    if (Test-Path $versionFile) { Get-Content $versionFile } else { Write-Host "unknown" }
    exit 0
}

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

# Find the config file (same directory as this script)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ConfigFile = Join-Path $ScriptDir "bedrock-config.json"

# Load expected values from bedrock-config.json (single source of truth)
function Load-Config {
    if (-not (Test-Path $ConfigFile)) {
        Write-Host "Error: Config file not found: $ConfigFile" -ForegroundColor Red
        exit 1
    }

    $config = Get-Content $ConfigFile -Raw | ConvertFrom-Json
    $envVars = @{}

    foreach ($property in $config.environment.PSObject.Properties) {
        $envVars[$property.Name] = $property.Value
    }

    return $envVars
}

$ExpectedEnvVars = Load-Config

# Variables that just need to be set (any value)
$RequiredEnvVars = @("AWS_REGION")

# Optional API key variable (for api-key auth mode)
$ApiKeyVar = "AWS_BEARER_TOKEN_BEDROCK"

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

function Get-AuthMode {
    $key = [Environment]::GetEnvironmentVariable($ApiKeyVar)
    if (-not [string]::IsNullOrEmpty($key)) {
        return "api-key"
    }
    return "iam"
}

function Test-CredentialConflicts {
    $hasApiKey = -not [string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($ApiKeyVar))
    $hasIamEnv = (-not [string]::IsNullOrEmpty($env:AWS_ACCESS_KEY_ID)) -or (-not [string]::IsNullOrEmpty($env:AWS_SECRET_ACCESS_KEY))
    $hasAwsProfile = -not [string]::IsNullOrEmpty($env:AWS_PROFILE)
    $hasAwsCredsFile = Test-Path "$env:USERPROFILE\.aws\credentials"
    $conflicts = @()

    Write-Host "Credential Conflict Check" -ForegroundColor Cyan

    # Check for conflicts when using API key
    if ($hasApiKey) {
        if ($hasIamEnv) {
            $conflicts += "AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY"
        }
        if ($hasAwsProfile) {
            $conflicts += "AWS_PROFILE=$env:AWS_PROFILE"
        }
        if ($hasAwsCredsFile) {
            $conflicts += "~\.aws\credentials file exists"
        }

        if ($conflicts.Count -gt 0) {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " API key mode active, but other credentials also present:"
            foreach ($conflict in $conflicts) {
                Write-Host "     - $conflict"
            }
            Write-Host "     API key takes precedence; other credentials are ignored."
            Write-Host "     Consider removing unused credentials to avoid confusion."
            $script:Warnings++
        }
        else {
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " No conflicting credentials detected"
        }
    }
    else {
        # IAM mode - check for credentials file
        if ($hasAwsCredsFile) {
            Write-Host "INFO" -ForegroundColor Green -NoNewline
            Write-Host " ~\.aws\credentials file found (may be used for auth)"
        }
        if ($hasAwsProfile) {
            Write-Host "INFO" -ForegroundColor Green -NoNewline
            Write-Host " AWS_PROFILE=$env:AWS_PROFILE is set"
        }
        if (-not $hasIamEnv -and -not $hasAwsProfile -and -not $hasAwsCredsFile) {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " No AWS credentials detected in environment or files"
            $script:Warnings++
        }
        else {
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " IAM credentials configuration looks reasonable"
        }
    }
    Write-Host ""
}

function Test-ApiKey {
    $key = [Environment]::GetEnvironmentVariable($ApiKeyVar)
    if (-not [string]::IsNullOrEmpty($key)) {
        # Mask the key for display
        $masked = $key.Substring(0, [Math]::Min(8, $key.Length)) + "..." + $key.Substring([Math]::Max(0, $key.Length - 4))
        Write-Host "PASS" -ForegroundColor Green -NoNewline
        Write-Host " $ApiKeyVar is set ($masked)"

        # Detect key type by prefix
        if ($key.StartsWith("bedrock-api-key-")) {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " Short-term API key detected (expires ≤12 hours)"
            Write-Host "     Consider using long-term key for persistent setups"
            $script:Warnings++
        }
        elseif ($key.StartsWith("ABSK")) {
            Write-Host "INFO" -ForegroundColor Green -NoNewline
            Write-Host " Long-term API key detected"
            Write-Host "     Check expiration in AWS console if issues occur"
        }
    }
    else {
        Write-Host "INFO" -ForegroundColor Yellow -NoNewline
        Write-Host " $ApiKeyVar not set (using IAM/SSO auth)"
    }
}

function Test-ApiKeyValidity {
    $key = [Environment]::GetEnvironmentVariable($ApiKeyVar)
    if ([string]::IsNullOrEmpty($key)) {
        return
    }

    $region = $env:AWS_REGION
    if ([string]::IsNullOrEmpty($region)) {
        $region = "us-west-2"
    }

    Write-Host "API Key Validity" -ForegroundColor Cyan
    Write-Host "  Testing API key with Bedrock..."

    try {
        # Make a minimal Bedrock API call to test the key
        # Use the configured fast model (cheapest available) for the probe
        $testModel = if ($ExpectedEnvVars["ANTHROPIC_DEFAULT_HAIKU_MODEL"]) { $ExpectedEnvVars["ANTHROPIC_DEFAULT_HAIKU_MODEL"] } else { "anthropic.claude-haiku-4-5-20251001-v1:0" }
        $result = aws bedrock-runtime converse `
            --region $region `
            --model-id $testModel `
            --messages '[{"role":"user","content":[{"text":"hi"}]}]' `
            --inference-config '{"maxTokens":1}' 2>&1

        if ($LASTEXITCODE -eq 0) {
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " API key is valid and working"
        }
        elseif ($result -match "expired|invalid.*token|unauthorized|forbidden|access denied") {
            Write-Host "FAIL" -ForegroundColor Red -NoNewline
            Write-Host " API key appears to be invalid or expired"
            Write-Host "     Claude Code will hang if you try to use it!"
            Write-Host "     Fix: Remove-Item Env:AWS_BEARER_TOKEN_BEDROCK"
            Write-Host "     Or:  Get a new API key and run setup with -Auth api-key"
            $script:Errors++
        }
        elseif ($result -match "could not connect|timeout|network") {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " Could not reach Bedrock (network issue?)"
            Write-Host "     Unable to verify API key validity"
            $script:Warnings++
        }
        else {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " API key test returned unexpected result"
            Write-Host "     $result"
            $script:Warnings++
        }
    }
    catch {
        Write-Host "WARN" -ForegroundColor Yellow -NoNewline
        Write-Host " Could not test API key: $_"
        $script:Warnings++
    }
    Write-Host ""
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

function Test-BedrockInferenceProfile {
    $region = $env:AWS_REGION
    if ([string]::IsNullOrEmpty($region)) { $region = "us-west-2" }

    $testModel = if ($ExpectedEnvVars["ANTHROPIC_DEFAULT_SONNET_MODEL"]) {
        $ExpectedEnvVars["ANTHROPIC_DEFAULT_SONNET_MODEL"]
    } else { "global.anthropic.claude-sonnet-4-6" }

    Write-Host "Bedrock Inference Profile Access" -ForegroundColor Cyan
    Write-Host "  Testing inference profile: $testModel"

    try {
        $body = '{"anthropic_version":"bedrock-2023-05-31","max_tokens":10,"messages":[{"role":"user","content":"test"}]}'
        $result = aws bedrock-runtime invoke-model `
            --region $region `
            --model-id $testModel `
            --body $body `
            --cli-binary-format raw-in-base64-out `
            NUL 2>&1

        if ($LASTEXITCODE -eq 0) {
            Write-Host "PASS" -ForegroundColor Green -NoNewline
            Write-Host " Inference profile accessible"
        }
        elseif ($result -match "access denied|not authorized|forbidden") {
            Write-Host "FAIL" -ForegroundColor Red -NoNewline
            Write-Host " Bedrock model access denied"
            Write-Host "     Did you complete the Anthropic FTU form?"
            Write-Host "     -> https://${region}.console.aws.amazon.com/bedrock/home?region=${region}#/anthropic-model-access"
            $script:Errors++
        }
        elseif ($result -match "could not connect|timeout|network") {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " Could not reach Bedrock (network issue?)"
            $script:Warnings++
        }
        else {
            Write-Host "WARN" -ForegroundColor Yellow -NoNewline
            Write-Host " Inference profile test returned unexpected result"
            Write-Host "     $result"
            $script:Warnings++
        }
    }
    catch {
        Write-Host "WARN" -ForegroundColor Yellow -NoNewline
        Write-Host " Could not test inference profile: $_"
        $script:Warnings++
    }
    Write-Host ""
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

# Detect auth mode
$authMode = Get-AuthMode

# System Info
Write-Host "System" -ForegroundColor Cyan
Write-Host "  OS:    Windows"
Write-Host "  Shell: PowerShell $($PSVersionTable.PSVersion)"
Write-Host "  Auth:  $authMode"
Write-Host ""

# Credential conflict check
Test-CredentialConflicts

# Environment Variables
Write-Host "Environment Variables" -ForegroundColor Cyan
foreach ($var in $ExpectedEnvVars.Keys) {
    Test-EnvVarExact -VarName $var -Expected $ExpectedEnvVars[$var]
}
foreach ($var in $RequiredEnvVars) {
    Test-EnvVarExists -VarName $var
}
Test-ApiKey
Write-Host ""

# Authentication check (depends on auth mode)
if ($authMode -eq "api-key") {
    Write-Host "Authentication (API Key)" -ForegroundColor Cyan
    Write-Host "PASS" -ForegroundColor Green -NoNewline
    Write-Host " Using Bedrock API key authentication"
    Write-Host "     (Skipping IAM credential check - not needed with API key)"
    Write-Host ""

    # Test if the API key actually works
    Test-ApiKeyValidity
}
else {
    # AWS Credentials
    Write-Host "AWS Credentials (IAM/SSO)" -ForegroundColor Cyan
    Test-AwsCredentials
    Write-Host ""

    # Bedrock Access (only test with IAM - API key can't be tested without making a call)
    Write-Host "Bedrock Access" -ForegroundColor Cyan
    Test-BedrockAccess
    Write-Host ""

    # Bedrock Inference Profile Access
    Test-BedrockInferenceProfile
}

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
