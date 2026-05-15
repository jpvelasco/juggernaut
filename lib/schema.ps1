# lib/schema.ps1 - Juggernaut v2 block schema for PowerShell. Mirrors lib/schema.sh.
# Uses native ConvertFrom-Json / ConvertTo-Json. PowerShell 5.1+ compatible.

$Script:JuggernautSchemaVersion = 1

function Get-SchemaDefaultRegion { 'us-east-1' }

function Get-SchemaBedrockConfig {
    param([string]$Path = "$PSScriptRoot/../bedrock-config.json")
    if (-not (Test-Path $Path)) {
        throw "bedrock-config.json not found at $Path"
    }
    Get-Content -Path $Path -Raw -Encoding utf8 | ConvertFrom-Json
}

function Get-SchemaSupportedRegions {
    param([string]$BedrockConfigPath)
    $config = if ($BedrockConfigPath) { Get-SchemaBedrockConfig -Path $BedrockConfigPath } else { Get-SchemaBedrockConfig }
    return $config.regions
}

function Test-SchemaSupportedRegion {
    param(
        [Parameter(Mandatory)][string]$Region,
        [string]$BedrockConfigPath
    )
    $regions = if ($BedrockConfigPath) { Get-SchemaSupportedRegions -BedrockConfigPath $BedrockConfigPath } else { Get-SchemaSupportedRegions }
    return $regions -contains $Region
}

function ConvertTo-SchemaAuthMode {
    param([string]$AuthMode)
    switch ($AuthMode) {
        'api-key' { 'bedrock-api-key' }
        'bedrock-api-key' { 'bedrock-api-key' }
        'iam' { 'iam' }
        '' { 'iam' }
        default { $AuthMode }
    }
}

function Get-SchemaDefaultModel { 'global.anthropic.claude-sonnet-4-6' }

function New-JuggernautBlock {
    [CmdletBinding()]
    param(
        [string]$Provider = 'bedrock',
        [bool]$UseMantle = $true,
        [string]$MantleBaseUrl = '',
        [string]$Model = 'global.anthropic.claude-sonnet-4-6',
        [string]$OpusModel = 'global.anthropic.claude-opus-4-7',
        [string]$SonnetModel = 'global.anthropic.claude-sonnet-4-6',
        [string]$HaikuModel = 'global.anthropic.claude-haiku-4-5-20251001-v1:0',
        [string]$SubagentModel = '',
        [bool]$Use1MContext = $true,
        [bool]$OpusPlan = $false,
        [ValidateSet('low','medium','high','xhigh','max')][string]$EffortLevel = 'xhigh',
        # "api-key" is accepted only as a legacy read alias. New writes emit
        # "bedrock-api-key" so persisted config is explicit about Bedrock auth.
        [ValidateSet('iam','api-key','bedrock-api-key')][string]$AuthMode = 'iam',
        [bool]$AuthValidated = $false,
        [ValidateSet('profile','keychain','dpapi')][string]$Storage = 'keychain',
        [string]$Region = '',
        [ValidateSet('both','settings-only','shell-only')][string]$ShellFallbackMode = 'settings-only',
        [ValidateSet('user','project')][string]$Scope = 'user',
        [string]$Version = '3.2.3',
        [string]$BedrockConfigPath
    )

    if ([string]::IsNullOrEmpty($Region)) { $Region = Get-SchemaDefaultRegion }
    if ([string]::IsNullOrEmpty($Model) -or $Model -eq 'opusplan') { $Model = Get-SchemaDefaultModel }
    $AuthMode = ConvertTo-SchemaAuthMode -AuthMode $AuthMode
    if ([string]::IsNullOrEmpty($SubagentModel)) { $SubagentModel = $HaikuModel }

    $bedrock = if ($BedrockConfigPath) { Get-SchemaBedrockConfig -Path $BedrockConfigPath } else { Get-SchemaBedrockConfig }

    $env = [ordered]@{}
    foreach ($prop in $bedrock.environment.PSObject.Properties) {
        $env[$prop.Name] = $prop.Value
    }
    # CLAUDE_CODE_USE_BEDROCK is gated behind validated auth — merge only when
    # apply has confirmed a working credential path. Prevents hang-on-launch
    # when Bedrock routing is enabled without a usable AWS/API-key credential.
    if ($AuthValidated -and $bedrock.environment_bedrock_auth) {
        foreach ($prop in $bedrock.environment_bedrock_auth.PSObject.Properties) {
            $env[$prop.Name] = $prop.Value
        }
    }
    $env['AWS_REGION']                     = $Region
    if ($OpusPlan) { $env['ANTHROPIC_MODEL'] = 'opusplan' }
    $env['ANTHROPIC_DEFAULT_OPUS_MODEL']   = $OpusModel
    $env['ANTHROPIC_DEFAULT_SONNET_MODEL'] = $SonnetModel
    $env['ANTHROPIC_DEFAULT_HAIKU_MODEL']  = $HaikuModel
    $env['CLAUDE_CODE_SUBAGENT_MODEL']     = $SubagentModel
    $env['CLAUDE_CODE_EFFORT_LEVEL']       = $EffortLevel

    if ($UseMantle) {
        $env['CLAUDE_CODE_USE_MANTLE'] = '1'
        if (-not [string]::IsNullOrEmpty($MantleBaseUrl)) {
            $env['ANTHROPIC_BEDROCK_MANTLE_BASE_URL'] = $MantleBaseUrl
        }
    } else {
        $env.Remove('CLAUDE_CODE_USE_MANTLE') | Out-Null
        $env.Remove('ANTHROPIC_BEDROCK_MANTLE_BASE_URL') | Out-Null
    }

    $now = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')

    [ordered]@{
        schemaVersion  = $Script:JuggernautSchemaVersion
        provider       = $Provider
        useMantle      = $UseMantle
        mantle         = [ordered]@{
            baseUrl = if ([string]::IsNullOrEmpty($MantleBaseUrl)) { $null } else { $MantleBaseUrl }
        }
        model          = $Model
        context        = [ordered]@{ maxContextTokens = 1000000; use1MContext = $Use1MContext }
        auth           = [ordered]@{ mode = $AuthMode; region = $Region; storage = $Storage }
        modelOverrides = [ordered]@{
            opus     = $OpusModel
            sonnet   = $SonnetModel
            haiku    = $HaikuModel
            subagent = $SubagentModel
        }
        effortLevel    = $EffortLevel
        opusplan       = $OpusPlan
        shellFallback  = [ordered]@{
            enabled              = ($ShellFallbackMode -ne 'settings-only')
            mode                 = $ShellFallbackMode
            lastWrittenProfiles  = @()
        }
        env            = $env
        legacyEnv      = $null
        meta           = [ordered]@{
            managedBy        = 'juggernaut'
            version          = $Version
            scope            = $Scope
            lastUpdated      = $now
            detectedClients  = @()
        }
    }
}

