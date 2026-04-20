# tests/v2/Show.Tests.ps1 — Pester tests for commands/show.ps1.

Describe 'show.ps1' {
    BeforeAll {
        $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
        . (Join-Path $repoRoot 'lib\schema.ps1')
        . (Join-Path $repoRoot 'lib\config_manager.ps1')
        . (Join-Path $repoRoot 'lib\profile_writer.ps1')
    }

    It 'prints a calm inactive message when v2 is off' {
        $oldFlag = $env:JUGGERNAUT_USE_V2
        try {
            $env:JUGGERNAUT_USE_V2 = '0'
            $output = & (Join-Path $repoRoot 'commands\show.ps1') 2>&1 | Out-String
            $output | Should Match 'Juggernaut v2 is not active. Use --v2 to enable v2 commands.'
        } finally {
            $env:JUGGERNAUT_USE_V2 = $oldFlag
        }
    }

    It 'prints the calm section headers and values' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-show-" + [Guid]::NewGuid().ToString('N'))
        $projectRoot = Join-Path $tmpHome 'project'
        $projectWork = Join-Path $projectRoot 'work\inner'
        New-Item -ItemType Directory -Path $projectWork -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $tmpHome '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $projectRoot '.claude') -Force | Out-Null

        $oldUserProfile = $env:USERPROFILE
        $oldFlag = $env:JUGGERNAUT_USE_V2
        $oldBedrock = $env:BEDROCK_CONFIG_PATH
        try {
            $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_USE_V2 = '1'
            $env:BEDROCK_CONFIG_PATH = Join-Path $repoRoot 'bedrock-config.json'

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

            Push-Location $projectWork
            try {
                $output = & (Join-Path $repoRoot 'commands\show.ps1') 2>&1 | Out-String
                $output | Should Match 'Juggernaut show'
                $output | Should Match 'Current Juggernaut Block'
                $output | Should Match 'Scope:'
                $output | Should Match 'Auth:'
                $output | Should Match 'Region:'
                $output | Should Match 'Model:'
                $output | Should Match 'Effort:'
                $output | Should Match 'Opus Plan:'
                $output | Should Match 'Mantle:'
                $output | Should Match 'Effective Config'
                $output | Should Match 'Shell Fallback'
                $output | Should Match 'Present:'
                $output | Should Match 'Storage:'
            } finally {
                Pop-Location
            }
        } finally {
            $env:USERPROFILE = $oldUserProfile
            $env:JUGGERNAUT_USE_V2 = $oldFlag
            $env:BEDROCK_CONFIG_PATH = $oldBedrock
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
