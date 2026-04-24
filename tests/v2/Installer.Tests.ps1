# tests/v2/Installer.Tests.ps1 - static acceptance checks for installer robustness.

BeforeAll {
    $script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    $script:InstallSh = Get-Content (Join-Path $script:RepoRoot 'install.sh') -Raw
    $script:InstallPs1 = Get-Content (Join-Path $script:RepoRoot 'install.ps1') -Raw
    . (Join-Path $script:RepoRoot 'lib\schema.ps1')
    . (Join-Path $script:RepoRoot 'lib\config_manager.ps1')
    $script:BedrockConfigPath = Join-Path $script:RepoRoot 'bedrock-config.json'
}

Describe 'install.sh robustness' {
    It 'repairs executable bits for v2 entrypoints and libraries' {
        foreach ($needle in @('chmod +x', 'commands/*.sh', 'lib/*.sh', 'juggernaut', 'setup')) {
            $script:InstallSh | Should -Match ([regex]::Escape($needle))
        }
    }

    It 'creates a user-local juggernaut launcher and prints verification guidance' {
        $script:InstallSh | Should -Match ([regex]::Escape('.local/bin'))
        $script:InstallSh | Should -Match 'ln -sfn'
        $script:InstallSh | Should -Match 'juggernaut doctor --v2'
    }
}

Describe 'install.ps1 robustness' {
    It 'creates PowerShell and cmd shims' {
        $script:InstallPs1 | Should -Match ([regex]::Escape('.local\bin'))
        $script:InstallPs1 | Should -Match 'juggernaut\.ps1'
        $script:InstallPs1 | Should -Match 'juggernaut\.cmd'
    }

    It 'prints PATH and execution-policy guidance' {
        $script:InstallPs1 | Should -Match 'PATH'
        $script:InstallPs1 | Should -Match ([regex]::Escape('If PowerShell blocks first run scripts, run:'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'))
        $script:InstallPs1 | Should -Match 'juggernaut doctor --v2'
    }

    It 'can run doctor through the launcher entrypoint after a clean install' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-inst-h-" + [Guid]::NewGuid().ToString('N'))
        $tmpWork = Join-Path ([IO.Path]::GetTempPath()) ("jug-inst-w-" + [Guid]::NewGuid().ToString('N'))
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
            Remove-Item Env:AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $block = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' -BedrockConfigPath $script:BedrockConfigPath
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block `
                -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $block)
            Write-SettingsAtomic -Path (Join-Path $tmpHome '.claude/settings.json') -Content $merged

            Set-Location $tmpWork
            $output = & (Join-Path $script:RepoRoot 'juggernaut.ps1') doctor --v2 2>&1 | Out-String
            $text = $output -replace "`r`n", "`n"
            $LASTEXITCODE | Should -Be 0
            $text | Should -Match ([regex]::Escape('Status: OK'))
            $text | Should -Match ([regex]::Escape('No issues found'))
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
