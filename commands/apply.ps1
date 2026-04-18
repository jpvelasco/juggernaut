# commands/apply.ps1 — Juggernaut v2 apply subcommand (PowerShell).
# Configures Claude Code to use Amazon Bedrock via settings.json (+ optional shell fallback).

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
    [switch]$Mantle,
    [string]$MantleUrl     = '',
    [string]$Scope         = 'user',
    [switch]$NoShellFallback,
    [switch]$ShellFallbackOnly,
    [switch]$DryRun,
    [switch]$Force,
    [switch]$SkipPreflight,
    [switch]$Help
)

$ErrorActionPreference = 'Continue'
$PSScriptRoot_ = Split-Path -Parent $PSCommandPath
$RepoRoot      = Split-Path -Parent $PSScriptRoot_

$env:JUGGERNAUT_USE_V2 = '1'
if (-not $env:BEDROCK_CONFIG_PATH) {
    $env:BEDROCK_CONFIG_PATH = Join-Path $RepoRoot 'bedrock-config.json'
}

. (Join-Path $RepoRoot 'lib\schema.ps1')
. (Join-Path $RepoRoot 'lib\config_manager.ps1')
. (Join-Path $RepoRoot 'lib\migrator.ps1')
. (Join-Path $RepoRoot 'lib\keychain.ps1')
. (Join-Path $RepoRoot 'lib\profile_writer.ps1')

if ($Help) {
    @'
juggernaut apply — configure Claude Code for Amazon Bedrock

Usage: juggernaut.ps1 apply [options]

  -Auth             iam|api-key  (default: iam, auto-detected on re-run)
  -BedrockKey       Bedrock API key (api-key mode)
  -PreserveKey      Reuse existing key from env/keychain
  -Storage          profile|keychain
  -Region           AWS region
  -Model / -OpusModel / -SonnetModel / -HaikuModel   Model overrides
  -Effort           low|medium|high|xhigh|max (default: xhigh)
  -OpusPlan         Use Opus for planning, Sonnet for execution
  -NoOpusPlan       Disable opusplan
  -Use1MContext     Enable 1M token context
  -No1MContext      Disable 1M token context
  -Mantle           Enable Mantle routing
  -MantleUrl        Mantle base URL
  -Scope            user|project (default: user)
  -NoShellFallback  Write settings.json only
  -ShellFallbackOnly  Write profile block only
  -DryRun           Preview without writing
  -Force            Skip confirmation prompts
'@
    exit 0
}

if ($NoShellFallback -and $ShellFallbackOnly) {
    Write-Error 'apply: -NoShellFallback and -ShellFallbackOnly are mutually exclusive'
    exit 1
}
if ($Scope -notin 'user','project') {
    Write-Error "apply: -Scope must be 'user' or 'project' (got: '$Scope')"
    exit 1
}

$shellMode = if ($NoShellFallback)       { 'settings-only' }
             elseif ($ShellFallbackOnly) { 'shell-only' }
             else                        { 'both' }

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
        Write-Error "apply: cannot read $SettingsPath — may be corrupted: $_"
        exit 1
    }
}
$hasV2Block = Test-HasJuggernautBlock -Settings $existingSettings

# ---------------------------------------------------------------------------
# Step 2: Implicit migration — detect and migrate v1 profile blocks.
# Defensive: warn and continue if migration fails.
# ---------------------------------------------------------------------------
if (-not $hasV2Block) {
    $v1Candidates = @(
        (Join-Path $env:HOME '.bashrc'),
        (Join-Path $env:HOME '.zshrc'),
        (Join-Path $env:HOME '.config\fish\config.fish')
    )
    foreach ($candidate in $v1Candidates) {
        if (Test-Path $candidate) {
            $hasMig = $false
            try { $hasMig = Test-MigratorHasV1Block -ProfileFile $candidate } catch {}
            if ($hasMig) {
                Write-Host "apply: found an existing v1 block in $candidate." -ForegroundColor DarkYellow
                Write-Host "apply: migrating it to ~/.claude/settings.json, Claude Code's native config." -ForegroundColor DarkYellow
                try {
                    $ok = Invoke-MigratorRun -ProfileFile $candidate `
                                             -SettingsPath $SettingsPath `
                                             -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
                    if ($ok) {
                        $existingSettings = Read-Settings -Path $SettingsPath
                        $hasV2Block = $true
                        Write-Host "apply: migration complete. Your new settings are now in $SettingsPath." -ForegroundColor Green
                        Write-Host 'apply: your shell profile was left in place as a fallback, so nothing is lost.' -ForegroundColor Green
                    } else {
                        Write-Warning "apply: migration from $candidate returned false — continuing with defaults."
                    }
                } catch {
                    Write-Warning "apply: migration from $candidate failed — continuing with defaults: $_"
                }
                break
            }
        }
    }
}

# ---------------------------------------------------------------------------
# Step 3: Load existing block and overlay CLI flags.
# ---------------------------------------------------------------------------
$existingBlock = if ($hasV2Block) { Get-JuggernautBlockFromSettings -Settings $existingSettings } else { $null }

