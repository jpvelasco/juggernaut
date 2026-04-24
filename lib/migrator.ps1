# lib/migrator.ps1 - v1.7.x profile-block -> v2 settings.json migration for Juggernaut.
# Mirrors lib/migrator.sh. PowerShell 5.1+ compatible.

# ---------------------------------------------------------------------------
# Detection
# ---------------------------------------------------------------------------

function Test-MigratorHasV1Block {
    param([Parameter(Mandatory)][string]$ProfileFile)
    if (-not (Test-Path $ProfileFile)) { return $false }
    return (Get-Content -Path $ProfileFile -Raw) -match '# BEGIN: Claude Code Bedrock Configuration'
}

function Get-MigratorV1BlockRaw {
    param([Parameter(Mandatory)][string]$ProfileFile)
    $lines = Get-Content -Path $ProfileFile
    $capture = $false
    $result = [System.Collections.Generic.List[string]]::new()
    foreach ($line in $lines) {
        if ($line -eq '# BEGIN: Claude Code Bedrock Configuration') { $capture = $true }
        if ($capture) { $result.Add($line) }
        if ($line -eq '# END: Claude Code Bedrock Configuration') { break }
    }
    return $result -join "`n"
}

# ---------------------------------------------------------------------------
# Parser
# ---------------------------------------------------------------------------

function ConvertFrom-MigratorV1Block {
    param([Parameter(Mandatory)][string]$RawBlock)

    $lines = $RawBlock -split "`n"

    $authMode    = ($lines | Where-Object { $_ -match '^# Auth mode: (.+)' } | Select-Object -First 1) -replace '^# Auth mode: ', ''
    if (-not $authMode) { $authMode = 'iam' }

    $storage = 'profile'
    if ($lines | Where-Object { $_ -match '^# Storage: keychain' }) { $storage = 'keychain' }

    $use1M    = [bool]($lines | Where-Object { $_ -eq '# 1MContext: true' })
    $opusPlan = [bool]($lines | Where-Object { $_ -eq '# OpusPlan: true' })

    $effortLevel = ($lines | Where-Object { $_ -match '^# EffortLevel: (.+)' } | Select-Object -First 1) -replace '^# EffortLevel: ', ''
    if (-not $effortLevel) { $effortLevel = 'xhigh' }

    $model       = ($lines | Where-Object { $_ -match '^# Model: (.+)' }       | Select-Object -First 1) -replace '^# Model: ', ''
    $opusModel   = ($lines | Where-Object { $_ -match '^# OpusModel: (.+)' }   | Select-Object -First 1) -replace '^# OpusModel: ', ''
    $sonnetModel = ($lines | Where-Object { $_ -match '^# SonnetModel: (.+)' } | Select-Object -First 1) -replace '^# SonnetModel: ', ''
    $haikuModel  = ($lines | Where-Object { $_ -match '^# HaikuModel: (.+)' }  | Select-Object -First 1) -replace '^# HaikuModel: ', ''

    # Fall back to export lines (bash/zsh) then fish set -gx lines for values
    # when metadata comments are absent.
    $exportLines  = $lines | Where-Object { $_ -match '^export ([A-Z_][A-Z0-9_]*)=' }
    $fishSetLines = $lines | Where-Object { $_ -match '^set -gx ([A-Z_][A-Z0-9_]*) ' }

    $getExport = {
        param($key)
        $hit = $exportLines | Where-Object { $_ -match "^export ${key}=" } | Select-Object -First 1
        if ($hit -match "^export ${key}=[`"']?(.+?)[`"']?$") { return $Matches[1] }
        return ''
    }
    $getFish = {
        param($key)
        $hit = $fishSetLines | Where-Object { $_ -match "^set -gx ${key} " } | Select-Object -First 1
        if ($hit -match "^set -gx ${key} (.+)$") { return $Matches[1] }
        return ''
    }

    if (-not $model) {
        $model = & $getExport 'ANTHROPIC_MODEL'
        if (-not $model) { $model = & $getFish 'ANTHROPIC_MODEL' }
        if ($model -eq 'opusplan') { $model = '' }
    }
    if (-not $opusModel)   { $opusModel   = & $getExport 'ANTHROPIC_DEFAULT_OPUS_MODEL';   if (-not $opusModel)   { $opusModel   = & $getFish 'ANTHROPIC_DEFAULT_OPUS_MODEL' } }
    if (-not $sonnetModel) { $sonnetModel = & $getExport 'ANTHROPIC_DEFAULT_SONNET_MODEL'; if (-not $sonnetModel) { $sonnetModel = & $getFish 'ANTHROPIC_DEFAULT_SONNET_MODEL' } }
    if (-not $haikuModel)  { $haikuModel  = & $getExport 'ANTHROPIC_DEFAULT_HAIKU_MODEL';  if (-not $haikuModel)  { $haikuModel  = & $getFish 'ANTHROPIC_DEFAULT_HAIKU_MODEL' } }

    # Region: export line -> fish set -gx -> default.
    # auth.region is the single source of truth in v2; sourced from AWS_REGION.
    $region = & $getExport 'AWS_REGION'
    if (-not $region) { $region = & $getFish 'AWS_REGION' }
    if (-not $region) { $region = 'us-east-1' }

    # Build legacyEnv snapshot from export lines and fish set -gx lines.
    $legacyEnv = [ordered]@{}
    foreach ($line in $exportLines) {
        if ($line -match "^export ([A-Z_][A-Z0-9_]*)=(.+)$") {
            $legacyEnv[$Matches[1]] = $Matches[2] -replace '^[''"]|[''"]$', ''
        }
    }
    foreach ($line in $fishSetLines) {
        if ($line -match "^set -gx ([A-Z_][A-Z0-9_]*) (.+)$") {
            if (-not $legacyEnv.Contains($Matches[1])) {
                $legacyEnv[$Matches[1]] = $Matches[2]
            }
        }
    }

    return [ordered]@{
        authMode    = $authMode
        region      = $region
        model       = $model
        opusModel   = $opusModel
        sonnetModel = $sonnetModel
        haikuModel  = $haikuModel
        effortLevel = $effortLevel
        storage     = $storage
        use1MContext = $use1M
        opusPlan    = $opusPlan
        legacyEnv   = $legacyEnv
    }
}

