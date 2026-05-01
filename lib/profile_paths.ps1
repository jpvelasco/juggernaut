# lib/profile_paths.ps1 — Canonical list of shell profile candidates for v1 block detection.
# PowerShell mirror of lib/profile_paths.sh.

# Get-ProfilePathsV1Candidates
# Returns an array of absolute paths for each profile file that may contain a
# Juggernaut v1 BEGIN/END block. Caller filters with Test-MigratorHasV1Block.
function Get-ProfilePathsV1Candidates {
    $homePath = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { '' }
    if (-not $homePath) { return @() }

    $candidates = [System.Collections.Generic.List[string]]::new()

    # Bash / zsh / fish / POSIX
    foreach ($rel in @('.bashrc', '.bash_profile', '.zshrc', '.config/fish/config.fish', '.profile')) {
        $candidates.Add((Join-Path $homePath $rel))
    }

    # PowerShell profiles
    if ($env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS) {
        foreach ($p in ($env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -split [IO.Path]::PathSeparator | Where-Object { $_ })) {
            $candidates.Add($p)
        }
    } else {
        try {
            if ($PROFILE.CurrentUserAllHosts) { $candidates.Add([string]$PROFILE.CurrentUserAllHosts) }
        } catch {}
        $documents = [Environment]::GetFolderPath('MyDocuments')
        if ($documents) {
            $candidates.Add((Join-Path $documents 'PowerShell\profile.ps1'))
            $candidates.Add((Join-Path $documents 'WindowsPowerShell\profile.ps1'))
        }
    }

    return @($candidates | Select-Object -Unique)
}
