# Claude Code - Amazon Bedrock Setup Script for Windows
# Usage: .\setup-claude-bedrock.ps1 [-Auth <iam|api-key>] [-BedrockKey <key>] [-PreserveKey] [-Region <region>] [-Force] [-DryRun]

param(
    [ValidateSet("iam", "api-key")]
    [string]$Auth = "",
    [string]$BedrockKey = "",
    [switch]$PreserveKey,
    [ValidateSet("profile", "keychain")]
    [string]$Storage = "profile",
    [string]$Region = "",
    [string]$Model = "",
    [string]$FastModel = "",
    [switch]$Force,
    [switch]$DryRun,
    [switch]$Help
)

# Track if parameters were explicitly provided by the user
$AuthExplicit = $PSBoundParameters.ContainsKey('Auth')
$ModelExplicit = $PSBoundParameters.ContainsKey('Model')
$FastModelExplicit = $PSBoundParameters.ContainsKey('FastModel')

$ErrorActionPreference = "Stop"

#───────────────────────────────────────────────────────────────────────────────
# Load Configuration from JSON
#───────────────────────────────────────────────────────────────────────────────

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ConfigFile = Join-Path $ScriptDir "bedrock-config.json"
$Config = $null

if (Test-Path $ConfigFile) {
    try {
        $Config = Get-Content $ConfigFile -Raw | ConvertFrom-Json
    } catch {
        Write-Host "Warning: Could not parse config file: $ConfigFile" -ForegroundColor Yellow
    }
}

# Detect existing auth mode from profile file
# Returns: "iam", "api-key", or $null if not found
function Get-ExistingAuthMode {
    param([string]$ProfilePath)

    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# Auth mode: (iam|api-key)") {
            return $Matches[1]
        }
    }
    return $null
}

# Detect existing custom model from profile file
function Get-ExistingModel {
    param([string]$ProfilePath)

    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# Model: (.+)$" ) {
            return $Matches[1].Trim()
        }
    }
    return $null
}

# Detect existing custom fast model from profile file
function Get-ExistingFastModel {
    param([string]$ProfilePath)

    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# FastModel: (.+)$") {
            return $Matches[1].Trim()
        }
    }
    return $null
}

# Validate model ID format
function Test-ModelIdFormat {
    param([string]$ModelId, [string]$ModelType)

    # "default" is a special value to reset to bedrock-config.json
    if ($ModelId -eq "default") {
        return $true
    }

    # Non-empty check
    if ([string]::IsNullOrEmpty($ModelId)) {
        Write-Host "Error: -$ModelType model ID cannot be empty" -ForegroundColor Red
        return $false
    }

    # Basic format check (Bedrock model ID patterns)
    if ($ModelId -notmatch "^(global\.)?anthropic\.") {
        Write-Host "Warning: '$ModelId' doesn't match expected Bedrock model ID format" -ForegroundColor Yellow
        Write-Host "Expected patterns: anthropic.claude-* or global.anthropic.claude-*" -ForegroundColor Yellow
    }

    return $true
}

# Warn user about custom model usage
function Show-CustomModelWarning {
    param([string]$ModelId, [string]$ModelType)

    Write-Host ""
    Write-Host "Warning: Custom $ModelType model: $ModelId" -ForegroundColor Yellow
    Write-Host "   Cannot validate without working AWS credentials."
    Write-Host "   Ensure this model is available in your Bedrock region."
    Write-Host ""

    if (-not $Force -and -not $DryRun) {
        $response = Read-Host "Continue with custom model? (y/n)"
        if ($response -ne "y" -and $response -ne "Y") {
            Write-Host "Setup cancelled" -ForegroundColor Red
            exit 0
        }
    }
}

# Apply defaults from config or use hardcoded fallbacks
if ([string]::IsNullOrEmpty($Region)) {
    $Region = if ($Config -and $Config.defaults.region) { $Config.defaults.region } else { "us-west-2" }
}

# Detect existing auth mode if user didn't explicitly specify -Auth
$ProfilePathForDetection = $PROFILE.CurrentUserAllHosts
if (-not $AuthExplicit -and [string]::IsNullOrEmpty($Auth)) {
    $existingAuth = Get-ExistingAuthMode -ProfilePath $ProfilePathForDetection
    if ($existingAuth) {
        $Auth = $existingAuth
        Write-Host "Preserving existing auth mode: $Auth"
    }
}

