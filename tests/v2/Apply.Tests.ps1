# tests/v2/Apply.Tests.ps1 — Pester 5.x tests for commands/apply.ps1 and lib/profile_writer.ps1.
# Run with: Invoke-Pester -Path tests/v2/Apply.Tests.ps1

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib\schema.ps1')
    . (Join-Path $repoRoot 'lib\config_manager.ps1')
    . (Join-Path $repoRoot 'lib\migrator.ps1')
    . (Join-Path $repoRoot 'lib\keychain.ps1')
    . (Join-Path $repoRoot 'lib\profile_writer.ps1')
    $script:BedrockConfigPath = Join-Path $repoRoot 'bedrock-config.json'
    $script:Fixtures          = Join-Path $repoRoot 'tests\v2\fixtures'
    $env:JUGGERNAUT_USE_V2    = '1'
    $env:BEDROCK_CONFIG_PATH  = $script:BedrockConfigPath
}

# ---------------------------------------------------------------------------
# Block building — schema wrapper
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — IAM defaults' {
    BeforeAll {
        $script:block = New-JuggernautBlock `
            -AuthMode  'iam' `
            -Storage   'profile' `
            -Region    'us-east-1' `
            -Effort    'xhigh' `
            -BedrockConfigPath $script:BedrockConfigPath
    }

    It 'auth.mode = iam'       { $script:block.auth.mode     | Should -Be 'iam' }
    It 'auth.region = us-east-1' { $script:block.auth.region | Should -Be 'us-east-1' }
    It 'auth.storage = profile'  { $script:block.auth.storage | Should -Be 'profile' }
    It 'effortLevel = xhigh'   { $script:block.effortLevel   | Should -Be 'xhigh' }
    It 'opusplan = false'      { $script:block.opusplan       | Should -BeFalse }
    It 'meta.managedBy = juggernaut' { $script:block.meta.managedBy | Should -Be 'juggernaut' }
    It 'meta.version = 2.0.0'  { $script:block.meta.version  | Should -Be '2.0.0' }
    It 'env.AWS_REGION = us-east-1' {
        $script:block.env['AWS_REGION'] | Should -Be 'us-east-1'
    }
    It 'env.CLAUDE_CODE_USE_BEDROCK = 1' {
        $script:block.env['CLAUDE_CODE_USE_BEDROCK'] | Should -Be '1'
    }
}

# ---------------------------------------------------------------------------
# Opusplan
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — opusplan=true' {
    It 'env.ANTHROPIC_MODEL = opusplan' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -OpusPlan $true `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.env['ANTHROPIC_MODEL'] | Should -Be 'opusplan'
        $b.opusplan               | Should -BeTrue
    }
}

# ---------------------------------------------------------------------------
# Mantle
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — Mantle flags' {
    It 'useMantle=true sets CLAUDE_CODE_USE_MANTLE=1' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -UseMantle $true `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.env['CLAUDE_CODE_USE_MANTLE'] | Should -Be '1'
    }
    It 'useMantle=false omits CLAUDE_CODE_USE_MANTLE' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -UseMantle $false `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.env.Contains('CLAUDE_CODE_USE_MANTLE') | Should -BeFalse
    }
    It 'MantleBaseUrl sets ANTHROPIC_BEDROCK_MANTLE_BASE_URL' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
                                 -UseMantle $true -MantleBaseUrl 'https://mantle.example.com' `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.env['ANTHROPIC_BEDROCK_MANTLE_BASE_URL'] | Should -Be 'https://mantle.example.com'
        $b.mantle.baseUrl | Should -Be 'https://mantle.example.com'
    }
}

# ---------------------------------------------------------------------------
# Effort level
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — effort level' {
    It 'effort=high propagates to env' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -EffortLevel 'high' `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.effortLevel                          | Should -Be 'high'
        $b.env['CLAUDE_CODE_EFFORT_LEVEL']      | Should -Be 'high'
    }
}

