# lib/upgrade_banner.ps1 — Upgrade/migration state detection and user-facing banner.
# PowerShell mirror of lib/upgrade_banner.sh.
# Requires: lib/profile_paths.ps1, lib/migrator.ps1, lib/config_manager.ps1.

# ---------------------------------------------------------------------------
# Get-UpgradeBannerState [SettingsPath]
# Returns a hashtable describing the current upgrade/migration state.
# ---------------------------------------------------------------------------
function Get-UpgradeBannerState {
    param([string]$SettingsPath = '')

    if (-not $SettingsPath) {
        $h = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { '' }
        $SettingsPath = if ($h) { Join-Path $h '.claude/settings.json' } else { '' }
    }

    # Detect v1 profile blocks using the canonical candidate list.
    $v1Profiles = [System.Collections.Generic.List[string]]::new()
    $migrationDeclined = $true
    $anyV1 = $false

    foreach ($candidate in (Get-ProfilePathsV1Candidates)) {
        if (-not (Test-Path $candidate)) { continue }
        $content = try { Get-Content -Path $candidate -Raw -Encoding utf8 -ErrorAction Stop } catch { '' }
        $content = $content -replace "`r", ''
        if ($content -notmatch '# BEGIN: Claude Code Bedrock Configuration') { continue }
        if ($content.Contains('# Juggernaut v2 shell fallback')) { continue }
        $v1Profiles.Add($candidate)
        $anyV1 = $true
        if ($env:JUGGERNAUT_FORCE_MIGRATION_PROMPT -eq '1' -or $content -notmatch '(?m)^# MigrationDeclined:') {
            $migrationDeclined = $false
        }
    }

    $hasV1 = $anyV1 -and (-not $migrationDeclined)

    # Detect v2 settings.json.
    $hasV2Settings = $false
    if ($SettingsPath -and (Test-Path $SettingsPath) -and (Get-Item $SettingsPath).Length -gt 0) {
        try {
            $j = Get-Content -Path $SettingsPath -Raw -Encoding utf8 | ConvertFrom-Json -ErrorAction Stop
            $hasV2Settings = ($j.juggernaut.meta.managedBy -eq 'juggernaut')
        } catch {}
    }

    # Version strings.
    $installDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } elseif ($env:HOME) { Join-Path $env:HOME '.juggernaut' } elseif ($env:USERPROFILE) { Join-Path $env:USERPROFILE '.juggernaut' } else { '' }
    $installedVersion = ''
    if ($installDir -and (Test-Path (Join-Path $installDir 'VERSION'))) {
        $installedVersion = (Get-Content (Join-Path $installDir 'VERSION') -Raw).Trim()
    }
    $releaseVersion = ''
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $releaseVersionFile = Join-Path $repoRoot 'VERSION'
    if (Test-Path $releaseVersionFile) {
        $releaseVersion = (Get-Content $releaseVersionFile -Raw).Trim()
    }

    $isUpgrade = ($installedVersion -ne '' -and $releaseVersion -ne '' -and $installedVersion -ne $releaseVersion)

    return @{
        has_v1             = $hasV1
        v1_profiles        = @($v1Profiles)
        has_v2_settings    = $hasV2Settings
        installed_version  = $installedVersion
        release_version    = $releaseVersion
        is_upgrade         = $isUpgrade
        migration_declined = $migrationDeclined
    }
}

# ---------------------------------------------------------------------------
# Write-UpgradeBanner <State>
# Prints the banner to stderr. No-op if state does not require a banner.
# ---------------------------------------------------------------------------
function Write-UpgradeBanner {
    param([Parameter(Mandatory)][hashtable]$State)

    if (-not $State.has_v1 -and -not $State.is_upgrade) { return }

    $err = [Console]::Error
    $err.WriteLine('')
    $err.WriteLine('+--------------------------------------------------------------+')
    if ($State.is_upgrade -and $State.installed_version) {
        $err.WriteLine("  Juggernaut: upgrading $($State.installed_version) -> $($State.release_version)")
    } else {
        $err.WriteLine("  Juggernaut $($State.release_version)")
    }

    if ($State.has_v1) {
        $err.WriteLine('')
        $err.WriteLine('  v1 configuration detected in your shell profile.')
        $err.WriteLine('  This release migrates your settings to settings.json (v2).')
        $err.WriteLine('')
        foreach ($p in $State.v1_profiles) {
            $err.WriteLine("    $p")
        }
        $err.WriteLine('')
        $err.WriteLine('  Continue?  y = migrate to v2   n = exit')
        $err.WriteLine('  Keep v1?   pass -LegacyV1 to stay on v1 for now')
    }
    $err.WriteLine('+--------------------------------------------------------------+')
    $err.WriteLine('')
}

# ---------------------------------------------------------------------------
# Confirm-UpgradeBanner <State> <Yes> <LegacyV1>
# Returns:
#   'proceed'  — migrate to v2
#   'abort'    — non-TTY, no -Yes or -LegacyV1
#   'legacy'   — stay on v1
# ---------------------------------------------------------------------------
function Confirm-UpgradeBanner {
    param(
        [Parameter(Mandatory)][hashtable]$State,
        [bool]$Yes = $false,
        [bool]$LegacyV1 = $false
    )

    if (-not $State.has_v1) { return 'proceed' }
    if ($LegacyV1) { return 'legacy' }
    if ($Yes) { return 'proceed' }

    $noTty = ($env:JUGGERNAUT_NO_TTY_PROMPTS -eq '1') -or (-not [Console]::IsInputRedirected -eq $false)
    if (-not [Environment]::UserInteractive -or [Console]::IsInputRedirected) {
        [Console]::Error.WriteLine('juggernaut: non-TTY install with v1 configuration detected.')
        [Console]::Error.WriteLine('Pass -Yes to migrate to v2, or -LegacyV1 to keep v1.')
        return 'abort'
    }

    [Console]::Error.Write('Migrate to v2? [Y/n/legacy-v1] ')
    $answer = [Console]::ReadLine()
    switch -Regex ($answer.ToLower().Trim()) {
        '^$|^y|^yes'     { return 'proceed' }
        '^l|^legacy'     { return 'legacy' }
        default          { return 'abort' }
    }
}

# Test-UpgradeBannerSentinel <InstallDir> <Version>
# Returns $true if the banner has already been shown for this version.
function Test-UpgradeBannerSentinel {
    param([string]$InstallDir, [string]$Version)
    Test-Path (Join-Path $InstallDir ".juggernaut_banner_seen.$Version")
}

# Set-UpgradeBannerSentinel <InstallDir> <Version>
function Set-UpgradeBannerSentinel {
    param([string]$InstallDir, [string]$Version)
    $sentinel = Join-Path $InstallDir ".juggernaut_banner_seen.$Version"
    try { $null | Set-Content -Path $sentinel -ErrorAction SilentlyContinue } catch {}
}
