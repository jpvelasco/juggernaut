# tests/v2/ConfigManager.Tests.ps1 — Pester 5.x tests for lib/config_manager.ps1.
# Mirrors tests/v2/test_config_manager.sh.

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib/schema.ps1')
    . (Join-Path $repoRoot 'lib/config_manager.ps1')
    $script:BedrockConfigPath = Join-Path $repoRoot 'bedrock-config.json'
}

Describe 'Write → Read round trip' {
    BeforeEach {
        $script:tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("juggernaut-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:tmpDir -Force | Out-Null
        $script:target = Join-Path $script:tmpDir 'settings.json'
    }
    AfterEach {
        Remove-Item -Path $script:tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'fresh write then read preserves juggernaut.meta.managedBy' {
        $block  = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
        $native = Get-NativeKeysFromJuggernautBlock -Block $block
        $existing = Read-Settings -Path $script:target
        $merged = Merge-JuggernautBlock -Existing $existing -NewBlock $block -NativeKeys $native
        Write-SettingsAtomic -Path $script:target -Content $merged

        $readBack = Read-Settings -Path $script:target
        $readBack['juggernaut']['meta']['managedBy'] | Should -Be 'juggernaut'
        $readBack['env']['CLAUDE_CODE_USE_BEDROCK']  | Should -Be '1'
        (Test-HasJuggernautBlock -Settings $readBack) | Should -BeTrue
    }
}

Describe 'Merge preserves unrelated user keys' {
    BeforeEach {
        $script:tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("juggernaut-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $script:tmpDir -Force | Out-Null
        $script:target = Join-Path $script:tmpDir 'settings.json'
    }
    AfterEach {
        Remove-Item -Path $script:tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'theme and permissions survive an apply' {
        $userContent = '{"theme":"dark","permissions":{"allow":["npm"]}}'
        Set-Content -Path $script:target -Value $userContent -NoNewline -Encoding utf8

        $block  = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
        $native = Get-NativeKeysFromJuggernautBlock -Block $block
        $existing = Read-Settings -Path $script:target
        $merged = Merge-JuggernautBlock -Existing $existing -NewBlock $block -NativeKeys $native
        Write-SettingsAtomic -Path $script:target -Content $merged

        $readBack = Read-Settings -Path $script:target
        $readBack['theme']                          | Should -Be 'dark'
        $readBack['permissions']['allow'][0]        | Should -Be 'npm'
        $readBack['juggernaut']['meta']['managedBy']| Should -Be 'juggernaut'
    }
}

Describe 'Remove leaves unrelated user keys' {
    It 'strips juggernaut + native keys; keeps theme' {
        $existing = [ordered]@{
            theme = 'dark'
            juggernaut = [ordered]@{ meta = [ordered]@{ managedBy = 'juggernaut' } }
            env = [ordered]@{ CLAUDE_CODE_USE_BEDROCK = '1' }
            model = 'foo'
            modelOverrides = [ordered]@{ opus = 'x' }
            availableModels = @('a','b')
        }
        $stripped = Remove-JuggernautBlockFromSettings -Existing $existing
        $stripped.Contains('juggernaut')      | Should -BeFalse
        $stripped.Contains('env')             | Should -BeFalse
        $stripped.Contains('model')           | Should -BeFalse
        $stripped.Contains('availableModels') | Should -BeFalse
        $stripped['theme']                    | Should -Be 'dark'
    }
}

Describe 'Invoke-WithSettingsLock — happy path' {
    It 'runs the action and releases the mutex' {
        $p = Join-Path ([IO.Path]::GetTempPath()) ("juggernaut-lock-" + [Guid]::NewGuid().ToString('N'))
        $hit = $false
        Invoke-WithSettingsLock -Path $p -Action { $script:hit = $true }
        # Immediately re-acquire — would block if the first call didn't release.
        { Invoke-WithSettingsLock -Path $p -Action { 1 + 1 } } | Should -Not -Throw
    }
}

Describe 'Test-HasJuggernautBlock' {
    It 'true when managedBy = juggernaut' {
        $s = [ordered]@{ juggernaut = [ordered]@{ meta = [ordered]@{ managedBy = 'juggernaut' } } }
        (Test-HasJuggernautBlock -Settings $s) | Should -BeTrue
    }
    It 'false when juggernaut key is missing' {
        $s = [ordered]@{ theme = 'dark' }
        (Test-HasJuggernautBlock -Settings $s) | Should -BeFalse
    }
    It 'false when managedBy is not juggernaut' {
        $s = [ordered]@{ juggernaut = [ordered]@{ meta = [ordered]@{ managedBy = 'other' } } }
        (Test-HasJuggernautBlock -Settings $s) | Should -BeFalse
    }
}
