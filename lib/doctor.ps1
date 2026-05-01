# lib/doctor.ps1 - read-only diagnostics for Juggernaut v2.

$script:DoctorFails = 0
$script:DoctorWarns = 0

function Show-DoctorHomePath {
    param([AllowNull()][string]$Path)
    if (-not $Path) { return '' }
    $homePath = if ($env:HOME) { $env:HOME } elseif ($env:USERPROFILE) { $env:USERPROFILE } else { '' }
    if ($homePath -and $Path.StartsWith($homePath, [System.StringComparison]::OrdinalIgnoreCase)) {
        return '~' + ($Path.Substring($homePath.Length) -replace '\\', '/')
    }
    return ($Path -replace '\\', '/')
}

function Get-DoctorProp {
    param([AllowNull()]$Object, [Parameter(Mandatory)][string]$Name)
    if ($null -eq $Object) { return $null }
    if ($Object -is [hashtable] -or $Object -is [System.Collections.Specialized.OrderedDictionary]) {
        if ($Object.Contains($Name)) { return $Object[$Name] }
        return $null
    }
    $prop = $Object.PSObject.Properties[$Name]
    if ($prop) { return $prop.Value }
    return $null
}

function Get-DoctorNestedProp {
    param([AllowNull()]$Object, [Parameter(Mandatory)][string[]]$Path)
    $cur = $Object
    foreach ($part in $Path) {
        $cur = Get-DoctorProp -Object $cur -Name $part
        if ($null -eq $cur) { return $null }
    }
    return $cur
}

function Get-DoctorShellBlock {
    param([Parameter(Mandatory)][string]$ProfilePath)
    if (-not (Test-Path $ProfilePath)) { return @() }
    $lines = Get-Content -Path $ProfilePath -Encoding utf8
    $inside = $false
    $out = New-Object System.Collections.Generic.List[string]
    foreach ($line in $lines) {
        if ($line -eq $script:ProfileWriterBegin) { $inside = $true; continue }
        if ($line -eq $script:ProfileWriterEnd) { $inside = $false; continue }
        if ($inside) { $out.Add($line) }
    }
    return $out.ToArray()
}

function Get-DoctorShellValue {
    param([Parameter(Mandatory)][string]$ProfilePath, [Parameter(Mandatory)][string]$Key)
    foreach ($line in (Get-DoctorShellBlock -ProfilePath $ProfilePath)) {
        if ($line -match "^\s*export\s+$([regex]::Escape($Key))=(.+)$") {
            return (($Matches[1] -replace '^"', '') -replace '"$', '')
        }
        if ($line -match "^\s*set\s+-gx\s+$([regex]::Escape($Key))\s+(.+)$") {
            return (($Matches[1] -replace '^"', '') -replace '"$', '')
        }
        if ($line -match "^\s*\`$env:$([regex]::Escape($Key))\s*=\s*(.+)$") {
            return ((($Matches[1] -replace "^'", '') -replace "'$", '') -replace '^"', '') -replace '"$', ''
        }
    }
    return ''
}

function Test-DoctorShellHasKeyAssignment {
    param([Parameter(Mandatory)][string]$ProfilePath, [Parameter(Mandatory)][string]$Key)
    foreach ($line in (Get-DoctorShellBlock -ProfilePath $ProfilePath)) {
        if ($line -match "^\s*export\s+$([regex]::Escape($Key))=") { return $true }
        if ($line -match "^\s*set\s+-gx\s+$([regex]::Escape($Key))\s+") { return $true }
        if ($line -match "^\s*\`$env:$([regex]::Escape($Key))\s*=") { return $true }
    }
    return $false
}

