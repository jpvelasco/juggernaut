# tests/v2/Doctor.Tests.ps1 — Pester 5 tests for v3 commands/doctor.ps1.

Describe 'doctor.ps1' {
    BeforeAll {
        function Get-RepoRoot {
            if ($env:GITHUB_WORKSPACE) { return $env:GITHUB_WORKSPACE }
            return (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
        }

        $script:repoRoot = Get-RepoRoot
        . (Join-Path $script:repoRoot 'lib\schema.ps1')
        . (Join-Path $script:repoRoot 'lib\config_manager.ps1')
        . (Join-Path $script:repoRoot 'lib\keychain.ps1')
        $script:BedrockConfigPath = Join-Path $script:repoRoot 'bedrock-config.json'
        $script:ExpectedVersion   = (Get-Content (Join-Path $script:repoRoot 'VERSION') -Raw).Trim()
    }

    Context 'fresh IAM apply passes doctor' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-doc-ok-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH
            $script:oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK
            $script:oldAwsProfile = $env:AWS_PROFILE
            $script:oldKeychainService = $env:JUGGERNAUT_KEYCHAIN_SERVICE

            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-doctor-$([Guid]::NewGuid().ToString('N'))"
            $env:AWS_PROFILE = 'juggernaut-pester-iam'
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            $block = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'profile' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
            Write-SettingsAtomic -Path (Join-Path $script:tmpHome '.claude\settings.json') -Content $merged

            $script:output = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
        }
        AfterAll {
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock
            $env:AWS_BEARER_TOKEN_BEDROCK = $script:oldBearer
            $env:AWS_PROFILE = $script:oldAwsProfile
            if ($null -eq $script:oldKeychainService) {
                Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
            } else {
                $env:JUGGERNAUT_KEYCHAIN_SERVICE = $script:oldKeychainService
            }
            Remove-Item -Recurse -Force $script:tmpHome -ErrorAction SilentlyContinue
        }

        It 'shows User Scope header'    { $script:output | Should -Match 'User Scope' }
        It 'shows Project Scope header' { $script:output | Should -Match 'Project Scope' }
        It 'shows Active Scope header'  { $script:output | Should -Match 'Active Scope' }
        It 'shows Credentials header'   { $script:output | Should -Match 'Credentials' }
        It 'shows Region & Models'      { $script:output | Should -Match 'Region & Models' }
        It 'shows Mantle section'       { $script:output | Should -Match 'Mantle' }
        It 'shows Opusplan section'     { $script:output | Should -Match 'Opusplan' }
        It 'does NOT show profile drift section' { $script:output | Should -Not -Match 'Drift' }
        It 'does NOT show Shell Fallback section' { $script:output | Should -Not -Match 'Shell Fallback' }
        It 'Auth: IAM surfaced'         { $script:output | Should -Match 'Auth: IAM' }
        It 'Region: us-west-2 (OK)'     { $script:output | Should -Match 'Region: us-west-2 \(OK\)' }
    }

    Context 'opusplan drift detection' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-doc-op-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH

            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:AWS_PROFILE = 'juggernaut-pester-iam'

            $block = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
                -Region 'us-west-2' -OpusPlan $true -Storage 'profile' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native

            $script:settingsPath = Join-Path $script:tmpHome '.claude\settings.json'
            Write-SettingsAtomic -Path $script:settingsPath -Content $merged

            # Clean run — matches, no drift.
            $script:okOutput = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String

            # Drift: tamper with env.ANTHROPIC_MODEL at top level.
            $json = Get-Content $script:settingsPath -Raw | ConvertFrom-Json
            $json.env.ANTHROPIC_MODEL = 'global.anthropic.claude-sonnet-4-6'
            $json | ConvertTo-Json -Depth 20 | Set-Content -Path $script:settingsPath -Encoding utf8
            $script:driftOutput = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
        }
        AfterAll {
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock
            Remove-Item -Recurse -Force $script:tmpHome -ErrorAction SilentlyContinue
        }

        It 'opusplan enabled + env matches → no mismatch warning' {
            $script:okOutput | Should -Match 'Opusplan'
            $script:okOutput | Should -Not -Match 'ANTHROPIC_MODEL mismatch'
        }
        It 'opusplan enabled + env overridden → mismatch warning with fix hint' {
            $script:driftOutput | Should -Match 'ANTHROPIC_MODEL mismatch'
            $script:driftOutput | Should -Match 'juggernaut apply --opusplan'
        }
    }

    Context 'bedrock-api-key auth with bearer token' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-doc-api-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH
            $script:oldBearer = $env:AWS_BEARER_TOKEN_BEDROCK

            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:AWS_BEARER_TOKEN_BEDROCK = 'br-test'

            $block = New-JuggernautBlock -AuthMode 'bedrock-api-key' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'profile' -UseMantle $true `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
            Write-SettingsAtomic -Path (Join-Path $script:tmpHome '.claude\settings.json') -Content $merged

            $script:output = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
        }
        AfterAll {
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock
            $env:AWS_BEARER_TOKEN_BEDROCK = $script:oldBearer
            Remove-Item -Recurse -Force $script:tmpHome -ErrorAction SilentlyContinue
        }

        It 'reports Bedrock API key auth' { $script:output | Should -Match 'Auth: Bedrock API key' }
        It 'reports AWS_BEARER_TOKEN_BEDROCK source' { $script:output | Should -Match 'Source: AWS_BEARER_TOKEN_BEDROCK' }
    }

    Context 'top-level .model poisoning' {
        BeforeAll {
            $script:tmpHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-doc-poison-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome '.claude') -Force | Out-Null
            $script:oldHome = $env:HOME; $script:oldProfile = $env:USERPROFILE
            $script:oldBedrock = $env:BEDROCK_CONFIG_PATH
            $env:HOME = $script:tmpHome; $env:USERPROFILE = $script:tmpHome
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:AWS_PROFILE = 'juggernaut-pester-iam'

            $block = New-JuggernautBlock -AuthMode 'iam' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'profile' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
            $script:settingsPath = Join-Path $script:tmpHome '.claude\settings.json'
            Write-SettingsAtomic -Path $script:settingsPath -Content $merged

            # Tamper: overwrite top-level .model with the literal "opusplan".
            $json = Get-Content $script:settingsPath -Raw | ConvertFrom-Json
            $json.model = 'opusplan'
            $json | ConvertTo-Json -Depth 20 | Set-Content -Path $script:settingsPath -Encoding utf8

            $script:output = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
        }
        AfterAll {
            $env:HOME = $script:oldHome; $env:USERPROFILE = $script:oldProfile
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock
            Remove-Item -Recurse -Force $script:tmpHome -ErrorAction SilentlyContinue
        }
        It 'emits Top-level model WARN'          { $script:output | Should -Match 'Top-level model: WARN' }
        It 'explains opusplan is not a model ID' { $script:output | Should -Match 'not a Bedrock model ID' }
        It 'points at juggernaut apply as fix'   { $script:output | Should -Match 'juggernaut apply' }
    }

    Context 'fresh install with no Juggernaut config' {
        BeforeAll {
            $script:tmpNoConfigHome = Join-Path ([IO.Path]::GetTempPath()) ("jug-doc-empty-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpNoConfigHome '.claude') -Force | Out-Null
            $script:oldNoConfigHome = $env:HOME
            $script:oldNoConfigProfile = $env:USERPROFILE
            $env:HOME = $script:tmpNoConfigHome
            $env:USERPROFILE = $script:tmpNoConfigHome

            '{}' | Set-Content -Path (Join-Path $script:tmpNoConfigHome '.claude\settings.json') -Encoding utf8
            $script:noConfigOutput = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
        }
        AfterAll {
            $env:HOME = $script:oldNoConfigHome
            $env:USERPROFILE = $script:oldNoConfigProfile
            Remove-Item -Recurse -Force $script:tmpNoConfigHome -ErrorAction SilentlyContinue
        }

        It 'does not recommend bare apply' { $script:noConfigOutput | Should -Not -Match "Run 'juggernaut apply'" }
        It 'recommends explicit IAM auth' { $script:noConfigOutput | Should -Match 'juggernaut apply -Auth iam' }
        It 'recommends explicit Bedrock API key auth' { $script:noConfigOutput | Should -Match 'juggernaut apply -Auth bedrock-api-key' }
    }

    Context 'help and unknown flags' {
        It '--help exits cleanly and mentions v3' {
            $out = & (Join-Path $script:repoRoot 'commands\doctor.ps1') --help 2>&1 | Out-String
            $out | Should -Match 'Juggernaut v3'
            $out | Should -Not -Match 'JUGGERNAUT_USE_V2'
        }
        It 'unknown flag exits non-zero and names the flag' {
            $out = & (Join-Path $script:repoRoot 'commands\doctor.ps1') --not-a-real-flag 2>&1 | Out-String
            $LASTEXITCODE | Should -Not -Be 0
            $out | Should -Match 'unknown option'
            $out | Should -Match 'not-a-real-flag'
        }
    }

    Context 'bedrock-api-key with profile token storage detected by doctor' {
        BeforeAll {
            $script:tmpHome2 = Join-Path ([IO.Path]::GetTempPath()) ("jug-doc-prof-" + [Guid]::NewGuid().ToString('N'))
            New-Item -ItemType Directory -Path (Join-Path $script:tmpHome2 '.claude') -Force | Out-Null
            $script:oldHome2 = $env:HOME; $script:oldProfile2 = $env:USERPROFILE
            $script:oldBedrock2 = $env:BEDROCK_CONFIG_PATH
            $script:oldBearer2 = $env:AWS_BEARER_TOKEN_BEDROCK
            $script:oldKeychainService2 = $env:JUGGERNAUT_KEYCHAIN_SERVICE
            $script:oldProfileTokenPath = $env:JUGGERNAUT_PROFILE_TOKEN_PATH

            $env:HOME = $script:tmpHome2; $env:USERPROFILE = $script:tmpHome2
            $env:BEDROCK_CONFIG_PATH = $script:BedrockConfigPath
            $env:JUGGERNAUT_KEYCHAIN_SERVICE = "juggernaut-absent-docprof-$([Guid]::NewGuid().ToString('N'))"
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue

            # Write profile token directly
            $profFile = Join-Path $script:tmpHome2 '.config\juggernaut\bearer-token'
            New-Item -ItemType Directory -Path (Split-Path -Parent $profFile) -Force | Out-Null
            'sk-brk-doctor-profile-test' | Set-Content -Path $profFile -Encoding utf8 -NoNewline
            $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $profFile

            $block = New-JuggernautBlock -AuthMode 'bedrock-api-key' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'profile' -UseMantle $true `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $native = Get-NativeKeysFromJuggernautBlock -Block $block
            $merged = Merge-JuggernautBlock -Existing ([ordered]@{}) -NewBlock $block -NativeKeys $native
            Write-SettingsAtomic -Path (Join-Path $script:tmpHome2 '.claude\settings.json') -Content $merged

            $script:profOutput = & (Join-Path $script:repoRoot 'commands\doctor.ps1') 2>&1 | Out-String
        }
        AfterAll {
            $env:HOME = $script:oldHome2; $env:USERPROFILE = $script:oldProfile2
            $env:BEDROCK_CONFIG_PATH = $script:oldBedrock2
            $env:AWS_BEARER_TOKEN_BEDROCK = $script:oldBearer2
            if ($null -eq $script:oldKeychainService2) {
                Remove-Item Env:\JUGGERNAUT_KEYCHAIN_SERVICE -ErrorAction SilentlyContinue
            } else {
                $env:JUGGERNAUT_KEYCHAIN_SERVICE = $script:oldKeychainService2
            }
            if ($null -eq $script:oldProfileTokenPath) {
                Remove-Item Env:\JUGGERNAUT_PROFILE_TOKEN_PATH -ErrorAction SilentlyContinue
            } else {
                $env:JUGGERNAUT_PROFILE_TOKEN_PATH = $script:oldProfileTokenPath
            }
            Remove-Item -Recurse -Force $script:tmpHome2 -ErrorAction SilentlyContinue
        }

        It 'reports Bedrock API key auth' { $script:profOutput | Should -Match 'Auth: Bedrock API key' }
        It 'reports profile token file as source' { $script:profOutput | Should -Match 'profile token file' }
        It 'reports Storage: profile' { $script:profOutput | Should -Match 'Storage: profile' }
    }

    Context 'Write-DoctorLauncher source label reflects auth.storage' {
        BeforeAll {
            . (Join-Path $script:repoRoot 'lib\doctor.ps1')
            $script:oldBearerLabel = $env:AWS_BEARER_TOKEN_BEDROCK
            Remove-Item Env:\AWS_BEARER_TOKEN_BEDROCK -ErrorAction SilentlyContinue
        }
        AfterAll {
            if ($null -ne $script:oldBearerLabel) {
                $env:AWS_BEARER_TOKEN_BEDROCK = $script:oldBearerLabel
            }
        }
        # Pester Mock intercepts Test-LauncherInstalled within this Context so
        # Write-DoctorLauncher sees a launcher installed at a fake path.
        BeforeEach {
            Mock Test-LauncherInstalled { return @{ Installed = $true; Path = 'C:\fake\profile.ps1' } }
        }

        It 'labels profile storage as profile token file via launcher' {
            $block = New-JuggernautBlock -AuthMode 'bedrock-api-key' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'profile' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $out = Write-DoctorLauncher -Block $block | Out-String
            $out | Should -Match 'profile token file via launcher'
        }

        It 'labels keychain storage as system keychain via launcher' {
            $block = New-JuggernautBlock -AuthMode 'bedrock-api-key' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'keychain' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $out = Write-DoctorLauncher -Block $block | Out-String
            $out | Should -Match 'system keychain via launcher'
        }

        It 'labels dpapi storage as DPAPI file via launcher' {
            $block = New-JuggernautBlock -AuthMode 'bedrock-api-key' -AuthValidated $true `
                -Region 'us-west-2' -Storage 'dpapi' -UseMantle $false `
                -Version $script:ExpectedVersion `
                -BedrockConfigPath $script:BedrockConfigPath
            $out = Write-DoctorLauncher -Block $block | Out-String
            $out | Should -Match 'DPAPI file via launcher'
        }
    }
}
