# tests/v2/MigratorPs1.Tests.ps1 — Pester 5 tests for the PowerShell v1 parser
# extension in lib/migrator.ps1 (JUGGERNAUT_PS_V1_SCAN=1 branch).

BeforeAll {
    function Get-RepoRoot {
        if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
        return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    }
    $script:RepoRoot = Get-RepoRoot
    . (Join-Path $script:RepoRoot 'lib\schema.ps1')
    . (Join-Path $script:RepoRoot 'lib\config_manager.ps1')
    . (Join-Path $script:RepoRoot 'lib\migrator.ps1')
    . (Join-Path $script:RepoRoot 'lib\profile_paths.ps1')
    . (Join-Path $script:RepoRoot 'lib\profile_writer.ps1')
    $script:BedrockConfigPath = Join-Path $script:RepoRoot 'bedrock-config.json'
    $script:Fixtures = Join-Path $script:RepoRoot 'tests\v2\fixtures'

    $script:OldPsScan = $env:JUGGERNAUT_PS_V1_SCAN
    $env:JUGGERNAUT_PS_V1_SCAN = '1'
}

AfterAll {
    if ($null -eq $script:OldPsScan) { Remove-Item Env:\JUGGERNAUT_PS_V1_SCAN -ErrorAction SilentlyContinue }
    else { $env:JUGGERNAUT_PS_V1_SCAN = $script:OldPsScan }
}

# ---------------------------------------------------------------------------
# Detection: Test-MigratorHasV1Block works on PS profiles (no new logic needed)
# ---------------------------------------------------------------------------
Describe 'Test-MigratorHasV1Block — PowerShell fixtures' {
    It 'detects v1 block in powershell_v1_iam.ps1' {
        Test-MigratorHasV1Block -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_iam.ps1') | Should -BeTrue
    }
    It 'detects v1 block in powershell_v1_apikey_profile.ps1' {
        Test-MigratorHasV1Block -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_apikey_profile.ps1') | Should -BeTrue
    }
    It 'detects v1 block in powershell_v1_keychain.ps1' {
        Test-MigratorHasV1Block -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_keychain.ps1') | Should -BeTrue
    }
    It 'detects v1 block in powershell_v1_opusplan.ps1' {
        Test-MigratorHasV1Block -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_opusplan.ps1') | Should -BeTrue
    }
}

# ---------------------------------------------------------------------------
# Parser: IAM profile
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — powershell_v1_iam.ps1' {
    BeforeAll {
        $raw = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_iam.ps1')
        $script:parsedPsIam = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode = iam'       { $script:parsedPsIam.authMode    | Should -Be 'iam' }
    It 'region = us-west-2'   { $script:parsedPsIam.region      | Should -Be 'us-west-2' }
    It 'storage = profile'    { $script:parsedPsIam.storage     | Should -Be 'profile' }
    It 'effortLevel = xhigh'  { $script:parsedPsIam.effortLevel | Should -Be 'xhigh' }
    It 'opusPlan = false'     { $script:parsedPsIam.opusPlan    | Should -BeFalse }
    It 'model parsed from $env:ANTHROPIC_MODEL' {
        $script:parsedPsIam.model | Should -Be 'global.anthropic.claude-sonnet-4-6'
    }
    It 'legacyEnv has AWS_REGION' {
        $script:parsedPsIam.legacyEnv.Contains('AWS_REGION') | Should -BeTrue
        $script:parsedPsIam.legacyEnv['AWS_REGION'] | Should -Be 'us-west-2'
    }
    It 'legacyEnv has CLAUDE_CODE_USE_BEDROCK' {
        $script:parsedPsIam.legacyEnv.Contains('CLAUDE_CODE_USE_BEDROCK') | Should -BeTrue
    }
}

# ---------------------------------------------------------------------------
# Parser: API key (plaintext in profile)
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — powershell_v1_apikey_profile.ps1' {
    BeforeAll {
        $raw = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_apikey_profile.ps1')
        $script:parsedPsApiKey = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode = bedrock-api-key (from metadata comment)' {
        $script:parsedPsApiKey.authMode | Should -Be 'bedrock-api-key'
    }
    It 'region = eu-west-1'   { $script:parsedPsApiKey.region  | Should -Be 'eu-west-1' }
    It 'storage = profile'    { $script:parsedPsApiKey.storage | Should -Be 'profile' }
    It 'legacyEnv captures AWS_BEARER_TOKEN_BEDROCK from $env: line' {
        $script:parsedPsApiKey.legacyEnv.Contains('AWS_BEARER_TOKEN_BEDROCK') | Should -BeTrue
        $script:parsedPsApiKey.legacyEnv['AWS_BEARER_TOKEN_BEDROCK'] | Should -Be 'my-test-api-key-plaintext'
    }
}