function Test-DoctorNativeKeysMatch {
    param([Parameter(Mandatory)]$Settings, [Parameter(Mandatory)]$Block)
    $expected = Get-NativeKeysFromJuggernautBlock -Block $Block
    if ((Get-DoctorProp -Object $Settings -Name 'model') -ne $expected.model) { return $false }
    foreach ($name in @('opus','sonnet','haiku','subagent')) {
        $actual = Get-DoctorNestedProp -Object $Settings -Path @('modelOverrides', $name)
        $want = Get-DoctorNestedProp -Object $expected -Path @('modelOverrides', $name)
        if ($actual -ne $want) { return $false }
    }
    foreach ($prop in $expected.env.Keys) {
        $actual = Get-DoctorNestedProp -Object $Settings -Path @('env', $prop)
        if ($actual -ne $expected.env[$prop]) { return $false }
    }
    return $true
}

function Get-DoctorProfilePath {
    param([AllowNull()]$Block)

    $profiles = @()
    if ($Block) {
        $rawProfiles = Get-DoctorNestedProp -Object $Block -Path @('shellFallback','lastWrittenProfiles')
        if ($rawProfiles) { $profiles = @($rawProfiles) }
    }
    foreach ($profilePath in $profiles) {
        if ($profilePath -and (Test-ProfileWriterHasBlock -ProfileFile $profilePath)) { return $profilePath }
    }
    if ($profiles.Count -gt 0) { return [string]$profiles[0] }

    if ($IsWindows -or $env:OS -match 'Windows') {
        $targets = @(Get-ProfileWriterPowerShellProfileTargets)
        foreach ($target in $targets) {
            if ($target -and (Test-ProfileWriterHasBlock -ProfileFile $target)) { return $target }
        }
        if ($targets.Count -gt 0) { return [string]$targets[0] }
    }

    $shellName = if ($env:SHELL) { Split-Path -Leaf $env:SHELL } else { 'bash' }
    Get-ProfileWriterShellConfigPath -Shell $shellName
}

function Write-DoctorScopeBlock {
    param([Parameter(Mandatory)][string]$Path, [AllowNull()]$Settings)
    Write-Output (Show-DoctorHomePath $Path)
    if (-not (Test-Path $Path)) {
        Write-Output 'Status: not found'
        return
    }
    if (-not $Settings) {
        $script:DoctorFails += 1
        Write-Output 'Status: FAIL'
        Write-Output 'Details: not valid JSON'
        return
    }
    if (-not (Test-HasJuggernautBlock -Settings $Settings)) {
        Write-Output 'Status: no Juggernaut config'
        return
    }
    $block = Get-JuggernautBlockFromSettings -Settings $Settings
    if (Test-JuggernautBlock -Block $block -WarningAction SilentlyContinue) {
        Write-Output 'Status: OK'
        Write-Output 'Juggernaut block: present and valid'
    } else {
        $script:DoctorFails += 1
        Write-Output 'Status: FAIL'
        Write-Output 'Juggernaut block: present but invalid'
    }
}

function Write-DoctorCredentials {
    param([Parameter(Mandatory)]$Block, [AllowNull()][string]$ProfilePath)
    $authMode = Get-DoctorNestedProp -Object $Block -Path @('auth','mode')
    $storage = Get-DoctorNestedProp -Object $Block -Path @('auth','storage')
    if ($authMode -eq 'api-key') { $authMode = 'bedrock-api-key' }
    $keychainEntry = $null
    $keychainError = ''
    if ($authMode -eq 'bedrock-api-key' -and $storage -eq 'keychain' -and (Test-KeychainAvailable)) {
        try { $keychainEntry = Get-KeychainEntry } catch { $keychainError = "$_"; $keychainEntry = $null }
    }
    switch ($authMode) {
        'iam' {
            Write-Output 'Auth: IAM'
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                # Bearer token present but config says IAM — surface as the primary status.
                $script:DoctorWarns += 1
                Write-Output 'Status: WARN'
                Write-Output "Details: AWS_BEARER_TOKEN_BEDROCK is set but auth mode is 'iam' - possible misconfiguration"
                Write-Output 'Fix: run: juggernaut apply --v2 (auto-corrects to bedrock-api-key)'
            } elseif ($env:AWS_PROFILE) {
                Write-Output 'Status: OK'
                Write-Output 'Details: AWS_PROFILE is set'
            } elseif ($env:AWS_ACCESS_KEY_ID -and $env:AWS_SECRET_ACCESS_KEY) {
                Write-Output 'Status: OK'
                Write-Output 'Details: access key variables are set'
            } else {
                $script:DoctorWarns += 1
                Write-Output 'Status: WARN'
                Write-Output 'Details: no IAM credentials in environment'
            }
        }
        'bedrock-api-key' {
            Write-Output 'Auth: Bedrock API key'
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                Write-Output 'Source: AWS_BEARER_TOKEN_BEDROCK'
                Write-Output 'Status: OK'
            } elseif ($keychainEntry) {
                Write-Output 'Source: system keychain'
                Write-Output 'Status: OK'
            } elseif ($ProfilePath -and (Test-DoctorShellHasKeyAssignment -ProfilePath $ProfilePath -Key 'AWS_BEARER_TOKEN_BEDROCK')) {
                Write-Output 'Source: shell profile'
                Write-Output 'Status: OK'
            } else {
                if ($keychainError) {
                    $script:DoctorWarns += 1
                    Write-Output ('Keychain: WARN ({0})' -f $keychainError)
                }
                $script:DoctorFails += 1
                Write-Output 'Status: FAIL'
                Write-Output 'Details: no API key found in env, keychain, or shell profile'
            }
        }
        default {
            $script:DoctorFails += 1
            Write-Output 'Status: FAIL'
            Write-Output 'Details: missing or unsupported auth mode'
        }
    }
}

