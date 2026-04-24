# tests/v2/Installer.Tests.ps1 - static acceptance checks for installer robustness.

BeforeAll {
    $script:RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    $script:InstallSh = Get-Content (Join-Path $script:RepoRoot 'install.sh') -Raw
    $script:InstallPs1 = Get-Content (Join-Path $script:RepoRoot 'install.ps1') -Raw
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
        $script:InstallPs1 | Should -Match ([regex]::Escape('Set-ExecutionPolicy RemoteSigned -Scope CurrentUser'))
        $script:InstallPs1 | Should -Match 'juggernaut doctor --v2'
    }
}
