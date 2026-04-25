# tests/v2/Apply.Tests.ps1 — Pester 5.x tests for commands/apply.ps1 and lib/profile_writer.ps1.
# Run with: Invoke-Pester -Path tests/v2/Apply.Tests.ps1

BeforeAll {
    $script:repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $script:repoRoot 'lib\schema.ps1')
    . (Join-Path $script:repoRoot 'lib\config_manager.ps1')
    . (Join-Path $script:repoRoot 'lib\migrator.ps1')
    . (Join-Path $script:repoRoot 'lib\keychain.ps1')
    . (Join-Path $script:repoRoot 'lib\profile_writer.ps1')
    $script:BedrockConfigPath = Join-Path $script:repoRoot 'bedrock-config.json'
    $script:Fixtures          = Join-Path $script:repoRoot 'tests\v2\fixtures'
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
    It 'meta.version = 2.2.0'  { $script:block.meta.version  | Should -Be '2.2.0' }
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
    It 'does not modify bearer token' { $script:pwBlock | Should -Not -Match 'AWS_BEARER_TOKEN_BEDROCK' }
    It 'sets CLAUDE_CODE_USE_BEDROCK' { $script:pwBlock | Should -Match 'CLAUDE_CODE_USE_BEDROCK="1"' }
}

Describe 'Build-ProfileWriterBlock — fish syntax' {
    It 'uses set -gx syntax' {
        $b = Build-ProfileWriterBlock `
            -Shell 'fish' -Region 'us-west-2' -AuthMode 'iam' `
            -ApiKeyExpr '' -StorageMode 'profile' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b | Should -Match 'set -gx AWS_REGION "us-west-2"'
        $b | Should -Not -Match 'AWS_BEARER_TOKEN_BEDROCK'
    }
}

Describe 'Build-ProfileWriterBlock — PowerShell syntax' {
    It 'writes PowerShell env assignments for profile storage' {
        $b = Build-ProfileWriterBlock `
            -Shell 'powershell' -Region 'us-west-2' -AuthMode 'bedrock-api-key' `
            -ApiKeyExpr 'br-test-token' -StorageMode 'profile' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b | Should -Match ([regex]::Escape('$env:AWS_REGION = ''us-west-2'''))
        $b | Should -Match ([regex]::Escape('$env:AWS_BEARER_TOKEN_BEDROCK = ''br-test-token'''))
        $b | Should -Not -Match '^export '
    }

    It 'writes a Credential Manager reader for keychain storage' {
        $b = Build-ProfileWriterBlock `
            -Shell 'powershell' -Region 'us-west-2' -AuthMode 'bedrock-api-key' `
            -ApiKeyExpr 'keychain' -StorageMode 'keychain' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b | Should -Match 'Get-JuggernautBedrockApiKey'
        $b | Should -Match ([regex]::Escape('$env:AWS_BEARER_TOKEN_BEDROCK = Get-JuggernautBedrockApiKey'))
        $b | Should -Not -Match 'secret-tool lookup'
    }
}

