# tests/v2/Migrator.Tests.ps1 — Pester 5.x tests for lib/migrator.ps1.
# Run with: Invoke-Pester -Path tests/v2/Migrator.Tests.ps1

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib/schema.ps1')
    . (Join-Path $repoRoot 'lib/config_manager.ps1')
    . (Join-Path $repoRoot 'lib/migrator.ps1')
    $script:BedrockConfigPath = Join-Path $repoRoot 'bedrock-config.json'
    $script:Fixtures          = Join-Path $repoRoot 'tests/v2/fixtures'
}

# ---------------------------------------------------------------------------
# Feature flag gate
# ---------------------------------------------------------------------------
Describe 'migrate.ps1 feature flag' {
    It 'exits 0 without JUGGERNAUT_USE_V2=1' {
        $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
        $migrate  = Join-Path $repoRoot 'commands\migrate.ps1'
        $proc = Start-Process pwsh -ArgumentList "-NoProfile -NonInteractive -File `"$migrate`"" `
            -PassThru -Wait -NoNewWindow `
            -Environment @{ JUGGERNAUT_USE_V2 = '' }
        $proc.ExitCode | Should -Be 0
    }
}

# ---------------------------------------------------------------------------
# Detection
# ---------------------------------------------------------------------------
Describe 'Test-MigratorHasV1Block' {
    It 'returns true for a fixture with a v1 block' {
        Test-MigratorHasV1Block -ProfileFile (Join-Path $script:Fixtures 'v1_iam_default.sh') | Should -BeTrue
    }
    It 'returns false for a non-existent file' {
        Test-MigratorHasV1Block -ProfileFile 'C:\does\not\exist.sh' | Should -BeFalse
    }
    It 'returns false for a file without a v1 block' {
        $tmp = [IO.Path]::GetTempFileName()
        Set-Content -Path $tmp -Value 'export FOO=bar' -Encoding utf8
        Test-MigratorHasV1Block -ProfileFile $tmp | Should -BeFalse
        Remove-Item $tmp -Force
    }
}

# ---------------------------------------------------------------------------
# Parser — v1_iam_default
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — v1_iam_default' {
    BeforeAll {
        $raw   = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_iam_default.sh')
        $script:parsed = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode = iam'       { $script:parsed.authMode    | Should -Be 'iam' }
    It 'region  = us-east-1'  { $script:parsed.region      | Should -Be 'us-east-1' }
    It 'storage = profile'    { $script:parsed.storage     | Should -Be 'profile' }
    It 'effortLevel = xhigh'  { $script:parsed.effortLevel | Should -Be 'xhigh' }
    It 'opusPlan = false'     { $script:parsed.opusPlan    | Should -BeFalse }
    It 'legacyEnv has CLAUDE_CODE_USE_BEDROCK' {
        $script:parsed.legacyEnv['CLAUDE_CODE_USE_BEDROCK'] | Should -Be '1'
    }
}

# ---------------------------------------------------------------------------
# Parser — v1_opusplan_effort
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — v1_opusplan_effort' {
    BeforeAll {
        $raw   = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_opusplan_effort.sh')
        $script:parsed2 = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'opusPlan = true'      { $script:parsed2.opusPlan    | Should -BeTrue }
    It 'effortLevel = high'   { $script:parsed2.effortLevel | Should -Be 'high' }
    It 'region = us-west-2'   { $script:parsed2.region      | Should -Be 'us-west-2' }
}

# ---------------------------------------------------------------------------
# Parser — v1_apikey_keychain
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — v1_apikey_keychain' {
    BeforeAll {
        $raw   = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_apikey_keychain.sh')
        $script:parsed3 = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode = api-key'   { $script:parsed3.authMode | Should -Be 'api-key' }
    It 'storage  = keychain'  { $script:parsed3.storage  | Should -Be 'keychain' }
}

# ---------------------------------------------------------------------------
# Block building
# ---------------------------------------------------------------------------
Describe 'New-MigratorV2Block — from v1_iam_default' {
    BeforeAll {
        $raw    = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_iam_default.sh')
        $parsed = ConvertFrom-MigratorV1Block -RawBlock $raw
        $script:block = New-MigratorV2Block -Parsed $parsed -BedrockConfigPath $script:BedrockConfigPath
    }

    It 'meta.managedBy = juggernaut'  { $script:block.meta.managedBy  | Should -Be 'juggernaut' }
    It 'meta.migratedFrom = v1.7.x'  { $script:block.meta.migratedFrom | Should -Be 'v1.7.x' }
    It 'legacyEnv.source set'        { $script:block.legacyEnv.source  | Should -Be 'v1.7.x-profile-block' }
    It 'auth.region = us-east-1'     { $script:block.auth.region       | Should -Be 'us-east-1' }
    It 'useMantle = false'           { $script:block.useMantle          | Should -BeFalse }
    It 'env has CLAUDE_CODE_USE_BEDROCK' {
        $script:block.env['CLAUDE_CODE_USE_BEDROCK'] | Should -Be '1'
    }
}

Describe 'New-MigratorV2Block — opusplan sets ANTHROPIC_MODEL=opusplan' {
    It 'env.ANTHROPIC_MODEL = opusplan when opusPlan=true' {
        $raw    = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_opusplan_effort.sh')
        $parsed = ConvertFrom-MigratorV1Block -RawBlock $raw
        $block  = New-MigratorV2Block -Parsed $parsed -BedrockConfigPath $script:BedrockConfigPath
        $block.env['ANTHROPIC_MODEL'] | Should -Be 'opusplan'
    }
}

# ---------------------------------------------------------------------------
# legacyEnv snapshot captures keychain export
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — legacyEnv captures unquoted export' {
    It 'AWS_BEARER_TOKEN_BEDROCK present in legacyEnv' {
        $raw    = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_apikey_keychain.sh')
        $parsed = ConvertFrom-MigratorV1Block -RawBlock $raw
        $parsed.legacyEnv.Contains('AWS_BEARER_TOKEN_BEDROCK') | Should -BeTrue
    }
}

# ---------------------------------------------------------------------------
# Bare exports fixture — no metadata comments
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — v1_bare_exports' {
    BeforeAll {
        $raw = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_bare_exports.sh')
        $script:parsedBare = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode defaults to iam'       { $script:parsedBare.authMode    | Should -Be 'iam' }
    It 'region parsed from export line' { $script:parsedBare.region      | Should -Be 'us-west-2' }
    It 'model parsed from export line'  { $script:parsedBare.model       | Should -Be 'global.anthropic.claude-sonnet-4-6' }
    It 'legacyEnv has unquoted AWS_BEARER_TOKEN_BEDROCK' {
        $script:parsedBare.legacyEnv.Contains('AWS_BEARER_TOKEN_BEDROCK') | Should -BeTrue
    }
}

# ---------------------------------------------------------------------------
# Fish profile fixture — has_v1_block detects fish file
# ---------------------------------------------------------------------------
Describe 'Test-MigratorHasV1Block — fish profile' {
    It 'detects v1 block in fish fixture' {
        Test-MigratorHasV1Block -ProfileFile (Join-Path $script:Fixtures 'v1_fish_profile.fish') | Should -BeTrue
    }
}

Describe 'ConvertFrom-MigratorV1Block — v1_fish_profile' {
    BeforeAll {
        $raw = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'v1_fish_profile.fish')
        $script:parsedFish = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode from metadata comment'                { $script:parsedFish.authMode    | Should -Be 'iam' }
    It 'effortLevel from metadata comment'             { $script:parsedFish.effortLevel | Should -Be 'xhigh' }
    It 'region parsed from set -gx AWS_REGION'         { $script:parsedFish.region      | Should -Be 'us-west-2' }
    It 'model parsed from set -gx ANTHROPIC_MODEL'     { $script:parsedFish.model       | Should -Be 'global.anthropic.claude-sonnet-4-6' }
    It 'legacyEnv has AWS_REGION from set -gx'         { $script:parsedFish.legacyEnv.Contains('AWS_REGION') | Should -BeTrue }
}

# ---------------------------------------------------------------------------
# Full round-trip: Invoke-MigratorRun
# ---------------------------------------------------------------------------
Describe 'Invoke-MigratorRun — full round-trip' {
    BeforeAll {
        $tmpDir  = Join-Path ([IO.Path]::GetTempPath()) ("jug-mig-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:tmpSettings = Join-Path $tmpDir 'settings.json'
        $script:tmpProfile  = Join-Path $tmpDir 'profile.sh'
        Copy-Item (Join-Path $script:Fixtures 'v1_iam_default.sh') $script:tmpProfile
    }
    AfterAll {
        Remove-Item -Path (Split-Path $script:tmpSettings -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'returns true' {
        $result = Invoke-MigratorRun -ProfileFile $script:tmpProfile `
                                     -SettingsPath $script:tmpSettings `
                                     -BedrockConfigPath $script:BedrockConfigPath
        $result | Should -BeTrue
    }

    It 'writes settings.json with juggernaut block' {
        $s = Read-Settings -Path $script:tmpSettings
        $s['juggernaut']['meta']['managedBy'] | Should -Be 'juggernaut'
    }

    It 'native env.CLAUDE_CODE_USE_BEDROCK = 1' {
        $s = Read-Settings -Path $script:tmpSettings
        $s['env']['CLAUDE_CODE_USE_BEDROCK'] | Should -Be '1'
    }

    It 'annotates the profile (notice comment present)' {
        $content = Get-Content -Path $script:tmpProfile -Raw
        $content | Should -Match 'Juggernaut v2: PRIMARY config'
    }

    It 'removes metadata comments from profile' {
        $content = Get-Content -Path $script:tmpProfile -Raw
        $content | Should -Not -Match '^# Auth mode:'
    }
}

# ---------------------------------------------------------------------------
# Rollback
# ---------------------------------------------------------------------------
Describe 'Invoke-MigratorRollback' {
    BeforeAll {
        $tmpDir  = Join-Path ([IO.Path]::GetTempPath()) ("jug-rb-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:rbSettings = Join-Path $tmpDir 'settings.json'
        $script:rbProfile  = Join-Path $tmpDir 'profile.sh'
        Copy-Item (Join-Path $script:Fixtures 'v1_iam_default.sh') $script:rbProfile
        # Pre-existing file so config_write_atomic creates a backup on first migration.
        Set-Content -Path $script:rbSettings -Value '{"preexisting":true}' -Encoding utf8
        Invoke-MigratorRun -ProfileFile $script:rbProfile `
                           -SettingsPath $script:rbSettings `
                           -BedrockConfigPath $script:BedrockConfigPath | Out-Null
        # Clobber after migration.
        Set-Content -Path $script:rbSettings -Value '{"clobbered":true}' -Encoding utf8
    }
    AfterAll {
        Remove-Item -Path (Split-Path $script:rbSettings -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'rollback returns true' {
        Invoke-MigratorRollback -SettingsPath $script:rbSettings | Should -BeTrue
    }
    It 'restored content is not the clobbered version' {
        $s = Read-Settings -Path $script:rbSettings
        $s.Contains('clobbered') | Should -BeFalse
    }
}
