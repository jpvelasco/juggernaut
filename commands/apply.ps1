# commands/apply.ps1 - Juggernaut v3 apply subcommand (PowerShell).
# Configures Claude Code to use Amazon Bedrock via settings.json (sole output).

param(
    [string]$Auth          = '',
    [string]$BedrockKey    = '',
    [switch]$PreserveKey,
    [string]$Storage       = '',
    [string]$Region        = '',
    [string]$Model         = '',
    [string]$OpusModel     = '',
    [string]$SonnetModel   = '',
    [string]$HaikuModel    = '',
    [string]$Effort        = '',
    [switch]$OpusPlan,
    [switch]$NoOpusPlan,
    [switch]$Use1MContext,
    [switch]$No1MContext,
    [switch]$NoMantle,
    [string]$MantleUrl     = '',
    [string]$Scope         = 'user',
    [switch]$DryRun,
    [switch]$SkipPreflight,
    [switch]$Help
)

$ErrorActionPreference = 'Continue'
$PSScriptRoot_ = Split-Path -Parent $PSCommandPath
$RepoRoot      = Split-Path -Parent $PSScriptRoot_

$versionFile = Join-Path $RepoRoot 'VERSION'
$JuggernautVersion = if (Test-Path $versionFile) { (Get-Content $versionFile -Raw).Trim() } else { 'unknown' }

if (-not $env:BEDROCK_CONFIG_PATH) {
    $env:BEDROCK_CONFIG_PATH = Join-Path $RepoRoot 'bedrock-config.json'
}

. (Join-Path $RepoRoot 'lib\schema.ps1')
. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\keychain.ps1')

if ($Help) {
    @'
juggernaut apply - configure Claude Code for Amazon Bedrock

Usage: juggernaut.ps1 apply [options]

  -Auth             iam|bedrock-api-key  (required on first run)
  -BedrockKey       Bedrock API key
  -PreserveKey      Reuse existing key from env/keychain
  -Storage          profile|keychain
  -Region           AWS region
  -Model / -OpusModel / -SonnetModel / -HaikuModel   Model overrides
  -Effort           low|medium|high|xhigh|max (default: xhigh)
  -OpusPlan         Use Opus for planning, Sonnet for execution
  -NoOpusPlan       Disable opusplan
  -Use1MContext     Enable 1M token context
  -No1MContext      Disable 1M token context
  -NoMantle         Disable Mantle routing (Mantle is on by default)
  -MantleUrl        Mantle base URL
  -Scope            user|project (default: user)
  -DryRun           Preview without writing
'@
    exit 0
}

$authExplicit = $PSBoundParameters.ContainsKey('Auth')
$storageExplicit = $PSBoundParameters.ContainsKey('Storage')
if ($Auth -eq 'api-key') { $Auth = 'bedrock-api-key' }
$HomeDir = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { [Environment]::GetFolderPath('UserProfile') }