function Test-JuggernautBlock {
    [CmdletBinding()]
    param([Parameter(Mandatory)]$Block)

    # Works for hashtable / [ordered]@{} (from Read-Settings) AND PSCustomObject
    # (from ConvertFrom-Json without the converter).
    $hasField = {
        param($obj, $name)
        if ($obj -is [hashtable] -or $obj -is [System.Collections.Specialized.OrderedDictionary]) {
            return $obj.Contains($name)
        }
        return [bool]($obj.PSObject.Properties.Name -contains $name)
    }

    $errors = New-Object System.Collections.Generic.List[string]
    $required = @('schemaVersion','provider','useMantle','model','context','auth','modelOverrides','effortLevel','env','meta')
    foreach ($f in $required) {
        if (-not (& $hasField $Block $f)) {
            $errors.Add("missing required field: $f")
        }
    }

    $authMode = $Block.auth.mode
    # "api-key" remains valid for reading legacy blocks; builders rewrite it.
    if ($authMode -notin @('iam','api-key','bedrock-api-key')) { $errors.Add("auth.mode must be 'iam' or 'bedrock-api-key' (got: '$authMode')") }

    $storage = $Block.auth.storage
    if ($storage -notin @('profile','keychain','dpapi')) { $errors.Add("auth.storage must be 'profile', 'keychain', or 'dpapi' (got: '$storage')") }

    if ($Block.model -eq 'opusplan') {
        $errors.Add("model must be a Bedrock model ID; 'opusplan' is a routing mode for env.ANTHROPIC_MODEL only")
    }

    $effort = $Block.effortLevel
    if ($effort -notin @('low','medium','high','xhigh','max')) { $errors.Add("effortLevel must be one of low|medium|high|xhigh|max (got: '$effort')") }

    $shellMode = $Block.shellFallback.mode
    if ($shellMode -and $shellMode -notin @('both','settings-only','shell-only')) {
        $errors.Add("shellFallback.mode must be one of both|settings-only|shell-only (got: '$shellMode')")
    }

    if ($Block.meta.managedBy -ne 'juggernaut') {
        $errors.Add("meta.managedBy must be 'juggernaut' (got: '$($Block.meta.managedBy)')")
    }

    $region = $Block.auth.region
    if ([string]::IsNullOrEmpty($region)) {
        $errors.Add("auth.region is required")
    } elseif (-not (Test-SchemaSupportedRegion -Region $region)) {
        $errors.Add("auth.region '$region' is not in bedrock-config.json .regions")
    }

    if ($errors.Count -gt 0) {
        # Surface errors via Write-Warning so callers that use ` | Should -BeFalse`
        # still observe a clean $false. Use -ErrorVariable or check the return
        # directly when you need structured error access.
        Write-Warning ("Schema validation failed:`n  - " + ($errors -join "`n  - "))
        return $false
    }
    return $true
}

function Get-NativeKeysFromJuggernautBlock {
    param([Parameter(Mandatory)]$Block)
    [ordered]@{
        env            = $Block.env
        model          = $Block.model
        modelOverrides = [ordered]@{
            opus     = $Block.modelOverrides.opus
            sonnet   = $Block.modelOverrides.sonnet
            haiku    = $Block.modelOverrides.haiku
            subagent = $Block.modelOverrides.subagent
        }
    }
}

function Get-JuggernautNativeKeyNames { 'env','model','modelOverrides','availableModels' }
