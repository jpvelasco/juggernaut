# tests/v2/Show.Tests.ps1 — Pester tests for commands/show.ps1.

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib\schema.ps1')
    . (Join-Path $repoRoot 'lib\config_manager.ps1')
    $script:RepoRoot = $repoRoot
    $script:OldHome = $env:HOME
    $env:JUGGERNAUT_USE_V2 = '1'
    $env:BEDROCK_CONFIG_PATH = Join-Path $repoRoot 'bedrock-config.json'
}

AfterAll {
    $env:HOME = $script:OldHome
}

Describe 'show.ps1 v2 gate' {
    It 'prints an inactive message when the flag is absent' {
        $env:JUGGERNAUT_USE_V2 = '0'
        $output = & (Join-Path $script:RepoRoot 'commands\show.ps1') 2>&1 | Out-String
        $output | Should -Match 'v2 is not active'
        $env:JUGGERNAUT_USE_V2 = '1'
    }
}

Describe 'show.ps1 output' {
    BeforeAll {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-" + [Guid]::NewGuid().ToString('N'))
        $projectRoot = Join-Path $tmpHome 'project'
        $projectWork = Join-Path $projectRoot 'work\inner'
        New-Item -ItemType Directory -Path $projectWork -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $projectRoot '.claude') -Force | Out-Null
        $env:HOME = $tmpHome

        $userBlock = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                                         -EffortLevel 'high' -UseMantle $false -OpusPlan $false `
                                         -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
        $userMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $userBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $userBlock)
        Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $userMerged

        $projectBlock = New-JuggernautBlock -AuthMode 'api-key' -Region 'us-east-1' -Storage 'keychain' `
                                            -EffortLevel 'xhigh' -UseMantle $true -MantleBaseUrl 'https://mantle.example.com' `
                                            -OpusPlan $false -Scope 'project' -BedrockConfigPath $env:BEDROCK_CONFIG_PATH
        $projectMerged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $projectBlock -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $projectBlock)
        Write-SettingsAtomic -Path (Join-Path $projectRoot '.claude/settings.json') -Content $projectMerged

        $script:ShowCwd = $projectWork
        $script:ShowTmpHome = $tmpHome
    }

    AfterAll {
        Remove-Item -Path $script:ShowTmpHome -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'prints the main sections' {
        Push-Location $script:ShowCwd
        try {
            $output = & (Join-Path $script:RepoRoot 'commands\show.ps1') 2>&1 | Out-String
            $output | Should -Match 'Juggernaut show'
            $output | Should -Match 'Current block'
            $output | Should -Match 'Effective config'
            $output | Should -Match 'User'
            $output | Should -Match 'Project'
            $output | Should -Match 'Mantle'
        } finally {
            Pop-Location
        }
    }
}