if ($Scope -notin 'user','project') {
    Write-Error "apply: -Scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

$shellMode = 'settings-only'

# ---------------------------------------------------------------------------
# Resolve settings.json path  (--scope only changes write target, no merging)
# ---------------------------------------------------------------------------
$SettingsPath = Resolve-SettingsTarget -Scope $Scope

# ---------------------------------------------------------------------------
# Step 1: Read existing settings
# ---------------------------------------------------------------------------
$existingSettings = [ordered]@{}
if (Test-SettingsExists -Path $SettingsPath) {
    try { $existingSettings = Read-Settings -Path $SettingsPath }
    catch {
        Write-Error "apply: cannot read $SettingsPath - may be corrupted: $_"
        exit 1
    }
}
$hasBlock = Test-HasJuggernautBlock -Settings $existingSettings

# ---------------------------------------------------------------------------
# Step 2: Load existing block and overlay CLI flags.
# ---------------------------------------------------------------------------
$existingBlock = if ($hasBlock) { Get-JuggernautBlockFromSettings -Settings $existingSettings } else { $null }

if ($existingBlock) {
    if (-not $Auth)        { $Auth        = $existingBlock.auth.mode            }
    if ($Auth -eq 'api-key') { $Auth = 'bedrock-api-key' }
    if (-not $Storage)     { $Storage     = $existingBlock.auth.storage         }
    if (-not $Region)      { $Region      = $existingBlock.auth.region          }
    if (-not $Model)       { $Model       = $existingBlock.model                }
    if (-not $OpusModel)   { $OpusModel   = $existingBlock.modelOverrides.opus  }
    if (-not $SonnetModel) { $SonnetModel = $existingBlock.modelOverrides.sonnet }
    if (-not $HaikuModel)  { $HaikuModel  = $existingBlock.modelOverrides.haiku }
    if (-not $Effort)      { $Effort      = $existingBlock.effortLevel          }
    if (-not $OpusPlan -and -not $NoOpusPlan) {
        if ($existingBlock.opusplan) { $OpusPlan = $true } else { $NoOpusPlan = $true }
    }
    if (-not $Use1MContext -and -not $No1MContext) {
        if ($existingBlock.context.use1MContext) { $Use1MContext = $true } else { $No1MContext = $true }
    }
    if (-not $MantleUrl -and $existingBlock.mantle.baseUrl) { $MantleUrl = $existingBlock.mantle.baseUrl }
}

# ---------------------------------------------------------------------------
# Step 3: Auth validation gate.
# On first run (no stored auth), require an explicit -Auth flag UNLESS we can
# detect live credentials. Prevents the installer-poisons-auth bug class where
# CLAUDE_CODE_USE_BEDROCK=1 is written without a working credential path.
# ---------------------------------------------------------------------------
if (-not $authExplicit -and -not $existingBlock) {
    $detected = ''
    if ($env:AWS_BEARER_TOKEN_BEDROCK) {
        $detected = 'bedrock-api-key'
    } elseif ((Test-KeychainAvailable) -and (Get-KeychainEntry -ErrorAction SilentlyContinue)) {
        $detected = 'bedrock-api-key'
    } elseif (Get-Command 'aws' -ErrorAction SilentlyContinue) {
        & aws sts get-caller-identity 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $detected = 'iam' }
    }
    if (-not $detected) {
        Write-Error @'
apply: no authentication mode specified and no credentials detected.

Pass -Auth iam to use AWS IAM credentials (requires `aws configure` or `aws sso login`),
or pass -Auth bedrock-api-key to use a Bedrock API key (supply -BedrockKey KEY or set
AWS_BEARER_TOKEN_BEDROCK).

Juggernaut will not enable CLAUDE_CODE_USE_BEDROCK without a validated auth path --
this prevents Claude Code from hanging on launch.
'@
        exit 2
    }
    $Auth = $detected
}

# ---------------------------------------------------------------------------
# Step 4: Hard defaults for unset fields.
# ---------------------------------------------------------------------------
if (-not $Auth)   { $Auth   = 'iam' }
if (-not $Effort) { $Effort = 'xhigh' }
if (-not $Region) {
    $cfg = $null
    try { $cfg = Get-Content $env:BEDROCK_CONFIG_PATH -Raw -Encoding utf8 | ConvertFrom-Json } catch {}
    $Region = if ($cfg -and $cfg.defaults.region) { $cfg.defaults.region } else { 'us-west-2' }
}
if (-not $Use1MContext -and -not $No1MContext) { $Use1MContext = $true }

$useOpusPlan = [bool]($OpusPlan -and -not $NoOpusPlan)
$use1M       = [bool]($Use1MContext -and -not $No1MContext)
$useMantle   = -not $NoMantle

if (-not $Storage) {
    $os = Get-KeychainOS
    $Storage = if (($os -in 'windows','macos') -and (Test-KeychainAvailable)) { 'keychain' } else { 'profile' }
}

if ($Auth -notin 'iam','bedrock-api-key') {
    Write-Error "apply: -Auth must be 'iam' or 'bedrock-api-key' (got: '$Auth')"; exit 1
}
if ($Effort -notin 'low','medium','high','xhigh','max') {
    Write-Error "apply: -Effort must be one of low|medium|high|xhigh|max"; exit 1
}

if (-not $SkipPreflight -and $Auth -eq 'iam') {
    if (-not (Get-Command 'aws' -ErrorAction SilentlyContinue)) {
        Write-Warning 'apply: aws CLI not found; IAM auth requires it at runtime.'
    }
}

