# Claude Code - Amazon Bedrock Setup Script for Windows
# Usage: .\setup-claude-bedrock.ps1 [-Auth <iam|api-key>] [-BedrockKey <key>] [-Region <region>] [-Force] [-DryRun]

param(
    [ValidateSet("iam", "api-key")]
    [string]$Auth = "iam",
    [string]$BedrockKey = "",
    [string]$Region = "us-west-2",
    [switch]$Force,
    [switch]$DryRun,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

# Show help
if ($Help) {
    Write-Host "Claude Code - Amazon Bedrock Setup Script"
    Write-Host ""
    Write-Host "Usage: .\setup-claude-bedrock.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Auth <MODE>       Authentication: iam (default) or api-key"
    Write-Host "  -BedrockKey <KEY>  Bedrock API key (optional; prompts if not provided)"
    Write-Host "  -Region <REGION>   AWS region (default: us-west-2)"
    Write-Host "  -Force             Overwrite existing configuration without prompting"
    Write-Host "  -DryRun            Preview changes without modifying files"
    Write-Host "  -Help              Show this help message"
    Write-Host ""
    Write-Host "Authentication Modes:"
    Write-Host "  iam        Use AWS IAM/SSO credentials (default)"
    Write-Host "             Requires: aws configure, SSO login, or IAM role"
    Write-Host ""
    Write-Host "  api-key    Use Bedrock API key (simpler setup)"
    Write-Host "             Prompts securely if -BedrockKey not provided"
    Write-Host "             Get key from: AWS Console -> Bedrock -> API keys"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\setup-claude-bedrock.ps1                              # IAM/SSO (default)"
    Write-Host "  .\setup-claude-bedrock.ps1 -Auth api-key                # Prompts for key"
    Write-Host "  .\setup-claude-bedrock.ps1 -Auth api-key -BedrockKey br-xxx  # Inline key"
    Write-Host "  .\setup-claude-bedrock.ps1 -DryRun"
    exit 0
}

# Prompt for API key if using api-key auth and key not provided
if ($Auth -eq "api-key" -and [string]::IsNullOrEmpty($BedrockKey)) {
    if ($DryRun) {
        Write-Host "[DRY RUN] Would prompt for Bedrock API key" -ForegroundColor Magenta
        $BedrockKey = "dry-run-placeholder"
    } else {
        Write-Host "Get your Bedrock API key from:"
        Write-Host "  AWS Console -> Amazon Bedrock -> API keys"
        Write-Host ""
        $SecureKey = Read-Host "Enter your Bedrock API key" -AsSecureString
        $BedrockKey = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureKey))

        if ([string]::IsNullOrEmpty($BedrockKey)) {
            Write-Host "Error: API key cannot be empty" -ForegroundColor Red
            exit 1
        }
    }
}

# Valid AWS regions that support Bedrock (as of 2025)
$ValidRegions = @(
    "us-east-1", "us-east-2", "us-west-2",
    "eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-central-2",
    "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-northeast-1", "ap-northeast-2", "ap-south-1",
    "sa-east-1", "ca-central-1", "me-south-1", "me-central-1", "il-central-1"
)

# Validate region
if (-not ($ValidRegions -contains $Region)) {
    Write-Host "Warning: '$Region' may not be a valid Bedrock region" -ForegroundColor Yellow
    Write-Host "   Common Bedrock regions: us-east-1, us-west-2, eu-west-1, ap-northeast-1"
    if (-not $Force -and -not $DryRun) {
        $response = Read-Host "Continue anyway? (y/n)"
        if ($response -ne "y" -and $response -ne "Y") {
            Write-Host "Setup cancelled" -ForegroundColor Red
            exit 1
        }
    }
}

if ($DryRun) {
    Write-Host "[DRY RUN] No changes will be made" -ForegroundColor Magenta
    Write-Host ""
}

$AuthDisplay = if ($Auth -eq "api-key") { "API key" } else { "IAM/SSO" }
Write-Host "Setting up Claude Code with Amazon Bedrock ($AuthDisplay auth)..." -ForegroundColor Cyan

# Build configuration block
$ConfigBlock = @"

# BEGIN: Claude Code Bedrock Configuration
# Auth mode: $Auth
`$env:CLAUDE_CODE_USE_BEDROCK = "1"
`$env:AWS_REGION = "$Region"
`$env:CLAUDE_CODE_MAX_OUTPUT_TOKENS = "16384"
`$env:MAX_THINKING_TOKENS = "1024"
`$env:ANTHROPIC_MODEL = "global.anthropic.claude-opus-4-5-20251101-v1:0"
`$env:ANTHROPIC_SMALL_FAST_MODEL = "global.anthropic.claude-sonnet-4-5-20250929-v1:0"
`$env:DISABLE_ERROR_REPORTING = "1"
`$env:DISABLE_TELEMETRY = "1"
`$env:DISABLE_AUTOUPDATE = "1"
`$env:DISABLE_BUG_COMMAND = "1"
"@

