# tests/v2/DPAPI.Tests.ps1 - Pester 5.x tests for DPAPI storage in lib/keychain.ps1.
# Run with: Invoke-Pester -Path tests/v2/DPAPI.Tests.ps1
#
# Windows-only. On non-Windows hosts every It is skipped.

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib/keychain.ps1')

    $script:IsWindowsHost = (Get-KeychainOS) -eq 'windows'

    # Isolate DPAPI file and CredMan entry from anything the real install wrote.
    $script:TestHome    = Join-Path ([IO.Path]::GetTempPath()) ("juggernaut-dpapi-" + [Guid]::NewGuid().ToString('N'))
    $script:TestService = "juggernaut-bedrock-test-" + [Guid]::NewGuid().ToString('N').Substring(0,8)

    $env:JUGGERNAUT_HOME              = $script:TestHome
    $env:JUGGERNAUT_KEYCHAIN_SERVICE  = $script:TestService
    New-Item -ItemType Directory -Path $script:TestHome -Force | Out-Null
}

AfterAll {
    # Best-effort teardown — clear both backends and drop the temp home.
    try { Remove-BearerToken } catch {}
    Remove-Item Env:\JUGGERNAUT_HOME              -ErrorAction SilentlyContinue
    Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE  -ErrorAction SilentlyContinue
    if ($script:TestHome -and (Test-Path $script:TestHome)) {
        Remove-Item -LiteralPath $script:TestHome -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Describe 'DPAPI storage' -Skip:(-not $IsWindows) {

    BeforeEach {
        # Each test starts with both stores empty. Remove-BearerToken is idempotent.
        Remove-BearerToken
    }

    It 'Get-DPAPIEntryPath honors JUGGERNAUT_HOME' {
        $expected = Join-Path $script:TestHome '.juggernaut\bearer-token.dpapi.bin'
        Get-DPAPIEntryPath | Should -Be $expected
    }

    It 'Save-BearerToken -Mode auto picks DPAPI for a 2400-char key' {
        $longKey = -join ((1..2400) | ForEach-Object { [char](65 + ($_ % 26)) })
        $longKey.Length | Should -Be 2400

        $r = Save-BearerToken -Key $longKey -Mode 'auto'
        $r.Ok      | Should -BeTrue
        $r.Storage | Should -Be 'dpapi'

        Test-Path (Get-DPAPIEntryPath) | Should -BeTrue

        $read = Read-BearerToken
        $read.Storage | Should -Be 'dpapi'
        $read.Value   | Should -Be $longKey
    }

    It 'Save-BearerToken -Mode auto picks Credential Manager for a 500-char key' {
        $shortKey = -join ((1..500) | ForEach-Object { [char](65 + ($_ % 26)) })

        $r = Save-BearerToken -Key $shortKey -Mode 'auto'
        $r.Ok      | Should -BeTrue
        $r.Storage | Should -Be 'keychain'

        # Auto-success on CredMan must clean the DPAPI file so reads are unambiguous.
        Test-Path (Get-DPAPIEntryPath) | Should -BeFalse

        $read = Read-BearerToken
        $read.Storage | Should -Be 'keychain'
        $read.Value   | Should -Be $shortKey
    }

    It 'Save-BearerToken -Mode dpapi forces DPAPI even for a short key' {
        $shortKey = 'short-test-key-' + ('x' * 100)

        $r = Save-BearerToken -Key $shortKey -Mode 'dpapi'
        $r.Ok      | Should -BeTrue
        $r.Storage | Should -Be 'dpapi'

        Test-Path (Get-DPAPIEntryPath) | Should -BeTrue

        # DPAPI-mode success must also clear any stale CredMan entry.
        Get-KeychainEntry | Should -BeNullOrEmpty

        $read = Read-BearerToken
        $read.Storage | Should -Be 'dpapi'
        $read.Value   | Should -Be $shortKey
    }

    It 'Get-DPAPIEntry throws when the file is tampered' {
        $key = 'round-trip-' + ('y' * 200)
        (Save-BearerToken -Key $key -Mode 'dpapi').Ok | Should -BeTrue

        # Overwrite the ciphertext with random bytes. Unprotect must fail loudly
        # rather than returning $null — $null means "not found" and would let
        # callers silently fall through to another source.
        $path = Get-DPAPIEntryPath
        $rand = New-Object byte[] 256
        (New-Object Random).NextBytes($rand)
        [IO.File]::WriteAllBytes($path, $rand)

        { Get-DPAPIEntry } | Should -Throw
    }

    It 'Get-DPAPIEntry returns $null when the file does not exist' {
        # Post-BeforeEach, both stores are empty.
        Test-Path (Get-DPAPIEntryPath) | Should -BeFalse
        Get-DPAPIEntry | Should -BeNullOrEmpty
    }

    It 'Remove-BearerToken is idempotent and clears both backends' {
        $key = 'cleanup-' + ('z' * 300)
        (Save-BearerToken -Key $key -Mode 'dpapi').Ok | Should -BeTrue
        Test-Path (Get-DPAPIEntryPath) | Should -BeTrue

        Remove-BearerToken
        Test-Path (Get-DPAPIEntryPath) | Should -BeFalse
        Get-KeychainEntry | Should -BeNullOrEmpty

        # Second call must not error on a clean slate.
        { Remove-BearerToken } | Should -Not -Throw
        Test-Path (Get-DPAPIEntryPath) | Should -BeFalse

        $read = Read-BearerToken
        $read.Value   | Should -BeNullOrEmpty
        $read.Storage | Should -Be 'none'
    }
}