function Write-DoctorRegionModels {
    param([Parameter(Mandatory)]$Block)
    $region = Get-DoctorNestedProp -Object $Block -Path @('auth','region')
    $model = Get-DoctorProp -Object $Block -Name 'model'
    $effort = Get-DoctorProp -Object $Block -Name 'effortLevel'
    if ($region -and (Test-SchemaSupportedRegion -Region $region)) {
        Write-Output ('Region: {0} (OK)' -f $region)
    } else {
        $script:DoctorFails += 1
        $displayRegion = if ($region) { $region } else { '-' }
        Write-Output ('Region: {0} (FAIL)' -f $displayRegion)
    }
    if ($model) {
        Write-Output ('Model: {0} (OK)' -f $model)
    } else {
        $script:DoctorFails += 1
        Write-Output 'Model: - (FAIL)'
    }
    $displayEffort = if ($effort) { $effort } else { '-' }
    Write-Output ('Effort: {0}' -f $displayEffort)
    $overrideNames = @('opus','sonnet','haiku','subagent')
    $missing = $false
    foreach ($name in $overrideNames) {
        if (-not (Get-DoctorNestedProp -Object $Block -Path @('modelOverrides', $name))) { $missing = $true }
    }
    if ($missing) {
        $script:DoctorWarns += 1
        Write-Output 'Overrides: WARN (one or more missing)'
    }
}

function Write-DoctorMantle {
    param([Parameter(Mandatory)]$Block)
    $useMantle = Get-DoctorProp -Object $Block -Name 'useMantle'
    $mantleUrl = Get-DoctorNestedProp -Object $Block -Path @('mantle','baseUrl')
    $authMode = Get-DoctorNestedProp -Object $Block -Path @('auth','mode')
    if ($authMode -eq 'api-key') { $authMode = 'bedrock-api-key' }
    if (-not $useMantle) {
        Write-Output 'Status: disabled'
        return
    }
    Write-Output 'Status: enabled'
    if ($authMode -eq 'bedrock-api-key') { Write-Output 'Reason: Bedrock API key detected' }
    if ($mantleUrl) { Write-Output ('URL: {0}' -f $mantleUrl) }
    $mantleEnv = Get-DoctorNestedProp -Object $Block -Path @('env','CLAUDE_CODE_USE_MANTLE')
    if ($mantleEnv -ne '1') {
        $script:DoctorWarns += 1
        Write-Output 'Warning: CLAUDE_CODE_USE_MANTLE=1 missing from env'
    }
}

