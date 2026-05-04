# tests/v2/Installer.Tests.ps1 — Pester 5 static + runtime acceptance checks
# for the v3 install.ps1 wipe-and-reinstall installer.

BeforeAll {
    $script:RepoRoot   = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    $script:InstallSh  = Get-Content (Join-Path $script:RepoRoot 'install.sh')  -Raw
    $script:InstallPs1 = Get-Content (Join-Path $script:RepoRoot 'install.ps1') -Raw
}

Describe 'install.sh v3 wipe-and-reinstall' {
    It 'announces wipe-and-reinstall behavior and pre-wipe summary' {
        foreach ($needle in @(
            'wipe-and-reinstall',
            'Pre-wipe summary',
            'BEGIN: Juggernaut',
            'BEGIN: Claude Code Bedrock Configuration',
            'juggernaut-bedrock',
            '--dry-run',
            'juggernaut apply --auth=iam'
        )) {
            $script:InstallSh | Should -Match ([regex]::Escape($needle))
        }
    }

    It 'does NOT auto-apply or mention legacy v1/v2 flags' {
        foreach ($needle in @(
            '--configure',
            '--legacy-v1',
            '--v2',
            'setup-claude-bedrock',
            'JUGGERNAUT_USE_V2'
        )) {
            $script:InstallSh | Should -Not -Match ([regex]::Escape($needle))
        }
    }

    It 'repairs executable bits for v3 entrypoints and libraries' {
        foreach ($needle in @('chmod +x', 'commands/*.sh', 'lib/*.sh', 'juggernaut')) {
            $script:InstallSh | Should -Match ([regex]::Escape($needle))
        }
    }

    It 'creates a user-local juggernaut launcher' {
        $script:InstallSh | Should -Match ([regex]::Escape('.local/bin'))
        $script:InstallSh | Should -Match 'ln -sfn'
    }

    It 'supports installing an explicit branch or ref for PR testing' {
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

Describe 'install.ps1 v3 wipe-and-reinstall' {
    It 'announces wipe-and-reinstall behavior and pre-wipe summary' {
        foreach ($needle in @(
            'Pre-wipe summary',
            'BEGIN: Juggernaut',
            'BEGIN: Claude Code Bedrock Configuration',
            'juggernaut-bedrock',
            '-DryRun'
        )) {
            $script:InstallPs1 | Should -Match ([regex]::Escape($needle))
        }
    }

    It 'does NOT auto-apply or mention legacy v1/v2 flags' {
        foreach ($needle in @(
            '-Configure',
            '-LegacyV1',
            'setup-claude-bedrock.ps1',
            'JUGGERNAUT_USE_V2'
        )) {
            $script:InstallPs1 | Should -Not -Match ([regex]::Escape($needle))
        }
    }

    It 'creates PowerShell and cmd shims under ~/.local/bin' {
        $script:InstallPs1 | Should -Match ([regex]::Escape('.local\bin'))
        $script:InstallPs1 | Should -Match 'juggernaut\.ps1'
        $script:InstallPs1 | Should -Match 'juggernaut\.cmd'
        $script:InstallPs1 | Should -Match ([regex]::Escape('exit /b %ERRORLEVEL%'))
    }

    It 'prints PATH and ExecutionPolicy guidance' {
        $script:InstallPs1 | Should -Match 'PATH'
        $script:InstallPs1 | Should -Match ([regex]::Escape('Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'))
    }

    It 'rejects pipe-to-iex invocation and recommends download-then-run form' {
        $pwsh = (Get-Command pwsh -ErrorAction SilentlyContinue)
        if (-not $pwsh) { $pwsh = Get-Command powershell -ErrorAction Stop }
        $installerPath = Join-Path $script:RepoRoot 'install.ps1'
        $command = @"
`$installer = Get-Content -Path '$($installerPath -replace "'", "''")' -Raw
`$installer | iex
"@
        $output = & $pwsh.Source -NoProfile -ExecutionPolicy Bypass -Command $command 2>&1 | Out-String
        $LASTEXITCODE | Should -Be 1
        $output | Should -Match ([regex]::Escape('cannot be run with'))
        $output | Should -Match '-OutFile'
        $output | Should -Match 'Unblock-File'
    }

    It 'supports installing an explicit branch or ref for PR testing' {
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

    It 'tells the user to run apply explicitly after install' {
        $script:InstallPs1 | Should -Match ([regex]::Escape('No configuration has been written'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('juggernaut apply -Auth iam'))
        $script:InstallPs1 | Should -Match ([regex]::Escape('juggernaut apply -Auth bedrock-api-key'))
    }

    It '-Help exits cleanly and mentions v3 flags' {
        $pwsh = (Get-Command pwsh -ErrorAction SilentlyContinue)
        if (-not $pwsh) { $pwsh = Get-Command powershell -ErrorAction Stop }
        $installerPath = Join-Path $script:RepoRoot 'install.ps1'
        $output = & $pwsh.Source -NoProfile -ExecutionPolicy Bypass -File $installerPath -Help 2>&1 | Out-String
        $output | Should -Match ([regex]::Escape('-DryRun'))
        $output | Should -Match ([regex]::Escape('-Ref'))
        $output | Should -Not -Match ([regex]::Escape('-LegacyV1'))
        $output | Should -Not -Match ([regex]::Escape('-Configure'))
    }
}