# Add API key if using api-key auth
if ($Auth -eq "api-key") {
    $ConfigBlock += "`n`$env:AWS_BEARER_TOKEN_BEDROCK = `"$BedrockKey`""
}

$ConfigBlock += "`n# END: Claude Code Bedrock Configuration"

# Determine PowerShell profile path
$ProfilePath = $PROFILE.CurrentUserAllHosts

# Create profile directory if it doesn't exist
$ProfileDir = Split-Path -Parent $ProfilePath
if (-not (Test-Path $ProfileDir)) {
    if (-not $DryRun) {
        New-Item -ItemType Directory -Path $ProfileDir -Force | Out-Null
    }
}

# Create profile file if it doesn't exist
if (-not (Test-Path $ProfilePath)) {
    if (-not $DryRun) {
        New-Item -ItemType File -Path $ProfilePath -Force | Out-Null
    }
}

Write-Host "Target:   $ProfilePath" -ForegroundColor Gray
Write-Host "Region:   $Region" -ForegroundColor Gray
Write-Host "Auth:     $Auth" -ForegroundColor Gray
if ($Auth -eq "api-key") {
    $MaskedKey = $BedrockKey.Substring(0, [Math]::Min(8, $BedrockKey.Length)) + "..." + $BedrockKey.Substring([Math]::Max(0, $BedrockKey.Length - 4))
    Write-Host "API Key:  $MaskedKey" -ForegroundColor Gray
}
Write-Host ""

# Backup existing profile before any modifications
if ((Test-Path $ProfilePath) -and -not $DryRun) {
    $BackupPath = "$ProfilePath.backup.$(Get-Date -Format 'yyyyMMdd_HHmmss')"
    try {
        Copy-Item -Path $ProfilePath -Destination $BackupPath -ErrorAction Stop
        Write-Host "Backup created: $BackupPath" -ForegroundColor Gray
    } catch {
        Write-Host "Warning: Could not create backup at $BackupPath" -ForegroundColor Yellow
    }
} elseif ((Test-Path $ProfilePath) -and $DryRun) {
    Write-Host "[DRY RUN] Would create backup of $ProfilePath" -ForegroundColor Magenta
}

# Check if configuration already exists
$ProfileContent = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
if ($ProfileContent -match "CLAUDE_CODE_USE_BEDROCK") {
    Write-Host "Existing configuration found" -ForegroundColor Yellow

    if (-not $Force -and -not $DryRun) {
        $response = Read-Host "Replace existing configuration? (y/n)"
        if ($response -ne "y" -and $response -ne "Y") {
            Write-Host "Setup cancelled" -ForegroundColor Red
            exit 0
        }
    }

    if ($DryRun) {
        Write-Host "[DRY RUN] Would remove existing configuration" -ForegroundColor Magenta
    } else {
        # Remove old configuration (supports both old and new marker formats)
        $ProfileContent = $ProfileContent -replace "(?ms)\r?\n?# BEGIN: Claude Code Bedrock Configuration.*?# END: Claude Code Bedrock Configuration\r?\n?", "`n"
        $ProfileContent = $ProfileContent -replace "(?ms)\r?\n?# Claude Code - Amazon Bedrock Configuration.*?`$env:ANTHROPIC_SMALL_FAST_MODEL = `"[^`"]+`"\r?\n?", "`n"
        # Remove multiple consecutive blank lines
        $ProfileContent = $ProfileContent -replace "(\r?\n){3,}", "`n`n"
        Set-Content -Path $ProfilePath -Value $ProfileContent.TrimEnd() -NoNewline
        Add-Content -Path $ProfilePath -Value ""
        Write-Host "Removed existing configuration" -ForegroundColor Gray
    }
}

# Add new configuration
if ($DryRun) {
    Write-Host ""
    Write-Host "Would append to $ProfilePath`:" -ForegroundColor Cyan
    Write-Host "-------------------------------------" -ForegroundColor Gray
    Write-Host $ConfigBlock -ForegroundColor White
    Write-Host "-------------------------------------" -ForegroundColor Gray
    Write-Host ""
    Write-Host "[DRY RUN] No changes made" -ForegroundColor Green
    exit 0
}

try {
    Add-Content -Path $ProfilePath -Value $ConfigBlock -ErrorAction Stop
} catch {
    Write-Host "" -ForegroundColor Red
    Write-Host "ERROR: Cannot write to $ProfilePath" -ForegroundColor Red
    Write-Host "Possible causes:" -ForegroundColor Red
    Write-Host "  - File or directory is read-only" -ForegroundColor Red
    Write-Host "  - Insufficient permissions" -ForegroundColor Red
    Write-Host "  - Disk is full" -ForegroundColor Red
    Write-Host "" -ForegroundColor Red
    Write-Host "Try running PowerShell as Administrator or check disk space." -ForegroundColor Red
    exit 1
}

Write-Host "Configuration added to $ProfilePath" -ForegroundColor Green
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "1. Reload PowerShell profile:"
Write-Host "   . `$PROFILE" -ForegroundColor White
Write-Host ""

if ($Auth -eq "api-key") {
    Write-Host "2. Launch Claude Code:"
    Write-Host "   claude" -ForegroundColor White
    Write-Host ""
    Write-Host "   (No AWS credential setup needed - using API key)"
} else {
    Write-Host "2. Verify AWS credentials:"
    Write-Host "   aws sts get-caller-identity" -ForegroundColor White
    Write-Host ""
    Write-Host "3. Launch Claude Code:"
    Write-Host "   claude" -ForegroundColor White
}

Write-Host ""
Write-Host "Setup complete!" -ForegroundColor Green
