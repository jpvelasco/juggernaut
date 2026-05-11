# tests/v2/Apply.Tests.ps1 — Pester 5 tests for v3 commands/apply.ps1 and lib/schema.ps1.
# Focus: block construction, auth validation gate, Mantle default, opusplan, scope.

BeforeAll {
    $script:repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $script:repoRoot 'lib\schema.ps1')
    . (Join-Path $script:repoRoot 'lib\config_manager.ps1')
    . (Join-Path $script:repoRoot 'lib\keychain.ps1')
    $script:BedrockConfigPath = Join-Path $script:repoRoot 'bedrock-config.json'
    $script:ExpectedVersion   = (Get-Content (Join-Path $script:repoRoot 'VERSION') -Raw).Trim()
    $env:BEDROCK_CONFIG_PATH  = $script:BedrockConfigPath
    # Isolate keychain writes during tests.
    $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-pester-$([guid]::NewGuid().Guid)"
}

AfterAll {
    Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
}

# ---------------------------------------------------------------------------
# New-JuggernautBlock — IAM with validated auth
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — IAM validated' {
    BeforeAll {
        $script:block = New-JuggernautBlock `
            -AuthMode 'iam' -AuthValidated $true `
            -Storage 'profile' -Region 'us-east-1' `
            -EffortLevel 'xhigh' `
            -BedrockConfigPath $script:BedrockConfigPath
    }

    It 'auth.mode = iam'         { $script:block.auth.mode     | Should -Be 'iam' }
    It 'auth.region = us-east-1' { $script:block.auth.region   | Should -Be 'us-east-1' }
    It 'auth.storage = profile'  { $script:block.auth.storage  | Should -Be 'profile' }
    It 'effortLevel = xhigh'     { $script:block.effortLevel   | Should -Be 'xhigh' }
    It 'opusplan = false'        { $script:block.opusplan      | Should -BeFalse }
    It 'meta.managedBy = juggernaut' { $script:block.meta.managedBy | Should -Be 'juggernaut' }
    It 'meta.version matches VERSION' { $script:block.meta.version | Should -Be $script:ExpectedVersion }
    It 'env.AWS_REGION = us-east-1' { $script:block.env['AWS_REGION'] | Should -Be 'us-east-1' }
    It 'env.CLAUDE_CODE_USE_BEDROCK = 1 (auth validated)' {
        $script:block.env['CLAUDE_CODE_USE_BEDROCK'] | Should -Be '1'
    }
}

# ---------------------------------------------------------------------------
# New-JuggernautBlock — auth NOT validated → CLAUDE_CODE_USE_BEDROCK absent
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — auth NOT validated omits CLAUDE_CODE_USE_BEDROCK' {
    It 'env has no CLAUDE_CODE_USE_BEDROCK key' {
        $b = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $false `
            -Region 'us-west-2' -BedrockConfigPath $script:BedrockConfigPath
        $b.env.Contains('CLAUDE_CODE_USE_BEDROCK') | Should -BeFalse
    }
}

# ---------------------------------------------------------------------------
# Mantle default — on by default
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — Mantle default on' {
    It 'UseMantle defaults to true and emits CLAUDE_CODE_USE_MANTLE=1' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b.useMantle | Should -BeTrue
        $b.env['CLAUDE_CODE_USE_MANTLE'] | Should -Be '1'
    }
    It 'UseMantle=false omits CLAUDE_CODE_USE_MANTLE and sets block flag false' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -UseMantle $false `
            -BedrockConfigPath $script:BedrockConfigPath
        $b.useMantle | Should -BeFalse
        $b.env.Contains('CLAUDE_CODE_USE_MANTLE') | Should -BeFalse
    }
}