# ---------------------------------------------------------------------------
# Parser: API key with keychain (heredoc — must NOT parse token from function body)
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — powershell_v1_keychain.ps1' {
    BeforeAll {
        $raw = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_keychain.ps1')
        $script:parsedPsKeychain = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'authMode = bedrock-api-key (from metadata comment)' {
        $script:parsedPsKeychain.authMode | Should -Be 'bedrock-api-key'
    }
    It 'storage = keychain (from metadata comment)' {
        $script:parsedPsKeychain.storage | Should -Be 'keychain'
    }
    It 'region = us-east-1'  { $script:parsedPsKeychain.region | Should -Be 'us-east-1' }
    It 'legacyEnv does NOT contain a literal API key value from heredoc' {
        # The keychain Get-JuggernautBedrockApiKey function body contains
        # $env:AWS_BEARER_TOKEN_BEDROCK = Get-JuggernautBedrockApiKey which is NOT
        # a simple string assignment; the parser must not extract a bogus value.
        $val = $script:parsedPsKeychain.legacyEnv['AWS_BEARER_TOKEN_BEDROCK']
        # Either absent (correct) or if present it should be the function-call string, not a secret.
        if ($null -ne $val) {
            $val | Should -Not -Match '^[A-Za-z0-9+/]{20,}'
        }
        # The important assertion: storage correctly stayed 'keychain'.
        $script:parsedPsKeychain.storage | Should -Be 'keychain'
    }
}

# ---------------------------------------------------------------------------
# Parser: opusplan
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — powershell_v1_opusplan.ps1' {
    BeforeAll {
        $raw = Get-MigratorV1BlockRaw -ProfileFile (Join-Path $script:Fixtures 'powershell_v1_opusplan.ps1')
        $script:parsedPsOpus = ConvertFrom-MigratorV1Block -RawBlock $raw
    }

    It 'opusPlan = true'      { $script:parsedPsOpus.opusPlan    | Should -BeTrue }
    It 'effortLevel = high'   { $script:parsedPsOpus.effortLevel | Should -Be 'high' }
    It 'region = us-west-2'   { $script:parsedPsOpus.region      | Should -Be 'us-west-2' }
    It 'authMode = iam'       { $script:parsedPsOpus.authMode    | Should -Be 'iam' }
}

# ---------------------------------------------------------------------------
# PS v1 auth-mode override: $env:AWS_BEARER_TOKEN_BEDROCK without metadata comment
# ---------------------------------------------------------------------------
Describe 'ConvertFrom-MigratorV1Block — PS profile with $env:AWS_BEARER_TOKEN_BEDROCK, no metadata comment' {
    It 'infers authMode=bedrock-api-key from $env: line when metadata comment is absent' {
        $oldScan = $env:JUGGERNAUT_PS_V1_SCAN
        $rawBlock = @'
# BEGIN: Claude Code Bedrock Configuration
$env:AWS_REGION = 'us-west-2'
$env:CLAUDE_CODE_USE_BEDROCK = '1'
$env:AWS_BEARER_TOKEN_BEDROCK = 'inferred-key'
# END: Claude Code Bedrock Configuration
'@
        try {
            $env:JUGGERNAUT_PS_V1_SCAN = '1'
            $parsed = ConvertFrom-MigratorV1Block -RawBlock $rawBlock
            $parsed.authMode | Should -Be 'bedrock-api-key'
            $parsed.storage  | Should -Be 'profile'
            $parsed.legacyEnv.Contains('AWS_BEARER_TOKEN_BEDROCK') | Should -BeTrue
        } finally {
            if ($null -eq $oldScan) { Remove-Item Env:\JUGGERNAUT_PS_V1_SCAN -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_PS_V1_SCAN = $oldScan }
        }
    }
}

