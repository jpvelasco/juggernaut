# lib/config_manager.ps1 — settings.json read/merge/write for Juggernaut v2. Mirrors lib/config_manager.sh.
# UTF-8 throughout (no BOM preferred; PS 5.1 writes UTF-8-with-BOM when -Encoding utf8 is used, acceptable for JSON).

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

function Read-Settings {
    param([Parameter(Mandatory)][string]$Path)
    if (-not (Test-SettingsExists -Path $Path)) { return [ordered]@{} }
    try {
        $raw = Get-Content -Path $Path -Raw -Encoding utf8
        return $raw | ConvertFrom-Json -AsHashtable -ErrorAction Stop
    } catch {
        throw "Read-Settings: $Path is not valid JSON: $_"
    }
}

function Test-HasJuggernautBlock {
    param([Parameter(Mandatory)]$Settings)
    if (-not $Settings) { return $false }
    if ($Settings -is [hashtable] -or $Settings -is [System.Collections.Specialized.OrderedDictionary]) {
        return $Settings.Contains('juggernaut') -and $Settings.juggernaut.meta.managedBy -eq 'juggernaut'
    }
    return ($Settings.juggernaut -and $Settings.juggernaut.meta.managedBy -eq 'juggernaut')
}

function Get-JuggernautBlockFromSettings {
    param([Parameter(Mandatory)]$Settings)
    if (Test-HasJuggernautBlock -Settings $Settings) { return $Settings.juggernaut }
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
    foreach ($k in @('juggernaut','env','model','modelOverrides','availableModels')) {
        if ($Existing.Contains($k)) { $Existing.Remove($k) | Out-Null }
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

    $dir = Split-Path $Path -Parent
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }

    $json = $Content | ConvertTo-Json -Depth 32
    # Round-trip validation before touching disk.
    try { $null = $json | ConvertFrom-Json -ErrorAction Stop } catch {
        throw "Write-SettingsAtomic: refusing to write invalid JSON to $Path"
    }

    if (Test-Path $Path) { Backup-Settings -Path $Path | Out-Null }

    $tmp = "$Path.tmp.$PID"
    Set-Content -Path $tmp -Value $json -NoNewline -Encoding utf8

    # Atomic rename.
    Move-Item -Path $tmp -Destination $Path -Force
}

function Invoke-WithSettingsLock {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][scriptblock]$Action
    )
    $lockName = "Global\juggernaut_$([IO.Path]::GetFileName($Path))"
    $mutex = New-Object System.Threading.Mutex($false, $lockName)
    try {
        if (-not $mutex.WaitOne(5000)) {
            throw "Invoke-WithSettingsLock: could not acquire lock on $lockName within 5s"
        }
        & $Action
    } finally {
        $mutex.ReleaseMutex() | Out-Null
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