if ($existingBlock) {
    if (-not $Auth)        { $Auth        = $existingBlock.auth.mode            }
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
    if (-not $Mantle -and $existingBlock.useMantle) { $Mantle = $true }
    if (-not $MantleUrl -and $existingBlock.mantle.baseUrl) { $MantleUrl = $existingBlock.mantle.baseUrl }
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

$useOpusPlan = [bool]($OpusPlan -and -not $NoOpusPlan)
$use1M       = [bool]($Use1MContext -and -not $No1MContext)
$useMantle   = [bool]$Mantle

if (-not $Storage) {
    $os = Get-KeychainOS
    $Storage = if (($os -in 'windows','macos') -and (Test-KeychainAvailable)) { 'keychain' } else { 'profile' }
}

if ($Auth -notin 'iam','api-key') {
    Write-Error "apply: -Auth must be 'iam' or 'api-key' (got: '$Auth')"; exit 1
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
$apiKeyExpr = ''
if ($Auth -eq 'api-key') {
    if ($PreserveKey) {
        $BedrockKey = if ($env:AWS_BEARER_TOKEN_BEDROCK) {
            $env:AWS_BEARER_TOKEN_BEDROCK
        } elseif ($Storage -eq 'keychain' -and (Test-KeychainAvailable)) {
            Get-KeychainEntry
        } else { '' }
        if (-not $BedrockKey) {
            Write-Error 'apply: -PreserveKey specified but no existing key found'; exit 1
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
            Write-Warning 'apply: keychain store failed — falling back to profile storage'
            $Storage = 'profile'
        }
        $apiKeyExpr = Get-KeychainRetrievalExpression -Shell 'bash'
    } else {
        $apiKeyExpr = $BedrockKey
    }
}

# ---------------------------------------------------------------------------
# Step 6: Build and validate juggernaut block.
# ---------------------------------------------------------------------------
$buildParams = @{
    Provider       = 'bedrock'
    AuthMode       = $Auth
    Storage        = $Storage
    Region         = $Region
    EffortLevel    = $Effort
    OpusPlan       = $useOpusPlan
    Use1MContext   = $use1M
    UseMantle      = $useMantle
    MantleBaseUrl  = $MantleUrl
    ShellFallbackMode = $shellMode
    Scope          = $Scope
    Version        = '2.0.0'
    BedrockConfigPath = $env:BEDROCK_CONFIG_PATH
}
if ($Model)       { $buildParams['Model']       = $Model }
if ($OpusModel)   { $buildParams['OpusModel']   = $OpusModel }
if ($SonnetModel) { $buildParams['SonnetModel'] = $SonnetModel }
if ($HaikuModel)  { $buildParams['HaikuModel']  = $HaikuModel; $buildParams['SubagentModel'] = $HaikuModel }

$newBlock = New-JuggernautBlock @buildParams

if (-not (Test-JuggernautBlock -Block $newBlock)) {
    Write-Error 'apply: block validation failed — check your options'
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
    Write-Host '─────────────────────────────────────────'
    $mergedSettings | ConvertTo-Json -Depth 20
    Write-Host '─────────────────────────────────────────'
    if ($shellMode -ne 'settings-only') {
        Write-Host "Would also update shell profile (bash): $(Join-Path $env:HOME '.bashrc')"
    }
    Write-Host ''
    Write-Host '[dry-run] Done.'
    exit 0
}

# ---------------------------------------------------------------------------
# Step 9: Write settings.json (unless shell-fallback-only).
# ---------------------------------------------------------------------------
if (-not $ShellFallbackOnly) {
    try {
        Write-SettingsAtomic -Path $SettingsPath -Content $mergedSettings
        Write-Host "Settings written to: $SettingsPath" -ForegroundColor Green
    } catch {
        Write-Error "apply: failed to write settings.json: $_"
        exit 1
    }
}

# ---------------------------------------------------------------------------
# Step 10: Write shell profile block (unless no-shell-fallback).
# On Windows, default shell profile is PowerShell $PROFILE.
# For cross-platform consistency the bash profile is written when present.
# ---------------------------------------------------------------------------
if (-not $NoShellFallback) {
    # Windows: update PowerShell profile with Set-Gx-less export comment block.
    # For now we write the bash-style block to $HOME/.bashrc if it exists,
    # mirroring what the PS migrator does. Full PS profile_writer is Phase 4.
    $profilePath = Join-Path $env:HOME '.bashrc'
    if (Test-Path (Split-Path $profilePath -Parent)) {
        $blockContent = Build-ProfileWriterBlock `
            -Shell       'bash' `
            -Region      $Region `
            -AuthMode    $Auth `
            -ApiKeyExpr  $apiKeyExpr `
            -StorageMode $Storage `
            -BedrockConfigPath $env:BEDROCK_CONFIG_PATH `
            -Model       $Model `
            -OpusModel   $OpusModel `
            -SonnetModel $SonnetModel `
            -HaikuModel  $HaikuModel `
            -EffortLevel $Effort `
            -OpusPlan    $useOpusPlan `
            -UseMantle   $useMantle `
            -MantleUrl   $MantleUrl
        try {
            Write-ProfileWriterBlock -ProfileFile $profilePath -BlockContent $blockContent
            Write-Host "Profile block written to: $profilePath" -ForegroundColor Green
        } catch {
            Write-Warning "apply: could not write profile block to $profilePath`: $_"
        }
    }
}

# ---------------------------------------------------------------------------
# Step 11: Summary
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host 'Juggernaut v2 apply complete.' -ForegroundColor Cyan
Write-Host "  Auth:     $Auth"
Write-Host "  Region:   $Region"
Write-Host "  Effort:   $Effort"
Write-Host "  Opusplan: $useOpusPlan"
Write-Host ''
Write-Host 'Restart your terminal to apply changes, then:'
if ($Auth -eq 'iam') { Write-Host '  aws sts get-caller-identity && claude' }
else                 { Write-Host '  claude' }