function Write-DoctorV1Artifacts {
    param([AllowNull()]$Settings, [bool]$HasV2Block)

    $foundV1 = $false
    $v1Profiles = [System.Collections.Generic.List[string]]::new()

    foreach ($candidate in (Get-ProfilePathsV1Candidates)) {
        if (-not (Test-Path $candidate)) { continue }
        $content = try { Get-Content -Path $candidate -Raw -Encoding utf8 -ErrorAction Stop } catch { '' }
        if ($content -match '# BEGIN: Claude Code Bedrock Configuration') {
            $foundV1 = $true
            $v1Profiles.Add($candidate)
        }
    }

    if (-not $foundV1) { return }

    if ($HasV2Block) {
        $script:DoctorWarns += 1
        Write-Output 'v1 profile block: WARN — found alongside v2 settings.json'
        foreach ($p in $v1Profiles) { Write-Output "  Profile: $p" }
        Write-Output '  Fix: run juggernaut migrate --clean'
    } else {
        Write-Output 'v1 profile block: INFO — v1 configuration detected'
        foreach ($p in $v1Profiles) { Write-Output "  Profile: $p" }
        Write-Output '  Upgrade: run juggernaut apply   (or pass --legacy-v1 to keep v1)'
    }
}

function Write-DoctorDrift {
    param([Parameter(Mandatory)]$Settings, [Parameter(Mandatory)]$Block, [AllowNull()][string]$ProfilePath)
    if (Test-DoctorNativeKeysMatch -Settings $Settings -Block $Block) {
        Write-Output 'Settings native keys: OK (in sync)'
    } else {
        $script:DoctorWarns += 1
        Write-Output 'Settings native keys: WARN (differ from juggernaut block)'
    }
    $enabled = [bool](Get-DoctorNestedProp -Object $Block -Path @('shellFallback','enabled'))
    $mode = Get-DoctorNestedProp -Object $Block -Path @('shellFallback','mode')
    if (-not $enabled -or $mode -eq 'settings-only') {
        Write-Output 'Settings vs Shell Fallback: OK (no fallback configured)'
        return
    }
    if (-not $ProfilePath -or -not (Test-Path $ProfilePath) -or -not (Test-ProfileWriterHasBlock -ProfileFile $ProfilePath)) {
        $script:DoctorWarns += 1
        Write-Output 'Settings vs Shell Fallback: WARN (expected but not found)'
        return
    }
    $mismatches = 0
    foreach ($key in @('AWS_REGION','ANTHROPIC_MODEL','ANTHROPIC_DEFAULT_OPUS_MODEL','ANTHROPIC_DEFAULT_SONNET_MODEL','ANTHROPIC_DEFAULT_HAIKU_MODEL','CLAUDE_CODE_SUBAGENT_MODEL','CLAUDE_CODE_EFFORT_LEVEL','CLAUDE_CODE_USE_MANTLE','ANTHROPIC_BEDROCK_MANTLE_BASE_URL')) {
        $expected = Get-DoctorNestedProp -Object $Block -Path @('env', $key)
        if ($null -eq $expected -or $expected -eq '') { continue }
        $actual = Get-DoctorShellValue -ProfilePath $ProfilePath -Key $key
        if ($actual -ne $expected) { $mismatches += 1 }
    }
    if ($mismatches -eq 0) {
        Write-Output 'Settings vs Shell Fallback: OK (no drift detected)'
    } else {
        $script:DoctorWarns += 1
        Write-Output ('Settings vs Shell Fallback: WARN ({0} differing value(s))' -f $mismatches)
    }
}

function Write-DoctorSummary {
    Write-Output ''
    Write-Output 'Summary'
    if ($script:DoctorFails -gt 0) {
        Write-Output 'Status: FAIL'
        Write-Output ('{0} failure(s), {1} warning(s)' -f $script:DoctorFails, $script:DoctorWarns)
        Write-Output "Run 'juggernaut apply' to fix configuration issues."
        return
    }
    if ($script:DoctorWarns -gt 0) {
        Write-Output 'Status: WARN'
        Write-Output ('{0} warning(s)' -f $script:DoctorWarns)
        return
    }
    Write-Output 'Status: OK'
    Write-Output 'No issues found'
}
