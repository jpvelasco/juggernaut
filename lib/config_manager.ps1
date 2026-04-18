# lib/config_manager.ps1 — settings.json read/merge/write for Juggernaut v2. Mirrors lib/config_manager.sh.
# UTF-8 throughout. Note: Set-Content -Encoding utf8 emits UTF-8-with-BOM on
# Windows PowerShell 5.1; all mainstream JSON parsers tolerate a leading BOM.

$Script:ConfigBackupRetain = 5

function Get-UserSettingsPath {
    Join-Path $HOME '.claude/settings.json'
}

function Get-ProjectSettingsPath {
    param([string]$StartDir = (Get-Location).Path)
    $dir = $StartDir
    while ($dir -and $dir -ne $HOME -and $dir -ne [IO.Path]::GetPathRoot($dir)) {
        $candidate = Join-Path $dir '.claude/settings.json'
        if (Test-Path $candidate) { return $candidate }
        $parent = Split-Path $dir -Parent
        if ($parent -eq $dir) { break }
        $dir = $parent
    }
    return $null
}

function Resolve-SettingsTarget {
    param([ValidateSet('user','project')][string]$Scope = 'user')
    switch ($Scope) {
        'user'    { Get-UserSettingsPath }
        'project' { Join-Path (Get-Location).Path '.claude/settings.json' }
    }
}

function Test-SettingsExists {
    param([Parameter(Mandatory)][string]$Path)
    (Test-Path $Path) -and ((Get-Item $Path).Length -gt 0)
}

function ConvertTo-HashtableRecursive {
    # PS 5.1 has no ConvertFrom-Json -AsHashtable. Walk PSCustomObject → [ordered]@{}
    param([Parameter(Mandatory)][AllowNull()]$InputObject)
    if ($null -eq $InputObject) { return $null }
    if ($InputObject -is [System.Collections.IList]) {
        return ,@($InputObject | ForEach-Object { ConvertTo-HashtableRecursive $_ })
    }
    if ($InputObject -is [PSCustomObject]) {
        $out = [ordered]@{}
        foreach ($p in $InputObject.PSObject.Properties) {
            $out[$p.Name] = ConvertTo-HashtableRecursive $p.Value
        }
        return $out
    }
    return $InputObject
}

function Read-Settings {
    param([Parameter(Mandatory)][string]$Path)
    if (-not (Test-SettingsExists -Path $Path)) { return [ordered]@{} }
    $raw = Get-Content -Path $Path -Raw -Encoding utf8
    try {
        $parsed = $raw | ConvertFrom-Json -ErrorAction Stop
    } catch {
        throw "Read-Settings: $Path is not valid JSON: $_"
    }
    # Force hashtable/ordered-dict shape so callers can .Contains() / .Remove() uniformly on PS 5.1+.
    $result = ConvertTo-HashtableRecursive $parsed
    if ($null -eq $result) { return [ordered]@{} }
    return $result
}

function Test-HasJuggernautBlock {
    param([Parameter(Mandatory)]$Settings)
    if (-not $Settings) { return $false }
    $hasKey = $false
    if ($Settings -is [hashtable] -or $Settings -is [System.Collections.Specialized.OrderedDictionary]) {
        $hasKey = $Settings.Contains('juggernaut')
    } else {
        $hasKey = [bool]($Settings.PSObject.Properties.Name -contains 'juggernaut')
    }
    if (-not $hasKey) { return $false }
    return ($Settings['juggernaut'].meta.managedBy -eq 'juggernaut')
}

function Get-JuggernautBlockFromSettings {
    param([Parameter(Mandatory)]$Settings)
    if (Test-HasJuggernautBlock -Settings $Settings) { return $Settings['juggernaut'] }
    return $null
}

function Merge-JuggernautBlock {
    param(
        [Parameter(Mandatory)]$Existing,
        [Parameter(Mandatory)]$NewBlock,
        [Parameter(Mandatory)]$NativeKeys
    )
    if (-not $Existing) { $Existing = [ordered]@{} }
    $Existing['juggernaut']     = $NewBlock
    $Existing['env']            = $NativeKeys.env
    $Existing['model']          = $NativeKeys.model
    $Existing['modelOverrides'] = $NativeKeys.modelOverrides
    return $Existing
}

function Remove-JuggernautBlockFromSettings {
    param([Parameter(Mandatory)]$Existing)
    if (-not $Existing) { return [ordered]@{} }
    foreach ($k in @('juggernaut','env','model','modelOverrides','availableModels')) {
        if ($Existing -is [hashtable] -or $Existing -is [System.Collections.Specialized.OrderedDictionary]) {
            if ($Existing.Contains($k)) { $Existing.Remove($k) | Out-Null }
        } elseif ($Existing.PSObject.Properties.Name -contains $k) {
            $Existing.PSObject.Properties.Remove($k) | Out-Null
        }
    }
    return $Existing
}

