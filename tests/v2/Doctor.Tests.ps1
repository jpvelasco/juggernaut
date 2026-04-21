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
}
