#Requires -Version 5.1
# install.ps1 - Juggernaut v3 installer (wipe-and-reinstall)
#
# Usage:
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1)))
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version v3.0.0
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Ref fix-branch
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Latest
#
# Or after downloading:
#   .\install.ps1 -Version v3.0.0
#   .\install.ps1 -Ref fix-branch
#   .\install.ps1 -Latest -DryRun
#
# Destructive behavior (v3):
#   - Strips Juggernaut and legacy "Claude Code Bedrock Configuration" BEGIN/END
#     blocks from every known shell profile (including both PS 5.1 and PS 7
#     CurrentUser/AllUsers AllHosts profiles).
#   - Removes the 'juggernaut' key from ~/.claude/settings.json.
#   - Removes the 'juggernaut-bedrock' Windows Credential Manager entry.
#   - Does NOT auto-apply. Run 'juggernaut apply -Auth iam' or
#     'juggernaut apply -Auth bedrock-api-key' explicitly after install.

param(
    [string]$Version = '',
    [string]$Ref = '',
    [switch]$Latest,
    [switch]$DryRun,
    [switch]$Help
)

$ErrorActionPreference = 'Stop'

if ($Help) {
    @'
Juggernaut v3 installer

Usage:
  install.ps1 [-Version <tag>] [-Ref <branch|sha>] [-Latest] [-DryRun]

Installs Juggernaut to $HOME\.juggernaut (override with $env:JUGGERNAUT_DIR).
Before installing, strips legacy Juggernaut/Claude-Code-Bedrock blocks from
shell profiles, removes the 'juggernaut' key from ~/.claude/settings.json,
and deletes the 'juggernaut-bedrock' Credential Manager entry.

-DryRun prints what would be wiped and exits without writing anything.
'@
    return
}

if (-not $PSCommandPath -and
    $MyInvocation.CommandOrigin -eq [System.Management.Automation.CommandOrigin]::Runspace -and
    [string]::IsNullOrEmpty($MyInvocation.InvocationName)) {
    Write-Error @'
The Windows installer cannot be run with:
  irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1 | iex

Use the safer scriptblock form instead:
  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1)))

For a pinned release:
  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.0.0/install.ps1))) -Version v3.0.0
'@
    exit 1
}

if (-not $Ref -and $env:JUGGERNAUT_REF) { $Ref = $env:JUGGERNAUT_REF }
if ($Latest) { $Version = ''; $Ref = '' }
if ($Ref)    { $Version = '' }

if ($Version -and -not $Version.StartsWith('v')) { $Version = "v$Version" }

$RepoUrl    = if ($env:JUGGERNAUT_REPO_URL) { $env:JUGGERNAUT_REPO_URL } else { 'https://github.com/jpvelasco/juggernaut.git' }
$InstallDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } else { Join-Path $HOME '.juggernaut' }

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Error 'git is required but not installed'
    exit 1
}

# ---------------------------------------------------------------------------
# Pre-wipe discovery
# ---------------------------------------------------------------------------
$SettingsPath = Join-Path $HOME '.claude\settings.json'
$KeychainServiceName = 'juggernaut-bedrock'

function Get-ProfileCandidates {
    $home2 = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { $HOME }
    $candidates = [System.Collections.Generic.List[string]]::new()

    foreach ($rel in @('.bashrc', '.bash_profile', '.zshrc', '.config/fish/config.fish', '.profile')) {
        $candidates.Add((Join-Path $home2 $rel))
    }

    try {
        if ($PROFILE.CurrentUserAllHosts) { $candidates.Add([string]$PROFILE.CurrentUserAllHosts) }
        if ($PROFILE.AllUsersAllHosts)    { $candidates.Add([string]$PROFILE.AllUsersAllHosts) }
    } catch {}

    $documents = [Environment]::GetFolderPath('MyDocuments')
    if ($documents) {
        $candidates.Add((Join-Path $documents 'PowerShell\profile.ps1'))
        $candidates.Add((Join-Path $documents 'WindowsPowerShell\profile.ps1'))
    }

    return @($candidates | Where-Object { $_ } | Select-Object -Unique)
}