function Backup-Settings {
    param([Parameter(Mandatory)][string]$Path)
    if (-not (Test-Path $Path)) { return $null }
    $stamp = Get-Date -Format 'yyyyMMdd_HHmmss'
    $backup = "$Path.backup.$stamp"
    Copy-Item -Path $Path -Destination $backup -Force
    Invoke-SettingsBackupRotation -Path $Path
    return $backup
}

function Invoke-SettingsBackupRotation {
    param([Parameter(Mandatory)][string]$Path)
    $pattern = "$Path.backup.*"
    $backups = Get-ChildItem -Path (Split-Path $Path -Parent) -Filter (Split-Path $pattern -Leaf) -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending
    if ($backups.Count -gt $Script:ConfigBackupRetain) {
        $backups | Select-Object -Skip $Script:ConfigBackupRetain | Remove-Item -Force -ErrorAction SilentlyContinue
    }
}

function Write-SettingsAtomic {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)]$Content
    )

    $json = $Content | ConvertTo-Json -Depth 32
    # Round-trip validation before touching disk or taking the lock.
    try { $null = $json | ConvertFrom-Json -ErrorAction Stop } catch {
        throw "Write-SettingsAtomic: refusing to write invalid JSON to $Path"
    }

    $dir = Split-Path $Path -Parent
    try {
        if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    } catch {
        throw "Write-SettingsAtomic: cannot create parent directory ${dir}: $_"
    }

    # Acquire the per-file mutex inline. Passing a scriptblock to
    # Invoke-WithSettingsLock and having it resolve Backup-Settings through
    # scope chains is fragile on PS 7; inline form keeps everything in the
    # current module's scope.
    $lockName = "Global\juggernaut_$([IO.Path]::GetFileName($Path))"
    $mutex = New-Object System.Threading.Mutex($false, $lockName)
    $acquired = $false
    try {
        try { $acquired = $mutex.WaitOne(5000) }
        catch [System.Threading.AbandonedMutexException] { $acquired = $true }
        catch { throw "Write-SettingsAtomic: mutex acquisition failed on ${lockName}: $_" }
        if (-not $acquired) { throw "Write-SettingsAtomic: could not acquire lock on ${lockName} within 5s" }

        if (Test-Path $Path) { Backup-Settings -Path $Path | Out-Null }
        $tmp = "$Path.tmp.$PID"
        try {
            Set-Content -Path $tmp -Value $json -NoNewline -Encoding utf8
            Move-Item -Path $tmp -Destination $Path -Force
        } catch {
            Remove-Item -Path $tmp -Force -ErrorAction SilentlyContinue
            throw
        }
    } finally {
        if ($acquired) { try { $mutex.ReleaseMutex() | Out-Null } catch {} }
        $mutex.Dispose()
    }
}

function Invoke-WithSettingsLock {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][scriptblock]$Action
    )
    # Per-file named mutex so multiple settings.json files don't serialize against each other.
    $lockName = "Global\juggernaut_$([IO.Path]::GetFileName($Path))"
    $mutex = New-Object System.Threading.Mutex($false, $lockName)
    $acquired = $false
    try {
        try {
            $acquired = $mutex.WaitOne(5000)
        } catch [System.Threading.AbandonedMutexException] {
            # A prior holder died without releasing; we've now acquired it safely.
            $acquired = $true
        } catch {
            throw "Invoke-WithSettingsLock: mutex acquisition failed on ${lockName}: $_"
        }
        if (-not $acquired) {
            throw "Invoke-WithSettingsLock: could not acquire lock on ${lockName} within 5s"
        }
        & $Action
    } finally {
        if ($acquired) {
            try { $mutex.ReleaseMutex() | Out-Null } catch {}
        }
        $mutex.Dispose()
    }
}

function Get-EffectiveSettings {
    $userPath = Get-UserSettingsPath
    $projectPath = Get-ProjectSettingsPath
    $userSettings = if (Test-SettingsExists -Path $userPath) { Read-Settings -Path $userPath } else { $null }
    $projectSettings = if ($projectPath) { Read-Settings -Path $projectPath } else { $null }

    $result = [ordered]@{
        user    = [ordered]@{ path = $userPath; settings = $userSettings }
        project = if ($projectPath) { [ordered]@{ path = $projectPath; settings = $projectSettings } } else { $null }
    }
    return $result
}
