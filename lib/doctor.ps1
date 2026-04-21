# lib/doctor.ps1 - read-only diagnostics for Juggernaut v2.

$script:DoctorFails = 0
$script:DoctorWarns = 0
$script:DoctorIndent = 2

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
    $prefix = ' ' * $script:DoctorIndent
    if ($Detail) {
        Write-Output ('{0}{1,-5} {2}: {3}' -f $prefix, $Status, $Label, $Detail)
    } else {
        Write-Output ('{0}{1,-5} {2}' -f $prefix, $Status, $Label)
    }
}

function Write-DoctorSubsection {
    param([Parameter(Mandatory)][string]$Name)
    Write-Output ('  ' + $Name)
}

function Show-DoctorBoolState {
    param([AllowNull()]$Value)
    if ($Value -is [bool]) { return $(if ($Value) { 'enabled' } else { 'disabled' }) }
    if ($Value -eq 'true') { return 'enabled' }
    if ($Value -eq 'false') { return 'disabled' }
    return '-'
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

function Invoke-DoctorNativeDriftCheck {
    param(
        [Parameter(Mandatory)]$Settings,
        [Parameter(Mandatory)]$Block
    )
    if (Test-DoctorNativeKeysMatch -Settings $Settings -Block $Block) {
        Write-DoctorStatus -Status OK -Label 'settings native keys' -Detail 'match juggernaut block'
    } else {
        Write-DoctorStatus -Status WARN -Label 'settings native keys' -Detail 'differ from juggernaut block'
    }
}

function Invoke-DoctorShellDriftCheck {
    param(
        [Parameter(Mandatory)]$Block,
        [AllowNull()][string]$ProfilePath
    )
    $enabled = [bool](Get-DoctorNestedProp -Object $Block -Path @('shellFallback','enabled'))
    $mode = Get-DoctorNestedProp -Object $Block -Path @('shellFallback','mode')
    if (-not $enabled -or $mode -eq 'settings-only') {
        Write-DoctorStatus -Status OK -Label 'shell fallback' -Detail 'not required for this scope'
        return
    }

    if (-not $ProfilePath -or -not (Test-Path $ProfilePath) -or -not (Test-ProfileWriterHasBlock -ProfileFile $ProfilePath)) {
        Write-DoctorStatus -Status WARN -Label 'shell fallback' -Detail 'expected but not found'
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
        Write-DoctorStatus -Status OK -Label 'shell fallback' -Detail "$(Show-DoctorHomePath $ProfilePath) matches settings.json"
    } else {
        Write-DoctorStatus -Status WARN -Label 'shell fallback' -Detail "$(Show-DoctorHomePath $ProfilePath) has $mismatches differing value(s)"
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
            Write-DoctorStatus -Status OK -Label 'auth mode' -Detail 'iam'
            if ($env:AWS_PROFILE) {
                Write-DoctorStatus -Status OK -Label 'IAM env' -Detail 'AWS_PROFILE is set'
            } elseif ($env:AWS_ACCESS_KEY_ID -and $env:AWS_SECRET_ACCESS_KEY) {
                Write-DoctorStatus -Status OK -Label 'IAM env' -Detail 'access key variables are set'
            } else {
                Write-DoctorStatus -Status WARN -Label 'IAM env' -Detail 'no IAM environment variables detected'
            }
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                Write-DoctorStatus -Status WARN -Label 'API key env' -Detail 'AWS_BEARER_TOKEN_BEDROCK is set while auth mode is iam'
            }
        }
        'api-key' {
            Write-DoctorStatus -Status OK -Label 'auth mode' -Detail 'api-key'
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                Write-DoctorStatus -Status OK -Label 'API key source' -Detail 'AWS_BEARER_TOKEN_BEDROCK is set'
            } elseif ($storage -eq 'keychain' -and (Test-KeychainAvailable) -and (Get-KeychainEntry)) {
                Write-DoctorStatus -Status OK -Label 'API key source' -Detail 'keychain entry present'
            } elseif ($ProfilePath -and (Test-DoctorShellHasKeyAssignment -ProfilePath $ProfilePath -Key 'AWS_BEARER_TOKEN_BEDROCK')) {
                Write-DoctorStatus -Status OK -Label 'API key source' -Detail 'shell fallback contains API key assignment'
            } else {
                Write-DoctorStatus -Status FAIL -Label 'API key source' -Detail 'no API key found in env, keychain, or shell fallback'
            }
        }
        default {
            Write-DoctorStatus -Status FAIL -Label 'auth mode' -Detail 'missing or unsupported'
        }
    }
}

