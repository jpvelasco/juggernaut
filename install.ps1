#Requires -Version 5.1
# install.ps1 - Juggernaut installer
#
# Usage:
#   irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1 | iex
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.1.2
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Ref fix-branch
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Latest
#
# Or after downloading:
#   .\install.ps1 -Version 2.1.2
#   .\install.ps1 -Ref fix-branch
#   .\install.ps1 -Latest

param(
    [string]$Version = '',
    [string]$Ref = '',
    [switch]$Latest,
    [switch]$Configure,
    [switch]$Yes,
    [switch]$LegacyV1,
    [switch]$KeepAllBackups,
    [Parameter(ValueFromRemainingArguments=$true)][string[]]$SetupArgs
)

$ErrorActionPreference = 'Stop'

if (-not $Ref -and $env:JUGGERNAUT_REF) { $Ref = $env:JUGGERNAUT_REF }
if ($Latest) { $Version = ''; $Ref = '' }
if ($Ref) { $Version = '' }

# Normalize version: accept "2.1.2" or "v2.1.2" - tags are always v-prefixed.
if ($Version -and -not $Version.StartsWith('v')) { $Version = "v$Version" }

$RepoUrl    = if ($env:JUGGERNAUT_REPO_URL) { $env:JUGGERNAUT_REPO_URL } else { 'https://github.com/jpvelasco/juggernaut.git' }
$InstallDir = if ($env:JUGGERNAUT_DIR) { $env:JUGGERNAUT_DIR } else { Join-Path $HOME '.juggernaut' }

if ($Ref) {
    Write-Host "Installing Juggernaut $Ref..."
} elseif ($Version) {
    Write-Host "Installing Juggernaut $Version..."
} else {
    Write-Host 'Installing Juggernaut (latest)...'
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Error 'git is required but not installed'
    exit 1
}

function Clone-Install {
    param([string]$Target = $InstallDir)
    if ($Ref) {
        git clone --branch $Ref --depth 1 --quiet $RepoUrl $Target
    } elseif ($Version) {
        git clone --branch $Version --depth 1 --quiet $RepoUrl $Target
    } else {
        git clone --quiet $RepoUrl $Target
    }
    if ($LASTEXITCODE -ne 0) { throw 'git clone failed' }
}

