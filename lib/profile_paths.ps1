# lib/profile_paths.ps1 - Shell profile paths scanned for legacy Juggernaut
# and v1 "Claude Code Bedrock Configuration" blocks during installer wipe.
# Juggernaut v3 does not write to shell profiles; this list exists only so
# the installer can strip leftover blocks from earlier versions.

function Get-ProfilePathsScanTargets {
    $homePath = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { '' }
    if (-not $homePath) { return @() }

    $candidates = [System.Collections.Generic.List[string]]::new()

    foreach ($rel in @('.bashrc', '.bash_profile', '.zshrc', '.config/fish/config.fish', '.profile')) {
        $candidates.Add((Join-Path $homePath $rel))
    }

    if ($env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS) {
        foreach ($p in ($env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -split [IO.Path]::PathSeparator | Where-Object { $_ })) {
            $candidates.Add($p)
        }
    } else {
        try {
            if ($PROFILE.CurrentUserAllHosts) { $candidates.Add([string]$PROFILE.CurrentUserAllHosts) }
            if ($PROFILE.AllUsersAllHosts)    { $candidates.Add([string]$PROFILE.AllUsersAllHosts) }
        } catch {}
        $documents = [Environment]::GetFolderPath('MyDocuments')
        if ($documents) {
            $candidates.Add((Join-Path $documents 'PowerShell\profile.ps1'))
            $candidates.Add((Join-Path $documents 'WindowsPowerShell\profile.ps1'))
        }
    }

    return @($candidates | Select-Object -Unique)
}

# Legacy alias kept for any callers we may have missed.
function Get-ProfilePathsV1Candidates { Get-ProfilePathsScanTargets }
