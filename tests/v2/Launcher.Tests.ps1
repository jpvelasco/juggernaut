# tests/v2/Launcher.Tests.ps1 — Pester 5 tests for the Juggernaut launcher
# profile block (install.ps1 Install-LauncherProfileBlock + commands/uninstall.ps1
# Remove-LauncherProfileBlock) and the emitted `function claude` runtime.

BeforeAll {
    $script:RepoRoot    = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    $script:InstallPs1  = Join-Path $script:RepoRoot 'install.ps1'
    $script:InstallSrc  = Get-Content $script:InstallPs1 -Raw
    $script:UninstallSrc= Get-Content (Join-Path $script:RepoRoot 'commands\uninstall.ps1') -Raw

    # Extract the launcher block as a standalone string by locating the single-
    # quoted here-string that opens with "# BEGIN: Juggernaut Launcher".
    $pattern = "@'\s*(# BEGIN: Juggernaut Launcher[\s\S]*?# END: Juggernaut Launcher)\s*'@"
    if ($script:InstallSrc -match $pattern) {
        $script:LauncherBlock = $Matches[1]
    } else {
        throw 'Could not locate launcher block in install.ps1 source'
    }

    function New-TestProfile {
        $dir = Join-Path ([IO.Path]::GetTempPath()) ("jug-launcher-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        return (Join-Path $dir 'profile.ps1')
    }

    function Install-BlockToPath([string]$Path) {
        # Mirror the install.ps1 logic: strip existing, append fresh.
        $existing = @()
        if (Test-Path $Path) { $existing = Get-Content -Path $Path }
        $out = New-Object System.Collections.Generic.List[string]
        $skip = $false
        foreach ($line in $existing) {
            if ($line -match '^# BEGIN: Juggernaut Launcher') { $skip = $true; continue }
            if ($line -match '^# END: Juggernaut Launcher')   { $skip = $false; continue }
            if (-not $skip) { $out.Add($line) }
        }
        if ($out.Count -gt 0 -and $out[$out.Count - 1] -ne '') { $out.Add('') }
        foreach ($line in ($script:LauncherBlock -split "`r?`n")) { $out.Add($line) }
        Set-Content -Path $Path -Value $out -Encoding utf8
    }

    function Remove-BlockFromPath([string]$Path) {
        # Mirror the uninstall.ps1 logic.
        if (-not (Test-Path $Path)) { return }
        $lines = Get-Content -Path $Path
        $out = New-Object System.Collections.Generic.List[string]
        $skip = $false
        foreach ($line in $lines) {
            if ($line -match '^# BEGIN: Juggernaut Launcher') { $skip = $true; continue }
            if ($line -match '^# END: Juggernaut Launcher')   { $skip = $false; continue }
            if (-not $skip) { $out.Add($line) }
        }
        while ($out.Count -gt 0 -and $out[$out.Count - 1] -eq '') {
            $out.RemoveAt($out.Count - 1)
        }
        Set-Content -Path $Path -Value $out -Encoding utf8
    }

    function New-StubClaudeCmd([string]$Dir, [string]$Name = 'claude.exe') {
        if (-not (Test-Path $Dir)) { New-Item -ItemType Directory -Path $Dir -Force | Out-Null }
        $cmdPath = Join-Path $Dir ($Name -replace '\.exe$', '.cmd')
        @"
@echo off
echo STUB_BEARER=%AWS_BEARER_TOKEN_BEDROCK%
echo STUB_ARGS=%*
exit /b 42
"@ | Set-Content -Path $cmdPath -Encoding ascii
        return $cmdPath
    }

    function Invoke-LauncherWith([string]$ProfilePath, [string]$Command, [hashtable]$EnvOverrides = @{}) {
        $pwsh = Get-Command pwsh.exe -ErrorAction SilentlyContinue
        if (-not $pwsh) { $pwsh = Get-Command powershell.exe -ErrorAction Stop }
        $effectiveEnv = @{
            AWS_BEARER_TOKEN_BEDROCK      = $null
            HOME                          = $script:IsolatedHome
            USERPROFILE                   = $script:IsolatedHome
            XDG_CONFIG_HOME               = (Join-Path $script:IsolatedHome '.config')
            JUGGERNAUT_HOME               = $script:IsolatedHome
            JUGGERNAUT_PROFILE_TOKEN_PATH = (Join-Path (Join-Path $script:IsolatedHome '.config\juggernaut') 'bearer-token')
            JUGGERNAUT_KEYCHAIN_SERVICE   = $script:IsolatedService
        }
        foreach ($k in $EnvOverrides.Keys) {
            $effectiveEnv[$k] = $EnvOverrides[$k]
        }
        # Build env assignments as a prelude.
        $envPrelude = ''
        foreach ($k in $effectiveEnv.Keys) {
            $v = $effectiveEnv[$k]
            if ($null -eq $v) {
                $envPrelude += "Remove-Item Env:\$k -ErrorAction SilentlyContinue; "
            } else {
                $escaped = $v -replace "'", "''"
                $envPrelude += "`$env:$k = '$escaped'; "
            }
        }
        $dotSource = ". '$($ProfilePath -replace "'", "''")'; "
        # Force LASTEXITCODE propagation: wrap the payload so any native call's
        # exit code (or the function's $global:LASTEXITCODE assignment) flows
        # back to powershell.exe as its process exit code.
        $full = "$envPrelude$dotSource$Command; if (`$null -ne `$LASTEXITCODE) { exit `$LASTEXITCODE }"
        $out = & $pwsh.Source -NoProfile -ExecutionPolicy Bypass -Command $full 2>&1 | Out-String
        return @{ Output = $out; ExitCode = $LASTEXITCODE }
    }
}

Describe 'install.ps1 launcher block source' {
    It 'contains a single BEGIN/END launcher pair' {
        ([regex]::Matches($script:InstallSrc, '^# BEGIN: Juggernaut Launcher', 'Multiline')).Count | Should -Be 1
        ([regex]::Matches($script:InstallSrc, '^# END: Juggernaut Launcher',   'Multiline')).Count | Should -Be 1
    }
    It 'defines a function named claude' {
        $script:LauncherBlock | Should -Match '(?m)^function claude\s*\{'
    }
    It 'reads AWS_BEARER_TOKEN_BEDROCK from env before keychain' {
        $script:LauncherBlock | Should -Match 'if \(-not \$env:AWS_BEARER_TOKEN_BEDROCK\)'
    }
    It 'uses the Juggernaut.Launcher.Cred namespace to avoid collision with lib/keychain.ps1 Win32.Cred' {
        $script:LauncherBlock | Should -Match 'Juggernaut\.Launcher\.Cred'
        $script:LauncherBlock | Should -Not -Match 'Win32\.Cred'
    }
    It 'honors JUGGERNAUT_KEYCHAIN_SERVICE override' {
        $script:LauncherBlock | Should -Match 'JUGGERNAUT_KEYCHAIN_SERVICE'
    }
    It 'honors JUGGERNAUT_CLAUDE_BIN override for tests' {
        $script:LauncherBlock | Should -Match 'JUGGERNAUT_CLAUDE_BIN'
    }
    It 'excludes juggernaut-dir sources from Get-Command scan' {
        $script:LauncherBlock | Should -Match ([regex]::Escape('*\juggernaut*'))
        $script:LauncherBlock | Should -Match ([regex]::Escape('*\.juggernaut*'))
    }
    It 'swallows Add-Type / CredRead failures silently (fall-through)' {
        $script:LauncherBlock | Should -Match '(?s)try\s*\{[\s\S]+?Add-Type[\s\S]+?\}\s*catch'
    }
}

Describe 'Install-LauncherProfileBlock idempotency (simulated)' {
    It 'writing twice leaves exactly one BEGIN/END pair' {
        $p = New-TestProfile
        try {
            Install-BlockToPath $p
            Install-BlockToPath $p
            $content = Get-Content -Path $p -Raw
            ([regex]::Matches($content, '^# BEGIN: Juggernaut Launcher', 'Multiline')).Count | Should -Be 1
            ([regex]::Matches($content, '^# END: Juggernaut Launcher',   'Multiline')).Count | Should -Be 1
        } finally {
            Remove-Item -Path (Split-Path -Parent $p) -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'preserves lines before the block' {
        $p = New-TestProfile
        try {
            '# user content above' | Set-Content -Path $p -Encoding utf8
            Install-BlockToPath $p
            $raw = Get-Content -Path $p -Raw
            $raw | Should -Match '# user content above'
            $raw | Should -Match '(?m)^# BEGIN: Juggernaut Launcher'
        } finally {
            Remove-Item -Path (Split-Path -Parent $p) -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Describe 'Remove-LauncherProfileBlock (simulated)' {
    It 'removes BEGIN/END markers and trailing blank separator' {
        $p = New-TestProfile
        try {
            '# user content' | Set-Content -Path $p -Encoding utf8
            Install-BlockToPath $p
            Remove-BlockFromPath $p
            $raw = Get-Content -Path $p -Raw
            $raw | Should -Not -Match 'BEGIN: Juggernaut Launcher'
            $raw | Should -Not -Match 'END: Juggernaut Launcher'
            $raw | Should -Match '# user content'
        } finally {
            Remove-Item -Path (Split-Path -Parent $p) -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'leaves an empty file empty (no markers remain)' {
        $p = New-TestProfile
        try {
            Install-BlockToPath $p
            Remove-BlockFromPath $p
            $content = ''
            if (Test-Path $p) { $content = (Get-Content -Path $p -Raw) }
            if ($null -eq $content) { $content = '' }
            $content | Should -Not -Match 'Juggernaut Launcher'
        } finally {
            Remove-Item -Path (Split-Path -Parent $p) -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Describe 'function claude runtime behavior' {
    BeforeAll {
        $script:TestProfile = New-TestProfile
        Install-BlockToPath $script:TestProfile
        $script:StubDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-stub-" + [Guid]::NewGuid().ToString('N'))
        $script:StubBin = New-StubClaudeCmd $script:StubDir
        $script:IsolatedHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-launcher-home-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:IsolatedHome -Force | Out-Null
        # Isolate the keychain service name so the stub never hits a real entry.
        $script:IsolatedService = "juggernaut-absent-pester-$([Guid]::NewGuid().ToString('N'))"
    }
    AfterAll {
        Remove-Item -Path (Split-Path -Parent $script:TestProfile) -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path $script:StubDir -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -Path $script:IsolatedHome -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'function precedence: Get-Command claude resolves to a function after dot-sourcing the profile' {
        $r = Invoke-LauncherWith $script:TestProfile '(Get-Command claude).CommandType'
        $r.Output | Should -Match 'Function'
    }

    It 'env preset: pre-existing AWS_BEARER_TOKEN_BEDROCK is passed through unchanged' {
        $r = Invoke-LauncherWith $script:TestProfile 'claude --foo bar' @{
            AWS_BEARER_TOKEN_BEDROCK    = 'preset-token'
            JUGGERNAUT_CLAUDE_BIN       = $script:StubBin
            JUGGERNAUT_KEYCHAIN_SERVICE = $script:IsolatedService
        }
        $r.Output | Should -Match 'STUB_BEARER=preset-token'
        $r.ExitCode | Should -Be 42
    }

    It 'argv passthrough: stub claude sees all args verbatim' {
        $r = Invoke-LauncherWith $script:TestProfile 'claude --one two --three=four' @{
            AWS_BEARER_TOKEN_BEDROCK    = 'x'
            JUGGERNAUT_CLAUDE_BIN       = $script:StubBin
            JUGGERNAUT_KEYCHAIN_SERVICE = $script:IsolatedService
        }
        $r.Output | Should -Match 'STUB_ARGS=.*--one.*two.*--three=four'
    }

    It 'keychain miss: isolated service returns nothing; stub launched with empty bearer' {
        $r = Invoke-LauncherWith $script:TestProfile 'claude' @{
            AWS_BEARER_TOKEN_BEDROCK    = $null
            JUGGERNAUT_CLAUDE_BIN       = $script:StubBin
            JUGGERNAUT_KEYCHAIN_SERVICE = $script:IsolatedService
        }
        $r.Output | Should -Match 'STUB_BEARER='
        $r.Output | Should -Not -Match 'STUB_BEARER=\S+'
        $r.ExitCode | Should -Be 42
    }

    It 'no upstream binary: Write-Error fires with "no upstream claude.exe" message' {
        $r = Invoke-LauncherWith $script:TestProfile 'claude' @{
            AWS_BEARER_TOKEN_BEDROCK    = 'x'
            JUGGERNAUT_CLAUDE_BIN       = ''
            JUGGERNAUT_KEYCHAIN_SERVICE = $script:IsolatedService
            PATH                        = [IO.Path]::GetTempPath()
        }
        $r.Output | Should -Match 'no upstream claude\.exe'
    }
}

Describe 'uninstall.ps1 has launcher-block strip logic' {
    It 'defines Remove-LauncherProfileBlock' {
        $script:UninstallSrc | Should -Match 'function Remove-LauncherProfileBlock'
    }
    It 'defines Test-ProfileHasLauncherBlock' {
        $script:UninstallSrc | Should -Match 'function Test-ProfileHasLauncherBlock'
    }
    It 'detects sibling host profile via path substitution' {
        $script:UninstallSrc | Should -Match 'WindowsPowerShell'
        $script:UninstallSrc | Should -Match '\\\\PowerShell\\\\'
    }
    It 'includes launcher-block state in nothing-to-uninstall detection' {
        $script:UninstallSrc | Should -Match '\$hasLauncher'
    }
}
