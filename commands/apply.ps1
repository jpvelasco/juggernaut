# commands/apply.ps1 - Juggernaut v2 apply subcommand (PowerShell).
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
    [switch]$Yes,
    [switch]$SkipPreflight,
    [switch]$ForceMigrationPrompt,
    [switch]$Help
)

$ErrorActionPreference = 'Continue'
$PSScriptRoot_ = Split-Path -Parent $PSCommandPath
$RepoRoot      = Split-Path -Parent $PSScriptRoot_

$versionFile = Join-Path $RepoRoot 'VERSION'
$JuggernautVersion = if (Test-Path $versionFile) { (Get-Content $versionFile -Raw).Trim() } else { 'unknown' }

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
juggernaut apply - configure Claude Code for Amazon Bedrock

Usage: juggernaut.ps1 apply [options]

  -Auth             iam|bedrock-api-key  (legacy: api-key)
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
  -Mantle           Enable Mantle routing
  -MantleUrl        Mantle base URL
  -Scope            user|project (default: user)
  -NoShellFallback  Write settings.json only
  -ShellFallbackOnly  Write profile block only
  -DryRun           Preview without writing
  -Yes / -Force     Confirm migration prompts
'@
    exit 0
}

$authExplicit = $PSBoundParameters.ContainsKey('Auth')
$storageExplicit = $PSBoundParameters.ContainsKey('Storage')
$mantleExplicit = $PSBoundParameters.ContainsKey('Mantle') -or $PSBoundParameters.ContainsKey('MantleUrl')
if ($Auth -eq 'api-key') { $Auth = 'bedrock-api-key' }
if ($Force) { $Yes = $true }
$HomeDir = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { [Environment]::GetFolderPath('UserProfile') }

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
        Write-Error "apply: cannot read $SettingsPath - may be corrupted: $_"
        exit 1
    }
}
$hasV2Block = Test-HasJuggernautBlock -Settings $existingSettings

