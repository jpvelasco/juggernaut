# tests/v2/UpgradeBanner.Tests.ps1 — Pester 5 tests for lib/upgrade_banner.ps1

BeforeAll {
    function Get-RepoRoot {
        if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
        return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    }
    $script:RepoRoot = Get-RepoRoot
    . (Join-Path $script:RepoRoot 'lib\schema.ps1')
    . (Join-Path $script:RepoRoot 'lib\config_manager.ps1')
    . (Join-Path $script:RepoRoot 'lib\migrator.ps1')
    . (Join-Path $script:RepoRoot 'lib\profile_paths.ps1')
    . (Join-Path $script:RepoRoot 'lib\upgrade_banner.ps1')
    $script:BedrockConfigPath = Join-Path $script:RepoRoot 'bedrock-config.json'
    $script:ReleaseVersion = (Get-Content (Join-Path $script:RepoRoot 'VERSION') -Raw -ErrorAction SilentlyContinue).Trim()

    function New-V1Profile($path) {
        $dir = Split-Path $path -Parent
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        @'
# BEGIN: Claude Code Bedrock Configuration
export AWS_REGION="us-west-2"
export CLAUDE_CODE_USE_BEDROCK="1"
# END: Claude Code Bedrock Configuration
'@ | Set-Content -Path $path -Encoding utf8
    }

    function New-V2FallbackProfile($path) {
        $dir = Split-Path $path -Parent
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        @'
# BEGIN: Claude Code Bedrock Configuration
# Juggernaut v2 shell fallback
$env:AWS_REGION = 'us-west-2'
# END: Claude Code Bedrock Configuration
'@ | Set-Content -Path $path -Encoding utf8
    }

    function New-V2Settings($path, $region = 'us-west-2') {
        $dir = Split-Path $path -Parent
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $block = New-JuggernautBlock -AuthMode 'iam' -Region $region -Storage 'profile' `
            -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' `
            -BedrockConfigPath $script:BedrockConfigPath
        $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block `
            -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $block)
        Write-SettingsAtomic -Path $path -Content $merged
    }
}

Describe 'Get-UpgradeBannerState' {

    It 'returns has_v1=false and has_v2_settings=false for a clean environment' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-clean-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.has_v1           | Should -BeFalse
            $state.has_v2_settings  | Should -BeFalse
            $state.v1_profiles      | Should -BeNullOrEmpty
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'returns has_v1=true when a bash profile has a v1 block' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-v1-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            New-V1Profile (Join-Path $tmpHome '.bashrc')
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.has_v1           | Should -BeTrue
            $state.has_v2_settings  | Should -BeFalse
            $state.v1_profiles.Count | Should -BeGreaterOrEqual 1
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'ignores marked v2 shell fallback blocks when detecting v1' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-v2fb-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $profilePath = Join-Path $tmpHome 'PowerShell\profile.ps1'
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $profilePath
            New-V2FallbackProfile $profilePath
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.has_v1      | Should -BeFalse
            $state.v1_profiles | Should -BeNullOrEmpty
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'returns has_v2_settings=true when settings.json is managed' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-v2-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            New-V2Settings (Join-Path $tmpHome '.claude\settings.json')
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.has_v1           | Should -BeFalse
            $state.has_v2_settings  | Should -BeTrue
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'MigrationDeclined marker suppresses has_v1' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-declined-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            New-V1Profile (Join-Path $tmpHome '.bashrc')
            Add-Content -Path (Join-Path $tmpHome '.bashrc') -Value "`n# MigrationDeclined: 2024-01-01"
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.has_v1 | Should -BeFalse
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'JUGGERNAUT_FORCE_MIGRATION_PROMPT=1 re-enables detection despite MigrationDeclined' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-force-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        $oldForce = $env:JUGGERNAUT_FORCE_MIGRATION_PROMPT
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            $env:JUGGERNAUT_FORCE_MIGRATION_PROMPT = '1'
            New-V1Profile (Join-Path $tmpHome '.bashrc')
            Add-Content -Path (Join-Path $tmpHome '.bashrc') -Value "`n# MigrationDeclined: 2024-01-01"
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.has_v1 | Should -BeTrue
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            if ($null -eq $oldForce) { Remove-Item Env:\JUGGERNAUT_FORCE_MIGRATION_PROMPT -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_FORCE_MIGRATION_PROMPT = $oldForce }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'is_upgrade=true when installed version differs from release' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-upgrade-" + [Guid]::NewGuid().ToString('N'))
        $tmpInstallDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-inst-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome,$tmpInstallDir -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldJugDir = $env:JUGGERNAUT_DIR
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            $env:JUGGERNAUT_DIR = $tmpInstallDir
            Set-Content -Path (Join-Path $tmpInstallDir 'VERSION') -Value '1.0.0' -Encoding ascii
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.is_upgrade          | Should -BeTrue
            $state.installed_version   | Should -Be '1.0.0'
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldJugDir) { Remove-Item Env:\JUGGERNAUT_DIR -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_DIR = $oldJugDir }
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome,$tmpInstallDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'is_upgrade=false when installed version matches release' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-noupgrade-" + [Guid]::NewGuid().ToString('N'))
        $tmpInstallDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-inst2-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpHome,$tmpInstallDir -Force | Out-Null
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldJugDir = $env:JUGGERNAUT_DIR
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = Join-Path $tmpHome 'nonexistent.ps1'
            $env:JUGGERNAUT_DIR = $tmpInstallDir
            Set-Content -Path (Join-Path $tmpInstallDir 'VERSION') -Value $script:ReleaseVersion -Encoding ascii
            $state = Get-UpgradeBannerState -SettingsPath (Join-Path $tmpHome '.claude\settings.json')
            $state.is_upgrade | Should -BeFalse
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldJugDir) { Remove-Item Env:\JUGGERNAUT_DIR -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_DIR = $oldJugDir }
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome,$tmpInstallDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Describe 'Write-UpgradeBanner' {

    It 'produces no output when state has no v1 and no upgrade' {
        $state = @{
            has_v1 = $false; v1_profiles = @(); has_v2_settings = $false
            installed_version = ''; release_version = 'test'; is_upgrade = $false
            migration_declined = $true
        }
        $output = & {
            $null = [Console]::Error
            $writer = [System.IO.StringWriter]::new()
            [Console]::SetError($writer)
            try { Write-UpgradeBanner -State $state }
            finally { [Console]::SetError([Console]::Out) }
            $writer.ToString()
        }
        # No meaningful banner output for clean state.
        $output.Trim() | Should -BeNullOrEmpty
    }

    It 'emits v1 banner text when has_v1=true' {
        $state = @{
            has_v1 = $true
            v1_profiles = @('/home/user/.bashrc')
            has_v2_settings = $false
            installed_version = ''; release_version = '2.3.0'; is_upgrade = $false
            migration_declined = $false
        }
        $captured = [System.Text.StringBuilder]::new()
        $oldErr = [Console]::Error
        $writer = [System.IO.StringWriter]::new($captured)
        [Console]::SetError($writer)
        try { Write-UpgradeBanner -State $state }
        finally { [Console]::SetError($oldErr) }
        $text = $captured.ToString()
        $text | Should -Match 'v1 configuration detected'
        $text | Should -Match '\.bashrc'
    }

    It 'emits version diff when is_upgrade=true' {
        $state = @{
            has_v1 = $false; v1_profiles = @(); has_v2_settings = $true
            installed_version = '2.2.5'; release_version = '2.3.0'; is_upgrade = $true
            migration_declined = $false
        }
        $captured = [System.Text.StringBuilder]::new()
        $oldErr = [Console]::Error
        $writer = [System.IO.StringWriter]::new($captured)
        [Console]::SetError($writer)
        try { Write-UpgradeBanner -State $state }
        finally { [Console]::SetError($oldErr) }
        $text = $captured.ToString()
        $text | Should -Match '2\.2\.5'
        $text | Should -Match '2\.3\.0'
    }
}

