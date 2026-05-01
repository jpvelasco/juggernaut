# tests/v2/Doctor.Tests.ps1 - Pester tests for commands/doctor.ps1.

Describe 'doctor.ps1' {
    BeforeAll {
        function Get-RepoRoot {
            if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
            return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
        }

        $repoRoot = Get-RepoRoot
        . (Join-Path $repoRoot 'lib\schema.ps1')
        . (Join-Path $repoRoot 'lib\config_manager.ps1')
        . (Join-Path $repoRoot 'lib\profile_writer.ps1')
        $script:BedrockConfigPath = Join-Path $repoRoot 'bedrock-config.json'
    }

    It 'exits 2 with safety error when JUGGERNAUT_USE_V2=0' {
        $oldFlag = $env:JUGGERNAUT_USE_V2
        try {
            $env:JUGGERNAUT_USE_V2 = '0'
            & (Join-Path $repoRoot 'commands\doctor.ps1') 2>&1 | Out-Null
            $LASTEXITCODE | Should -Be 2
        } finally {
            $env:JUGGERNAUT_USE_V2 = $oldFlag
        }
    }

    It 'loads and reports missing keychain credentials under Windows PowerShell' {
        if (-not (Get-Command powershell -ErrorAction SilentlyContinue)) {
            Set-ItResult -Skipped -Because 'Windows PowerShell is not available'
            return
        }

        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-winps-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null

        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH; $oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
        $oldKeychainService = $env:JUGGERNAUT_KEYCHAIN_SERVICE
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'; $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-test-absent-$([Guid]::NewGuid().ToString('N'))"
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $ub = New-JuggernautBlock -AuthMode 'bedrock-api-key' -Region 'us-west-2' -Storage 'keychain' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $ub.shellFallback.lastWrittenProfiles = @((Join-Path $tmpHome 'missing-profile.ps1'))
            $um = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $ub -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $ub)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $um

            $output = & powershell -NoProfile -File (Join-Path $repoRoot 'commands\doctor.ps1') --v2 -Scope user 2>&1 | Out-String
            if ($output -match 'ParserError' -or $output -match "The term 'try' is not recognized") {
                throw "Expected Windows PowerShell to parse and run doctor, got: $output"
            }
            if ($output -notmatch [regex]::Escape('Details: no API key found in env, keychain, or shell profile')) {
                throw "Expected missing API key report, got: $output"
            }
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile; $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            if ($null -eq $oldBearer) { Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue }
            else { $env:AWS_BEARER_TOKEN_BEDROCK = $oldBearer }
            if ($null -eq $oldKeychainService) { Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_KEYCHAIN_SERVICE = $oldKeychainService }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'dispatcher returns success for successful doctor despite stale native exit code' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-dispatch-" + [Guid]::NewGuid().ToString('N'))
        $tmpWork = Join-Path $tmpHome 'work'
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path $tmpWork -Force | Out-Null

        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH; $oldShell = $env:SHELL
        $oldAwsProfile = $env:AWS_PROFILE; $oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
        $oldLocation = (Get-Location).Path
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '0'; $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = 'bash'; $env:AWS_PROFILE = 'juggernaut-test'
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $block = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $block)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $merged

            git --not-a-real-git-option *> $null
            Set-Location $tmpWork
            $output = & (Join-Path $repoRoot 'juggernaut.ps1') doctor --v2 2>&1 | Out-String
            $LASTEXITCODE | Should -Be 0
            $output | Should -Match ([regex]::Escape('No issues found'))
        } finally {
            Set-Location $oldLocation
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile; $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock; $env:SHELL = $oldShell; $env:AWS_PROFILE = $oldAwsProfile
            if ($null -eq $oldBearer) { Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue }
            else { $env:AWS_BEARER_TOKEN_BEDROCK = $oldBearer }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'shows section headers and active scope' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-h-" + [Guid]::NewGuid().ToString('N'))
        $tmpWork = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-w-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $tmpWork '.claude') -Force | Out-Null

        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH; $oldShell = $env:SHELL
        $oldAwsProfile = $env:AWS_PROFILE; $oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
        $oldLocation = (Get-Location).Path
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'; $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = 'bash'; $env:AWS_PROFILE = 'juggernaut-test'
            Remove-Item Env:AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $ub = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $um = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $ub -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $ub)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $um

            $pb = New-JuggernautBlock -AuthMode 'iam' -Region 'eu-west-1' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'project' -BedrockConfigPath $script:BedrockConfigPath
            $pm = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $pb -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $pb)
            Write-SettingsAtomic -Path (Join-Path $tmpWork '.claude/settings.json') -Content $pm

            Set-Location $tmpWork
            $output = & (Join-Path $repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
            $text = ($output -replace "`r`n", "`n") -replace '\\', '/'

            foreach ($needle in @(
                'User Scope', 'Project Scope', 'Active Scope',
                'Credentials', 'Region & Models', 'Mantle', 'Drift', 'Summary',
                'Status: OK', 'No issues found',
                'Region: eu-west-1 (OK)'
            )) {
                if ($text -notmatch [regex]::Escape($needle)) {
                    throw "Expected '$needle' in output, got: $output"
                }
            }
        } finally {
            Set-Location $oldLocation
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile; $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock; $env:SHELL = $oldShell; $env:AWS_PROFILE = $oldAwsProfile
            if ($oldBearer) { $env:AWS_BEARER_TOKEN_BEDROCK = $oldBearer }
            Remove-Item -Path $tmpHome,$tmpWork -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'honours --scope flag for detail sections' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-s-" + [Guid]::NewGuid().ToString('N'))
        $tmpWork = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-sw-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $tmpWork '.claude') -Force | Out-Null

        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH; $oldShell = $env:SHELL
        $oldAwsProfile = $env:AWS_PROFILE; $oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
        $oldLocation = (Get-Location).Path
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'; $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = 'bash'; $env:AWS_PROFILE = 'juggernaut-test'
            Remove-Item Env:AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $ub = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $um = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $ub -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $ub)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $um

            $pb = New-JuggernautBlock -AuthMode 'iam' -Region 'eu-west-1' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'project' -BedrockConfigPath $script:BedrockConfigPath
            $pm = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $pb -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $pb)
            Write-SettingsAtomic -Path (Join-Path $tmpWork '.claude/settings.json') -Content $pm

            Set-Location $tmpWork
            $output = & (Join-Path $repoRoot 'commands\doctor.ps1') -Scope user 2>&1 | Out-String
            if ($output -notmatch [regex]::Escape('Region: us-west-2 (OK)')) {
                throw "Expected user region us-west-2 when -Scope user, got: $output"
            }
        } finally {
            Set-Location $oldLocation
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile; $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock; $env:SHELL = $oldShell; $env:AWS_PROFILE = $oldAwsProfile
            if ($oldBearer) { $env:AWS_BEARER_TOKEN_BEDROCK = $oldBearer }
            Remove-Item -Path $tmpHome,$tmpWork -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'does not treat home settings as project settings when running from home' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-home-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null

        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH; $oldShell = $env:SHELL
        $oldAwsProfile = $env:AWS_PROFILE; $oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
        $oldLocation = (Get-Location).Path
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'; $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = 'bash'; $env:AWS_PROFILE = 'juggernaut-test'
            Remove-Item Env:AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $ub = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $um = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $ub -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $ub)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $um

            Set-Location $tmpHome
            $output = & (Join-Path $repoRoot 'commands\doctor.ps1') -Scope user 2>&1 | Out-String
            $text = ($output -replace "`r`n", "`n") -replace '\\', '/'

            if ($text -notmatch [regex]::Escape('Active Scope') -or
                $text -notmatch [regex]::Escape("Active Scope`nuser")) {
                throw "Expected user active scope when running from home, got: $output"
            }
            if ($text -notmatch [regex]::Escape('Project Scope') -or
                $text -notmatch [regex]::Escape('Status: not found')) {
                throw "Expected missing project scope when running from home, got: $output"
            }
        } finally {
            Set-Location $oldLocation
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile; $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock; $env:SHELL = $oldShell; $env:AWS_PROFILE = $oldAwsProfile
            if ($null -eq $oldBearer) { Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue }
            else { $env:AWS_BEARER_TOKEN_BEDROCK = $oldBearer }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'uses recorded PowerShell profile paths for fallback drift on Windows' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-dr-psprofile-" + [Guid]::NewGuid().ToString('N'))
        $profilePath = Join-Path $tmpHome 'WindowsPowerShell\profile.ps1'
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Split-Path $profilePath -Parent) -Force | Out-Null

        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH; $oldShell = $env:SHELL
        $oldAwsProfile = $env:AWS_PROFILE; $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'; $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = ''; $env:AWS_PROFILE = 'juggernaut-test'
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $profilePath

            $ub = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $true -ShellFallbackMode 'both' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $ub.shellFallback.lastWrittenProfiles = @($profilePath)
            $um = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $ub -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $ub)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $um

            $profileBlock = Build-ProfileWriterBlock -Shell 'powershell' -Region 'us-west-2' -AuthMode 'iam' `
                -StorageMode 'profile' -BedrockConfigPath $script:BedrockConfigPath -UseMantle $true `
                -EffortLevel 'xhigh'
            Write-ProfileWriterBlock -ProfileFile $profilePath -Block $profileBlock

            $output = & (Join-Path $repoRoot 'commands\doctor.ps1') -Scope user 2>&1 | Out-String
            if ($output -notmatch [regex]::Escape('Settings vs Shell Fallback: OK (no drift detected)')) {
                throw "Expected PowerShell fallback drift OK, got: $output"
            }
        } finally {
            Set-Variable -Name HOME -Value $oldHome -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile; $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock; $env:SHELL = $oldShell; $env:AWS_PROFILE = $oldAwsProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
