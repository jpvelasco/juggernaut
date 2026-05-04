# lib/doctor.ps1 - read-only diagnostics for Juggernaut v3.

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
    param([Parameter(Mandatory)]$Block)
    $authMode = Get-DoctorNestedProp -Object $Block -Path @('auth','mode')
    $storage = Get-DoctorNestedProp -Object $Block -Path @('auth','storage')
    if ($authMode -eq 'api-key') { $authMode = 'bedrock-api-key' }
    $read = $null
    $readError = ''
    if ($authMode -eq 'bedrock-api-key' -and $storage -in 'keychain','dpapi') {
        try { $read = Read-BearerToken } catch { $readError = "$_"; $read = $null }
        if (-not $read.Value -and $read.Error) { $readError = $read.Error; $read = $null }
    }
    switch ($authMode) {
        'iam' {
            Write-Output 'Auth: IAM'
            if ($env:AWS_BEARER_TOKEN_BEDROCK) {
                $script:DoctorWarns += 1
                Write-Output 'Status: WARN'
                Write-Output "Details: AWS_BEARER_TOKEN_BEDROCK is set but auth mode is 'iam' - possible misconfiguration"
                Write-Output 'Fix: run: juggernaut apply --auth=bedrock-api-key'
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
            } elseif ($read -and $read.Value) {
                $label = if ($read.Storage -eq 'dpapi') { 'DPAPI file' }
                         elseif ($read.Storage -eq 'keychain') { 'system keychain' }
                         else { $read.Storage }
                Write-Output ("Source: {0}" -f $label)
                Write-Output ("Storage: {0}" -f $read.Storage)
                Write-Output 'Status: OK'
            } else {
                if ($readError) {
                    $script:DoctorWarns += 1
                    Write-Output ('Keychain/DPAPI: WARN ({0})' -f $readError)
                }
                $script:DoctorFails += 1
                Write-Output 'Status: FAIL'
                Write-Output 'Details: no API key found in env, keychain, or DPAPI file'
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
    if (-not $useMantle) {
        Write-Output 'Status: disabled (INFO)'
        return
    }
    Write-Output 'Status: enabled'
    if ($mantleUrl) { Write-Output ('URL: {0}' -f $mantleUrl) }
    $mantleEnv = Get-DoctorNestedProp -Object $Block -Path @('env','CLAUDE_CODE_USE_MANTLE')
    if ($mantleEnv -ne '1') {
        $script:DoctorWarns += 1
        Write-Output 'Warning: CLAUDE_CODE_USE_MANTLE=1 missing from env'
    }
}

function Write-DoctorTopLevelModel {
    param([Parameter(Mandatory)]$Settings)
    $topModel = Get-DoctorProp -Object $Settings -Name 'model'
    if ($topModel -eq 'opusplan') {
        $script:DoctorWarns += 1
        Write-Output 'Top-level model: WARN ("opusplan" is not a Bedrock model ID)'
        Write-Output 'Details: Claude Code will send this to Bedrock and hang on retries'
        Write-Output 'Fix: run: juggernaut apply'
    }
}

function Write-DoctorOpusplan {
    param([Parameter(Mandatory)]$Settings, [Parameter(Mandatory)]$Block)
    $opusplanOn = [bool](Get-DoctorNestedProp -Object $Settings -Path @('juggernaut','opusplan'))
    if (-not $opusplanOn) {
        Write-Output 'Status: disabled'
        return
    }
    Write-Output 'Status: enabled'
    # Expected: both .env.ANTHROPIC_MODEL in the block and the top-level .env.ANTHROPIC_MODEL
    # in settings.json should read "opusplan".
    $settingsModel = Get-DoctorNestedProp -Object $Settings -Path @('env','ANTHROPIC_MODEL')
    $envModel      = Get-DoctorNestedProp -Object $Block    -Path @('env','ANTHROPIC_MODEL')
    if ($settingsModel -eq 'opusplan' -and $envModel -eq 'opusplan') {
        Write-Output 'Status: OK'
    } else {
        $script:DoctorWarns += 1
        $sm = if ($settingsModel) { $settingsModel } else { '' }
        $em = if ($envModel) { $envModel } else { '' }
        Write-Output ("Warning: ANTHROPIC_MODEL mismatch (settings.env='{0}', block.env='{1}'; expected 'opusplan')" -f $sm, $em)
        Write-Output 'Fix: run: juggernaut apply --opusplan'
    }
}

function Test-LauncherInstalled {
    # True when the PS launcher block is present in the current-host profile
    # (or its sibling host's profile). Match the install.ps1 target list.
    $targets = @()
    try {
        if ($PROFILE -and $PROFILE.CurrentUserCurrentHost) {
            $targets += [string]$PROFILE.CurrentUserCurrentHost
            $curr = [string]$PROFILE.CurrentUserCurrentHost
            if ($curr -match '\\WindowsPowerShell\\') {
                $targets += ($curr -replace '\\WindowsPowerShell\\', '\PowerShell\')
            } elseif ($curr -match '\\PowerShell\\') {
                $targets += ($curr -replace '\\PowerShell\\', '\WindowsPowerShell\')
            }
        }
    } catch {}
    foreach ($p in ($targets | Where-Object { $_ } | Select-Object -Unique)) {
        if (-not (Test-Path $p)) { continue }
        try {
            $content = Get-Content -Path $p -Raw -ErrorAction Stop
            if ($content -match '(?m)^# BEGIN: Juggernaut Launcher') { return @{ Installed = $true; Path = $p } }
        } catch {}
    }
    return @{ Installed = $false; Path = '' }
}

function Write-DoctorLauncher {
    param([Parameter(Mandatory)]$Block)
    $useBedrock = Get-DoctorNestedProp -Object $Block -Path @('env','CLAUDE_CODE_USE_BEDROCK')
    $authMode   = Get-DoctorNestedProp -Object $Block -Path @('auth','mode')
    if ($authMode -eq 'api-key') { $authMode = 'bedrock-api-key' }

    # The launcher injects AWS_BEARER_TOKEN_BEDROCK from the OS keychain. It is
    # only relevant when auth.mode is bedrock-api-key; IAM auth never reads the
    # bearer token.
    if ($authMode -ne 'bedrock-api-key') {
        Write-Output 'Status: not applicable (IAM auth does not use a bearer token)'
        return
    }
    if ($useBedrock -ne '1') {
        Write-Output 'Status: not applicable (CLAUDE_CODE_USE_BEDROCK not set)'
        return
    }

    if ($env:AWS_BEARER_TOKEN_BEDROCK) {
        Write-Output 'Status: OK'
        Write-Output 'Source: AWS_BEARER_TOKEN_BEDROCK already in env'
        $launcher = Test-LauncherInstalled
        if ($launcher.Installed) {
            Write-Output ('Launcher: {0} (also installed)' -f (Show-DoctorHomePath $launcher.Path))
        }
        return
    }

    $launcher = Test-LauncherInstalled
    if ($launcher.Installed) {
        Write-Output 'Status: OK'
        Write-Output ('Launcher: {0}' -f (Show-DoctorHomePath $launcher.Path))
        Write-Output 'Source: OS keychain via launcher'
        return
    }

    $script:DoctorWarns += 1
    Write-Output 'Status: WARN'
    Write-Output 'Launcher: not installed (PS profile has no Juggernaut Launcher block)'
    Write-Output 'Details: claude will hang on launch - no bearer token in env and no launcher to inject it'
    Write-Output 'Fix: re-run the installer (install.ps1) or set $env:AWS_BEARER_TOKEN_BEDROCK'
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
