# commands/migrate.ps1 — juggernaut migrate subcommand (PowerShell).
# Usage:
#   .\migrate.ps1                  Detect v1 block and migrate to settings.json.
#   .\migrate.ps1 -Rollback        Restore most recent settings.json backup.
#   .\migrate.ps1 -Clean           Remove profile block after successful migration.
#   .\migrate.ps1 -DryRun          Preview without writing.
[CmdletBinding()]
param(
    [switch]$Rollback,
    [switch]$Clean,
    [switch]$DryRun,
    [ValidateSet('user','project')][string]$Scope = 'user'
)

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
. (Join-Path $RepoRoot 'lib/schema.ps1')
. (Join-Path $RepoRoot 'lib/config_manager.ps1')
. (Join-Path $RepoRoot 'lib/migrator.ps1')

$SettingsPath = Resolve-SettingsTarget -Scope $Scope

# ---------------------------------------------------------------------------
# Rollback
# ---------------------------------------------------------------------------
if ($Rollback) {
    if ($DryRun) {
        Write-Host "[dry-run] Would rollback $SettingsPath to most recent backup"
        exit 0
    }
    $ok = Invoke-MigratorRollback -SettingsPath $SettingsPath
    exit ($ok ? 0 : 1)
}

# ---------------------------------------------------------------------------
# Standard migration — scan Windows shell profiles
# ---------------------------------------------------------------------------
$candidates = @(
    $PROFILE,
    (Join-Path $HOME 'Documents\PowerShell\Microsoft.PowerShell_profile.ps1'),
    (Join-Path $HOME 'Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1')
) | Where-Object { $_ -and (Test-Path $_) } | Select-Object -Unique

$found = 0
foreach ($profile in $candidates) {
    if (Test-MigratorHasV1Block -ProfileFile $profile) {
        $found++
        Write-Host "Found v1 block: $profile"

        if ($DryRun) {
            Write-Host "[dry-run] Would migrate $profile → $SettingsPath"
            continue
        }

        $bcPath = Join-Path $RepoRoot 'bedrock-config.json'
        $ok = Invoke-MigratorRun -ProfileFile $profile -SettingsPath $SettingsPath -BedrockConfigPath $bcPath

        if ($ok -and $Clean) {
            $lines = Get-Content -Path $profile
            $inBlock = $false
            $filtered = foreach ($line in $lines) {
                if ($line -eq '# BEGIN: Claude Code Bedrock Configuration') { $inBlock = $true; continue }
                if ($line -eq '# END: Claude Code Bedrock Configuration')   { $inBlock = $false; continue }
                if (-not $inBlock) { $line }
            }
            $filtered | Set-Content -Path $profile -Encoding utf8
            Write-Host "Removed v1 block from $profile (--clean)"
        }
    }
}

if ($found -eq 0) {
    Write-Host "No v1 profile blocks found. Nothing to migrate."
    exit 0
}

Write-Host "Migration done. Verify with: juggernaut doctor"
