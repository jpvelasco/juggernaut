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
            $expected = @'
Juggernaut show

Current Juggernaut Block
  Scope: user
  Auth: iam
  Region: us-west-2
  Model: global.anthropic.claude-sonnet-4-6
  Effort: xhigh
  Opus Plan: disabled
  Mantle: disabled

Effective Config
  ~/.claude/settings.json
    Region: us-west-2
    Model: global.anthropic.claude-sonnet-4-6

Shell Fallback
  ~/.zshrc
    Present: yes
    Storage: keychain
'@
            $actualText = (($output -replace '\\', '/') -replace "`r`n", "`n").TrimEnd("`r", "`n")
            $expectedText = ($expected -replace "`r`n", "`n").TrimEnd("`r", "`n")
            if ($actualText -ne $expectedText) {
                throw "Expected show output to match the calm layout, got: $output"
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
}
