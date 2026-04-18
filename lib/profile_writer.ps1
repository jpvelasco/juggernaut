# lib/profile_writer.ps1 — Shell profile block write/update/detect for Juggernaut v2.
# PowerShell mirror of lib/profile_writer.sh. Requires PowerShell 5.1+.

$script:ProfileWriterBegin = '# BEGIN: Claude Code Bedrock Configuration'
$script:ProfileWriterEnd   = '# END: Claude Code Bedrock Configuration'

function Get-ProfileWriterShellConfigPath {
    param([Parameter(Mandatory)][string]$Shell)
    switch ($Shell) {
        'bash' { return (Join-Path $env:HOME '.bashrc') }
        'zsh'  { return (Join-Path $env:HOME '.zshrc') }
        'fish' { return (Join-Path $env:HOME '.config/fish/config.fish') }
        default { return '' }
    }
}

function Get-ProfileWriterExportSyntax {
    param([Parameter(Mandatory)][string]$Shell)
    if ($Shell -eq 'fish') { return 'set -gx' }
    return 'export'
}

function Test-ProfileWriterHasBlock {
    param([Parameter(Mandatory)][string]$ProfileFile)
    if (-not (Test-Path $ProfileFile)) { return $false }
    return (Select-String -Path $ProfileFile -Pattern ([regex]::Escape($script:ProfileWriterBegin)) -Quiet)
}

function Remove-ProfileWriterBlock {
    param(
        [Parameter(Mandatory)][string]$ProfileFile,
        [switch]$DryRun
    )
    if (-not (Test-Path $ProfileFile)) { return }
    if ($DryRun) { Write-Host "[dry-run] would remove block from $ProfileFile"; return }

    $lines  = Get-Content -Path $ProfileFile -Encoding utf8
    $out    = [System.Collections.Generic.List[string]]::new()
    $inside = $false
    foreach ($line in $lines) {
        if ($line -eq $script:ProfileWriterBegin) { $inside = $true; continue }
        if ($line -eq $script:ProfileWriterEnd)   { $inside = $false; continue }
        if (-not $inside) { $out.Add($line) }
    }
    Set-Content -Path $ProfileFile -Value $out -Encoding utf8 -NoNewline:$false
}

