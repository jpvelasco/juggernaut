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
        $script:InstallSh | Should -Match 'juggernaut doctor'
    }

    It 'does not run setup by default' {
        $script:InstallSh | Should -Match ([regex]::Escape('--configure'))
        $script:InstallSh | Should -Match ([regex]::Escape('./juggernaut apply'))
        $script:InstallSh | Should -Not -Match ([regex]::Escape('exec bash ./setup'))
    }

    It 'can install an explicit branch or ref for PR testing' {
        $script:InstallSh | Should -Match ([regex]::Escape('--ref'))
        $script:InstallSh | Should -Match ([regex]::Escape('JUGGERNAUT_REF'))
        $script:InstallSh | Should -Match ([regex]::Escape('git clone --branch "$REF"'))
    }

    It 'backs up a dirty existing install before cloning a fresh copy' {
        foreach ($needle in @('JUGGERNAUT_REPO_URL', 'install_tree_dirty', 'backup_existing_install', '.backup.', 'Backup created:')) {
            $script:InstallSh | Should -Match ([regex]::Escape($needle))
        }
    }
}

Describe 'install.ps1 robustness' {
    It 'creates PowerShell and cmd shims' {
        $script:InstallPs1 | Should -Match ([regex]::Escape('.local\bin'))
        $script:InstallPs1 | Should -Match 'juggernaut\.ps1'
        $script:InstallPs1 | Should -Match 'juggernaut\.cmd'
        $script:InstallPs1 | Should -Not -Match ([regex]::Escape('exit `$LASTEXITCODE'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('exit /b %ERRORLEVEL%'))
    }

    It 'prints PATH and execution-policy guidance' {
        $script:InstallPs1 | Should -Match 'PATH'
        $script:InstallPs1 | Should -Match ([regex]::Escape('If PowerShell blocks first run scripts, run:'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'))
        $script:InstallPs1 | Should -Match 'juggernaut doctor'
    }

    It 'does not run setup by default' {
        $script:InstallPs1 | Should -Match ([regex]::Escape('[switch]$Configure'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('Convert-GnuStyleArgs'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('commands\apply.ps1'))
        $script:InstallPs1 | Should -Not -Match ([regex]::Escape('juggernaut.ps1 apply --v2'))
        $script:InstallPs1 | Should -Not -Match ([regex]::Escape('exit $LASTEXITCODE'))
        $script:InstallPs1 | Should -Not -Match ([regex]::Escape('setup-claude-bedrock.ps1 @SetupArgs'))
    }

    It 'can install an explicit branch or ref for PR testing' {
        $script:InstallPs1 | Should -Match ([regex]::Escape('[string]$Ref'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('JUGGERNAUT_REF'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('git clone --branch $Ref'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('checkout --quiet FETCH_HEAD'))
    }

    It 'backs up a dirty existing install before cloning a fresh copy' {
        foreach ($needle in @('JUGGERNAUT_REPO_URL', 'Test-InstallTreeDirty', 'Backup-ExistingInstall', '.backup.', 'Backup created:')) {
            $script:InstallPs1 | Should -Match ([regex]::Escape($needle))
        }
    }

    It 'does not reject default or bedrock API-key auth before setup can run' {
        $setupScript = Join-Path $script:RepoRoot 'setup-claude-bedrock.ps1'
        $pwsh = (Get-Command pwsh -ErrorAction SilentlyContinue)
        if (-not $pwsh) { $pwsh = Get-Command powershell -ErrorAction Stop }

        $defaultOutput = & $pwsh.Source -NoProfile -ExecutionPolicy Bypass -File $setupScript -Help 2>&1 | Out-String
        $LASTEXITCODE | Should -Be 0
        $defaultOutput | Should -Match ([regex]::Escape('Authentication: iam (default) or bedrock-api-key'))

        $bedrockOutput = & $pwsh.Source -NoProfile -ExecutionPolicy Bypass -File $setupScript -Auth bedrock-api-key -Help 2>&1 | Out-String
        $LASTEXITCODE | Should -Be 0
        $bedrockOutput | Should -Match ([regex]::Escape('bedrock-api-key'))
    }

    It 'can run doctor through the launcher entrypoint after a clean install' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-inst-h-" + [Guid]::NewGuid().ToString('N'))
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

    It 'scriptblock installer -Ref -Configure returns and writes API-key settings' {
        $tmpRoot = Join-Path ([IO.Path]::GetTempPath()) ("jug-inst-scenario-" + [Guid]::NewGuid().ToString('N'))
        $src = Join-Path $tmpRoot 'src'
        $remote = Join-Path $tmpRoot 'repo.git'
        $testHome = Join-Path $tmpRoot 'home'
        New-Item -ItemType Directory -Path $tmpRoot,$testHome -Force | Out-Null

        $oldHomeVar = $HOME; $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldRepo = $env:JUGGERNAUT_REPO_URL
        $oldDir = $env:JUGGERNAUT_DIR; $oldLocation = (Get-Location).Path
        try {
            git clone -q $script:RepoRoot $src
            git -C $src checkout -q -b scenario-ref
            git -C $src config user.email test@example.invalid
            git -C $src config user.name 'Juggernaut Test'
            Set-Content -Path (Join-Path $src 'VERSION') -Value 'scenario-ref' -Encoding ascii
            git -C $src add VERSION
            git -C $src commit -q -m 'scenario ref'
            git clone --bare -q $src $remote

            Set-Variable -Name HOME -Value $testHome -Scope Global -Force
            $env:HOME = $testHome; $env:USERPROFILE = $testHome
            $env:JUGGERNAUT_REPO_URL = $remote
            $env:JUGGERNAUT_DIR = Join-Path $testHome '.juggernaut'

            $installer = Get-Content (Join-Path $script:RepoRoot 'install.ps1') -Raw
            $output = & ([scriptblock]::Create($installer)) -Ref scenario-ref -Configure `
                -Auth bedrock-api-key -BedrockKey br-ci-token -Storage profile -NoShellFallback 2>&1 | Out-String
            $afterConfigure = 'still-running-after-configure'

            $output | Should -Not -Match ([regex]::Escape("unknown option '--ref'"))
            $output | Should -Not -Match ([regex]::Escape("got: '-Auth'"))
            (Get-Content (Join-Path $testHome '.juggernaut\VERSION') -Raw).Trim() | Should -Be 'scenario-ref'
            $settings = Read-Settings -Path (Join-Path $testHome '.claude\settings.json')
            $settings['juggernaut']['auth']['mode'] | Should -Be 'bedrock-api-key'
            $settings['juggernaut']['auth']['storage'] | Should -Be 'profile'
            $afterConfigure | Should -Be 'still-running-after-configure'

            $doctorOutput = & (Join-Path $testHome '.local\bin\juggernaut.cmd') doctor --v2 2>&1 | Out-String
            $LASTEXITCODE | Should -Be 0
            $doctorOutput | Should -Match ([regex]::Escape('Auth: Bedrock API key'))
            $doctorOutput | Should -Match ([regex]::Escape('Status: OK'))
            $doctorOutput | Should -Match ([regex]::Escape('No issues found'))
            $doctorOutput | Should -Not -Match 'ParserError|Unexpected token'
        } finally {
            Set-Location $oldLocation
            Set-Variable -Name HOME -Value $oldHomeVar -Scope Global -Force
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldRepo) { Remove-Item Env:\JUGGERNAUT_REPO_URL -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_REPO_URL = $oldRepo }
            if ($null -eq $oldDir) { Remove-Item Env:\JUGGERNAUT_DIR -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_DIR = $oldDir }
            Remove-Item -Path $tmpRoot -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
