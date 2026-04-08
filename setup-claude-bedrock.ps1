# Claude Code - Amazon Bedrock Setup Script for Windows
# Usage: .\setup-claude-bedrock.ps1 [-Auth <iam|api-key>] [-BedrockKey <key>] [-PreserveKey] [-Region <region>] [-Model <id>] [-FastModel <id>] [-OpusModel <id>] [-SonnetModel <id>] [-HaikuModel <id>] [-Global] [-ModelPrefix <prefix>] [-Force] [-DryRun]

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
    [string]$OpusModel = "",
    [string]$SonnetModel = "",
    [string]$HaikuModel = "",
    [switch]$Global,
    [string]$ModelPrefix = "",
    [Alias("1m-context")]
    [switch]$OneM,
    [Alias("standard-context")]
    [switch]$NoOneM,
    [switch]$Force,
    [switch]$DryRun,
    [switch]$Help,
    [Alias("v")]
    [switch]$Version
)

# Track if parameters were explicitly provided by the user
$AuthExplicit = $PSBoundParameters.ContainsKey('Auth')
$ModelExplicit = $PSBoundParameters.ContainsKey('Model')
$FastModelExplicit = $PSBoundParameters.ContainsKey('FastModel')
$OpusModelExplicit = $PSBoundParameters.ContainsKey('OpusModel')
$SonnetModelExplicit = $PSBoundParameters.ContainsKey('SonnetModel')
$HaikuModelExplicit = $PSBoundParameters.ContainsKey('HaikuModel')

$StorageExplicit = $PSBoundParameters.ContainsKey('Storage')

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

function Get-ExistingOpusModel {
    param([string]$ProfilePath)
    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# OpusModel: (.+)$") {
            return $Matches[1].Trim()
        }
    }
    return $null
}

function Get-ExistingSonnetModel {
    param([string]$ProfilePath)
    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# SonnetModel: (.+)$") {
            return $Matches[1].Trim()
        }
    }
    return $null
}

function Get-ExistingHaikuModel {
    param([string]$ProfilePath)
    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# HaikuModel: (.+)$") {
            return $Matches[1].Trim()
        }
    }
    return $null
}