# ---------------------------------------------------------------------------
# Build v2 block
# ---------------------------------------------------------------------------

function New-MigratorV2Block {
    param(
        [Parameter(Mandatory)][hashtable]$Parsed,
        [string]$BedrockConfigPath = ''
    )

    $model       = if ($Parsed.model)       { $Parsed.model }       else { '' }
    $opusModel   = if ($Parsed.opusModel)   { $Parsed.opusModel }   else { '' }
    $sonnetModel = if ($Parsed.sonnetModel) { $Parsed.sonnetModel } else { '' }
    $haikuModel  = if ($Parsed.haikuModel)  { $Parsed.haikuModel }  else { '' }

    $blockParams = @{
        AuthMode     = $Parsed.authMode
        Region       = $Parsed.region
        EffortLevel  = $Parsed.effortLevel
        Storage      = $Parsed.storage
        Use1MContext = $Parsed.use1MContext
        OpusPlan     = $Parsed.opusPlan
        UseMantle    = $false
    }
    if ($model)       { $blockParams['Model']       = $model }
    if ($opusModel)   { $blockParams['OpusModel']   = $opusModel }
    if ($sonnetModel) { $blockParams['SonnetModel'] = $sonnetModel }
    if ($haikuModel)  { $blockParams['HaikuModel']  = $haikuModel }
    if ($BedrockConfigPath) { $blockParams['BedrockConfigPath'] = $BedrockConfigPath }

    $block = New-JuggernautBlock @blockParams

    # Inject legacyEnv and migration provenance.
    $now = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $block['legacyEnv'] = [ordered]@{
        source     = 'v1.7.x-profile-block'
        migratedAt = $now
        snapshot   = $Parsed.legacyEnv
    }
    $block['meta']['migratedFrom'] = 'v1.7.x'

    return $block
}

# ---------------------------------------------------------------------------
# Profile annotation
# ---------------------------------------------------------------------------

function Set-MigratorProfileAnnotation {
    param([Parameter(Mandatory)][string]$ProfileFile)
    if (-not (Test-Path $ProfileFile)) { return }

    $lines = Get-Content -Path $ProfileFile
    $result = [System.Collections.Generic.List[string]]::new()
    $inBlock = $false
    $headerWritten = $false
    $metaPattern = '^# (Auth mode|Model|FastModel|OpusModel|SonnetModel|HaikuModel|Storage|1MContext|OpusPlan|EffortLevel):'

    foreach ($line in $lines) {
        if ($line -eq '# BEGIN: Claude Code Bedrock Configuration') {
            $inBlock = $true
            $headerWritten = $false
            $result.Add($line)
            continue
        }
        if ($line -eq '# END: Claude Code Bedrock Configuration') {
            $inBlock = $false
            $result.Add($line)
            continue
        }
        if ($inBlock) {
            if (-not $headerWritten) {
                $result.Add('# Juggernaut v2: PRIMARY config is now in ~/.claude/settings.json.')
                $result.Add('# This block is a compatibility fallback. Run `juggernaut migrate --clean` to remove it.')
                $headerWritten = $true
            }
            if ($line -match $metaPattern) { continue }
        }
        $result.Add($line)
    }

    $result | Set-Content -Path $ProfileFile -Encoding utf8
}

# ---------------------------------------------------------------------------
# Top-level entry points
# ---------------------------------------------------------------------------

function Invoke-MigratorRun {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$ProfileFile,
        [Parameter(Mandatory)][string]$SettingsPath,
        [string]$BedrockConfigPath = ''
    )

    if (-not (Test-MigratorHasV1Block -ProfileFile $ProfileFile)) {
        Write-Warning "Invoke-MigratorRun: no v1 block found in $ProfileFile"
        return $false
    }

    $rawBlock  = Get-MigratorV1BlockRaw   -ProfileFile $ProfileFile
    $parsed    = ConvertFrom-MigratorV1Block -RawBlock $rawBlock
    $newBlock  = New-MigratorV2Block -Parsed $parsed -BedrockConfigPath $BedrockConfigPath
    $native    = Get-NativeKeysFromJuggernautBlock -Block $newBlock
    $existing  = Read-Settings -Path $SettingsPath
    $merged    = Merge-JuggernautBlock -Existing $existing -NewBlock $newBlock -NativeKeys $native

    Write-SettingsAtomic -Path $SettingsPath -Content $merged
    Set-MigratorProfileAnnotation -ProfileFile $ProfileFile

    Write-Host "Migration complete: $ProfileFile -> $SettingsPath"
    return $true
}

function Invoke-MigratorRollback {
    param([Parameter(Mandatory)][string]$SettingsPath)

    $dir  = Split-Path $SettingsPath -Parent
    $base = Split-Path $SettingsPath -Leaf
    $latest = Get-ChildItem -Path $dir -Filter "${base}.backup.*" -ErrorAction SilentlyContinue |
              Sort-Object LastWriteTime -Descending |
              Select-Object -First 1

    if (-not $latest) {
        Write-Warning "Invoke-MigratorRollback: no backup found for $SettingsPath"
        return $false
    }

    Copy-Item -Path $latest.FullName -Destination $SettingsPath -Force
    Write-Host "Rolled back $SettingsPath from $($latest.FullName)"
    return $true
}