# ---------------------------------------------------------------------------
# Full round-trip: Invoke-MigratorRun on a PowerShell v1 profile
# ---------------------------------------------------------------------------
Describe 'Invoke-MigratorRun — round-trip from powershell_v1_iam.ps1' {
    BeforeAll {
        $tmpDir      = Join-Path ([IO.Path]::GetTempPath()) ("jug-mig-ps-" + [Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
        $script:psSettingsPath = Join-Path $tmpDir 'settings.json'
        $script:psProfilePath  = Join-Path $tmpDir 'profile.ps1'
        Copy-Item (Join-Path $script:Fixtures 'powershell_v1_iam.ps1') $script:psProfilePath
    }
    AfterAll {
        Remove-Item -Path (Split-Path $script:psSettingsPath -Parent) -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'Invoke-MigratorRun returns true' {
        $result = Invoke-MigratorRun -ProfileFile $script:psProfilePath `
                                     -SettingsPath $script:psSettingsPath `
                                     -BedrockConfigPath $script:BedrockConfigPath
        $result | Should -BeTrue
    }
    It 'settings.json has juggernaut.meta.managedBy = juggernaut' {
        $s = Read-Settings -Path $script:psSettingsPath
        $s['juggernaut']['meta']['managedBy'] | Should -Be 'juggernaut'
    }
    It 'settings.json has auth.region = us-west-2' {
        $s = Read-Settings -Path $script:psSettingsPath
        $s['juggernaut']['auth']['region'] | Should -Be 'us-west-2'
    }
    It 'settings.json has env.AWS_REGION = us-west-2' {
        $s = Read-Settings -Path $script:psSettingsPath
        $s['env']['AWS_REGION'] | Should -Be 'us-west-2'
    }
}

# ---------------------------------------------------------------------------
# Opt-in gate: without JUGGERNAUT_PS_V1_SCAN=1, $env: lines are not parsed
# ---------------------------------------------------------------------------
Describe 'JUGGERNAUT_PS_V1_SCAN opt-in gate' {
    It 'without opt-in, $env: lines are NOT parsed for region' {
        $oldScan = $env:JUGGERNAUT_PS_V1_SCAN
        try {
            Remove-Item Env:\JUGGERNAUT_PS_V1_SCAN -ErrorAction SilentlyContinue
            $rawBlock = @'
# BEGIN: Claude Code Bedrock Configuration
$env:AWS_REGION = 'ap-southeast-1'
$env:CLAUDE_CODE_USE_BEDROCK = '1'
# END: Claude Code Bedrock Configuration
'@
            $parsed = ConvertFrom-MigratorV1Block -RawBlock $rawBlock
            # Without opt-in, region falls back to default 'us-east-1' (no metadata comment, no export lines).
            $parsed.region | Should -Be 'us-east-1'
        } finally {
            if ($null -eq $oldScan) { Remove-Item Env:\JUGGERNAUT_PS_V1_SCAN -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_PS_V1_SCAN = $oldScan }
        }
    }
}

Describe 'migrate.ps1 -Clean cleanup' {
    It 'removes stale marked v2 fallback blocks even when no v1 blocks remain' {
        $tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-clean-ps-" + [Guid]::NewGuid().ToString('N'))
        $profilePath = Join-Path $tmpHome 'PowerShell\profile.ps1'
        $settingsPath = Join-Path $tmpHome '.claude\settings.json'
        $oldHome = $env:HOME; $oldProfile = $env:USERPROFILE; $oldTargets = $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS
        try {
            $env:HOME = $tmpHome; $env:USERPROFILE = $tmpHome
            $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $profilePath
            New-Item -ItemType Directory -Path (Split-Path $profilePath -Parent) -Force | Out-Null
            @'
# BEGIN: Claude Code Bedrock Configuration
# Juggernaut v2 shell fallback
$env:AWS_REGION = 'us-west-2'
# END: Claude Code Bedrock Configuration
'@ | Set-Content -Path $profilePath -Encoding utf8

            $block = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Storage 'profile' `
                -UseMantle $false -ShellFallbackMode 'settings-only' -Scope 'user' `
                -BedrockConfigPath $script:BedrockConfigPath
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block `
                -NativeKeys (Get-NativeKeysFromJuggernautBlock -Block $block)
            Write-SettingsAtomic -Path $settingsPath -Content $merged

            & (Join-Path $script:RepoRoot 'commands\migrate.ps1') -Clean -Yes
            $LASTEXITCODE | Should -Be 0
            (Get-Content $profilePath -Raw) | Should -Not -Match 'BEGIN: Claude Code Bedrock Configuration'
        } finally {
            $env:HOME = $oldHome; $env:USERPROFILE = $oldProfile
            if ($null -eq $oldTargets) { Remove-Item Env:\JUGGERNAUT_POWERSHELL_PROFILE_TARGETS -ErrorAction SilentlyContinue }
            else { $env:JUGGERNAUT_POWERSHELL_PROFILE_TARGETS = $oldTargets }
            Remove-Item -Path $tmpHome -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}
