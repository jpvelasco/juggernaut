# tests/v2/Uninstall.Tests.ps1 - Pester 5 tests for commands/uninstall.ps1

BeforeAll {
    function Get-RepoRoot {
        if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
        return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    }
    $script:RepoRoot = Get-RepoRoot
    . (Join-Path $script:RepoRoot 'lib\schema.ps1')
    . (Join-Path $script:RepoRoot 'lib\config_manager.ps1')
    . (Join-Path $script:RepoRoot 'lib\keychain.ps1')
    . (Join-Path $script:RepoRoot 'lib\profile_writer.ps1')
    $script:BedrockConfigPath = Join-Path $script:RepoRoot 'bedrock-config.json'
    $script:UninstallScript   = Join-Path $script:RepoRoot 'commands\uninstall.ps1'
    $env:BEDROCK_CONFIG_PATH  = $script:BedrockConfigPath

    function New-TestDirs {
        $h = Join-Path ([IO.Path]::GetTempPath()) ("jug-ui-h-" + [Guid]::NewGuid().ToString('N'))
        $w = Join-Path ([IO.Path]::GetTempPath()) ("jug-ui-w-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $h '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $w '.claude') -Force | Out-Null
        return @{ Home = $h; Work = $w }
    }

    function Write-TestSettings($path, $region = 'us-west-2') {
        $block = New-JuggernautBlock -AuthMode 'iam' -Region $region -Storage 'profile' `
            -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' `
            -BedrockConfigPath $script:BedrockConfigPath
        $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block `
            -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $block)
        Write-SettingsAtomic -Path $path -Content $merged
    }

    function Invoke-Uninstall($d, $params = @{}) {
        $oldHome    = $env:HOME
        $oldProfile = $env:USERPROFILE
        $oldV2      = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        $oldLoc     = (Get-Location).Path
        try {
            $env:HOME = $d.Home; $env:USERPROFILE = $d.Home
            $env:JUGGERNAUT_USE_V2 = '1'
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $d.Home 'PowerShell\profile.ps1'
            Set-Variable -Name HOME -Value $d.Home -Scope Global -Force
            Set-Location $d.Work
            $output = & $script:UninstallScript @params 2>&1 | Out-String
            return @{ Output = $output; ExitCode = $LASTEXITCODE }
        } finally {
            Set-Location $oldLoc
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            $env:JUGGERNAUT_USE_V2 = $oldV2
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
        }
    }

    function Remove-TestDirs($d) {
        Remove-Item -Path $d.Home, $d.Work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Describe 'uninstall.ps1' {

    # ---------------------------------------------------------------------------
    # v2 gate
    # ---------------------------------------------------------------------------
    It 'exits 2 with safety error when JUGGERNAUT_USE_V2=0' {
        $oldV2 = $env:JUGGERNAUT_USE_V2
        try {
            $env:JUGGERNAUT_USE_V2 = '0'
            & $script:UninstallScript 2>&1 | Out-Null
            $LASTEXITCODE | Should -Be 2
        } finally {
            $env:JUGGERNAUT_USE_V2 = $oldV2
        }
    }

    # ---------------------------------------------------------------------------
    # nothing installed
    # ---------------------------------------------------------------------------
    It 'reports nothing to uninstall when no block present' {
        $d = New-TestDirs
        try {
            $r = Invoke-Uninstall $d @{ DryRun = $true }
            $r.Output | Should -Match 'Nothing to uninstall'
        } finally { Remove-TestDirs $d }
    }

    # ---------------------------------------------------------------------------
    # dry-run
    # ---------------------------------------------------------------------------
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

    # ---------------------------------------------------------------------------
    # real uninstall: removes block, preserves unrelated keys
    # ---------------------------------------------------------------------------
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

    # ---------------------------------------------------------------------------
    # default scope: both user and project
    # ---------------------------------------------------------------------------
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

    # ---------------------------------------------------------------------------
    # explicit scope
    # ---------------------------------------------------------------------------
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

    # ---------------------------------------------------------------------------
    # idempotent
    # ---------------------------------------------------------------------------
    It 'is idempotent: second call reports nothing to uninstall' {
        $d = New-TestDirs
        try {
            Write-TestSettings (Join-Path $d.Home '.claude\settings.json')
            Invoke-Uninstall $d @{ Force = $true } | Out-Null
            $r = Invoke-Uninstall $d @{ Force = $true }
            $r.Output | Should -Match 'Nothing to uninstall'
        } finally { Remove-TestDirs $d }
    }

    # ---------------------------------------------------------------------------
    # Windows PowerShell profiles
    # ---------------------------------------------------------------------------
    It 'removes Juggernaut blocks from PowerShell profile targets on Windows' {
        if ((Get-KeychainOS) -ne 'windows') {
            Set-ItResult -Skipped -Because 'PowerShell profile fallback is Windows-specific'
            return
        }

        $d = New-TestDirs
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $profilePath = Join-Path $d.Home 'PowerShell\profile.ps1'
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $profilePath
            $block = Build-ProfileWriterBlock -Shell 'powershell' -Region 'us-west-2' `
                -AuthMode 'iam' -ApiKeyExpr '' -StorageMode 'profile' `
                -BedrockConfigPath $script:BedrockConfigPath
            Write-ProfileWriterBlock -ProfileFile $profilePath -BlockContent $block

            Invoke-Uninstall $d @{ Force = $true } | Out-Null
            Get-Content $profilePath -Raw | Should -Not -Match 'BEGIN: Claude Code Bedrock Configuration'
        } finally {
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-TestDirs $d
        }
    }

    # ---------------------------------------------------------------------------
    # invalid scope
    # ---------------------------------------------------------------------------
    It 'rejects an invalid scope value' {
        $oldV2 = $env:JUGGERNAUT_USE_V2
        try {
            $env:JUGGERNAUT_USE_V2 = '1'
            $caught = $false
            try {
                & $script:UninstallScript -Scope 'invalid' 2>&1 | Out-Null
            } catch {
                $caught = $true
                $_.ToString() | Should -Match 'scope'
            }
            $caught | Should -Be $true
        } finally {
            $env:JUGGERNAUT_USE_V2 = $oldV2
        }
    }
}
