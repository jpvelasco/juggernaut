# tests/v2/Schema.Tests.ps1 — Pester 5.x tests for lib/schema.ps1.
# Mirrors coverage of tests/v2/test_schema.sh. Invoke with:
#   Invoke-Pester -Path tests/v2/Schema.Tests.ps1

BeforeAll {
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
    . (Join-Path $repoRoot 'lib/schema.ps1')
    $script:BedrockConfigPath = Join-Path $repoRoot 'bedrock-config.json'
}

Describe 'New-JuggernautBlock — defaults' {
    BeforeAll {
        $script:block = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
    }

    It 'sets managedBy = juggernaut' {
        $script:block.meta.managedBy | Should -Be 'juggernaut'
    }
    It 'schemaVersion is 1' {
        $script:block.schemaVersion | Should -Be 1
    }
    It 'useMantle defaults to true' {
        $script:block.useMantle | Should -BeTrue
    }
    It 'default region is us-east-1' {
        $script:block.auth.region | Should -Be 'us-east-1'
    }
    It 'default effortLevel is xhigh' {
        $script:block.effortLevel | Should -Be 'xhigh'
    }
    It 'useMantle=true emits CLAUDE_CODE_USE_MANTLE=1 in env' {
        $script:block.env['CLAUDE_CODE_USE_MANTLE'] | Should -Be '1'
    }
    It 'absent mantle baseUrl does NOT emit ANTHROPIC_BEDROCK_MANTLE_BASE_URL' {
        $script:block.env.Contains('ANTHROPIC_BEDROCK_MANTLE_BASE_URL') | Should -BeFalse
    }
    It 'AWS_REGION in env is derived from auth.region' {
        $script:block.env['AWS_REGION'] | Should -Be $script:block.auth.region
    }
}

Describe 'New-JuggernautBlock — useMantle=false' {
    It 'omits both Mantle env keys' {
        $b = New-JuggernautBlock -UseMantle:$false -BedrockConfigPath $script:BedrockConfigPath
        $b.env.Contains('CLAUDE_CODE_USE_MANTLE')             | Should -BeFalse
        $b.env.Contains('ANTHROPIC_BEDROCK_MANTLE_BASE_URL')  | Should -BeFalse
    }
}

Describe 'New-JuggernautBlock — mantle baseUrl' {
    It 'writes baseUrl to env and to juggernaut.mantle.baseUrl' {
        $b = New-JuggernautBlock -MantleBaseUrl 'https://mantle.example.com' -BedrockConfigPath $script:BedrockConfigPath
        $b.env['ANTHROPIC_BEDROCK_MANTLE_BASE_URL'] | Should -Be 'https://mantle.example.com'
        $b.mantle.baseUrl                           | Should -Be 'https://mantle.example.com'
    }
}

Describe 'New-JuggernautBlock — opusplan' {
    It 'opusplan=true sets ANTHROPIC_MODEL literal "opusplan"' {
        $b = New-JuggernautBlock -OpusPlan:$true -BedrockConfigPath $script:BedrockConfigPath
        $b.env['ANTHROPIC_MODEL'] | Should -Be 'opusplan'
    }
}

Describe 'New-JuggernautBlock — region override' {
    It 'passes through to both auth.region and env.AWS_REGION' {
        $b = New-JuggernautBlock -Region 'us-west-2' -BedrockConfigPath $script:BedrockConfigPath
        $b.auth.region         | Should -Be 'us-west-2'
        $b.env['AWS_REGION']   | Should -Be 'us-west-2'
    }
}

Describe 'Test-JuggernautBlock — validation' {
    BeforeAll {
        $script:good = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
    }

    It 'accepts a default block' {
        Test-JuggernautBlock -Block $script:good 2>$null | Should -BeTrue
    }
    It 'rejects empty auth.region' {
        $bad = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
        $bad.auth.region = ''
        Test-JuggernautBlock -Block $bad 2>$null | Should -BeFalse
    }
    It 'rejects unsupported auth.region' {
        $bad = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
        $bad.auth.region = 'mars-central-1'
        Test-JuggernautBlock -Block $bad 2>$null | Should -BeFalse
    }
    It 'rejects bad managedBy' {
        $bad = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
        $bad.meta.managedBy = 'someoneElse'
        Test-JuggernautBlock -Block $bad 2>$null | Should -BeFalse
    }
}

Describe 'Get-NativeKeysFromJuggernautBlock' {
    It 'returns env + model + modelOverrides' {
        $b = New-JuggernautBlock -BedrockConfigPath $script:BedrockConfigPath
        $n = Get-NativeKeysFromJuggernautBlock -Block $b
        $n.Keys | Should -Contain 'env'
        $n.Keys | Should -Contain 'model'
        $n.Keys | Should -Contain 'modelOverrides'
        $n.env['CLAUDE_CODE_USE_BEDROCK'] | Should -Be '1'
        $n.env['AWS_REGION']              | Should -Be $b.auth.region
    }
}