# ---------------------------------------------------------------------------
# Opusplan
# ---------------------------------------------------------------------------
Describe 'New-JuggernautBlock — opusplan' {
    It 'OpusPlan=true sets env.ANTHROPIC_MODEL = opusplan' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -OpusPlan $true `
            -BedrockConfigPath $script:BedrockConfigPath
        $b.env['ANTHROPIC_MODEL'] | Should -Be 'opusplan'
        $b.opusplan               | Should -BeTrue
    }

    It 'OpusPlan=false omits ANTHROPIC_MODEL from env' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -OpusPlan $false `
            -BedrockConfigPath $script:BedrockConfigPath
        $b.env.Contains('ANTHROPIC_MODEL') | Should -Be $false
    }

    It 'does not allow opusplan as the primary model ID' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' -Model 'opusplan' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b.model | Should -Be 'global.anthropic.claude-sonnet-4-6'
    }
}

# ---------------------------------------------------------------------------
# Test-JuggernautBlock validation
# ---------------------------------------------------------------------------
Describe 'Test-JuggernautBlock' {
    It 'valid block passes' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
            -BedrockConfigPath $script:BedrockConfigPath
        Test-JuggernautBlock -Block $b | Should -BeTrue
    }
    It 'missing region fails' {
        $b = New-JuggernautBlock -AuthMode 'iam' -Region 'us-west-2' `
            -BedrockConfigPath $script:BedrockConfigPath
        $b.auth.region = ''
        Test-JuggernautBlock -Block $b 3> $null | Should -BeFalse
    }
}

# ---------------------------------------------------------------------------
# End-to-end apply.ps1 invocations
# ---------------------------------------------------------------------------
Describe 'apply.ps1 — auth validation gate' {
    BeforeEach {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path $script:fakeHome | Out-Null
        $script:oldHome = $env:HOME
        $script:oldUserProfile = $env:USERPROFILE
        $env:HOME = $script:fakeHome
        $env:USERPROFILE = $script:fakeHome
        # Clear any credential-like vars.
        Remove-Item Env:\AWS_PROFILE           -ErrorAction SilentlyContinue
        Remove-Item Env:\AWS_ACCESS_KEY_ID     -ErrorAction SilentlyContinue
        Remove-Item Env:\AWS_SECRET_ACCESS_KEY -ErrorAction SilentlyContinue
        Remove-Item Env:\AWS_SESSION_TOKEN     -ErrorAction SilentlyContinue
        Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue
    }
    AfterEach {
        $env:HOME = $script:oldHome
        $env:USERPROFILE = $script:oldUserProfile
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It '-Auth=iam -DryRun proceeds (explicit auth bypasses preflight with -SkipPreflight)' {
        $null = & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -DryRun -SkipPreflight 2>&1
        $LASTEXITCODE | Should -Be 0
    }

    It 'AWS_BEARER_TOKEN_BEDROCK present → apply proceeds without -Auth' {
        $env:AWS_BEARER_TOKEN_BEDROCK = 'br-test-token'
        try {
            $null = & (Join-Path $script:repoRoot 'commands\apply.ps1') -DryRun -SkipPreflight 2>&1
            $LASTEXITCODE | Should -Be 0
        } finally {
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue
        }
    }
}

Describe 'apply.ps1 — writes settings.json with Mantle default on' {
    BeforeAll {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path $script:fakeHome | Out-Null
        $script:oldHome = $env:HOME
        $script:oldUserProfile = $env:USERPROFILE
        $env:HOME = $script:fakeHome
        $env:USERPROFILE = $script:fakeHome

        & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -Region 'us-west-2' -SkipPreflight 2>&1 | Out-Null
        $script:settingsPath = Join-Path $script:fakeHome '.claude\settings.json'
        $script:settings = Get-Content $script:settingsPath -Raw | ConvertFrom-Json
    }
    AfterAll {
        $env:HOME = $script:oldHome
        $env:USERPROFILE = $script:oldUserProfile
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It 'settings.json written' { Test-Path $script:settingsPath | Should -BeTrue }
    It 'juggernaut.auth.mode = iam' { $script:settings.juggernaut.auth.mode | Should -Be 'iam' }
    It 'juggernaut.useMantle = true (default)' { $script:settings.juggernaut.useMantle | Should -BeTrue }
    It 'env.CLAUDE_CODE_USE_BEDROCK = 1 (auth validated)' {
        $script:settings.env.CLAUDE_CODE_USE_BEDROCK | Should -Be '1'
    }
    It 'env.CLAUDE_CODE_USE_MANTLE = 1' {
        $script:settings.env.CLAUDE_CODE_USE_MANTLE | Should -Be '1'
    }
    It 'env.AWS_REGION = us-west-2' { $script:settings.env.AWS_REGION | Should -Be 'us-west-2' }
}

Describe 'apply.ps1 — -NoMantle disables Mantle' {
    BeforeAll {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path $script:fakeHome | Out-Null
        $script:oldHome = $env:HOME
        $script:oldUserProfile = $env:USERPROFILE
        $env:HOME = $script:fakeHome
        $env:USERPROFILE = $script:fakeHome

        & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -NoMantle -SkipPreflight 2>&1 | Out-Null
        $script:settings = Get-Content (Join-Path $script:fakeHome '.claude\settings.json') -Raw | ConvertFrom-Json
    }
    AfterAll {
        $env:HOME = $script:oldHome
        $env:USERPROFILE = $script:oldUserProfile
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It 'juggernaut.useMantle = false' { $script:settings.juggernaut.useMantle | Should -BeFalse }
    It 'env.CLAUDE_CODE_USE_MANTLE absent' {
        $script:settings.env.PSObject.Properties.Name -contains 'CLAUDE_CODE_USE_MANTLE' | Should -BeFalse
    }
}

Describe 'apply.ps1 — -OpusPlan wiring' {
    BeforeAll {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path $script:fakeHome | Out-Null
        $script:oldHome = $env:HOME
        $script:oldUserProfile = $env:USERPROFILE
        $env:HOME = $script:fakeHome
        $env:USERPROFILE = $script:fakeHome

        & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -OpusPlan -SkipPreflight 2>&1 | Out-Null
        $script:settings = Get-Content (Join-Path $script:fakeHome '.claude\settings.json') -Raw | ConvertFrom-Json
    }
    AfterAll {
        $env:HOME = $script:oldHome
        $env:USERPROFILE = $script:oldUserProfile
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It 'juggernaut.opusplan = true' { $script:settings.juggernaut.opusplan | Should -BeTrue }
    It 'env.ANTHROPIC_MODEL = opusplan' { $script:settings.env.ANTHROPIC_MODEL | Should -Be 'opusplan' }
    It 'top-level model remains a Bedrock model ID' {
        $script:settings.model | Should -Be 'global.anthropic.claude-sonnet-4-6'
    }
}

Describe 'apply.ps1 — repairs opusplan poisoned primary model' {
    BeforeAll {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-poison-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path (Join-Path $script:fakeHome '.claude') | Out-Null
        $script:oldHome = $env:HOME
        $script:oldUserProfile = $env:USERPROFILE
        $env:HOME = $script:fakeHome
        $env:USERPROFILE = $script:fakeHome

        & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -Region 'us-west-2' -SkipPreflight 2>&1 | Out-Null
        $settingsPath = Join-Path $script:fakeHome '.claude\settings.json'
        $json = Get-Content $settingsPath -Raw | ConvertFrom-Json
        $json.model = 'opusplan'
        $json.juggernaut.model = 'opusplan'
        $json | ConvertTo-Json -Depth 30 | Set-Content -Path $settingsPath -Encoding utf8

        & (Join-Path $script:repoRoot 'commands\apply.ps1') -Auth 'iam' -SkipPreflight 2>&1 | Out-Null
        $script:settings = Get-Content $settingsPath -Raw | ConvertFrom-Json
    }
    AfterAll {
        $env:HOME = $script:oldHome
        $env:USERPROFILE = $script:oldUserProfile
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It 'rewrites top-level model to a Bedrock model ID' {
        $script:settings.model | Should -Be 'global.anthropic.claude-sonnet-4-6'
    }
    It 'rewrites juggernaut.model to a Bedrock model ID' {
        $script:settings.juggernaut.model | Should -Be 'global.anthropic.claude-sonnet-4-6'
    }
    It 'preserves opusplan as a mode' {
        $script:settings.juggernaut.opusplan | Should -BeTrue
        $script:settings.env.ANTHROPIC_MODEL | Should -Be 'opusplan'
    }
}

Describe 'juggernaut.ps1 version subcommand' {
    It 'prints semver string and exits 0' {
        $dispatcher = Join-Path $script:repoRoot 'juggernaut.ps1'
        $out = & $dispatcher version 2>&1 | Out-String
        $LASTEXITCODE | Should -Be 0
        $out.Trim() | Should -Match '^\d+\.\d+\.\d+'
        $out.Trim() | Should -Be $script:ExpectedVersion
    }
}

Describe 'apply.ps1 — help / unknown args' {
    It '-Help exits 0 and mentions -Auth and -NoMantle' {
        $out = & (Join-Path $script:repoRoot 'commands\apply.ps1') -Help 2>&1 | Out-String
        $LASTEXITCODE | Should -Be 0
        $out | Should -Match '-Auth'
        $out | Should -Match '-NoMantle'
    }
    It 'unknown parameter causes non-zero exit' {
        & (Join-Path $script:repoRoot 'commands\apply.ps1') -NotARealFlag 2>$null
        $LASTEXITCODE | Should -Not -Be 0
    }
}

# ---------------------------------------------------------------------------
# Piped stdin key input and short-key truncation check
# These tests spin up a child pwsh process so that [Console]::IsInputRedirected
# is true. Skipped if pwsh (PowerShell 7+) is not available.
# ---------------------------------------------------------------------------
Describe 'apply.ps1 — piped stdin key capture' -Skip:(-not (Get-Command pwsh -ErrorAction SilentlyContinue)) {
    BeforeEach {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-pipe-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path $script:fakeHome | Out-Null
    }
    AfterEach {
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It '110-char piped key is captured and written to settings.json' {
        $fakeKey = 'A' * 110
        $applyScript = Join-Path $script:repoRoot 'commands\apply.ps1'
        $null = $fakeKey | pwsh -NoProfile -NonInteractive -Command "
            `$env:HOME = '$($script:fakeHome -replace "'","''")';
            `$env:USERPROFILE = `$env:HOME;
            `$env:BEDROCK_CONFIG_PATH = '$($script:BedrockConfigPath -replace "'","''")';
            `$env:JUGGERNAUT_KEYCHAIN_SERVICE = 'juggernaut-absent-pipe-test';
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue;
            Remove-Item Env:\AWS_PROFILE -ErrorAction SilentlyContinue;
            & '$($applyScript -replace "'","''")' -Auth bedrock-api-key -Storage profile
        " 2>&1
        $LASTEXITCODE | Should -Be 0
        $settingsPath = Join-Path $script:fakeHome '.claude\settings.json'
        Test-Path $settingsPath | Should -BeTrue
        $s = Get-Content $settingsPath -Raw | ConvertFrom-Json
        $s.juggernaut.auth.mode | Should -Be 'bedrock-api-key'
    }

    It 'clipboard key is captured and written to settings.json' {
        $fakeKey = 'B' * 110
        Set-Clipboard -Value $fakeKey
        $applyScript = Join-Path $script:repoRoot 'commands\apply.ps1'
        $out = pwsh -NoProfile -NonInteractive -Command "
            `$env:HOME = '$($script:fakeHome -replace "'","''")';
            `$env:USERPROFILE = `$env:HOME;
            `$env:BEDROCK_CONFIG_PATH = '$($script:BedrockConfigPath -replace "'","''")';
            `$env:JUGGERNAUT_KEYCHAIN_SERVICE = 'juggernaut-absent-clipboard-test';
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue;
            Remove-Item Env:\AWS_PROFILE -ErrorAction SilentlyContinue;
            & '$($applyScript -replace "'","''")' -Auth bedrock-api-key -Storage profile -BedrockKeyFromClipboard
        " 2>&1 | Out-String
        $LASTEXITCODE | Should -Be 0
        $settingsPath = Join-Path $script:fakeHome '.claude\settings.json'
        Test-Path $settingsPath | Should -BeTrue
        $s = Get-Content $settingsPath -Raw | ConvertFrom-Json
        $s.juggernaut.auth.mode | Should -Be 'bedrock-api-key'
    }

    It 'short key (<40 chars) is rejected with truncation error' {
        $shortKey = 'tooshort'
        $applyScript = Join-Path $script:repoRoot 'commands\apply.ps1'
        $out = $shortKey | pwsh -NoProfile -NonInteractive -Command "
            `$env:HOME = '$($script:fakeHome -replace "'","''")';
            `$env:USERPROFILE = `$env:HOME;
            `$env:BEDROCK_CONFIG_PATH = '$($script:BedrockConfigPath -replace "'","''")';
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue;
            Remove-Item Env:\AWS_PROFILE -ErrorAction SilentlyContinue;
            & '$($applyScript -replace "'","''")' -Auth bedrock-api-key
        " 2>&1 | Out-String
        $LASTEXITCODE | Should -Not -Be 0
        $out | Should -Match 'truncated'
    }
}

Describe 'apply.ps1 — profile token storage persistence' -Skip:(-not (Get-Command pwsh -ErrorAction SilentlyContinue)) {
    BeforeEach {
        $script:fakeHome = Join-Path ([IO.Path]::GetTempPath()) "juggernaut-apply-pf-$([guid]::NewGuid().Guid)"
        New-Item -ItemType Directory -Force -Path $script:fakeHome | Out-Null
    }
    AfterEach {
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $script:fakeHome
    }

    It '-Storage profile persists the key to the profile token file' {
        $fakeKey = 'C' * 110
        $applyScript = Join-Path $script:repoRoot 'commands\apply.ps1'
        $null = $fakeKey | pwsh -NoProfile -NonInteractive -Command "
            `$env:HOME = '$($script:fakeHome -replace "'","''")';
            `$env:USERPROFILE = `$env:HOME;
            `$env:BEDROCK_CONFIG_PATH = '$($script:BedrockConfigPath -replace "'","''")';
            `$env:JUGGERNAUT_KEYCHAIN_SERVICE = 'juggernaut-absent-profile-test';
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue;
            Remove-Item Env:\AWS_PROFILE -ErrorAction SilentlyContinue;
            & '$($applyScript -replace "'","''")' -Auth bedrock-api-key -Storage profile
        " 2>&1
        $LASTEXITCODE | Should -Be 0
        $settingsPath = Join-Path $script:fakeHome '.claude\settings.json'
        Test-Path $settingsPath | Should -BeTrue
        $s = Get-Content $settingsPath -Raw | ConvertFrom-Json
        $s.juggernaut.auth.mode | Should -Be 'bedrock-api-key'
        $s.juggernaut.auth.storage | Should -Be 'profile'
        # Verify the token was written to the profile token file
        $profileDir = Join-Path $script:fakeHome '.config\juggernaut'
        $profileFile = Join-Path $profileDir 'bearer-token'
        if (Test-Path $profileFile) {
            $stored = Get-Content $profileFile -Raw
            $stored | Should -Be $fakeKey
        } else {
            # On Windows profile path is ~\.config\juggernaut\bearer-token
            # which is under $env:HOME\.config
            $altProfileDir = Join-Path $script:fakeHome '.config\juggernaut'
            $altProfileFile = Join-Path $altProfileDir 'bearer-token'
            Test-Path $altProfileFile | Should -BeTrue
            $stored = Get-Content $altProfileFile -Raw
            $stored | Should -Be $fakeKey
        }
    }
}