# Apply default if still unset
if ([string]::IsNullOrEmpty($Auth)) {
    $Auth = if ($Config -and $Config.defaults.auth_mode) { $Config.defaults.auth_mode } else { "iam" }
}

# Detect existing custom models if user didn't explicitly specify
if (-not $ModelExplicit) {
    $existingModel = Get-ExistingModel -ProfilePath $ProfilePathForDetection
    if ($existingModel) {
        $Model = $existingModel
        Write-Host "Preserving existing custom model: $Model"
    }
}

if (-not $FastModelExplicit) {
    $existingFastModel = Get-ExistingFastModel -ProfilePath $ProfilePathForDetection
    if ($existingFastModel) {
        $FastModel = $existingFastModel
        Write-Host "Preserving existing custom fast model: $FastModel"
    }
}

# Handle "default" reset value - clears custom model
if ($Model -eq "default") {
    $Model = ""
    Write-Host "Resetting primary model to default from bedrock-config.json"
}
if ($FastModel -eq "default") {
    $FastModel = ""
    Write-Host "Resetting fast model to default from bedrock-config.json"
}

# Validate and warn for custom models
if (-not [string]::IsNullOrEmpty($Model)) {
    if (-not (Test-ModelIdFormat -ModelId $Model -ModelType "Model")) {
        exit 1
    }
    Show-CustomModelWarning -ModelId $Model -ModelType "primary"
}

if (-not [string]::IsNullOrEmpty($FastModel)) {
    if (-not (Test-ModelIdFormat -ModelId $FastModel -ModelType "FastModel")) {
        exit 1
    }
    Show-CustomModelWarning -ModelId $FastModel -ModelType "fast"
}

# Load valid regions from config or use defaults
$ValidRegions = if ($Config -and $Config.regions) {
    $Config.regions
} else {
    @(
        "us-east-1", "us-east-2", "us-west-2",
        "eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-central-2",
        "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-northeast-1", "ap-northeast-2", "ap-south-1",
        "sa-east-1", "ca-central-1", "me-south-1", "me-central-1", "il-central-1"
    )
}

# Show help
if ($Help) {
    Write-Host "Claude Code - Amazon Bedrock Setup Script"
    Write-Host ""
    Write-Host "Usage: .\setup-claude-bedrock.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Auth <MODE>       Authentication: iam (default) or api-key"
    Write-Host "  -BedrockKey <KEY>  Bedrock API key (optional; prompts if not provided)"
    Write-Host "  -PreserveKey       Reuse existing API key from environment (no prompt)"
    Write-Host "  -Storage <MODE>    Where to store API key: profile (default) or keychain"
    Write-Host "  -Region <REGION>   AWS region (default: us-west-2)"
    Write-Host "  -Model <ID>        Custom primary model (use 'default' to reset)"
    Write-Host "  -FastModel <ID>    Custom fast model (use 'default' to reset)"
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
    Write-Host "Storage Modes:"
    Write-Host "  profile    Store API key directly in PowerShell profile (default)"
    Write-Host "  keychain   Store API key in Windows Credential Manager (more secure)"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\setup-claude-bedrock.ps1                              # IAM/SSO (default)"
    Write-Host "  .\setup-claude-bedrock.ps1 -Auth api-key                # Prompts for key"
    Write-Host "  .\setup-claude-bedrock.ps1 -Auth api-key -Storage keychain  # Secure storage"
    Write-Host "  .\setup-claude-bedrock.ps1 -Auth api-key -BedrockKey br-xxx  # Inline key"
    Write-Host "  .\setup-claude-bedrock.ps1 -Auth api-key -PreserveKey   # Reuse existing key"
    Write-Host "  .\setup-claude-bedrock.ps1 -Model anthropic.claude-3-opus-20240229-v1:0  # Custom model"
    Write-Host "  .\setup-claude-bedrock.ps1 -Model default               # Reset to default"
    Write-Host "  .\setup-claude-bedrock.ps1 -DryRun"
    exit 0
}

