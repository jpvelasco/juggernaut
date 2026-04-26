# tests/v2/Keychain.Tests.ps1 - Pester 5.x tests for lib/keychain.ps1.
# Run with: Invoke-Pester -Path tests/v2/Keychain.Tests.ps1
# Focus: the 3-state Get-KeychainEntry contract (value | $null | throw).

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib/keychain.ps1')
}

Describe 'Keychain service/account constants' {
    It 'JUGGERNAUT_KEYCHAIN_SERVICE env overrides default' {
        $env:JUGGERNAUT_KEYCHAIN_SERVICE = 'test-override'
        try {
            Get-KeychainServiceName | Should -Be 'test-override'
        } finally {
            Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
        }
    }
    It 'Get-KeychainServiceName defaults to juggernaut-bedrock' {
        Get-KeychainServiceName | Should -Be 'juggernaut-bedrock'
    }
    It 'Get-KeychainAccountName defaults to api-key' {
        Get-KeychainAccountName | Should -Be 'api-key'
    }
}

Describe 'Get-KeychainOS' {
    It 'returns a known value' {
        Get-KeychainOS | Should -BeIn @('macos','linux','wsl','windows','unknown')
    }
}

Describe 'Get-KeychainEntry — not found returns $null' {
    It 'returns $null for a service that cannot exist' {
        $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-$([Guid]::NewGuid().ToString('N'))"
        try {
            $os = Get-KeychainOS
            if ($os -notin @('macos','linux','wsl','windows')) {
                Set-ItResult -Skipped -Because "OS '$os' has no keychain backend"
                return
            }
            if ($os -in @('macos','linux','wsl') -and -not (Test-KeychainAvailable)) {
                Set-ItResult -Skipped -Because "Keychain tooling not installed on this runner"
                return
            }
            $result = Get-KeychainEntry
            $result | Should -BeNullOrEmpty
        } finally {
            Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
        }
    }
}

Describe 'Get-KeychainRetrievalExpression' {
    It 'bash expression references the service' {
        $expr = Get-KeychainRetrievalExpression -Shell 'bash'
        $expr | Should -Not -BeNullOrEmpty
        $expr | Should -Match 'juggernaut-bedrock'
    }
    It 'fish expression wraps with () instead of $()' {
        $expr = Get-KeychainRetrievalExpression -Shell 'fish'
        $expr | Should -Not -BeNullOrEmpty
        $expr.StartsWith('$(') | Should -BeFalse
        $expr.StartsWith('(')  | Should -BeTrue
    }
}