function Build-ProfileWriterBlock {
    param(
        [Parameter(Mandatory)][string]$Shell,
        [Parameter(Mandatory)][string]$Region,
        [Parameter(Mandatory)][string]$AuthMode,
        [string]$ApiKeyExpr     = '',
        [string]$StorageMode    = 'profile',
        [Parameter(Mandatory)][string]$BedrockConfigPath,
        [string]$Model          = '',
        [string]$OpusModel      = '',
        [string]$SonnetModel    = '',
        [string]$HaikuModel     = '',
        [string]$EffortLevel    = '',
        [bool]  $OpusPlan       = $false,
        [bool]  $UseMantle      = $false,
        [string]$MantleUrl      = ''
    )

    $syntax = Get-ProfileWriterExportSyntax -Shell $Shell

    # Load defaults from bedrock-config.json
    $cfg = $null
    if (Test-Path $BedrockConfigPath) {
        try { $cfg = Get-Content $BedrockConfigPath -Raw -Encoding utf8 | ConvertFrom-Json }
        catch {}
    }
    $env = if ($cfg) { $cfg.environment } else { $null }

    $effModel   = if ($Model)       { $Model }       else { if ($env) { $env.ANTHROPIC_MODEL } else { '' } }
    $effOpus    = if ($OpusModel)   { $OpusModel }   else { if ($env) { $env.ANTHROPIC_DEFAULT_OPUS_MODEL } else { '' } }
    $effSonnet  = if ($SonnetModel) { $SonnetModel } else { if ($env) { $env.ANTHROPIC_DEFAULT_SONNET_MODEL } else { '' } }
    $effHaiku   = if ($HaikuModel)  { $HaikuModel }  else { if ($env) { $env.ANTHROPIC_DEFAULT_HAIKU_MODEL } else { '' } }
    $effEffort  = if ($EffortLevel) { $EffortLevel }  else { if ($env) { $env.CLAUDE_CODE_EFFORT_LEVEL } else { 'xhigh' } }
    $effMax     = if ($env -and $env.CLAUDE_CODE_MAX_OUTPUT_TOKENS) { $env.CLAUDE_CODE_MAX_OUTPUT_TOKENS } else { '32768' }
    $effThink   = if ($env -and $env.MAX_THINKING_TOKENS)          { $env.MAX_THINKING_TOKENS }          else { '65536' }
    $effCache   = if ($env -and $env.ENABLE_PROMPT_CACHING_1H)     { $env.ENABLE_PROMPT_CACHING_1H }     else { '1' }

    if ($OpusPlan) { $effModel = 'opusplan' }

    $sb = [System.Text.StringBuilder]::new()

    $nl = [Environment]::NewLine
    [void]$sb.AppendLine('')
    [void]$sb.AppendLine($script:ProfileWriterBegin)

    [void]$sb.AppendLine("# Auth mode: $AuthMode")
    if ($StorageMode -eq 'keychain') { [void]$sb.AppendLine('# Storage: keychain (encrypted)') }
    if ($Model)       { [void]$sb.AppendLine("# Model: $Model") }
    if ($OpusModel)   { [void]$sb.AppendLine("# OpusModel: $OpusModel") }
    if ($SonnetModel) { [void]$sb.AppendLine("# SonnetModel: $SonnetModel") }
    if ($HaikuModel)  { [void]$sb.AppendLine("# HaikuModel: $HaikuModel") }
    if ($OpusPlan)    { [void]$sb.AppendLine('# OpusPlan: true') }
    if ($EffortLevel) { [void]$sb.AppendLine("# EffortLevel: $EffortLevel") }

    # Unset conflicting auth vars
    if ($AuthMode -eq 'api-key') {
        if ($Shell -eq 'fish') {
            [void]$sb.AppendLine('set -e AWS_ACCESS_KEY_ID 2>/dev/null')
            [void]$sb.AppendLine('set -e AWS_SECRET_ACCESS_KEY 2>/dev/null')
            [void]$sb.AppendLine('set -e AWS_SESSION_TOKEN 2>/dev/null')
            [void]$sb.AppendLine('set -e AWS_PROFILE 2>/dev/null')
        } else {
            [void]$sb.AppendLine('unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_PROFILE 2>/dev/null || true')
        }
    } else {
        if ($Shell -eq 'fish') {
            [void]$sb.AppendLine('set -e AWS_BEARER_TOKEN_BEDROCK 2>/dev/null')
        } else {
            [void]$sb.AppendLine('unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true')
        }
    }

    # Helper: emit one export line
    $line = {
        param($k, $v)
        $ev = $v -replace '"', '\"'
        if ($Shell -eq 'fish') { "$syntax $k `"$ev`"" }
        else                   { "$syntax $k=`"$ev`"" }
    }

    [void]$sb.AppendLine((& $line 'AWS_REGION'                      $Region))
    [void]$sb.AppendLine((& $line 'CLAUDE_CODE_USE_BEDROCK'         '1'))
    [void]$sb.AppendLine((& $line 'CLAUDE_CODE_MAX_OUTPUT_TOKENS'   $effMax))
    [void]$sb.AppendLine((& $line 'MAX_THINKING_TOKENS'             $effThink))
    [void]$sb.AppendLine((& $line 'ANTHROPIC_MODEL'                 $effModel))
    [void]$sb.AppendLine((& $line 'ANTHROPIC_DEFAULT_OPUS_MODEL'    $effOpus))
    [void]$sb.AppendLine((& $line 'ANTHROPIC_DEFAULT_SONNET_MODEL'  $effSonnet))
    [void]$sb.AppendLine((& $line 'ANTHROPIC_DEFAULT_HAIKU_MODEL'   $effHaiku))
    [void]$sb.AppendLine((& $line 'CLAUDE_CODE_SUBAGENT_MODEL'      $effHaiku))
    [void]$sb.AppendLine((& $line 'CLAUDE_CODE_EFFORT_LEVEL'        $effEffort))
    [void]$sb.AppendLine((& $line 'ENABLE_PROMPT_CACHING_1H'        $effCache))
    [void]$sb.AppendLine((& $line 'DISABLE_ERROR_REPORTING'         '1'))
    [void]$sb.AppendLine((& $line 'DISABLE_TELEMETRY'               '1'))
    [void]$sb.AppendLine((& $line 'DISABLE_AUTOUPDATE'              '1'))
    [void]$sb.AppendLine((& $line 'DISABLE_BUG_COMMAND'             '1'))

    if ($UseMantle) {
        [void]$sb.AppendLine((& $line 'CLAUDE_CODE_USE_MANTLE' '1'))
        if ($MantleUrl) { [void]$sb.AppendLine((& $line 'ANTHROPIC_BEDROCK_MANTLE_BASE_URL' $MantleUrl)) }
    }

    if ($AuthMode -eq 'api-key' -and $ApiKeyExpr) {
        if ($Shell -eq 'fish') {
            [void]$sb.AppendLine("$syntax AWS_BEARER_TOKEN_BEDROCK $ApiKeyExpr")
        } else {
            [void]$sb.AppendLine("$syntax AWS_BEARER_TOKEN_BEDROCK=$ApiKeyExpr")
        }
    }

    [void]$sb.Append($script:ProfileWriterEnd)
    return $sb.ToString()
}

function Write-ProfileWriterBlock {
    param(
        [Parameter(Mandatory)][string]$ProfileFile,
        [Parameter(Mandatory)][string]$BlockContent,
        [switch]$DryRun
    )
    if ($DryRun) {
        Write-Host "[dry-run] would write block to $ProfileFile"
        Write-Host $BlockContent
        return
    }

    $dir = Split-Path $ProfileFile -Parent
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    if (-not (Test-Path $ProfileFile)) { New-Item -ItemType File -Path $ProfileFile -Force | Out-Null }

    Remove-ProfileWriterBlock -ProfileFile $ProfileFile

    Add-Content -Path $ProfileFile -Value $BlockContent -Encoding utf8
}

function Set-ProfileWriterAnnotation {
    param(
        [Parameter(Mandatory)][string]$ProfileFile,
        [switch]$DryRun
    )
    if (-not (Test-ProfileWriterHasBlock -ProfileFile $ProfileFile)) { return }
    if ($DryRun) { Write-Host "[dry-run] would annotate v1 block in $ProfileFile"; return }

    $notice = @(
        '# Juggernaut v2: PRIMARY config is now in ~/.claude/settings.json.',
        '# This block remains as a compatibility fallback.',
        '# Run `juggernaut migrate --clean` to remove it once Claude Code works.'
    )

    $metaPattern = '^# (Auth mode|Storage|Model|FastModel|OpusModel|SonnetModel|HaikuModel|1MContext|OpusPlan|EffortLevel):'
    $lines  = Get-Content -Path $ProfileFile -Encoding utf8
    $out    = [System.Collections.Generic.List[string]]::new()
    $noticeInserted = $false

    foreach ($line in $lines) {
        if ($line -eq $script:ProfileWriterBegin) {
            $out.Add($line)
            if (-not $noticeInserted) { foreach ($n in $notice) { $out.Add($n) }; $noticeInserted = $true }
            continue
        }
        if ($line -match $metaPattern) { continue }
        $out.Add($line)
    }
    Set-Content -Path $ProfileFile -Value $out -Encoding utf8 -NoNewline:$false
}
