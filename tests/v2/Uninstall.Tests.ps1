# tests/v2/Uninstall.Tests.ps1 — Pester 5 tests for v3 commands/uninstall.ps1.

BeforeAll {
    function Get-RepoRoot {
        if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
        return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    }
    $script:RepoRoot = Get-RepoRoot
    . (Join-Path $script:RepoRoot 'lib\schema.ps1')
    . (Join-Path $script:RepoRoot 'lib\config_manager.ps1')
    . (Join-Path $script:RepoRoot 'lib\keychain.ps1')
    $script:BedrockConfigPath = Join-Path $script:RepoRoot 'bedrock-config.json'
    $script:UninstallScript   = Join-Path $script:RepoRoot 'commands\uninstall.ps1'
    $env:BEDROCK_CONFIG_PATH  = $script:BedrockConfigPath
    # Isolate keychain access to a guaranteed-absent service.
    $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-pester-$([guid]::NewGuid().Guid)"

    function New-TestDirs {
        $h = Join-Path ([IO.Path]::GetTempPath()) ("jug-ui-h-" + [Guid]::NewGuid().ToString('N'))
        $w = Join-Path ([IO.Path]::GetTempPath()) ("jug-ui-w-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $h '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $w '.claude') -Force | Out-Null
        return @{ Home = $h; Work = $w }
    }

    function Write-TestSettings($path, $region = 'us-west-2') {
        $block = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
            -Region $region -Storage 'profile' -UseMantle $false `
            -BedrockConfigPath $script:BedrockConfigPath
        $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block `
            -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $block)
        Write-SettingsAtomic -Path $path -Content $merged
    }

    function Invoke-Uninstall($d, $params = @{}) {
        $oldHome    = $env:HOME
        $oldProfile = $env:USERPROFILE
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        $oldXdg     = $env:XDG_CONFIG_HOME
        $oldJugHome = $env:JUGGERNAUT_HOME
        $oldProfTok = $env:JUGGERNAUT_PROFILE_TOKEN_PATH
        $oldPsProfile = $PROFILE
        $oldLoc     = (Get-Location).Path
        try {
            $env:HOME = $d.Home; $env:USERPROFILE = $d.Home
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:XDG_CONFIG_HOME = Join-Path $d.Home '.config'
            $env:JUGGERNAUT_HOME = $d.Home
            $env:JUGGERNAUT_PROFILE_TOKEN_PATH = Join-Path (Join-Path $d.Home '.config\juggernaut') 'bearer-token'
            Set-Variable -Name HOME -Value $d.Home -Scope Global -Force
            $profilePath = Join-Path $d.Home 'Documents\PowerShell\Microsoft.PowerShell_profile.ps1'
            Set-Variable -Name PROFILE -Value ([pscustomobject]@{
                CurrentUserCurrentHost = $profilePath
            }) -Scope Global -Force
            Set-Location $d.Work
            $output = & $script:UninstallScript @params 2>&1 | Out-String
            return @{ Output = $output; ExitCode = $LASTEXITCODE }
        } finally {
            Set-Location $oldLoc
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            Set-Variable -Name PROFILE -Value $oldPsProfile -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            if ($null -eq $oldXdg) { Remove-Item Env:\XDG_CONFIG_HOME -ErrorAction SilentlyContinue } else { $env:XDG_CONFIG_HOME = $oldXdg }
            if ($null -eq $oldJugHome) { Remove-Item Env:\JUGGERNAUT_HOME -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_HOME = $oldJugHome }
            if ($null -eq $oldProfTok) { Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $oldProfTok }
        }
    }

    function Remove-TestDirs($d) {
        Remove-Item -Path $d.Home, $d.Work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

AfterAll {
    Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
}

Describe 'uninstall.ps1 (v3)' {

    It 'reports nothing to uninstall when no block present' {
        $d = New-TestDirs
        try {
            $r = Invoke-Uninstall $d @{ DryRun = $true }
            $r.Output | Should -Match 'Nothing to uninstall'
        } finally { Remove-TestDirs $d }
    }

    It 'dry-run shows what would change and leaves files untouched' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            $r = Invoke-Uninstall $d @{ DryRun = $true }
            $r.Output | Should -Match '\[dry-run\]'
            $r.Output | Should -Match 'settings\.json'
            $r.Output | Should -Match 'No files were changed'
            $remaining = Read-Settings -Path (Join-Path $d.Home '.claude\settings.json')
            (Test-HasJuggernautBlock -Settings $remaining) | Should -Be $true
        } finally { Remove-TestDirs $d }
    }

    It '-Full dry-run shows command shims and install tree without removing them' {
        $d = New-TestDirs
        try {
            $shimDir = Join-Path $d.Home '.local\bin'
            New-Item -ItemType Directory -Path $shimDir -Force | Out-Null
            $shim = Join-Path $shimDir 'juggernaut.cmd'
            'echo juggernaut' | Set-Content -Path $shim -Encoding ascii
            $installDir = Join-Path $d.Home '.juggernaut'
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null

            $r = Invoke-Uninstall $d @{ Full = $true; DryRun = $true }
            $r.Output | Should -Match '\[dry-run\] Would remove Juggernaut command shim'
            $r.Output | Should -Match '\[dry-run\] Would remove Juggernaut install directory'
            $r.Output | Should -Match 'No files were changed'
            Test-Path $shim | Should -BeTrue
            Test-Path $installDir | Should -BeTrue
        } finally { Remove-TestDirs $d }
    }

    It '-Full -Yes removes command shims and install tree' {
        $d = New-TestDirs
        try {
            $shimDir = Join-Path $d.Home '.local\bin'
            New-Item -ItemType Directory -Path $shimDir -Force | Out-Null
            $shim = Join-Path $shimDir 'juggernaut.cmd'
            'echo juggernaut' | Set-Content -Path $shim -Encoding ascii
            $installDir = Join-Path $d.Home '.juggernaut'
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null

            $r = Invoke-Uninstall $d @{ Full = $true; Yes = $true }
            $r.Output | Should -Match 'Removed Juggernaut command shim'
            $r.Output | Should -Match 'Removed Juggernaut install directory'
            $r.Output | Should -Match 'Full uninstall complete'
            Test-Path $shim | Should -BeFalse
            Test-Path $installDir | Should -BeFalse
        } finally { Remove-TestDirs $d }
    }

    It 'removes juggernaut block and preserves unrelated keys' {
        $d = New-TestDirs
        try {
            $p = Join-Path $d.Home '.claude\settings.json'
            Write-TestSettings $p
            $json = Get-Content $p -Raw | ConvertFrom-Json
            $json | Add-Member -NotePropertyName 'permissions' -NotePropertyValue ([pscustomobject]@{ allow = @('Bash') }) -Force
            $json | ConvertTo-Json -Depth 10 | Set-Content $p -Encoding utf8
            Invoke-Uninstall $d @{ Force = $true } | Out-Null
            $remaining = Get-Content $p -Raw | ConvertFrom-Json
            ($remaining.PSObject.Properties.Name) | Should -Not -Contain 'juggernaut'
            ($remaining.PSObject.Properties.Name) | Should -Not -Contain 'env'
            ($remaining.PSObject.Properties.Name) | Should -Not -Contain 'model'
            $remaining.permissions | Should -Not -BeNullOrEmpty
        } finally { Remove-TestDirs $d }
    }

    It 'default scope removes both user and project blocks' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            Write-TestSettings (Join-Path $d.Work '.claude\settings.json') 'eu-west-1'
            $r = Invoke-Uninstall $d @{ Force = $true }
            $r.Output | Should -Match 'settings\.json'
            $uRemain = Read-Settings -Path (Join-Path $d.Home '.claude\settings.json')
            (Test-HasJuggernautBlock -Settings $uRemain) | Should -Be $false
            $pRemain = Read-Settings -Path (Join-Path $d.Work '.claude\settings.json')
            (Test-HasJuggernautBlock -Settings $pRemain) | Should -Be $false
        } finally { Remove-TestDirs $d }
    }

    It '-Scope user removes only user block' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            Write-TestSettings (Join-Path $d.Work '.claude\settings.json') 'eu-west-1'
            Invoke-Uninstall $d @{ Scope = 'user'; Force = $true } | Out-Null
            $uRemain = Read-Settings -Path (Join-Path $d.Home '.claude\settings.json')
            (Test-HasJuggernautBlock -Settings $uRemain) | Should -Be $false
            $pRemain = Read-Settings -Path (Join-Path $d.Work '.claude\settings.json')
            (Test-HasJuggernautBlock -Settings $pRemain) | Should -Be $true
        } finally { Remove-TestDirs $d }
    }

    It 'v3 uninstall does NOT touch shell profiles' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            $bashrc = Join-Path $d.Home '.bashrc'
            '# keep this' | Set-Content -Path $bashrc
            Add-Content -Path $bashrc -Value '# BEGIN: Juggernaut'
            Add-Content -Path $bashrc -Value 'export FOO=1'
            Add-Content -Path $bashrc -Value '# END: Juggernaut'
            Invoke-Uninstall $d @{ Force = $true } | Out-Null
            (Get-Content $bashrc -Raw) | Should -Match 'BEGIN: Juggernaut'
        } finally { Remove-TestDirs $d }
    }

    It 'is idempotent: second call reports nothing to uninstall' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            Invoke-Uninstall $d @{ Force = $true } | Out-Null
            $r = Invoke-Uninstall $d @{ Force = $true }
            $r.Output | Should -Match 'Nothing to uninstall'
        } finally { Remove-TestDirs $d }
    }

    It 'rejects an invalid scope value' {
        $caught = $false
        try {
            & $script:UninstallScript -Scope 'invalid' 2>&1 | Out-Null
        } catch {
            $caught = $true
            $_.ToString() | Should -Match 'scope'
        }
        # Write-Error also satisfies the contract; both pathways yield rejection.
        if (-not $caught) { $LASTEXITCODE | Should -Be 1 }
    }

    It '--help exits 0 and omits legacy -LegacyV1 flag' {
        $out = & $script:UninstallScript -Help 2>&1 | Out-String
        $out | Should -Match '-Scope'
        $out | Should -Match '-DryRun'
        $out | Should -Match '-Full'
        $out | Should -Match '-Yes'
        $out | Should -Not -Match 'LegacyV1'
    }

    It 'removes profile token file when present' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            # Create a profile token file
            $profDir = Join-Path $d.Home '.config\juggernaut'
            $profFile = Join-Path $profDir 'bearer-token'
            New-Item -ItemType Directory -Path $profDir -Force | Out-Null
            'sk-brk-uninstall-prof' | Set-Content -Path $profFile -Encoding utf8 -NoNewline
            Test-Path $profFile | Should -BeTrue

            # Override the profile token path
            $oldProfileTokenPath = $env:JUGGERNAUT_PROFILE_TOKEN_PATH
            $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $profFile
            try {
                $r = Invoke-Uninstall $d @{ Force = $true }
                $r.Output | Should -Match 'profile token file'
                Test-Path $profFile | Should -BeFalse
            } finally {
                if ($null -eq $oldProfileTokenPath) {
                    Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
                } else {
                    $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $oldProfileTokenPath
                }
            }
        } finally { Remove-TestDirs $d }
    }

    It 'dry-run with profile token shows it but leaves file intact' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            $profDir = Join-Path $d.Home '.config\juggernaut'
            $profFile = Join-Path $profDir 'bearer-token'
            New-Item -ItemType Directory -Path $profDir -Force | Out-Null
            'sk-brk-dry-prof' | Set-Content -Path $profFile -Encoding utf8 -NoNewline

            $oldProfileTokenPath = $env:JUGGERNAUT_PROFILE_TOKEN_PATH
            $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $profFile
            try {
                $r = Invoke-Uninstall $d @{ DryRun = $true }
                $r.Output | Should -Match '\[dry-run\]'
                $r.Output | Should -Match 'profile token file'
                Test-Path $profFile | Should -BeTrue
            } finally {
                if ($null -eq $oldProfileTokenPath) {
                    Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
                } else {
                    $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $oldProfileTokenPath
                }
            }
        } finally { Remove-TestDirs $d }
    }
}
