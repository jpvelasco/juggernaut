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
        $script:BedrockConfigPath = Join-Path $repoRoot 'bedrock-config.json'
    }

    It 'prints a calm inactive message when v2 is off' {
        $oldFlag = $env:JUGGERNAUT_USE_V2
        try {
            $env:JUGGERNAUT_USE_V2 = '0'
            $output = & (Join-Path $repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
            if ($output -notmatch 'Juggernaut v2 is not active. Use --v2 to enable v2 commands.') {
                throw "Expected inactive message, got: $output"
            }
        } finally {
            $env:JUGGERNAUT_USE_V2 = $oldFlag
        }
    }

    It 'shows both scopes and marks selected versus active' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-doctor-home-" + [Guid]::NewGuid().ToString('N'))
        $tmpWork = Join-Path ([IO.Path]::GetTempPath()) ("jug-doctor-work-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $tmpWork '.claude') -Force | Out-Null

        $oldHome = $env:HOME
        $oldHomeVar = $HOME
        $oldUserProfile = $env:USERPROFILE
        $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        $oldShell = $env:SHELL
        $oldAwsProfile = $env:AWS_PROFILE
        $oldLocation = (Get-Location).Path
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome
            $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = 'bash'
            $env:AWS_PROFILE = 'juggernaut-test'

            $userBlock = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                                             -UseMantle $false -ShellFallbackMode 'settings-only' `
                                             -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $userMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $userBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $userBlock)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $userMerged

            $projectBlock = New-JuggernautBlock -AuthMode 'iam' -Region 'eu-west-1' -Storage 'profile' `
                                                -UseMantle $false -ShellFallbackMode 'settings-only' `
                                                -Scope 'project' -BedrockConfigPath $script:BedrockConfigPath
            $projectMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $projectBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $projectBlock)
            Write-SettingsAtomic -Path (Join-Path $tmpWork '.claude/settings.json') -Content $projectMerged

            Set-Location $tmpWork
            $output = & (Join-Path $repoRoot 'commands\doctor.ps1') -Scope user 2>&1 | Out-String
            $text = (($output -replace '\\', '/') -replace "`r`n", "`n")

            foreach ($needle in @(
                'Session',
                'selected scope: user',
                'active scope: project scope is active for this working tree',
                'User Scope (selected)',
                'Project Scope (active)',
                'Settings',
                'Configuration',
                'Auth',
                'Drift',
                'region: us-west-2',
                'region: eu-west-1'
            )) {
                if ($text -notmatch [regex]::Escape($needle)) {
                    throw "Expected doctor output to contain '$needle', got: $output"
                }
            }
        } finally {
            Set-Location $oldLocation
            Set-Variable -Name HOME -Value $oldHomeVar -Scope Global -Force
            $env:HOME = $oldHome
            $env:USERPROFILE = $oldUserProfile
            $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            $env:SHELL = $oldShell
            $env:AWS_PROFILE = $oldAwsProfile
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
            Remove-Item -Path $tmpWork -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
