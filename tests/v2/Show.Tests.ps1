# tests/v2/Show.Tests.ps1 — Pester 5 tests for v3 commands/show.ps1.

Describe 'show.ps1 (v3)' {
    BeforeAll {
        function Get-RepoRoot {
            if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
            return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
        }
        $script:repoRoot = Get-RepoRoot
        . (Join-Path $script:repoRoot 'lib\schema.ps1')
        . (Join-Path $script:repoRoot 'lib\config_manager.ps1')
        $script:BedrockConfigPath = Join-Path $script:repoRoot 'bedrock-config.json'
        $script:ExpectedVersion   = (Get-Content (Join-Path $script:repoRoot 'VERSION') -Raw).Trim()
    }

    Context 'IAM + user scope — human-readable output' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-iam-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH; $script:oldShell = $env:SHELL

            Set-Variable -Name HOME -Value $script:tmpHome -Scope Global -Force
            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:SHELL = 'zsh'

            $block = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'keychain' -EffortLevel 'xhigh' `
                -UseMantle $false -OpusPlan $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
            Write-SettingsAtomic -Path (Join-Path $script:tmpHome '.claude\settings.json') -Content $merged

            $script:output = & (Join-Path $script:repoRoot 'commands\show.ps1') 2>&1 | Out-String
            $script:text   = (($script:output -replace '\\', '/') -replace "`r`n", "`n")
        }
        AfterAll {
            Set-Variable -Name HOME -Value $script:oldHome -Scope Global -Force
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock; $env:SHELL = $script:oldShell
            Remove-Item -Path $script:tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }

        It 'shows Scope Awareness header'       { $script:text | Should -Match 'Scope Awareness' }
        It 'marks user scope active'            { $script:text | Should -Match 'Active Scope: user takes precedence for this session' }
        It 'labels user section (active)'       { $script:text | Should -Match 'User Scope \(active\)' }
        It 'reports IAM auth mode'              { $script:text | Should -Match 'Auth: IAM' }
        It 'reports region us-west-2'           { $script:text | Should -Match 'Region: us-west-2' }
        It 'shows empty project section'        { $script:text | Should -Match 'Project Scope' }
        It 'project: No Juggernaut block'       { $script:text | Should -Match 'Status: No Juggernaut block' }
        It 'does NOT print a Shell Fallback section (v3)' {
            $script:text | Should -Not -Match 'Shell Fallback'
        }
    }

    Context 'Bedrock API-key + opusplan + Mantle' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-api-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH

            Set-Variable -Name HOME -Value $script:tmpHome -Scope Global -Force
            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath

            $block = New-JuggernautBlock -AuthMode 'bedrock-api-key' -AuthValidated $true `
                -Region 'eu-west-1' -Storage 'keychain' -EffortLevel 'xhigh' `
                -UseMantle $true -OpusPlan $true `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
            Write-SettingsAtomic -Path (Join-Path $script:tmpHome '.claude\settings.json') -Content $merged

            $script:output = & (Join-Path $script:repoRoot 'commands\show.ps1') 2>&1 | Out-String
            $script:text   = (($script:output -replace '\\', '/') -replace "`r`n", "`n")
        }
        AfterAll {
            Set-Variable -Name HOME -Value $script:oldHome -Scope Global -Force
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock
            Remove-Item -Path $script:tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }

        It 'marks user scope active'         { $script:text | Should -Match 'User Scope \(active\)' }
        It 'reports Bedrock API key auth'    { $script:text | Should -Match 'Auth: Bedrock API key' }
        It 'reports region eu-west-1'        { $script:text | Should -Match 'Region: eu-west-1' }
        It 'reports Opus Plan enabled'       { $script:text | Should -Match 'Opus Plan: enabled' }
        It 'reports Mantle enabled'          { $script:text | Should -Match 'Mantle: enabled' }
    }

    Context 'both scopes — --Scope user shows selected hint while project stays active' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-two-" + [Guid]::NewGuid().ToString('N'))
            $script:tmpWork = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-two-work-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            New-Item -ItemType Directory -Path (Join-Path $script:tmpWork '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH
            $script:oldLocation = (Get-Location).Path

            Set-Variable -Name HOME -Value $script:tmpHome -Scope Global -Force
            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath

            $userBlock = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'profile' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $userMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $userBlock `
                -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $userBlock)
            Write-SettingsAtomic -Path (Join-Path $script:tmpHome '.claude\settings.json') -Content $userMerged

            $projectBlock = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
                -Region 'ap-southeast-1' -Storage 'profile' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $projectMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $projectBlock `
                -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $projectBlock)
            Write-SettingsAtomic -Path (Join-Path $script:tmpWork '.claude\settings.json') -Content $projectMerged

            Set-Location $script:tmpWork
            $script:output = & (Join-Path $script:repoRoot 'commands\show.ps1') -Scope user 2>&1 | Out-String
            $script:text   = (($script:output -replace '\\', '/') -replace "`r`n", "`n")
        }
        AfterAll {
            Set-Location $script:oldLocation
            Set-Variable -Name HOME -Value $script:oldHome -Scope Global -Force
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock
            Remove-Item -Path $script:tmpHome,$script:tmpWork -Recurse -Force -ErrorAction SilentlyContinue
        }

        It 'labels the selected scope'          { $script:text | Should -Match 'Selected Scope: user' }
        It 'reports project as the active scope' { $script:text | Should -Match 'Active Scope: project takes precedence for this session' }
        It 'labels user section (selected)'     { $script:text | Should -Match 'User Scope \(selected\)' }
        It 'labels project section (active)'    { $script:text | Should -Match 'Project Scope \(active\)' }
        It 'shows user region'                  { $script:text | Should -Match 'Region: us-west-2' }
        It 'shows project region'               { $script:text | Should -Match 'Region: ap-southeast-1' }
    }

    Context 'help and unknown flags' {
        It '--help exits cleanly and does not mention legacy flags' {
            $out = & (Join-Path $script:repoRoot 'commands\show.ps1') --help 2>&1 | Out-String
            $out | Should -Match '-Scope'
            $out | Should -Not -Match 'LegacyV1'
            $out | Should -Not -Match 'JUGGERNAUT_USE_V2'
            $out | Should -Not -Match 'Shell Fallback'
        }
    }
}