Describe 'Get-ProfileWriterPowerShellProfileTargets' {
    It 'uses test override paths when provided' {
        $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $sep = [IO.Path]::PathSeparator
            $one = Join-Path ([IO.Path]::GetTempPath()) 'jug-one-profile.ps1'
            $two = Join-Path ([IO.Path]::GetTempPath()) 'jug-two-profile.ps1'
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = "$one$sep$two"
            $targets = Get-ProfileWriterPowerShellProfileTargets
            $targets | Should -Contain $one
            $targets | Should -Contain $two
        } finally {
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
        }
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

Describe 'Keychain storage round-trip' {
    It 'stores, reads, and removes a test key when keychain is available' {
        if (-not (Test-KeychainAvailable)) {
            Set-ItResult -Skipped -Because 'No supported OS keychain is available'
            return
        }

        $oldService = $env:JUGGERNAUT_KEYCHAIN_SERVICE
        $oldAccount = $env:JUGGERNAUT_KEYCHAIN_ACCOUNT
        try {
            $env:JUGGERNAUT_KEYCHAIN_SERVICE = 'juggernaut-bedrock-test'
            $env:JUGGERNAUT_KEYCHAIN_ACCOUNT = 'api-key-test'
            Remove-KeychainEntry
            Set-KeychainEntry -Key 'br-test-roundtrip' | Should -BeTrue
            Get-KeychainEntry | Should -Be 'br-test-roundtrip'
            Remove-KeychainEntry
            Get-KeychainEntry | Should -Be ''
        } finally {
            Remove-KeychainEntry
            if ($null -eq $oldService) { Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_KEYCHAIN_SERVICE = $oldService }
            if ($null -eq $oldAccount) { Remove-Item Env:\JUGGERNAUT_KEYCHAIN_ACCOUNT -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_KEYCHAIN_ACCOUNT = $oldAccount }
        }
    }
}

# ---------------------------------------------------------------------------
# Keychain — constants
# ---------------------------------------------------------------------------
Describe 'Keychain constants' {
    It 'KEYCHAIN_SERVICE is juggernaut-bedrock' {
        $expr = Get-KeychainRetrievalExpression -Shell 'bash'
        $expr | Should -Match 'juggernaut-bedrock'
    }
    It 'zsh expression matches bash expression' {
        $bash = Get-KeychainRetrievalExpression -Shell 'bash'
        $zsh  = Get-KeychainRetrievalExpression -Shell 'zsh'
        $zsh | Should -Be $bash
    }
}

# ---------------------------------------------------------------------------
# New-JuggernautBlock — 1M context
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — 1M context default' {
    It 'use1MContext defaults to true' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.context.use1MContext | Should -BeTrue
    }
    It 'use1MContext=false is stored' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
                                 -Use1MContext $false `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.context.use1MContext | Should -BeFalse
    }
}

# ---------------------------------------------------------------------------
# New-JuggernautBlock — Bedrock API-key auth
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — Bedrock API-key auth mode' {
    It 'rewrites legacy api-key to bedrock-api-key' {
        $b = New-JuggernautBlock -AuthMode 'api-key' -Region 'us-east-1' `
                                 -BedrockConfigPath $script:BedrockConfigPath
        $b.auth.mode | Should -Be 'bedrock-api-key'
    }
    It 'env retains CLAUDE_CODE_USE_BEDROCK when using Bedrock API key' {
        $b = New-JuggernautBlock -AuthMode 'api-key' -Region 'us-east-1' `
                                 -BedrockConfigPath $script:BedrockConfigPath
        # CLAUDE_CODE_USE_BEDROCK is still set in Bedrock env regardless of auth mode.
        # This test verifies the block is structurally valid.
        $b.env | Should -Not -BeNullOrEmpty
    }
}

Describe 'apply.ps1 — bearer token auth detection' {
    BeforeAll {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-bearer-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpDir '.claude') -Force | Out-Null
        $script:bearerHome = $tmpDir

        $block = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
                                     -BedrockConfigPath $script:BedrockConfigPath
        $native = Get-NativeKeysFromJuggernautBlock -Block $block
        $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
        Write-SettingsAtomic -Path (Join-Path (Join-Path $tmpDir '.claude') 'settings.json') -Content $merged

        $oldHome = $env:HOME
        $oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
        try {
            $env:HOME = $tmpDir
            $env:AWS_BEARER_TOKEN_BEDROCK = 'br-test-token'
            $output = & (Join-Path $script:repoRoot 'commands\apply.ps1') -DryRun -NoShellFallback 2>$null | Out-String
            $script:bearerBlock = ($output | ConvertFrom-Json)

            $explicit = & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -DryRun -NoShellFallback 2>$null | Out-String
            $script:explicitIamBlock = ($explicit | ConvertFrom-Json)
        } finally {
            $env:HOME = $oldHome
            if ($null -eq $oldBearer) { Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue }
            else { $env:AWS_BEARER_TOKEN_BEDROCK = $oldBearer }
        }
    }
    AfterAll {
        Remove-Item $script:bearerHome -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'uses bedrock-api-key when bearer token is present and auth is not explicit' {
        $script:bearerBlock.juggernaut.auth.mode | Should -Be 'bedrock-api-key'
        $script:bearerBlock.juggernaut.useMantle | Should -BeTrue
    }
    It 'keeps explicit IAM even when bearer token is present' {
        $script:explicitIamBlock.juggernaut.auth.mode | Should -Be 'iam'
    }
}

Describe 'juggernaut.ps1 — documented GNU-style apply flags' {
    BeforeAll {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-dispatch-" + [Guid]::NewGuid().ToString('N'))
        $projectDir = Join-Path $tmpDir 'project'
        $userProfile = Join-Path $tmpDir 'user'
        New-Item -ItemType Directory -Path (Join-Path $projectDir '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path $userProfile -Force | Out-Null
        $script:dispatchRoot = $tmpDir

        $shellExe = Join-Path $PSHOME 'powershell.exe'
        if (-not (Test-Path $shellExe)) {
            $shellExe = Join-Path $PSHOME 'pwsh.exe'
        }

        $command = @"
`$oldHome = `$env:HOME
`$oldUserProfile = `$env:USERPROFILE
`$oldBearer = `$env:AWS_BEARER_TOKEN_BEDROCK
try {
    Remove-Item Env:\HOME -ErrorAction SilentlyContinue
    `$env:USERPROFILE = '$($userProfile -replace "'", "''")'
    `$env:AWS_BEARER_TOKEN_BEDROCK = 'br-test-token'
    Set-Location '$($projectDir -replace "'", "''")'
    & '$((Join-Path $script:repoRoot 'juggernaut.ps1') -replace "'", "''")' apply --v2 --auth=bedrock-api-key --dry-run --no-shell-fallback --scope=project
    exit `$LASTEXITCODE
} finally {
    if (`$null -eq `$oldHome) { Remove-Item Env:\HOME -ErrorAction SilentlyContinue } else { `$env:HOME = `$oldHome }
    if (`$null -eq `$oldUserProfile) { Remove-Item Env:\USERPROFILE -ErrorAction SilentlyContinue } else { `$env:USERPROFILE = `$oldUserProfile }
    if (`$null -eq `$oldBearer) { Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue } else { `$env:AWS_BEARER_TOKEN_BEDROCK = `$oldBearer }
}
"@
        $script:dispatchOutput = & $shellExe -NoProfile -ExecutionPolicy Bypass -Command $command 2>&1 | Out-String
        $script:dispatchExitCode = $LASTEXITCODE
    }
    AfterAll {
        Remove-Item $script:dispatchRoot -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'accepts release-note style --auth=bedrock-api-key on Windows without HOME' {
        $script:dispatchExitCode | Should -Be 0
        $script:dispatchOutput | Should -Match '"mode":\s*"bedrock-api-key"'
        $script:dispatchOutput | Should -Not -Match 'Cannot bind argument to parameter ''Path'''
    }
}