# Handle API key for api-key auth mode
if ($Auth -eq "api-key" -and [string]::IsNullOrEmpty($BedrockKey)) {
    if ($PreserveKey) {
        # Reuse existing key from environment
        $existingKey = [Environment]::GetEnvironmentVariable("AWS_BEARER_TOKEN_BEDROCK")
        if (-not [string]::IsNullOrEmpty($existingKey)) {
            $BedrockKey = $existingKey
            Write-Host "Using existing API key from environment"
        } else {
            Write-Host "Error: -PreserveKey specified but AWS_BEARER_TOKEN_BEDROCK is not set" -ForegroundColor Red
            Write-Host "Run setup without -PreserveKey to enter a new key"
            exit 1
        }
    } elseif ($DryRun) {
        Write-Host "[DRY RUN] Would prompt for Bedrock API key" -ForegroundColor Magenta
        $BedrockKey = "dry-run-placeholder"
    } elseif (-not [Environment]::UserInteractive -or [Console]::IsInputRedirected) {
        # Non-interactive mode (CI/CD, piped input, etc.)
        Write-Host "Error: -BedrockKey or -PreserveKey is required in non-interactive mode" -ForegroundColor Red
        Write-Host ""
        Write-Host "Usage: .\setup-claude-bedrock.ps1 -Auth api-key -BedrockKey YOUR_KEY"
        Write-Host "   or: .\setup-claude-bedrock.ps1 -Auth api-key -PreserveKey"
        exit 1
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

# Validate region (ValidRegions loaded from config above)
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

#───────────────────────────────────────────────────────────────────────────────
# Keychain Functions (Windows Credential Manager)
#───────────────────────────────────────────────────────────────────────────────

$KeychainTarget = "juggernaut-bedrock"

function Test-KeychainAvailable {
    # Windows Credential Manager is always available on Windows
    return $true
}

function Set-KeychainCredential {
    param([string]$Key)

    # Use cmdkey to store in Windows Credential Manager
    $null = cmdkey /delete:$KeychainTarget 2>$null
    $result = cmdkey /generic:$KeychainTarget /user:api-key /pass:$Key 2>&1
    return $LASTEXITCODE -eq 0
}

function Get-KeychainCredential {
    # Use PowerShell to retrieve from Credential Manager
    try {
        Add-Type -AssemblyName System.Security -ErrorAction SilentlyContinue
        $cred = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto(
            [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR(
                (Get-StoredCredential -Target $KeychainTarget -ErrorAction Stop).Password
            )
        )
        return $cred
    } catch {
        # Fallback: try using cmdkey list and parse (less reliable)
        return $null
    }
}

function Remove-KeychainCredential {
    $null = cmdkey /delete:$KeychainTarget 2>$null
}

# Generate the PowerShell command to retrieve key from Credential Manager
function Get-KeychainRetrievalCommand {
    # This generates the code that will run in the profile to get the credential
    return @'
(Get-StoredCredential -Target 'juggernaut-bedrock' -ErrorAction SilentlyContinue).GetNetworkCredential().Password
'@
}

if ($DryRun) {
    Write-Host "[DRY RUN] No changes will be made" -ForegroundColor Magenta
    Write-Host ""
}

$AuthDisplay = if ($Auth -eq "api-key") { "API key" } else { "IAM/SSO" }
Write-Host "Setting up Claude Code with Amazon Bedrock ($AuthDisplay auth)..." -ForegroundColor Cyan

# Build configuration block from JSON config or fallback to defaults
$ConfigBlock = "`n# BEGIN: Claude Code Bedrock Configuration`n# Auth mode: $Auth`n"
if (-not [string]::IsNullOrEmpty($Model)) {
    $ConfigBlock += "# Model: $Model`n"
}
if (-not [string]::IsNullOrEmpty($FastModel)) {
    $ConfigBlock += "# FastModel: $FastModel`n"
}
if ($Storage -eq "keychain") {
    $ConfigBlock += "# Storage: keychain (encrypted)`n"
}

# Unset conflicting auth variables to prevent credential conflicts
if ($Auth -eq "api-key") {
    # Using API key - unset AWS STS credentials that might interfere
    $ConfigBlock += "Remove-Item Env:AWS_ACCESS_KEY_ID -ErrorAction SilentlyContinue`n"
    $ConfigBlock += "Remove-Item Env:AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue`n"
    $ConfigBlock += "Remove-Item Env:AWS_SESSION_TOKEN -ErrorAction SilentlyContinue`n"
} else {
    # Using IAM/SSO - unset API key that might interfere
    $ConfigBlock += "Remove-Item Env:AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue`n"
}

# Add AWS_REGION first
$ConfigBlock += "`$env:AWS_REGION = `"$Region`"`n"

# Add environment variables from config
if ($Config -and $Config.environment) {
    $Config.environment.PSObject.Properties | ForEach-Object {
        $value = $_.Value
        # Use custom model if specified
        if ($_.Name -eq "ANTHROPIC_MODEL" -and -not [string]::IsNullOrEmpty($Model)) {
            $value = $Model
        }
        if ($_.Name -eq "ANTHROPIC_SMALL_FAST_MODEL" -and -not [string]::IsNullOrEmpty($FastModel)) {
            $value = $FastModel
        }
        $ConfigBlock += "`$env:$($_.Name) = `"$value`"`n"
    }
} else {
    Write-Host "Error: Could not load environment variables from config file" -ForegroundColor Red
    Write-Host "Ensure bedrock-config.json exists and is valid JSON" -ForegroundColor Red
    exit 1
}

# Add API key if using api-key auth
if ($Auth -eq "api-key") {
    if ($Storage -eq "keychain") {
        # Retrieve from Credential Manager at profile load
        $ConfigBlock += "`$env:AWS_BEARER_TOKEN_BEDROCK = (Get-StoredCredential -Target 'juggernaut-bedrock' -ErrorAction SilentlyContinue).GetNetworkCredential().Password`n"
    } else {
        # Store directly in profile (legacy behavior)
        $ConfigBlock += "`$env:AWS_BEARER_TOKEN_BEDROCK = `"$BedrockKey`"`n"
    }
}

$ConfigBlock += "`n# END: Claude Code Bedrock Configuration"

# Determine PowerShell profile path
$ProfilePath = $PROFILE.CurrentUserAllHosts

#───────────────────────────────────────────────────────────────────────────────
# File Locking - Prevent concurrent modifications
#───────────────────────────────────────────────────────────────────────────────

$LockFile = "$ProfilePath.lock"
$LockAcquired = $false
$LockTimeout = 30  # seconds to wait for lock
$StaleLockAge = 300  # 5 minutes - consider lock stale after this

function Get-FileLock {
    param([string]$LockPath)

    $startTime = Get-Date
    while (((Get-Date) - $startTime).TotalSeconds -lt $LockTimeout) {
        try {
            # Check for stale lock
            if (Test-Path $LockPath) {
                $lockAge = ((Get-Date) - (Get-Item $LockPath).LastWriteTime).TotalSeconds
                if ($lockAge -gt $StaleLockAge) {
                    Write-Host "Removing stale lock file (age: $([int]$lockAge)s)" -ForegroundColor Yellow
                    Remove-Item $LockPath -Force -ErrorAction SilentlyContinue
                }
            }

            # Try to create lock file atomically
            $null = New-Item -Path $LockPath -ItemType File -ErrorAction Stop
            return $true
        } catch {
            # Lock exists, wait and retry
            Start-Sleep -Milliseconds 500
        }
    }
    return $false
}

function Release-FileLock {
    param([string]$LockPath)
    Remove-Item $LockPath -Force -ErrorAction SilentlyContinue
}

if (-not $DryRun) {
    $LockAcquired = Get-FileLock -LockPath $LockFile
    if (-not $LockAcquired) {
        Write-Host "Error: Could not acquire lock on profile file" -ForegroundColor Red
        Write-Host "Another instance may be modifying the profile. Try again later." -ForegroundColor Red
        exit 1
    }
}

# Ensure lock is released on exit
$null = Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action {
    if ($LockAcquired -and (Test-Path $LockFile)) {
        Remove-Item $LockFile -Force -ErrorAction SilentlyContinue
    }
}

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
    Write-Host "Storage:  $Storage" -ForegroundColor Gray
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

# Store API key in Credential Manager if using keychain storage
if ($Auth -eq "api-key" -and $Storage -eq "keychain") {
    if ($DryRun) {
        Write-Host "[DRY RUN] Would store API key in Windows Credential Manager" -ForegroundColor Magenta
    } else {
        if (Set-KeychainCredential -Key $BedrockKey) {
            Write-Host "API key stored in Windows Credential Manager" -ForegroundColor Gray
        } else {
            Write-Host "Error: Failed to store API key in Credential Manager" -ForegroundColor Red
            Release-FileLock -LockPath $LockFile
            exit 1
        }
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
    if ($Storage -eq "keychain") {
        Write-Host "[DRY RUN] API key would be stored in Windows Credential Manager (encrypted)" -ForegroundColor Magenta
    }
    Write-Host "[DRY RUN] No changes made" -ForegroundColor Green
    exit 0
}

try {
    Add-Content -Path $ProfilePath -Value $ConfigBlock -ErrorAction Stop
} catch {
    Release-FileLock -LockPath $LockFile
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

# Release file lock
Release-FileLock -LockPath $LockFile