# ---------------------------------------------------------------------------
# settings.json round-trip
# ---------------------------------------------------------------------------
Describe 'settings.json round-trip' {
    BeforeAll {
        $tmpDir  = Join-Path ([IO.Path]::GetTempPath()) ("jug-apply-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:rtSettings = Join-Path $tmpDir 'settings.json'

        $block   = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
                                        -BedrockConfigPath $script:BedrockConfigPath
        $native  = Get-NativeKeysFromJuggernautBlock -Block $block
        $merged  = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
        Write-SettingsAtomic -Path $script:rtSettings -Content $merged
    }
    AfterAll {
        Remove-Item (Split-Path $script:rtSettings -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'settings.json is written' { Test-Path $script:rtSettings | Should -BeTrue }
    It 'juggernaut.meta.managedBy = juggernaut' {
        $s = Read-Settings -Path $script:rtSettings
        $s['juggernaut']['meta']['managedBy'] | Should -Be 'juggernaut'
    }
    It 'env.AWS_REGION = us-west-2' {
        $s = Read-Settings -Path $script:rtSettings
        $s['env']['AWS_REGION'] | Should -Be 'us-west-2'
    }
    It 'top-level model is set' {
        $s = Read-Settings -Path $script:rtSettings
        $s['model'] | Should -Not -BeNullOrEmpty
    }
}

# ---------------------------------------------------------------------------
# Implicit migration via Invoke-MigratorRun
# ---------------------------------------------------------------------------
Describe 'implicit migration — settings.json populated from v1 profile' {
    BeforeAll {
        $tmpDir   = Join-Path ([IO.Path]::GetTempPath()) ("jug-mig2-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:migSettings = Join-Path $tmpDir 'settings.json'
        $script:migProfile  = Join-Path $tmpDir 'profile.sh'
        Copy-Item (Join-Path $script:Fixtures 'v1_iam_default.sh') $script:migProfile
        Invoke-MigratorRun -ProfileFile $script:migProfile `
                           -SettingsPath $script:migSettings `
                           -BedrockConfigPath $script:BedrockConfigPath | Out-Null
    }
    AfterAll {
        Remove-Item (Split-Path $script:migSettings -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'settings.json written after migration' { Test-Path $script:migSettings | Should -BeTrue }
    It 'juggernaut block present'              {
        $s = Read-Settings -Path $script:migSettings
        Test-HasJuggernautBlock -Settings $s | Should -BeTrue
    }
    It 'auth.region from v1 AWS_REGION'        {
        $s = Read-Settings -Path $script:migSettings
        $s['juggernaut']['auth']['region'] | Should -Be 'us-east-1'
    }
}

# ---------------------------------------------------------------------------
# Profile writer — block shape
# ---------------------------------------------------------------------------
Describe 'Build-ProfileWriterBlock — bash IAM' {
    BeforeAll {
        $script:pwBlock = Build-ProfileWriterBlock `
            -Shell 'bash' -Region 'us-east-1' -AuthMode 'iam' `
            -ApiKeyExpr '' -StorageMode 'profile' `
            -BedrockConfigPath $script:BedrockConfigPath
    }

    It 'contains BEGIN marker'  { $script:pwBlock | Should -Match 'BEGIN: Claude Code Bedrock Configuration' }
    It 'contains END marker'    { $script:pwBlock | Should -Match 'END: Claude Code Bedrock Configuration' }
    It 'sets AWS_REGION'        { $script:pwBlock | Should -Match 'AWS_REGION="us-east-1"' }
    It 'unsets bearer token'    { $script:pwBlock | Should -Match 'unset AWS_BEARER_TOKEN_BEDROCK' }
    It 'sets CLAUDE_CODE_USE_BEDROCK' { $script:pwBlock | Should -Match 'CLAUDE_CODE_USE_BEDROCK="1"' }
}

Describe 'Build-ProfileWriterBlock — fish syntax' {
    It 'uses set -gx syntax' {
        $b = Build-ProfileWriterBlock `
            -Shell 'fish' -Region 'us-west-2' -AuthMode 'iam' `
            -ApiKeyExpr '' -StorageMode 'profile' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b | Should -Match 'set -gx AWS_REGION "us-west-2"'
        $b | Should -Match 'set -e AWS_BEARER_TOKEN_BEDROCK'
    }
}

# ---------------------------------------------------------------------------
# Profile writer — write / annotate
# ---------------------------------------------------------------------------
Describe 'Write-ProfileWriterBlock and Set-ProfileWriterAnnotation' {
    BeforeAll {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-pw-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:pwProfile = Join-Path $tmpDir 'profile.sh'
        Copy-Item (Join-Path $script:Fixtures 'v1_iam_default.sh') $script:pwProfile
    }
    AfterAll {
        Remove-Item (Split-Path $script:pwProfile -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'annotate inserts v2 notice' {
        Set-ProfileWriterAnnotation -ProfileFile $script:pwProfile
        Get-Content $script:pwProfile -Raw | Should -Match 'Juggernaut v2: PRIMARY config'
    }
    It 'annotate strips metadata comments' {
        Get-Content $script:pwProfile -Raw | Should -Not -Match '^# Auth mode:'
    }
    It 'annotate preserves BEGIN marker' {
        Get-Content $script:pwProfile -Raw | Should -Match 'BEGIN: Claude Code Bedrock Configuration'
    }
}

# ---------------------------------------------------------------------------
# Test-ProfileWriterHasBlock
# ---------------------------------------------------------------------------
Describe 'Test-ProfileWriterHasBlock' {
    It 'returns true for fixture with block' {
        Test-ProfileWriterHasBlock -ProfileFile (Join-Path $script:Fixtures 'v1_iam_default.sh') |
            Should -BeTrue
    }
    It 'returns false for missing file' {
        Test-ProfileWriterHasBlock -ProfileFile 'C:\does\not\exist.sh' | Should -BeFalse
    }
    It 'returns false for file without block' {
        $tmp = [IO.Path]::GetTempFileName()
        Set-Content -Path $tmp -Value 'export FOO=bar' -Encoding utf8
        Test-ProfileWriterHasBlock -ProfileFile $tmp | Should -BeFalse
        Remove-Item $tmp -Force
    }
}

# ---------------------------------------------------------------------------
# Keychain — OS detection and command shape
# ---------------------------------------------------------------------------
Describe 'Get-KeychainOS' {
    It 'returns a known value' {
        $os = Get-KeychainOS
        $os | Should -BeIn @('windows','macos','linux','wsl','unknown')
    }
}

Describe 'Get-KeychainRetrievalExpression' {
    It 'bash expression contains $(' {
        $expr = Get-KeychainRetrievalExpression -Shell 'bash'
        $expr | Should -Match '\$\('
        $expr | Should -Match 'juggernaut-bedrock'
    }
    It 'fish expression uses () not $()' {
        $expr = Get-KeychainRetrievalExpression -Shell 'fish'
        $expr | Should -Match '^\('
        $expr | Should -Not -Match '^\$\('
    }
}

Describe 'Test-KeychainAvailable' {
    It 'returns a boolean without throwing' {
        { Test-KeychainAvailable } | Should -Not -Throw
        Test-KeychainAvailable | Should -BeIn @($true, $false)
    }
}