function Backup-ExistingInstall {
    $timestamp = Get-Date -Format 'yyyyMMdd_HHmmss'
    $backup = "$InstallDir.backup.$timestamp"
    $n = 1
    while (Test-Path $backup) {
        $backup = "$InstallDir.backup.$timestamp.$n"
        $n++
    }
    Write-Host "Backup created: $backup"
    Move-Item -LiteralPath $InstallDir -Destination $backup

    # Rotate: keep only 5 most recent backups unless -KeepAllBackups was passed.
    if (-not $KeepAllBackups) {
        $oldBackups = Get-ChildItem -Path (Split-Path $InstallDir -Parent) -Filter "$(Split-Path $InstallDir -Leaf).backup.*" -Directory -ErrorAction SilentlyContinue |
                      Sort-Object LastWriteTime -Descending |
                      Select-Object -Skip 5
        foreach ($old in $oldBackups) {
            Remove-Item -LiteralPath $old.FullName -Recurse -Force -ErrorAction SilentlyContinue
        }
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

# Shared arg parser (extracted to lib/arg_parsing.ps1 so install.ps1 and juggernaut.ps1 share one copy).
# We dot-source it after $InstallDir is known (below), or fall back to the repo-local copy when running
# the installer directly from the repo.  At the point this function is needed $InstallDir already exists.

if (Test-Path $InstallDir) {
    if (Test-InstallTreeDirty) {
        Write-Host 'Existing installation has local changes or is not a clean Git checkout.'
        # Clone to a sibling directory first so a failed clone cannot destroy the
        # existing install. Only if the clone succeeds do we swap directories.
        $NewDir = "$InstallDir.new"
        if (Test-Path $NewDir) { Remove-Item -LiteralPath $NewDir -Recurse -Force }
        try {
            Clone-Install -Target $NewDir
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
    Clone-Install
}

Write-Host "Installed to $InstallDir"

$ShimDir = Join-Path $HOME '.local\bin'
New-Item -ItemType Directory -Path $ShimDir -Force | Out-Null

$ShimPs1 = Join-Path $ShimDir 'juggernaut.ps1'
$ShimCmd = Join-Path $ShimDir 'juggernaut.cmd'
# Write the install dir into a sidecar file so the shim resolves at runtime.
# This means moving the install dir only requires updating the .txt file.
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
$env:JUGGERNAUT_USE_V2 = '1'
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

# ---------------------------------------------------------------------------
# Upgrade banner — show version diff and handle v1→v2 migration prompt.
# ---------------------------------------------------------------------------
$argParsingLib = Join-Path $InstallDir 'lib\arg_parsing.ps1'
if (Test-Path $argParsingLib) { . $argParsingLib }

$bannerLib = Join-Path $InstallDir 'lib\upgrade_banner.ps1'
$profilePathsLib = Join-Path $InstallDir 'lib\profile_paths.ps1'
$migratorLib = Join-Path $InstallDir 'lib\migrator.ps1'
$configMgrLib = Join-Path $InstallDir 'lib\config_manager.ps1'
if ((Test-Path $bannerLib) -and (Test-Path $profilePathsLib) -and (Test-Path $migratorLib) -and (Test-Path $configMgrLib)) {
    . $profilePathsLib
    . $configMgrLib
    . (Join-Path $InstallDir 'lib\schema.ps1')
    . $migratorLib
    . $bannerLib

    $bannerState = Get-UpgradeBannerState
    Write-UpgradeBanner -State $bannerState
    $confirmResult = Confirm-UpgradeBanner -State $bannerState -Yes:([bool]$Yes) -LegacyV1:([bool]$LegacyV1)
    switch ($confirmResult) {
        'abort' {
            Write-Host 'Install complete. Re-run with -Yes to migrate to v2, or -LegacyV1 to keep v1.'
            exit 3
        }
        'legacy' {
            Write-Host 'Keeping v1 configuration. Run juggernaut apply whenever you are ready to upgrade.'
            if ($env:JUGGERNAUT_SUPPRESS_DEPRECATION -ne '1') {
                [Console]::Error.WriteLine('Note: Juggernaut v1 is deprecated and will be removed in v3.0.')
            }
        }
        'proceed' {
            if ($bannerState.has_v1) {
                Write-Host 'Migrating v1 configuration to v2...'
                $settingsPath = Join-Path (if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { $HOME }) '.claude/settings.json'
                foreach ($profile in $bannerState.v1_profiles) {
                    try {
                        Invoke-MigratorRun -ProfileFile $profile -SettingsPath $settingsPath -BedrockConfigPath (Join-Path $InstallDir 'bedrock-config.json')
                    } catch {
                        Write-Warning "Migration of $profile encountered an error: $_"
                    }
                }
            }
        }
    }
}

Write-Host 'Verify with: juggernaut doctor --v2'
Write-Host 'Configure with one of:'
Write-Host '  juggernaut apply --auth=bedrock-api-key'
Write-Host '  juggernaut apply --auth=iam'

if ($Configure) {
    $oldLocation = (Get-Location).Path
    try {
        Set-Location $InstallDir
        function Convert-InstallerApplyArgs {
            param([string[]]$InputArgs)
            Convert-GnuStyleArgs -InputArgs $InputArgs
        }
        $applyArgs = Convert-InstallerApplyArgs -InputArgs $SetupArgs
        & (Join-Path $InstallDir 'commands\apply.ps1') @applyArgs
    } finally {
        Set-Location $oldLocation
    }
    return
} elseif ($SetupArgs.Count -gt 0) {
    Write-Warning 'Install arguments after -Version were ignored. Use -Configure to run apply during install.'
}
