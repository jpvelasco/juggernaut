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

function Show-DoctorValue {
    param([AllowNull()]$Value)
    if ($null -eq $Value -or $Value -eq '') { return '-' }
    return [string]$Value
}

function Write-DoctorStatus {
    param(
        [Parameter(Mandatory)][ValidateSet('OK','WARN','FAIL','INFO')][string]$Status,
        [Parameter(Mandatory)][string]$Label,
        [AllowNull()][string]$Detail = ''
    )
    switch ($Status) {
        'FAIL' { $script:DoctorFails += 1 }
        'WARN' { $script:DoctorWarns += 1 }
    }
    if ($Detail) {
        Write-Output ('  {0,-5} {1}: {2}' -f $Status, $Label, $Detail)
    } else {
        Write-Output ('  {0,-5} {1}' -f $Status, $Label)
    }
}

function Get-DoctorProp {
    param(
        [AllowNull()]$Object,
        [Parameter(Mandatory)][string]$Name
    )
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
    param(
        [AllowNull()]$Object,
        [Parameter(Mandatory)][string[]]$Path
    )
    $cur = $Object
    foreach ($part in $Path) {
        $cur = Get-DoctorProp -Object $cur -Name $part
        if ($null -eq $cur) { return $null }
    }
    return $cur
}

function Write-DoctorScopeTitle {
    param(
        [Parameter(Mandatory)][string]$Scope,
        [bool]$Active,
        [bool]$Selected
    )
    $label = (Get-Culture).TextInfo.ToTitleCase($Scope) + ' Scope'
    if ($Active -and $Selected) { Write-Output "$label (active, selected)" }
    elseif ($Active) { Write-Output "$label (active)" }
    elseif ($Selected) { Write-Output "$label (selected)" }
    else { Write-Output $label }
}

function Get-DoctorProfilePath {
    $shellName = if ($env:SHELL) { Split-Path -Leaf $env:SHELL } else { 'bash' }
    Get-ProfileWriterShellConfigPath -Shell $shellName
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
    param(
        [Parameter(Mandatory)][string]$ProfilePath,
        [Parameter(Mandatory)][string]$Key
    )
    foreach ($line in (Get-DoctorShellBlock -ProfilePath $ProfilePath)) {
        if ($line -match "^\s*export\s+$([regex]::Escape($Key))=(.+)$") {
            return (($Matches[1] -replace '^"', '') -replace '"$', '')
        }
        if ($line -match "^\s*set\s+-gx\s+$([regex]::Escape($Key))\s+(.+)$") {
            return (($Matches[1] -replace '^"', '') -replace '"$', '')
        }
    }
    return ''
}

function Test-DoctorShellHasKeyAssignment {
    param(
        [Parameter(Mandatory)][string]$ProfilePath,
        [Parameter(Mandatory)][string]$Key
    )
    foreach ($line in (Get-DoctorShellBlock -ProfilePath $ProfilePath)) {
        if ($line -match "^\s*export\s+$([regex]::Escape($Key))=") { return $true }
        if ($line -match "^\s*set\s+-gx\s+$([regex]::Escape($Key))\s+") { return $true }
    }
    return $false
}

function Test-DoctorNativeKeysMatch {
    param(
        [Parameter(Mandatory)]$Settings,
        [Parameter(Mandatory)]$Block
    )
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

function Invoke-DoctorConfigCheck {
    param([Parameter(Mandatory)]$Block)
    $region = Get-DoctorNestedProp -Object $Block -Path @('auth','region')
    $model = Get-DoctorProp -Object $Block -Name 'model'
    $effort = Get-DoctorProp -Object $Block -Name 'effortLevel'
    $useMantle = Get-DoctorProp -Object $Block -Name 'useMantle'
    $mantleUrl = Get-DoctorNestedProp -Object $Block -Path @('mantle','baseUrl')

    if ($region -and (Test-SchemaSupportedRegion -Region $region)) {
        Write-DoctorStatus -Status OK -Label 'region' -Detail $region
    } else {
        Write-DoctorStatus -Status FAIL -Label 'region' -Detail "$(Show-DoctorValue $region) (unsupported)"
    }

    if ($model) {
        $modelDisplay = $model -replace '^global\.anthropic\.', ''
        Write-DoctorStatus -Status OK -Label 'model' -Detail $modelDisplay
    } else {
        Write-DoctorStatus -Status FAIL -Label 'model' -Detail 'missing'
    }

    $overrideNames = @('opus','sonnet','haiku','subagent')
    $missing = $false
    foreach ($name in $overrideNames) {
        if (-not (Get-DoctorNestedProp -Object $Block -Path @('modelOverrides', $name))) { $missing = $true }
    }
    if ($missing) { Write-DoctorStatus -Status WARN -Label 'overrides' -Detail 'one or more model overrides missing' }

    Write-DoctorStatus -Status OK -Label 'effort' -Detail (Show-DoctorValue $effort)

    if ($useMantle) {
        $mantleDetail = if ($mantleUrl) { "on  ($mantleUrl)" } else { 'on' }
        Write-DoctorStatus -Status OK -Label 'mantle' -Detail $mantleDetail
        $mantleEnv = Get-DoctorNestedProp -Object $Block -Path @('env','CLAUDE_CODE_USE_MANTLE')
        if ($mantleEnv -ne '1') {
            Write-DoctorStatus -Status WARN -Label 'mantle' -Detail 'on  (CLAUDE_CODE_USE_MANTLE=1 missing from env)'
        }
    } else {
        Write-DoctorStatus -Status OK -Label 'mantle' -Detail 'off'
    }
}

function Invoke-DoctorAuthCheck {
    param(
        [Parameter(Mandatory)]$Block,
        [AllowNull()][string]$ProfilePath
    )
    $authMode = Get-DoctorNestedProp -Object $Block -Path @('auth','mode')
    $storage = Get-DoctorNestedProp -Object $Block -Path @('auth','storage')

    switch ($authMode) {
        'iam' {
            if ($env:AWS_PROFILE) {
                Write-DoctorStatus -Status OK -Label 'auth' -Detail 'iam  (AWS_PROFILE set)'
            } elseif ($env:AWS_ACCESS_KEY_ID -and $env:AWS_SECRET_ACCESS_KEY) {
                Write-DoctorStatus -Status OK -Label 'auth' -Detail 'iam  (access key set)'
            } else {
                Write-DoctorStatus -Status WARN -Label 'auth' -Detail 'iam  (no credentials in environment)'
            }
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                Write-DoctorStatus -Status WARN -Label 'auth conflict' -Detail 'AWS_BEARER_TOKEN_BEDROCK set while mode is iam'
            }
        }
        'api-key' {
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                Write-DoctorStatus -Status OK -Label 'auth' -Detail 'api-key  (env var set)'
            } elseif ($storage -eq 'keychain' -and (Test-KeychainAvailable) -and (Get-KeychainEntry)) {
                Write-DoctorStatus -Status OK -Label 'auth' -Detail 'api-key  (keychain)'
            } elseif ($ProfilePath -and (Test-DoctorShellHasKeyAssignment -ProfilePath $ProfilePath -Key 'AWS_BEARER_TOKEN_BEDROCK')) {
                Write-DoctorStatus -Status OK -Label 'auth' -Detail 'api-key  (shell profile)'
            } else {
                Write-DoctorStatus -Status FAIL -Label 'auth' -Detail 'api-key  (no key in env, keychain, or profile)'
            }
        }
        default {
            Write-DoctorStatus -Status FAIL -Label 'auth' -Detail 'missing or unsupported mode'
        }
    }
}