Describe 'Confirm-UpgradeBanner' {

    It 'returns proceed when has_v1=false' {
        $state = @{ has_v1 = $false; v1_profiles = @() }
        Confirm-UpgradeBanner -State $state | Should -Be 'proceed'
    }

    It 'returns legacy when LegacyV1=$true' {
        $state = @{ has_v1 = $true; v1_profiles = @('/tmp/x') }
        Confirm-UpgradeBanner -State $state -LegacyV1 $true | Should -Be 'legacy'
    }

    It 'returns proceed when Yes=$true' {
        $state = @{ has_v1 = $true; v1_profiles = @('/tmp/x') }
        Confirm-UpgradeBanner -State $state -Yes $true | Should -Be 'proceed'
    }

    It 'returns abort in non-interactive mode without -Yes or -LegacyV1' {
        $state = @{ has_v1 = $true; v1_profiles = @('/tmp/x') }
        $oldNoTty = $env:JUGGERNAUT_NO_TTY_PROMPTS
        try {
            $env:JUGGERNAUT_NO_TTY_PROMPTS = '1'
            Confirm-UpgradeBanner -State $state | Should -Be 'abort'
        } finally {
            if ($null -eq $oldNoTty) { Remove-Item Env:\JUGGERNAUT_NO_TTY_PROMPTS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_NO_TTY_PROMPTS = $oldNoTty }
        }
    }
}

Describe 'Test-UpgradeBannerSentinel / Set-UpgradeBannerSentinel' {

    It 'returns false before sentinel is created' {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-sent-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        try {
            Test-UpgradeBannerSentinel -InstallDir $tmpDir -Version '2.3.0' | Should -BeFalse
        } finally {
            Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'returns true after sentinel is set' {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-sent2-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        try {
            Set-UpgradeBannerSentinel -InstallDir $tmpDir -Version '2.3.0'
            Test-UpgradeBannerSentinel -InstallDir $tmpDir -Version '2.3.0' | Should -BeTrue
        } finally {
            Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'sentinel for one version does not affect a different version' {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-ub-sent3-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        try {
            Set-UpgradeBannerSentinel -InstallDir $tmpDir -Version '2.3.0'
            Test-UpgradeBannerSentinel -InstallDir $tmpDir -Version '2.4.0' | Should -BeFalse
        } finally {
            Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