# ---------------------------------------------------------------------------
# Step 2: Explicit migration - detect and migrate v1 profile blocks.
# ---------------------------------------------------------------------------
if (-not $hasV2Block) {
    $v1Candidates = @(
        (Join-Path $HomeDir '.bashrc'),
        (Join-Path $HomeDir '.zshrc'),
        (Join-Path $HomeDir '.config\fish\config.fish')
    )
    if ($ForceMigrationPrompt) { $env:JUGGERNAUT_FORCE_MIGRATION_PROMPT = '1' }
    foreach ($candidate in $v1Candidates) {
        if (Test-Path $candidate) {
            $hasMig = $false
            try { $hasMig = Test-MigratorHasV1Block -ProfileFile $candidate } catch {}
            if ($hasMig) {
                Write-Host 'Juggernaut found an existing v1 profile block:'
                Write-Host "  $candidate"
                Write-Host 'Migration writes the equivalent v2 settings to:'
                Write-Host "  $SettingsPath"
                Write-Host 'The old profile block remains as a fallback unless you later run migrate -Clean.'
                if ($DryRun) {
                    Write-Host "[dry-run] Would migrate $candidate to $SettingsPath"
                    break
                }
                if (-not $Yes) {
                    if ([Console]::IsInputRedirected) {
                        Write-Error 'apply: migration requires confirmation. Re-run with -Yes, or run migrate -DryRun first.'
                        exit 1
                    }
                    $answer = Read-Host 'Migrate this v1 block now? [y/N]'
                    if ($answer -notmatch '^(y|yes)$') {
                        try { Set-MigratorDeclinedMarker -ProfileFile $candidate } catch {}
                        Write-Error 'apply: migration skipped. Re-run with -ForceMigrationPrompt to re-prompt, or -Yes to confirm non-interactively.'
                        exit 1
                    }
                }
                try {
                    $ok = Invoke-MigratorRun -ProfileFile $candidate `
                                             -SettingsPath $SettingsPath `
                                             -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
                    if ($ok) {
                        $existingSettings = Read-Settings -Path $SettingsPath
                        $hasV2Block = $true
                        Write-Host "Migration complete. Settings written to: $SettingsPath" -ForegroundColor Green
                    } else {
                        Write-Warning "apply: migration from $candidate returned false - continuing with defaults."
                    }
                } catch {
                    Write-Warning "apply: migration from $candidate failed - continuing with defaults: $_"
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
    if ($Auth -eq 'api-key') { $Auth = 'bedrock-api-key' }

    # Conflict guard: stored block says "iam" but live evidence of an API key
    # exists. Auto-correct unless -Auth iam was passed explicitly.
    if (-not $authExplicit -and $Auth -eq 'iam') {
        if ($env:AWS_BEARER_TOKEN_BEDROCK) {
            Write-Warning "apply: stored auth mode is 'iam' but AWS_BEARER_TOKEN_BEDROCK is set."
            Write-Warning "apply: Auto-correcting to bedrock-api-key. Pass -Auth iam to suppress."
            $Auth = 'bedrock-api-key'
            $PreserveKey = $true
            if (-not $mantleExplicit) { $Mantle = $true }
        } else {
            $existingStorage = if ($existingBlock.auth) { $existingBlock.auth.storage } else { '' }
            if ($existingStorage -eq 'keychain' -and (Test-KeychainAvailable)) {
                $kcVal = $null
                try { $kcVal = Get-KeychainEntry } catch {}
                if ($kcVal) {
                    Write-Warning "apply: stored auth mode is 'iam' but a key exists in the system keychain."
                    Write-Warning "apply: Auto-correcting to bedrock-api-key. Pass -Auth iam to suppress."
                    $Auth = 'bedrock-api-key'
                    $PreserveKey = $true
                }
            }
        }
    }
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
    if (-not $mantleExplicit -and -not $Mantle -and $existingBlock.useMantle) { $Mantle = $true }
    if (-not $MantleUrl -and $existingBlock.mantle.baseUrl) { $MantleUrl = $existingBlock.mantle.baseUrl }
}

# ---------------------------------------------------------------------------
# Step 4: Hard defaults for unset fields.
# ---------------------------------------------------------------------------
if (-not $authExplicit -and $env:AWS_BEARER_TOKEN_BEDROCK) {
    $Auth = 'bedrock-api-key'
    $PreserveKey = $true
    if (-not $mantleExplicit) { $Mantle = $true }
}
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
$useMantle   = [bool]$Mantle

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
$apiKeyExpr = ''
if ($Auth -eq 'bedrock-api-key') {
    if ($PreserveKey) {
        # Probe all sources regardless of stored storage preference — the storage
        # setting may have been corrupted alongside the auth mode.
        if ($env:AWS_BEARER_TOKEN_BEDROCK) {
            $BedrockKey = $env:AWS_BEARER_TOKEN_BEDROCK
        }
        if (-not $BedrockKey -and (Test-KeychainAvailable)) {
            try { $BedrockKey = Get-KeychainEntry }
            catch { Write-Warning "apply: keychain read failed: $_"; $BedrockKey = $null }
            if ($null -eq $BedrockKey) { $BedrockKey = '' }
        }
        if (-not $BedrockKey) {
            foreach ($profilePath in (Get-ProfileWriterPowerShellProfileTargets)) {
                if (Test-Path $profilePath) {
                    $profileContent = Get-Content $profilePath -Raw -ErrorAction SilentlyContinue
                    if ($profileContent -match '\$env:AWS_BEARER_TOKEN_BEDROCK\s*=\s*[''"]([^''"]+)[''"]') {
                        $BedrockKey = $Matches[1]
                        if ($BedrockKey) { break }
                    }
                }
            }
        }
        if (-not $BedrockKey) {
            Write-Error 'apply: -PreserveKey specified but no existing key found in env, keychain, or shell profile'; exit 1
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
                Write-Error 'apply: keychain store failed. Re-run with -Storage profile if you want plaintext profile storage.'
                exit 1
            }
            Write-Warning 'apply: keychain store failed; falling back to profile storage because storage was not explicit.'
            $Storage = 'profile'
        }
    }
    if ($Storage -eq 'keychain') {
        $apiKeyExpr = 'keychain'
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
    if ($shellMode -ne 'settings-only') {
        if ((Get-KeychainOS) -eq 'windows') {
            Write-Host "Would also update PowerShell profiles:"
            foreach ($profilePath in (Get-ProfileWriterPowerShellProfileTargets)) {
                Write-Host "  $profilePath"
            }
        } else {
            Write-Host "Would also update shell profile (bash): $(Join-Path $HomeDir '.bashrc')"
        }
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
# On Windows, write PowerShell all-hosts profiles so both Windows PowerShell 5.1
# and PowerShell 7 can load the keychain/profile fallback.
# ---------------------------------------------------------------------------
if (-not $NoShellFallback) {
    if ((Get-KeychainOS) -eq 'windows') {
        $blockContent = Build-ProfileWriterBlock `
            -Shell       'powershell' `
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

        foreach ($profilePath in (Get-ProfileWriterPowerShellProfileTargets)) {
            try {
                Write-ProfileWriterBlock -ProfileFile $profilePath -BlockContent $blockContent
                Write-Host "Profile block written to: $profilePath" -ForegroundColor Green
            } catch {
                Write-Warning "apply: could not write profile block to $profilePath`: $_"
            }
        }
    } else {
        $profilePath = Join-Path $HomeDir '.bashrc'
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
}

# ---------------------------------------------------------------------------
# Step 11: Summary
# ---------------------------------------------------------------------------
Write-Host ''
Write-Host 'Juggernaut v2 apply complete.' -ForegroundColor Cyan
if ($Auth -eq 'bedrock-api-key') { Write-Host '  Auth:     Bedrock API key' }
else { Write-Host '  Auth:     IAM' }
Write-Host "  Region:   $Region"
Write-Host "  Effort:   $Effort"
Write-Host "  Opusplan: $useOpusPlan"
Write-Host ''
Write-Host 'Restart your terminal to apply changes, then:'
if ($Auth -eq 'iam') { Write-Host '  aws sts get-caller-identity && claude' }
else                 { Write-Host '  claude' }
