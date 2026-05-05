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

Describe 'Profile token storage' {
    BeforeAll {
        $script:testTokenDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-pt-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:testTokenDir -Force | Out-Null
        $script:profilePath = Join-Path $script:testTokenDir 'bearer-token'
    }
    AfterAll {
        Remove-Item -Path $script:testTokenDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    BeforeEach {
        $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $script:profilePath
    }
    AfterEach {
        Remove-ProfileTokenEntry
        Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
    }

    It 'Get-ProfileTokenPath respects JUGGERNAUT_PROFILE_TOKEN_PATH' {
        Get-ProfileTokenPath | Should -Be $script:profilePath
    }

    It 'Get-ProfileTokenPath builds path from XDG_CONFIG_HOME without embedded separators' {
        $oldOverride = $env:JUGGERNAUT_PROFILE_TOKEN_PATH
        $oldXdg = $env:XDG_CONFIG_HOME
        try {
            Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
            $env:XDG_CONFIG_HOME = $script:testTokenDir
            Get-ProfileTokenPath | Should -Be (Join-Path (Join-Path $script:testTokenDir 'juggernaut') 'bearer-token')
        } finally {
            if ($null -eq $oldOverride) { Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $oldOverride }
            if ($null -eq $oldXdg) { Remove-Item Env:\XDG_CONFIG_HOME -ErrorAction SilentlyContinue } else { $env:XDG_CONFIG_HOME = $oldXdg }
        }
    }

    It 'Set-ProfileTokenEntry writes file, Get-ProfileTokenEntry reads it back' {
        $token = 'sk-brk-test-' + [Guid]::NewGuid().ToString('N')
        $ok = Set-ProfileTokenEntry -Key $token
        $ok | Should -BeTrue
        $val = Get-ProfileTokenEntry
        $val | Should -Be $token
    }

    It 'Get-ProfileTokenEntry returns $null when file does not exist' {
        Remove-ProfileTokenEntry
        $val = Get-ProfileTokenEntry
        $val | Should -BeNullOrEmpty
    }

    It 'Remove-ProfileTokenEntry deletes the file' {
        $token = 'sk-brk-deltest'
        Set-ProfileTokenEntry -Key $token | Out-Null
        $file = Get-ProfileTokenPath
        Test-Path $file | Should -BeTrue
        Remove-ProfileTokenEntry
        Test-Path $file | Should -BeFalse
    }

    It 'Set-ProfileTokenEntry is idempotent on re-write' {
        $first  = 'tok-one'
        $second = 'tok-two'
        Set-ProfileTokenEntry -Key $first  | Out-Null
        Set-ProfileTokenEntry -Key $second | Out-Null
        Get-ProfileTokenEntry | Should -Be $second
    }
}

Describe 'Save-BearerToken — profile mode' {
    BeforeAll {
        $script:testTokenDir2 = Join-Path ([IO.Path]::GetTempPath()) ("jug-pt2-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:testTokenDir2 -Force | Out-Null
        $script:profilePath2 = Join-Path $script:testTokenDir2 'bearer-token'
    }
    AfterAll {
        Remove-Item -Path $script:testTokenDir2 -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'Mode=profile writes to profile token file' {
        $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $script:profilePath2
        $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-save-prof-$([Guid]::NewGuid().ToString('N'))"
        try {
            $token = 'sk-profile-save-test'
            $result = Save-BearerToken -Key $token -Mode 'profile'
            $result.Ok      | Should -BeTrue
            $result.Storage | Should -Be 'profile'
            Test-Path $script:profilePath2 | Should -BeTrue
        } finally {
            Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
            Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
        }
    }
}

Describe 'Read-BearerToken — profile as fallback' {
    BeforeAll {
        $script:testTokenDir3 = Join-Path ([IO.Path]::GetTempPath()) ("jug-pt3-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:testTokenDir3 -Force | Out-Null
        $script:profilePath3 = Join-Path $script:testTokenDir3 'bearer-token'
    }
    AfterAll {
        Remove-Item -Path $script:testTokenDir3 -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'Read-BearerToken finds profile token when keychain and dpapi are empty' {
        $script:oldHome3 = $env:JUGGERNAUT_HOME
        $script:oldSvc3  = $env:JUGGERNAUT_KEYCHAIN_SERVICE
        $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $script:profilePath3
        $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-read-prof-$([Guid]::NewGuid().ToString('N'))"
        # Point DPAPI at a temp location so the default DPAPI file doesn't interfere
        $env:JUGGERNAUT_HOME = $script:testTokenDir3
        try {
            $token = 'sk-profile-read-test'
            Set-ProfileTokenEntry -Key $token | Out-Null
            $result = Read-BearerToken
            $result.Value   | Should -Be $token
            $result.Storage | Should -Be 'profile'
        } finally {
            Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
            if ($null -eq $script:oldSvc3) { Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_KEYCHAIN_SERVICE = $script:oldSvc3 }
            if ($null -eq $script:oldHome3) { Remove-Item Env:\JUGGERNAUT_HOME -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_HOME = $script:oldHome3 }
        }
    }
}

Describe 'Remove-BearerToken — cleans profile token' {
    BeforeAll {
        $script:testTokenDir4 = Join-Path ([IO.Path]::GetTempPath()) ("jug-pt4-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:testTokenDir4 -Force | Out-Null
        $script:profilePath4 = Join-Path $script:testTokenDir4 'bearer-token'
    }
    AfterAll {
        Remove-Item -Path $script:testTokenDir4 -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'Remove-BearerToken removes the profile token file' {
        $script:oldHome4 = $env:JUGGERNAUT_HOME
        $script:oldSvc4  = $env:JUGGERNAUT_KEYCHAIN_SERVICE
        $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $script:profilePath4
        $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-rm-prof-$([Guid]::NewGuid().ToString('N'))"
        $env:JUGGERNAUT_HOME = $script:testTokenDir4
        try {
            Set-ProfileTokenEntry -Key 'sk-test' | Out-Null
            Test-Path $script:profilePath4 | Should -BeTrue
            Remove-BearerToken
            Test-Path $script:profilePath4 | Should -BeFalse
        } finally {
            Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
            if ($null -eq $script:oldSvc4) { Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_KEYCHAIN_SERVICE = $script:oldSvc4 }
            if ($null -eq $script:oldHome4) { Remove-Item Env:\JUGGERNAUT_HOME -ErrorAction SilentlyContinue } else { $env:JUGGERNAUT_HOME = $script:oldHome4 }
        }
    }
}