# ---------------------------------------------------------------------------
# Step 5: Resolve API key.
# ---------------------------------------------------------------------------
if ($Auth -eq 'bedrock-api-key') {
    if ($PreserveKey) {
        if ($env:AWS_BEARER_TOKEN_BEDROCK) {
            $BedrockKey = $env:AWS_BEARER_TOKEN_BEDROCK
        }
        if (-not $BedrockKey -and (Test-KeychainAvailable)) {
            try { $BedrockKey = Get-KeychainEntry }
            catch { Write-Warning "apply: keychain read failed: $_"; $BedrockKey = $null }
            if ($null -eq $BedrockKey) { $BedrockKey = '' }
        }
        if (-not $BedrockKey) {
            Write-Error 'apply: -PreserveKey specified but no existing key found in env or keychain'; exit 1
        }
    }
    if (-not $BedrockKey) {
        if ($DryRun) {
            $BedrockKey = 'dry-run-placeholder'
        } else {
            $secKey = Read-Host 'Enter your Bedrock API key' -AsSecureString
            $BedrockKey = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
                [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secKey))
            if (-not $BedrockKey) { Write-Error 'apply: API key cannot be empty'; exit 1 }
        }
    }
    if ($Storage -eq 'keychain') {
        if ($DryRun) {
            Write-Host '[dry-run] would store API key in system keychain'
        } elseif (-not (Set-KeychainEntry -Key $BedrockKey)) {
            if ($storageExplicit) {
                Write-Error 'apply: keychain store failed. On Windows, the Credential Manager CredWrite API caps blob size at ~1280 unicode chars; keys larger than that must use IAM auth or an externally-managed AWS_BEARER_TOKEN_BEDROCK env var until Juggernaut adds DPAPI-backed storage.'
                exit 1
            }
            Write-Host ''
            Write-Host '[apply] WARNING: keychain store failed - Claude Code will hang without' -ForegroundColor Yellow
            Write-Host '[apply] a token source. On Windows the most common cause is a long-form'  -ForegroundColor Yellow
            Write-Host '[apply] Bedrock API key exceeding the ~1280 unicode char CredWrite cap.'  -ForegroundColor Yellow
            Write-Host '[apply] Workaround: export AWS_BEARER_TOKEN_BEDROCK yourself, or use'     -ForegroundColor Yellow
            Write-Host '[apply] -Auth iam with AWS SSO/IAM credentials.'                          -ForegroundColor Yellow
            Write-Host ''
            $Storage = 'profile'
        }
    }
}

# ---------------------------------------------------------------------------
# Step 6: Build and validate juggernaut block.
# ---------------------------------------------------------------------------
$buildParams = @{
    Provider       = 'bedrock'
    AuthMode       = $Auth
    AuthValidated  = $true
    Storage        = $Storage
    Region         = $Region
    EffortLevel    = $Effort
    OpusPlan       = $useOpusPlan
    Use1MContext   = $use1M
    UseMantle      = $useMantle
    MantleBaseUrl  = $MantleUrl
    ShellFallbackMode = $shellMode
    Scope          = $Scope
    Version        = $JuggernautVersion
    BedrockConfigPath = $env:BEDROCK_CONFIG_PATH
}
if ($Model)       { $buildParams['Model']       = $Model }
if ($OpusModel)   { $buildParams['OpusModel']   = $OpusModel }
if ($SonnetModel) { $buildParams['SonnetModel'] = $SonnetModel }
if ($HaikuModel)  { $buildParams['HaikuModel']  = $HaikuModel; $buildParams['SubagentModel'] = $HaikuModel }

$newBlock = New-JuggernautBlock @buildParams

if (-not (Test-JuggernautBlock -Block $newBlock)) {
    Write-Error 'apply: block validation failed - check your options'
    exit 1
}

# ---------------------------------------------------------------------------
# Step 7: Merge into full settings.json.
# ---------------------------------------------------------------------------
$nativeKeys  = Get-NativeKeysFromJuggernautBlock -Block $newBlock
$mergedSettings = Merge-JuggernautBlock -Existing $existingSettings -NewBlock $newBlock -NativeKeys $nativeKeys

# ---------------------------------------------------------------------------
# Step 8: Dry-run exit.
# ---------------------------------------------------------------------------
if ($DryRun) {
    Write-Host '[dry-run] No files will be written.'
    Write-Host ''
    Write-Host "Would write to: $SettingsPath"
    Write-Host '-----------------------------------------'
    $mergedSettings | ConvertTo-Json -Depth 20
    Write-Host '-----------------------------------------'
    Write-Host ''
    Write-Host '[dry-run] Done.'
    exit 0
}

# ---------------------------------------------------------------------------
# Step 9: Write settings.json.
# ---------------------------------------------------------------------------
try {
    Write-SettingsAtomic -Path $SettingsPath -Content $mergedSettings
    Write-Host "Settings written to: $SettingsPath" -ForegroundColor Green
} catch {
    Write-Error "apply: failed to write settings.json: $_"
    exit 1
}

# ---------------------------------------------------------------------------
# Step 10: Summary
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host 'Juggernaut v3 apply complete.' -ForegroundColor Cyan
if ($Auth -eq 'bedrock-api-key') { Write-Host '  Auth:     Bedrock API key' }
else { Write-Host '  Auth:     IAM' }
Write-Host "  Region:   $Region"
Write-Host "  Effort:   $Effort"
Write-Host "  Opusplan: $useOpusPlan"
Write-Host "  Mantle:   $useMantle"
Write-Host ''
Write-Host 'Launch Claude Code:'
if ($Auth -eq 'iam') { Write-Host '  aws sts get-caller-identity && claude' }
else                 { Write-Host '  claude' }