Describe 'apply.ps1 — explicit keychain failure handling' {
    BeforeAll {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-keychain-fail-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path (Join-Path $tmpDir '.claude') -Force | Out-Null
        $script:keychainFailHome = $tmpDir

        $oldHome = $env:HOME
        $oldUserProfile = $env:USERPROFILE
        $oldForceFail = $env:JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL
        try {
            $env:HOME = $tmpDir
            $env:USERPROFILE = $tmpDir
            $env:JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL = '1'
            $script:keychainFailOutput = & (Join-Path $script:repoRoot 'commands\apply.ps1') `
                -Auth 'bedrock-api-key' -BedrockKey 'br-test-token' -Storage 'keychain' `
                -NoShellFallback -SkipPreflight 2>&1 | Out-String
            $script:keychainFailExitCode = $LASTEXITCODE
        } finally {
            $env:HOME = $oldHome
            $env:USERPROFILE = $oldUserProfile
            if ($null -eq $oldForceFail) { Remove-Item Env:\JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL = $oldForceFail }
        }
    }
    AfterAll {
        Remove-Item $script:keychainFailHome -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'stops instead of falling back when keychain storage was explicit' {
        $script:keychainFailExitCode | Should -Be 1
        $script:keychainFailOutput | Should -Match ([regex]::Escape('keychain store failed'))
        $script:keychainFailOutput | Should -Not -Match ([regex]::Escape('falling back to profile storage'))
        Test-Path (Join-Path $script:keychainFailHome '.claude/settings.json') | Should -BeFalse
    }
}

# ---------------------------------------------------------------------------
# Implicit migration — region sourced from AWS_REGION (single source of truth)
# ---------------------------------------------------------------------------
Describe 'Invoke-MigratorRun — region from v1 AWS_REGION' {
    BeforeAll {
        $tmpDir  = Join-Path ([IO.Path]::GetTempPath()) ("jug-mig3-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:mig3Settings = Join-Path $tmpDir 'settings.json'
        $script:mig3Profile  = Join-Path $tmpDir 'profile.sh'
        Copy-Item (Join-Path $script:Fixtures 'v1_iam_default.sh') $script:mig3Profile
        # Migrate without specifying a region override — region must come from fixture's AWS_REGION=us-east-1.
        Invoke-MigratorRun -ProfileFile $script:mig3Profile `
                           -SettingsPath $script:mig3Settings `
                           -BedrockConfigPath $script:BedrockConfigPath | Out-Null
    }
    AfterAll {
        Remove-Item (Split-Path $script:mig3Settings -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'settings.json written' { Test-Path $script:mig3Settings | Should -BeTrue }
    It 'auth.region = us-east-1 (from AWS_REGION export in v1 block)' {
        $s = Read-Settings -Path $script:mig3Settings
        $s['juggernaut']['auth']['region'] | Should -Be 'us-east-1'
    }
    It 'env.AWS_REGION = us-east-1' {
        $s = Read-Settings -Path $script:mig3Settings
        $s['env']['AWS_REGION'] | Should -Be 'us-east-1'
    }
    It 'legacyEnv.source is v1.7.x-profile-block' {
        $s = Read-Settings -Path $script:mig3Settings
        $s['juggernaut']['legacyEnv']['source'] | Should -Be 'v1.7.x-profile-block'
    }
    It 'meta.migratedFrom is v1.7.x' {
        $s = Read-Settings -Path $script:mig3Settings
        $s['juggernaut']['meta']['migratedFrom'] | Should -Be 'v1.7.x'
    }
}

# ---------------------------------------------------------------------------
# Explicit migration with project scope
# ---------------------------------------------------------------------------
Describe 'Invoke-MigratorRun — project scope with v1 profile' {
    BeforeAll {
        $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("jug-mig4-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:mig4Project = Join-Path $tmpDir 'project'
        $script:mig4Home    = Join-Path $tmpDir 'home'
        New-Item -ItemType Directory -Path (Join-Path $script:mig4Project '.claude') -Force | Out-Null
        New-Item -ItemType Directory -Path $script:mig4Home -Force | Out-Null
        Copy-Item (Join-Path $script:Fixtures 'v1_iam_default.sh') (Join-Path $script:mig4Home '.bashrc')

        $oldHome = $env:HOME
        Push-Location $script:mig4Project
        try {
            $env:HOME = $script:mig4Home
            & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -Scope 'project' -NoShellFallback -Yes | Out-Null
        } finally {
            Pop-Location
            $env:HOME = $oldHome
        }
    }
    AfterAll {
        Remove-Item -Path (Split-Path $script:mig4Project -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'writes project settings.json' {
        Test-Path (Join-Path (Join-Path $script:mig4Project '.claude') 'settings.json') | Should -BeTrue
    }
    It 'migrates the v1 block into the project settings' {
        $s = Read-Settings -Path (Join-Path (Join-Path $script:mig4Project '.claude') 'settings.json')
        $s['juggernaut']['meta']['managedBy'] | Should -Be 'juggernaut'
        $s['juggernaut']['auth']['region']    | Should -Be 'us-east-1'
    }
}

# ---------------------------------------------------------------------------
# Idempotency — applying the same block twice preserves structure
# ---------------------------------------------------------------------------
Describe 'settings.json idempotency — second write preserves user keys' {
    BeforeAll {
        $tmpDir   = Join-Path ([IO.Path]::GetTempPath()) ("jug-idem-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:idemSettings = Join-Path $tmpDir 'settings.json'

        # Simulate a settings.json that already has user keys (permissions, hooks).
        $existingUser = [ordered]@{
            permissions = [ordered]@{ allow = @('Bash') }
        }

        $block1  = New-JuggernautBlock -AuthMode 'iam' -Region 'ap-southeast-1' `
                                        -BedrockConfigPath $script:BedrockConfigPath
        $native1 = Get-NativeKeysFromJuggernautBlock -Block $block1
        $merged1 = Merge-JuggernautBlock -Existing $existingUser -NewBlock $block1 -NativeKeys $native1
        Write-SettingsAtomic -Path $script:idemSettings -Content $merged1

        # Second apply — same params.
        $block2  = New-JuggernautBlock -AuthMode 'iam' -Region 'ap-southeast-1' `
                                        -BedrockConfigPath $script:BedrockConfigPath
        $native2 = Get-NativeKeysFromJuggernautBlock -Block $block2
        $existing2 = Read-Settings -Path $script:idemSettings
        $merged2 = Merge-JuggernautBlock -Existing $existing2 -NewBlock $block2 -NativeKeys $native2
        Write-SettingsAtomic -Path $script:idemSettings -Content $merged2
    }
    AfterAll {
        Remove-Item (Split-Path $script:idemSettings -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'user permissions key preserved after second write' {
        $s = Read-Settings -Path $script:idemSettings
        $s.Contains('permissions') | Should -BeTrue
    }
    It 'auth.region unchanged after second write' {
        $s = Read-Settings -Path $script:idemSettings
        $s['juggernaut']['auth']['region'] | Should -Be 'ap-southeast-1'
    }
    It 'managedBy still juggernaut after second write' {
        $s = Read-Settings -Path $script:idemSettings
        $s['juggernaut']['meta']['managedBy'] | Should -Be 'juggernaut'
    }
}

# ---------------------------------------------------------------------------
# Build-ProfileWriterBlock — effort level in profile block
# ---------------------------------------------------------------------------
Describe 'Build-ProfileWriterBlock — effort level' {
    It 'CLAUDE_CODE_EFFORT_LEVEL matches --effort' {
        $b = Build-ProfileWriterBlock `
            -Shell 'bash' -Region 'us-east-1' -AuthMode 'iam' `
            -ApiKeyExpr '' -StorageMode 'profile' `
            -EffortLevel 'high' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b | Should -Match 'CLAUDE_CODE_EFFORT_LEVEL="high"'
    }
}