function Test-ProfileHasJuggernautBlock {
    param([string]$Path)
    if (-not (Test-Path $Path)) { return $false }
    try {
        $content = Get-Content -Path $Path -Raw -ErrorAction Stop
    } catch { return $false }
    return ($content -match '(?m)^# BEGIN: Juggernaut' -or
            $content -match '(?m)^# BEGIN: Claude Code Bedrock Configuration')
}

function Test-SettingsHasJuggernautKey {
    if (-not (Test-Path $SettingsPath)) { return $false }
    try {
        $obj = Get-Content -Path $SettingsPath -Raw -Encoding utf8 | ConvertFrom-Json -ErrorAction Stop
    } catch { return $false }
    return ($null -ne $obj.juggernaut)
}

function Test-WindowsKeychainHasEntry {
    try {
        $src = @'
[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
public static extern bool CredRead(string target, int type, int flags, out IntPtr credential);
[DllImport("advapi32.dll")]
public static extern void CredFree(IntPtr credential);
'@
        Add-Type -Namespace 'Win32Installer' -Name 'CredProbe' -MemberDefinition $src -ErrorAction SilentlyContinue
        $ptr = [IntPtr]::Zero
        $ok = [Win32Installer.CredProbe]::CredRead($KeychainServiceName, 1, 0, [ref]$ptr)
        if ($ok) { [Win32Installer.CredProbe]::CredFree($ptr) | Out-Null }
        return [bool]$ok
    } catch { return $false }
}

$toStripProfiles = @(Get-ProfileCandidates | Where-Object { Test-ProfileHasJuggernautBlock $_ })
$stripSettings   = Test-SettingsHasJuggernautKey
$stripKeychain   = Test-WindowsKeychainHasEntry

Write-Host 'Juggernaut installer - wipe-and-reinstall'
Write-Host ''
Write-Host 'Pre-wipe summary:'
if ($toStripProfiles.Count -gt 0) {
    foreach ($p in $toStripProfiles) { Write-Host "  - strip Juggernaut/v1 block from $p" }
} else {
    Write-Host '  - shell profiles: no Juggernaut/v1 blocks found'
}
if ($stripSettings) { Write-Host "  - remove 'juggernaut' key from $SettingsPath" }
else                { Write-Host "  - settings.json: no 'juggernaut' key found" }
if ($stripKeychain) { Write-Host "  - remove Credential Manager entry '$KeychainServiceName'" }
else                { Write-Host "  - keychain: no '$KeychainServiceName' entry found" }
Write-Host ''

if ($DryRun) {
    Write-Host '-DryRun: no changes written. Exiting.'
    return
}

# ---------------------------------------------------------------------------
# Wipe
# ---------------------------------------------------------------------------
function Remove-ProfileBlocks {
    param([string]$Path)
    if (-not (Test-Path $Path)) { return }
    try {
        $lines = Get-Content -Path $Path -ErrorAction Stop
    } catch {
        Write-Warning "Could not read $Path - $($_.Exception.Message). Skipping (may require elevation)."
        return
    }
    $out = New-Object System.Collections.Generic.List[string]
    $skip = $false
    foreach ($line in $lines) {
        if ($line -match '^# BEGIN: Juggernaut' -or $line -match '^# BEGIN: Claude Code Bedrock Configuration') {
            $skip = $true; continue
        }
        if ($line -match '^# END: Juggernaut' -or $line -match '^# END: Claude Code Bedrock Configuration') {
            $skip = $false; continue
        }
        if (-not $skip) { $out.Add($line) }
    }
    try {
        Set-Content -Path $Path -Value $out -Encoding utf8 -ErrorAction Stop
        Write-Host "Stripped Juggernaut block from $Path"
    } catch {
        Write-Warning "Could not write $Path - $($_.Exception.Message). Skipping (may require elevation)."
    }
}

foreach ($p in $toStripProfiles) { Remove-ProfileBlocks $p }

if ($stripSettings) {
    try {
        $raw = Get-Content -Path $SettingsPath -Raw -Encoding utf8
        $obj = $raw | ConvertFrom-Json
        if ($obj.PSObject.Properties['juggernaut']) {
            $obj.PSObject.Properties.Remove('juggernaut')
        }
        ($obj | ConvertTo-Json -Depth 64) | Set-Content -Path $SettingsPath -Encoding utf8
        Write-Host "Removed 'juggernaut' key from $SettingsPath"
    } catch {
        Write-Warning "Could not update $SettingsPath - $($_.Exception.Message)"
    }
}

if ($stripKeychain) {
    try {
        $src = @'
[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
public static extern bool CredDelete(string target, int type, int flags);
'@
        Add-Type -Namespace 'Win32Installer' -Name 'CredDel' -MemberDefinition $src -ErrorAction SilentlyContinue
        [Win32Installer.CredDel]::CredDelete($KeychainServiceName, 1, 0) | Out-Null
        Write-Host "Removed keychain entry: $KeychainServiceName"
    } catch {
        Write-Warning "Could not remove keychain entry: $($_.Exception.Message)"
    }
}

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------
if ($Ref)         { Write-Host "Installing Juggernaut $Ref..." }
elseif ($Version) { Write-Host "Installing Juggernaut $Version..." }
else              { Write-Host 'Installing Juggernaut (latest)...' }

function Invoke-CloneInstall {
    param([string]$Target = $InstallDir)
    if ($Ref)         { git clone --branch $Ref      --depth 1 --quiet $RepoUrl $Target }
    elseif ($Version) { git clone --branch $Version  --depth 1 --quiet $RepoUrl $Target }
    else              { git clone --quiet                                $RepoUrl $Target }
    if ($LASTEXITCODE -ne 0) { throw 'git clone failed' }
}

function Backup-ExistingInstall {
    $timestamp = Get-Date -Format 'yyyyMMdd_HHmmss'
    $backup = "$InstallDir.backup.$timestamp"
    $n = 1
    while (Test-Path $backup) { $backup = "$InstallDir.backup.$timestamp.$n"; $n++ }
    Write-Host "Backup created: $backup"
    Move-Item -LiteralPath $InstallDir -Destination $backup

    # Always rotate: keep only 5 most recent backups.
    $oldBackups = Get-ChildItem -Path (Split-Path $InstallDir -Parent) -Filter "$(Split-Path $InstallDir -Leaf).backup.*" -Directory -ErrorAction SilentlyContinue |
                  Sort-Object LastWriteTime -Descending |
                  Select-Object -Skip 5
    foreach ($old in $oldBackups) {
        Remove-Item -LiteralPath $old.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Test-InstallTreeDirty {
    git -C $InstallDir rev-parse --git-dir *> $null
    if ($LASTEXITCODE -ne 0) { return $true }
    git -C $InstallDir diff --quiet --ignore-submodules --
    if ($LASTEXITCODE -ne 0) { return $true }
    git -C $InstallDir diff --cached --quiet --ignore-submodules --
    if ($LASTEXITCODE -ne 0) { return $true }
    $untracked = git -C $InstallDir ls-files --others --exclude-standard
    return [bool]$untracked
}

if (Test-Path $InstallDir) {
    if (Test-InstallTreeDirty) {
        Write-Host 'Existing installation has local changes or is not a clean Git checkout.'
        $NewDir = "$InstallDir.new"
        if (Test-Path $NewDir) { Remove-Item -LiteralPath $NewDir -Recurse -Force }
        try {
            Invoke-CloneInstall -Target $NewDir
            Backup-ExistingInstall
            Move-Item -LiteralPath $NewDir -Destination $InstallDir
        } catch {
            if (Test-Path $NewDir) { Remove-Item -LiteralPath $NewDir -Recurse -Force -ErrorAction SilentlyContinue }
            throw
        }
    } else {
        Write-Host "Updating existing installation in $InstallDir"
        git -C $InstallDir fetch --tags --quiet
        if ($LASTEXITCODE -ne 0) { throw 'git fetch failed' }
        if ($Ref) {
            git -C $InstallDir fetch --quiet origin $Ref
            if ($LASTEXITCODE -ne 0) { throw "git fetch $Ref failed" }
            git -C $InstallDir checkout --quiet FETCH_HEAD
            if ($LASTEXITCODE -ne 0) { throw "git checkout $Ref failed" }
        } elseif ($Version) {
            git -C $InstallDir checkout --quiet $Version
            if ($LASTEXITCODE -ne 0) { throw "git checkout $Version failed" }
        } else {
            git -C $InstallDir checkout --quiet main
            if ($LASTEXITCODE -ne 0) { throw 'git checkout main failed' }
            git -C $InstallDir pull --ff-only --quiet
            if ($LASTEXITCODE -ne 0) { throw 'git pull failed' }
        }
    }
} else {
    Invoke-CloneInstall
}

Write-Host "Installed to $InstallDir"

# ---------------------------------------------------------------------------
# Shim
# ---------------------------------------------------------------------------
$ShimDir = Join-Path $HOME '.local\bin'
New-Item -ItemType Directory -Path $ShimDir -Force | Out-Null

$ShimPs1       = Join-Path $ShimDir 'juggernaut.ps1'
$ShimCmd       = Join-Path $ShimDir 'juggernaut.cmd'
$InstallDirTxt = Join-Path $ShimDir 'juggernaut-install-dir.txt'
Set-Content -Path $InstallDirTxt -Value $InstallDir -Encoding utf8 -NoNewline -ErrorAction Stop

@'
param([Parameter(ValueFromRemainingArguments=$true)][string[]]$PassArgs)
$shimDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$dirFile  = Join-Path $shimDir 'juggernaut-install-dir.txt'
if (-not (Test-Path $dirFile)) {
    Write-Error "juggernaut shim: install-dir file not found at $dirFile. Re-run install.ps1."
    exit 1
}
$installDir = (Get-Content $dirFile -Raw -Encoding utf8).Trim()
$target = Join-Path $installDir 'juggernaut.ps1'
if (-not (Test-Path $target)) {
    Write-Error "juggernaut shim: target not found at $target. Re-run install.ps1."
    exit 1
}
& $target @PassArgs
$code = $LASTEXITCODE
exit $code
'@ | Set-Content -Path $ShimPs1 -Encoding utf8

@"
@echo off
where pwsh.exe >nul 2>nul
if %ERRORLEVEL% EQU 0 (
  pwsh.exe -NoProfile -ExecutionPolicy Bypass -File "$ShimPs1" %*
  exit /b %ERRORLEVEL%
) else (
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$ShimPs1" %*
  exit /b %ERRORLEVEL%
)
"@ | Set-Content -Path $ShimCmd -Encoding ascii

Write-Host "Launcher written to $ShimCmd"
if (-not (($env:PATH -split ';') -contains $ShimDir)) {
    Write-Host "Note: add $ShimDir to PATH to run 'juggernaut' from any directory."
}
Write-Host 'If PowerShell blocks first run scripts, run:'
Write-Host '  Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'

Write-Host ''
Write-Host 'Install complete. No configuration has been written.'
Write-Host 'Configure Juggernaut explicitly with one of:'
Write-Host '  juggernaut apply -Auth iam'
Write-Host '  juggernaut apply -Auth bedrock-api-key'
Write-Host ''
Write-Host 'Verify with: juggernaut doctor'