function Invoke-DoctorRegionModelsMantleCheck {
    param([Parameter(Mandatory)]$Block)
    $region = Get-DoctorNestedProp -Object $Block -Path @('auth','region')
    $model = Get-DoctorProp -Object $Block -Name 'model'
    $effort = Get-DoctorProp -Object $Block -Name 'effortLevel'
    $useMantle = Get-DoctorProp -Object $Block -Name 'useMantle'
    $mantleUrl = Get-DoctorNestedProp -Object $Block -Path @('mantle','baseUrl')

    if ($region -and (Test-SchemaSupportedRegion -Region $region)) {
        Write-DoctorStatus -Status OK -Label 'region' -Detail $region
    } else {
        Write-DoctorStatus -Status FAIL -Label 'region' -Detail "$(Show-DoctorValue $region) is not supported"
    }

    if ($model) {
        Write-DoctorStatus -Status OK -Label 'model' -Detail $model
    } else {
        Write-DoctorStatus -Status FAIL -Label 'model' -Detail 'missing'
    }

    $overrideNames = @('opus','sonnet','haiku','subagent')
    $missing = $false
    foreach ($name in $overrideNames) {
        if (-not (Get-DoctorNestedProp -Object $Block -Path @('modelOverrides', $name))) { $missing = $true }
    }
    if ($missing) {
        Write-DoctorStatus -Status WARN -Label 'overrides' -Detail 'one or more model overrides are missing'
    } else {
        Write-DoctorStatus -Status OK -Label 'overrides' -Detail 'opus, sonnet, haiku, subagent present'
    }

    Write-DoctorStatus -Status OK -Label 'effort' -Detail (Show-DoctorValue $effort)
    Write-DoctorStatus -Status OK -Label 'mantle' -Detail (Show-DoctorBoolState $useMantle)
    if ($useMantle) {
        $mantleEnv = Get-DoctorNestedProp -Object $Block -Path @('env','CLAUDE_CODE_USE_MANTLE')
        if ($mantleEnv -eq '1') {
            Write-DoctorStatus -Status OK -Label 'mantle env' -Detail 'CLAUDE_CODE_USE_MANTLE=1'
        } else {
            Write-DoctorStatus -Status WARN -Label 'mantle env' -Detail 'missing CLAUDE_CODE_USE_MANTLE=1'
        }
    }
    if ($useMantle -and $mantleUrl) {
        Write-DoctorStatus -Status INFO -Label 'mantle URL' -Detail $mantleUrl
    }
}

function Invoke-DoctorBlockChecks {
    param(
        [Parameter(Mandatory)]$Settings,
        [AllowNull()][string]$ProfilePath
    )
    $script:DoctorIndent = 4
    if (-not $Settings) {
        Write-DoctorStatus -Status FAIL -Label 'settings.json' -Detail 'not valid JSON'
        $script:DoctorIndent = 2
        return
    }
    Write-DoctorStatus -Status OK -Label 'settings.json' -Detail 'valid JSON'

    if (-not (Test-HasJuggernautBlock -Settings $Settings)) {
        Write-DoctorStatus -Status WARN -Label 'juggernaut block' -Detail 'missing'
        $script:DoctorIndent = 2
        return
    }

    $block = Get-JuggernautBlockFromSettings -Settings $Settings
    if (Test-JuggernautBlock -Block $block -WarningAction SilentlyContinue) {
        Write-DoctorStatus -Status OK -Label 'juggernaut block' -Detail 'present, schema valid'
    } else {
        Write-DoctorStatus -Status FAIL -Label 'juggernaut block' -Detail 'present, schema invalid'
    }

    Write-DoctorSubsection -Name 'Configuration'
    $script:DoctorIndent = 4
    Invoke-DoctorRegionModelsMantleCheck -Block $block

    Write-DoctorSubsection -Name 'Auth'
    $script:DoctorIndent = 4
    Invoke-DoctorAuthCheck -Block $block -ProfilePath $ProfilePath

    Write-DoctorSubsection -Name 'Drift'
    $script:DoctorIndent = 4
    Invoke-DoctorNativeDriftCheck -Settings $Settings -Block $block
    Invoke-DoctorShellDriftCheck -Block $block -ProfilePath $ProfilePath
    $script:DoctorIndent = 2
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
    $script:DoctorIndent = 2
    Write-DoctorStatus -Status INFO -Label 'path' -Detail (Show-DoctorHomePath $Path)

    if (-not (Test-Path $Path)) {
        Write-DoctorStatus -Status INFO -Label 'settings.json' -Detail 'not found'
        return
    }
    Write-DoctorSubsection -Name 'Settings'
    Invoke-DoctorBlockChecks -Settings $Settings -ProfilePath $ProfilePath
}

function Write-DoctorSummary {
    Write-Output 'Summary'
    if ($script:DoctorFails -gt 0) {
        Write-DoctorStatus -Status INFO -Label 'result' -Detail "$($script:DoctorFails) failure(s), $($script:DoctorWarns) warning(s)"
        Write-Output '  Next  Fix the failed checks above, then rerun doctor.'
        return
    }
    if ($script:DoctorWarns -gt 0) {
        Write-DoctorStatus -Status INFO -Label 'result' -Detail "$($script:DoctorWarns) warning(s)"
        Write-Output '  Next  Review the warnings above; rerun apply with --scope=user or --scope=project if you want to refresh a scope.'
        return
    }
    Write-DoctorStatus -Status INFO -Label 'result' -Detail 'No issues found'
}