# Detect existing storage mode from profile file
# Returns: "keychain" or "profile" (absence of marker = profile)
function Get-ExistingStorageMode {
    param([string]$ProfilePath)

    if (Test-Path $ProfilePath) {
        $content = Get-Content $ProfilePath -Raw -ErrorAction SilentlyContinue
        if ($content -match "# Storage: keychain") {
            return "keychain"
        }
    }
    return "profile"
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

    # Strip [1m] suffix before validation
    $checkId = $ModelId -replace '\[1m\]$', ''

    # Basic format check (Bedrock model ID patterns)
    if ($checkId -notmatch "^([a-z][-a-z0-9]*\.)?anthropic\.") {
        Write-Host "Warning: '$ModelId' doesn't match expected Bedrock model ID format" -ForegroundColor Yellow
        Write-Host "Expected patterns: anthropic.claude-*, global.anthropic.claude-*, us.anthropic.claude-*" -ForegroundColor Yellow
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

function Apply-ModelPrefix {
    param([string]$Model, [string]$Prefix)

    if ([string]::IsNullOrEmpty($Prefix) -or $Prefix -eq "global") {
        return $Model
    }

    if ($Model -like "global.anthropic.*") {
        return "$Prefix.anthropic.$($Model.Substring('global.anthropic.'.Length))"
    } elseif ($Model -like "anthropic.*") {
        return "$Prefix.anthropic.$($Model.Substring('anthropic.'.Length))"
    } else {
        return "$Prefix.$Model"
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

# Detect existing per-model overrides
if (-not $OpusModelExplicit) {
    $existingOpusModel = Get-ExistingOpusModel -ProfilePath $ProfilePathForDetection
    if ($existingOpusModel) {
        $OpusModel = $existingOpusModel
        Write-Host "Preserving existing custom opus model: $OpusModel"
    }
}
if (-not $SonnetModelExplicit) {
    $existingSonnetModel = Get-ExistingSonnetModel -ProfilePath $ProfilePathForDetection
    if ($existingSonnetModel) {
        $SonnetModel = $existingSonnetModel
        Write-Host "Preserving existing custom sonnet model: $SonnetModel"
    }
}
if (-not $HaikuModelExplicit) {
    $existingHaikuModel = Get-ExistingHaikuModel -ProfilePath $ProfilePathForDetection
    if ($existingHaikuModel) {
        $HaikuModel = $existingHaikuModel
        Write-Host "Preserving existing custom haiku model: $HaikuModel"
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

# Handle "default" reset for per-model overrides
if ($OpusModel -eq "default") { $OpusModel = ""; Write-Host "Resetting opus model to default" }
if ($SonnetModel -eq "default") { $SonnetModel = ""; Write-Host "Resetting sonnet model to default" }
if ($HaikuModel -eq "default") { $HaikuModel = ""; Write-Host "Resetting haiku model to default" }

# Detect existing storage mode if user didn't explicitly specify
if (-not $StorageExplicit) {
    $existingStorage = Get-ExistingStorageMode -ProfilePath $ProfilePathForDetection
    if ($existingStorage -eq "keychain") {
        $Storage = "keychain"
        Write-Host "Preserving existing storage mode: keychain"
    }
}

# Detect existing 1M context flag
if (-not $OneM -and -not $NoOneM) {
    $profileContent = Get-Content $ProfilePathForDetection -Raw -ErrorAction SilentlyContinue
    if ($profileContent -match '# 1MContext: true') {
        $OneM = $true
        Write-Host "Preserving existing 1M context setting"
    }
}

# Platform-aware storage default for new installs (Windows defaults to keychain)
if (-not $StorageExplicit -and $Storage -eq "profile") {
    # On Windows, Credential Manager is always available
    $profileContent = Get-Content $ProfilePathForDetection -Raw -ErrorAction SilentlyContinue
    if (-not ($profileContent -match "CLAUDE_CODE_USE_BEDROCK")) {
        # New install — default to keychain on Windows
        $Storage = "keychain"
    }
}

# Offer to migrate plaintext API keys to Credential Manager
if ($Auth -eq "api-key" -and -not $StorageExplicit -and $Storage -eq "profile") {
    $profileContent = Get-Content $ProfilePathForDetection -Raw -ErrorAction SilentlyContinue
    if ($profileContent -match "CLAUDE_CODE_USE_BEDROCK" -and $profileContent -notmatch "# Storage: keychain") {
        if ($Force) {
            $Storage = "keychain"
            Write-Host "Migrating API key to Windows Credential Manager (more secure)"
        } elseif (-not $DryRun) {
            Write-Host ""
            Write-Host "Your API key is stored in plaintext in your PowerShell profile." -ForegroundColor Yellow
            $response = Read-Host "Move to Windows Credential Manager for better security? (y/n)"
            if ($response -eq "y" -or $response -eq "Y") {
                $Storage = "keychain"
                Write-Host "Migrating API key to Windows Credential Manager"
            }
        }
    }
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
    # --fast-model sets ANTHROPIC_DEFAULT_HAIKU_MODEL (ANTHROPIC_SMALL_FAST_MODEL was removed)
    if ([string]::IsNullOrEmpty($HaikuModel)) {
        $HaikuModel = $FastModel
    }
}

# Validate per-model overrides
foreach ($entry in @(@{Id=$OpusModel; Type="OpusModel"}, @{Id=$SonnetModel; Type="SonnetModel"}, @{Id=$HaikuModel; Type="HaikuModel"})) {
    if (-not [string]::IsNullOrEmpty($entry.Id)) {
        if (-not (Test-ModelIdFormat -ModelId $entry.Id -ModelType $entry.Type)) { exit 1 }
        Show-CustomModelWarning -ModelId $entry.Id -ModelType $entry.Type
    }
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
    # Use Win32 CredRead API to retrieve from Credential Manager (no external modules)
    try {
        Add-Type -Namespace 'Win32' -Name 'Credential' -MemberDefinition '
            [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
            public static extern bool CredRead(string target, int type, int flags, out IntPtr credential);
            [DllImport("advapi32.dll")]
            public static extern void CredFree(IntPtr credential);
            [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
            public struct CREDENTIAL {
                public int Flags; public int Type;
                public string TargetName; public string Comment;
                public long LastWritten; public int CredentialBlobSize;
                public IntPtr CredentialBlob; public int Persist;
                public int AttributeCount; public IntPtr Attributes;
                public string TargetAlias; public string UserName;
            }
        ' -ErrorAction SilentlyContinue
        $ptr = [IntPtr]::Zero
        if ([Win32.Credential]::CredRead($KeychainTarget, 1, 0, [ref]$ptr)) {
            $cred = [Runtime.InteropServices.Marshal]::PtrToStructure($ptr, [Type][Win32.Credential+CREDENTIAL])
            $password = $null
            if ($cred.CredentialBlobSize -gt 0) {
                $password = [Runtime.InteropServices.Marshal]::PtrToStringUni($cred.CredentialBlob, $cred.CredentialBlobSize / 2)
            }
            [Win32.Credential]::CredFree($ptr)
            return $password
        }
        return $null
    } catch {
        return $null
    }
}

function Remove-KeychainCredential {
    $null = cmdkey /delete:$KeychainTarget 2>$null
}

# Generate the PowerShell command to retrieve key from Credential Manager
function Get-KeychainRetrievalCommand {
    # This generates the code that will run in the profile to get the credential
    # Uses Win32 CredRead API - no external modules required
    return @'
& { Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition '[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)] public static extern bool CredRead(string t, int ty, int f, out IntPtr c); [DllImport("advapi32.dll")] public static extern void CredFree(IntPtr c); [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] public struct CREDENTIAL { public int Flags; public int Type; public string TargetName; public string Comment; public long LastWritten; public int CredentialBlobSize; public IntPtr CredentialBlob; public int Persist; public int AttributeCount; public IntPtr Attributes; public string TargetAlias; public string UserName; }' -ErrorAction SilentlyContinue; $p=[IntPtr]::Zero; if([Win32.Cred]::CredRead('juggernaut-bedrock',1,0,[ref]$p)){ $c=[Runtime.InteropServices.Marshal]::PtrToStructure($p,[Type][Win32.Cred+CREDENTIAL]); $r=$null; if($c.CredentialBlobSize -gt 0){$r=[Runtime.InteropServices.Marshal]::PtrToStringUni($c.CredentialBlob,$c.CredentialBlobSize/2)}; [Win32.Cred]::CredFree($p); $r } }
'@
}

# Show version
if ($Version) {
    $versionFile = Join-Path $ScriptDir "VERSION"
    if (Test-Path $versionFile) { Get-Content $versionFile } else { Write-Host "unknown" }
    exit 0
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
    Write-Host "  -Storage <MODE>    Where to store API key: profile or keychain"
    Write-Host "                     Default: keychain on Windows, profile on Linux"
    Write-Host "  -Region <REGION>   AWS region (default: us-west-2)"
    Write-Host "  -Model <ID>        Custom primary model (use 'default' to reset)"
    Write-Host "  -FastModel <ID>    Custom fast model (use 'default' to reset)"
    Write-Host "  -OpusModel <ID>    Custom Opus model (use 'default' to reset)"
    Write-Host "  -SonnetModel <ID>  Custom Sonnet model (use 'default' to reset)"
    Write-Host "  -HaikuModel <ID>   Custom Haiku model (use 'default' to reset)"
    Write-Host "  -Global            Use global inference profiles (default)"
    Write-Host "  -ModelPrefix <PFX> Custom model prefix (e.g., 'eu', 'ap')"
    Write-Host "  -OneM              Enable 1M token context window (Opus & Sonnet only)"
    Write-Host "  -NoOneM            Disable 1M context (revert to standard ~200K)"
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
    Write-Host "  profile    Store API key directly in PowerShell profile"
    Write-Host "  keychain   Store API key in Windows Credential Manager (default, more secure)"
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
    } elseif (-not $AuthExplicit) {
        # Auth mode was auto-detected from existing config — try to reuse key automatically
        $existingKey = [Environment]::GetEnvironmentVariable("AWS_BEARER_TOKEN_BEDROCK")
        if (-not [string]::IsNullOrEmpty($existingKey)) {
            $BedrockKey = $existingKey
            Write-Host "Using existing API key from environment"
        } elseif ($Storage -eq "keychain") {
            $keychainKey = Get-KeychainCredential
            if (-not [string]::IsNullOrEmpty($keychainKey)) {
                $BedrockKey = $keychainKey
                Write-Host "Using existing API key from Credential Manager"
            }
        }
        # If still empty, fall through to prompt below
        if ([string]::IsNullOrEmpty($BedrockKey)) {
            if ($DryRun) {
                Write-Host "[DRY RUN] Would prompt for Bedrock API key" -ForegroundColor Magenta
                $BedrockKey = "dry-run-placeholder"
            } elseif (-not [Environment]::UserInteractive -or [Console]::IsInputRedirected) {
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

if ($DryRun) {
    Write-Host "[DRY RUN] No changes will be made" -ForegroundColor Magenta
    Write-Host ""
}

$AuthDisplay = if ($Auth -eq "api-key") { "API key" } else { "IAM/SSO" }
Write-Host "Setting up Claude Code with Amazon Bedrock ($AuthDisplay auth)..." -ForegroundColor Cyan

# Apply -Global or -ModelPrefix to model env vars
if ($Global) { $ModelPrefix = "global" }
if (-not [string]::IsNullOrEmpty($ModelPrefix)) {
    $modelKeys = @("ANTHROPIC_MODEL",
                    "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL")
    foreach ($key in $modelKeys) {
        $prop = $Config.environment.PSObject.Properties[$key]
        if ($prop) {
            $prop.Value = Apply-ModelPrefix -Model $prop.Value -Prefix $ModelPrefix
        }
    }

    # Update friendly names and descriptions to match the prefix
    if ($ModelPrefix -ne "global") {
        $prefixLabel = $ModelPrefix.ToUpper()
        $nameKeys = @("ANTHROPIC_DEFAULT_OPUS_MODEL_NAME", "ANTHROPIC_DEFAULT_SONNET_MODEL_NAME", "ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME")
        foreach ($key in $nameKeys) {
            $prop = $Config.environment.PSObject.Properties[$key]
            if ($prop) {
                $prop.Value = $prop.Value -replace 'Bedrock Global', "Bedrock $prefixLabel"
            }
        }
        $descKeys = @("ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION", "ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION", "ANTHROPIC_DEFAULT_HAIKU_MODEL_DESCRIPTION")
        foreach ($key in $descKeys) {
            $prop = $Config.environment.PSObject.Properties[$key]
            if ($prop) {
                $prop.Value = $prop.Value -replace 'Global inference profile', "$prefixLabel inference profile"
            }
        }
    }
}

# Apply 1M context (Opus & Sonnet only)
if ($OneM) {
    # Append [1m] to model IDs
    foreach ($key in @("ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL")) {
        $prop = $Config.environment.PSObject.Properties[$key]
        if ($prop) {
            $prop.Value = "$($prop.Value)[1m]"
        }
    }

    # Update names: append ", 1M Context" before closing paren
    foreach ($key in @("ANTHROPIC_DEFAULT_OPUS_MODEL_NAME", "ANTHROPIC_DEFAULT_SONNET_MODEL_NAME")) {
        $prop = $Config.environment.PSObject.Properties[$key]
        if ($prop -and $prop.Value -notlike '*1M Context*') {
            $prop.Value = $prop.Value -replace '\)$', ', 1M Context)'
        }
    }

    # Update descriptions: append "(1M Context)"
    foreach ($key in @("ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION", "ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION")) {
        $prop = $Config.environment.PSObject.Properties[$key]
        if ($prop) {
            $prop.Value = "$($prop.Value) (1M Context)"
        }
    }

    # Apply to custom overrides (idempotent - skip if already suffixed)
    if (-not [string]::IsNullOrEmpty($OpusModel) -and $OpusModel -ne "default") {
        if ($OpusModel -notmatch '\[1m\]$') { $OpusModel = "$OpusModel[1m]" }
    }
    if (-not [string]::IsNullOrEmpty($SonnetModel) -and $SonnetModel -ne "default") {
        if ($SonnetModel -notmatch '\[1m\]$') { $SonnetModel = "$SonnetModel[1m]" }
    }
}

# Build configuration block from JSON config or fallback to defaults
$ConfigBlock = "`n# BEGIN: Claude Code Bedrock Configuration`n# Auth mode: $Auth`n"
if (-not [string]::IsNullOrEmpty($Model)) {
    $ConfigBlock += "# Model: $Model`n"
}
if (-not [string]::IsNullOrEmpty($FastModel)) {
    $ConfigBlock += "# FastModel: $FastModel`n"
}
if (-not [string]::IsNullOrEmpty($OpusModel)) {
    $ConfigBlock += "# OpusModel: $OpusModel`n"
}
if (-not [string]::IsNullOrEmpty($SonnetModel)) {
    $ConfigBlock += "# SonnetModel: $SonnetModel`n"
}
if (-not [string]::IsNullOrEmpty($HaikuModel)) {
    $ConfigBlock += "# HaikuModel: $HaikuModel`n"
}
if ($OneM) {
    $ConfigBlock += "# 1MContext: true`n"
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
        if ($_.Name -eq "ANTHROPIC_DEFAULT_OPUS_MODEL" -and -not [string]::IsNullOrEmpty($OpusModel)) {
            $value = $OpusModel
        }
        if ($_.Name -eq "ANTHROPIC_DEFAULT_SONNET_MODEL" -and -not [string]::IsNullOrEmpty($SonnetModel)) {
            $value = $SonnetModel
        }
        if ($_.Name -eq "ANTHROPIC_DEFAULT_HAIKU_MODEL" -and -not [string]::IsNullOrEmpty($HaikuModel)) {
            $value = $HaikuModel
        }
        # Escape double quotes in values
        $escapedValue = $value -replace '"', '`"'
        $ConfigBlock += "`$env:$($_.Name) = `"$escapedValue`"`n"
    }
} else {
    Write-Host "Error: Could not load environment variables from config file" -ForegroundColor Red
    Write-Host "Ensure bedrock-config.json exists and is valid JSON" -ForegroundColor Red
    exit 1
}

# Add API key if using api-key auth
if ($Auth -eq "api-key") {
    if ($Storage -eq "keychain") {
        # Retrieve from Credential Manager at profile load using Win32 CredRead API
        $retrievalCmd = Get-KeychainRetrievalCommand
        $ConfigBlock += "`$env:AWS_BEARER_TOKEN_BEDROCK = $retrievalCmd`n"
    } else {
        # Store directly in profile (legacy behavior)
        $ConfigBlock += "`$env:AWS_BEARER_TOKEN_BEDROCK = `"$BedrockKey`"`n"
    }
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
    if ($BedrockKey.Length -gt 12) {
        $MaskedKey = $BedrockKey.Substring(0, 8) + "..." + $BedrockKey.Substring($BedrockKey.Length - 4)
    } else {
        $MaskedKey = "****"
    }
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
        $ProfileContent = $ProfileContent -replace "(?ms)\r?\n?# Claude Code - Amazon Bedrock Configuration.*?`$env:ANTHROPIC_(?:SMALL_FAST_)?MODEL = `"[^`"]+`"\r?\n?", "`n"
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
Write-Host "1. Restart PowerShell, or reload your profile:"
Write-Host "   . `$PROFILE.CurrentUserAllHosts" -ForegroundColor White
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
