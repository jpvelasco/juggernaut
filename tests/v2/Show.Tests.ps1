# tests/v2/Show.Tests.ps1 — Pester tests for commands/show.ps1.

Describe 'show.ps1' {
    BeforeAll {
        function Get-RepoRoot {
            if ($env:GITHUB_WORKSPACE) {
                return $env:GITHUB_WORKSPACE
            }
            return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
        }

        $repoRoot = Get-RepoRoot
        . (Join-Path $repoRoot 'lib\schema.ps1')
        . (Join-Path $repoRoot 'lib\config_manager.ps1')
        . (Join-Path $repoRoot 'lib\profile_writer.ps1')
    }

    It 'prints a calm inactive message when v2 is off' {
        $oldFlag = $env:JUGGERNAUT_USE_V2
        try {
            $env:JUGGERNAUT_USE_V2 = '0'
            $output = & (Join-Path $repoRoot 'commands\show.ps1') 2>&1 | Out-String
            if ($output -notmatch 'Juggernaut v2 is not active. Use --v2 to enable v2 commands.') {
                throw "Expected inactive message, got: $output"
            }
        } finally {
            $env:JUGGERNAUT_USE_V2 = $oldFlag
        }
    }

    It 'prints the calm section headers and values' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null

        $oldHome = $env:HOME
        $oldHomeVar = $HOME
        $oldUserProfile = $env:USERPROFILE
        $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        $oldShell = $env:SHELL
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome
            $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'
            $env:BEDROCK_CONFIG_PATH = Join-Path $repoRoot 'bedrock-config.json'
            $env:SHELL = 'zsh'

            $userBlock = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'keychain' `
                                             -EffortLevel 'xhigh' -UseMantle $false -OpusPlan $false `
                                             -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
            $userMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $userBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $userBlock)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $userMerged

            $output = & (Join-Path $repoRoot 'commands\show.ps1') 2>&1 | Out-String
            $actualText = (($output -replace '\\', '/') -replace "`r`n", "`n").TrimEnd("`r", "`n")
            foreach ($needle in @(
                'Scope Awareness',
                'Active Scope: user takes precedence for this session',
                'User Scope (active)',
                'Scope: user',
                'Auth: iam',
                'Region: us-west-2',
                'Project Scope',
                'Status: No Juggernaut block',
                'Shell Fallback',
                'Present: yes',
                'Storage: keychain'
            )) {
                if ($actualText -notmatch [regex]::Escape($needle)) {
                    throw "Expected show output to contain '$needle', got: $output"
                }
            }
        } finally {
            Set-Variable -Name HOME -Value $oldHomeVar -Scope Global -Force
            $env:HOME = $oldHome
            $env:USERPROFILE = $oldUserProfile
            $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            $env:SHELL = $oldShell
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'prints disabled shell fallback without storage' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null

        $oldHome = $env:HOME
        $oldHomeVar = $HOME
        $oldUserProfile = $env:USERPROFILE
        $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        $oldShell = $env:SHELL
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome
            $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'
            $env:BEDROCK_CONFIG_PATH = Join-Path $repoRoot 'bedrock-config.json'
            $env:SHELL = 'bash'

            $userBlock = New-JuggernautBlock -AuthMode 'api-key' -Region 'eu-west-1' -Storage 'keychain' `
                                             -EffortLevel 'xhigh' -UseMantle $true -OpusPlan $true `
                                             -ShellFallbackMode 'settings-only' `
                                             -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
            $userMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $userBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $userBlock)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $userMerged

            $output = & (Join-Path $repoRoot 'commands\show.ps1') 2>&1 | Out-String
            $actualText = (($output -replace '\\', '/') -replace "`r`n", "`n").TrimEnd("`r", "`n")
            foreach ($needle in @(
                'User Scope (active)',
                'Auth: Bedrock API key',
                'Region: eu-west-1',
                'Opus Plan: enabled',
                'Mantle: enabled',
                'Shell Fallback',
                'Present: no'
            )) {
                if ($actualText -notmatch [regex]::Escape($needle)) {
                    throw "Expected show output to contain '$needle', got: $output"
                }
            }
            if ($actualText -match [regex]::Escape('Storage: keychain')) {
                throw "Expected disabled shell fallback output to omit storage, got: $output"
            }
        } finally {
            Set-Variable -Name HOME -Value $oldHomeVar -Scope Global -Force
            $env:HOME = $oldHome
            $env:USERPROFILE = $oldUserProfile
            $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            $env:SHELL = $oldShell
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    It 'shows both scopes and selected scope' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-home-" + [Guid]::NewGuid().ToString('N'))
        $tmpWork = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-work-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $tmpWork '.claude') -Force | Out-Null

        $oldHome = $env:HOME
        $oldHomeVar = $HOME
        $oldUserProfile = $env:USERPROFILE
        $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        $oldShell = $env:SHELL
        $oldLocation = (Get-Location).Path
        try {
            Set-Variable -Name HOME -Value $tmpHome -Scope Global -Force
            $env:HOME = $tmpHome
            $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'
            $env:BEDROCK_CONFIG_PATH = Join-Path $repoRoot 'bedrock-config.json'
            $env:SHELL = 'bash'

            $userBlock = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                                             -UseMantle $false -ShellFallbackMode 'settings-only' `
                                             -Scope 'user' -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
            $userMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $userBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $userBlock)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $userMerged

            $projectBlock = New-JuggernautBlock -AuthMode 'iam' -Region 'ap-southeast-1' -Storage 'profile' `
                                                -UseMantle $false -ShellFallbackMode 'settings-only' `
                                                -Scope 'project' -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
            $projectMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $projectBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $projectBlock)
            Write-SettingsAtomic -Path (Join-Path $tmpWork '.claude/settings.json') -Content $projectMerged

            Set-Location $tmpWork
            $output = & (Join-Path $repoRoot 'commands\show.ps1') -Scope user 2>&1 | Out-String
            $actualText = (($output -replace '\\', '/') -replace "`r`n", "`n")
            foreach ($needle in @(
                'Selected Scope: user',
                'Active Scope: project takes precedence for this session',
                'User Scope (selected)',
                'Project Scope (active)',
                'Region: us-west-2',
                'Region: ap-southeast-1'
            )) {
                if ($actualText -notmatch [regex]::Escape($needle)) {
                    throw "Expected show output to contain '$needle', got: $output"
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
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
            Remove-Item -Path $tmpWork -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