function Invoke-DoctorDriftCheck {
    param(
        [Parameter(Mandatory)]$Settings,
        [Parameter(Mandatory)]$Block,
        [AllowNull()][string]$ProfilePath
    )
    if (Test-DoctorNativeKeysMatch -Settings $Settings -Block $Block) {
        Write-DoctorStatus -Status OK -Label 'drift' -Detail 'in sync'
    } else {
        Write-DoctorStatus -Status WARN -Label 'drift' -Detail 'native keys differ from juggernaut block'
    }

    $enabled = [bool](Get-DoctorNestedProp -Object $Block -Path @('shellFallback','enabled'))
    $mode = Get-DoctorNestedProp -Object $Block -Path @('shellFallback','mode')
    if (-not $enabled -or $mode -eq 'settings-only') { return }

    if (-not $ProfilePath -or -not (Test-Path $ProfilePath) -or -not (Test-ProfileWriterHasBlock -ProfileFile $ProfilePath)) {
        Write-DoctorStatus -Status WARN -Label 'shell' -Detail 'fallback expected but not found'
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
        Write-DoctorStatus -Status OK -Label 'shell' -Detail "$(Show-DoctorHomePath $ProfilePath) in sync"
    } else {
        Write-DoctorStatus -Status WARN -Label 'shell' -Detail "$(Show-DoctorHomePath $ProfilePath) has $mismatches differing value(s)"
    }
}

function Invoke-DoctorScopeCheck {
    param(
        [Parameter(Mandatory)][string]$Scope,
        [Parameter(Mandatory)][string]$Path,
        [AllowNull()]$Settings,
        [bool]$Active,
        [bool]$Selected,
        [AllowNull()][string]$ProfilePath
    )
    Write-DoctorScopeTitle -Scope $Scope -Active:$Active -Selected:$Selected
    Write-Output ('  ' + (Show-DoctorHomePath $Path))

    if (-not (Test-Path $Path)) {
        Write-DoctorStatus -Status INFO -Label 'status' -Detail 'not found'
        return
    }
    if (-not $Settings) {
        Write-DoctorStatus -Status FAIL -Label 'status' -Detail 'not valid JSON'
        return
    }
    if (-not (Test-HasJuggernautBlock -Settings $Settings)) {
        Write-DoctorStatus -Status WARN -Label 'status' -Detail 'no Juggernaut block found'
        return
    }

    $block = Get-JuggernautBlockFromSettings -Settings $Settings
    if (-not (Test-JuggernautBlock -Block $block -WarningAction SilentlyContinue)) {
        Write-DoctorStatus -Status FAIL -Label 'status' -Detail 'block schema invalid'
    }

    Invoke-DoctorConfigCheck -Block $block
    Invoke-DoctorAuthCheck -Block $block -ProfilePath $ProfilePath
    Invoke-DoctorDriftCheck -Settings $Settings -Block $block -ProfilePath $ProfilePath
}

function Write-DoctorSummary {
    Write-Output ''
    if ($script:DoctorFails -gt 0) {
        Write-Output "Summary: $($script:DoctorFails) failure(s), $($script:DoctorWarns) warning(s)"
        Write-Output "  Run 'juggernaut apply' to fix configuration issues."
        return
    }
    if ($script:DoctorWarns -gt 0) {
        Write-Output "Summary: $($script:DoctorWarns) warning(s)"
        return
    }
    Write-Output 'Summary: no issues found'
}
