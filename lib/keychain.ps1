# lib/keychain.ps1 - OS keychain abstraction for Juggernaut (PowerShell).
# Mirror of lib/keychain.sh. Requires PowerShell 5.1+.

$script:KeychainService = 'juggernaut-bedrock'
$script:KeychainAccount = 'api-key'

function Get-KeychainServiceName {
    if ($env:JUGGERNAUT_KEYCHAIN_SERVICE) { return $env:JUGGERNAUT_KEYCHAIN_SERVICE }
    return $script:KeychainService
}

function Get-KeychainAccountName {
    if ($env:JUGGERNAUT_KEYCHAIN_ACCOUNT) { return $env:JUGGERNAUT_KEYCHAIN_ACCOUNT }
    return $script:KeychainAccount
}

function Get-KeychainOS {
    if ($IsWindows -or $env:OS -match 'Windows') { return 'windows' }
    if ($IsMacOS)  { return 'macos' }
    if ($IsLinux)  {
        if (Test-Path '/proc/version') {
            $v = Get-Content '/proc/version' -Raw -ErrorAction SilentlyContinue
            if ($v -match 'microsoft') { return 'wsl' }
        }
        return 'linux'
    }
    return 'unknown'
}

function Test-KeychainAvailable {
    $os = Get-KeychainOS
    switch ($os) {
        'macos'   { return [bool](Get-Command 'security'    -ErrorAction SilentlyContinue) }
        'linux'   { return [bool](Get-Command 'secret-tool' -ErrorAction SilentlyContinue) }
        'wsl'     { return [bool](Get-Command 'secret-tool' -ErrorAction SilentlyContinue) }
        'windows' { return $true }  # CredentialManager via .NET always available
        default   { return $false }
    }
}

function Set-KeychainEntry {
    param([Parameter(Mandatory)][string]$Key)
    if ($env:JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL -eq '1') { return $false }
    $svc = Get-KeychainServiceName
    $acc = Get-KeychainAccountName
    $os  = Get-KeychainOS
    switch ($os) {
        'macos' {
            security delete-generic-password -s $svc -a $acc 2>$null | Out-Null
            $result = security add-generic-password -s $svc -a $acc -w $Key 2>&1
            return $LASTEXITCODE -eq 0
        }
        { $_ -in 'linux','wsl' } {
            $Key | secret-tool store --label='Juggernaut Bedrock API Key' `
                service $svc account $acc 2>$null
            return $LASTEXITCODE -eq 0
        }
        'windows' {
            $src = @'
[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
public static extern bool CredWrite(ref CREDENTIAL credential, int flags);
[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
public static extern bool CredDelete(string target, int type, int flags);
[StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
public struct CREDENTIAL {
    public int Flags; public int Type;
    public string TargetName; public string Comment;
    public long LastWritten; public int CredentialBlobSize;
    public IntPtr CredentialBlob; public int Persist;
    public int AttributeCount; public IntPtr Attributes;
    public string TargetAlias; public string UserName;
}
'@
            if (-not ('Win32.CredWriteApi' -as [type])) {
                try {
                    Add-Type -Namespace 'Win32' -Name 'CredWriteApi' -MemberDefinition $src -ErrorAction Stop
                } catch {
                    Write-Warning "Set-KeychainEntry: Add-Type failed: $_"
                    return $false
                }
            }
            try { [Win32.CredWriteApi]::CredDelete($svc, 1, 0) | Out-Null } catch {}

            $blob = [IntPtr]::Zero
            try {
                $blob = [Runtime.InteropServices.Marshal]::StringToCoTaskMemUni($Key)
                $cred = New-Object Win32.CredWriteApi+CREDENTIAL
                $cred.Flags = 0
                $cred.Type = 1
                $cred.TargetName = $svc
                $cred.UserName = $acc
                $cred.CredentialBlobSize = [Text.Encoding]::Unicode.GetByteCount($Key)
                $cred.CredentialBlob = $blob
                $cred.Persist = 2
                $ok = [Win32.CredWriteApi]::CredWrite([ref]$cred, 0)
                if (-not $ok) {
                    $errCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
                    Write-Warning "Set-KeychainEntry: CredWrite returned false (Win32 error $errCode, key length $($Key.Length))"
                }
                return $ok
            } catch {
                Write-Warning "Set-KeychainEntry: exception during CredWrite: $_"
                return $false
            } finally {
                if ($blob -ne [IntPtr]::Zero) {
                    [Runtime.InteropServices.Marshal]::ZeroFreeCoTaskMemUnicode($blob)
                }
            }
        }
        default { return $false }
    }
}

function Get-KeychainEntry {
    # Returns the stored key as a non-empty string when found.
    # Returns $null when the entry does not exist (silent).
    # Throws on tool/platform errors so callers can distinguish
    # "not found" from "something is broken".
    $svc = Get-KeychainServiceName
    $acc = Get-KeychainAccountName
    $os  = Get-KeychainOS
    switch ($os) {
        'macos' {
            if (-not (Get-Command 'security' -ErrorAction SilentlyContinue)) {
                throw "Get-KeychainEntry: 'security' not found on PATH"
            }
            $val = security find-generic-password -s $svc -a $acc -w 2>$null
            $rc  = $LASTEXITCODE
            switch ($rc) {
                0  { return ($val -replace "`r","").Trim() }
                44 { return $null }
                default { throw "Get-KeychainEntry: 'security' failed (exit $rc)" }
            }
        }
        { $_ -in 'linux','wsl' } {
            if (-not (Get-Command 'secret-tool' -ErrorAction SilentlyContinue)) {
                throw "Get-KeychainEntry: 'secret-tool' not found on PATH"
            }
            $val = secret-tool lookup service $svc account $acc 2>$null
            $rc  = $LASTEXITCODE
            if ($rc -in 0,1) {
                $val = if ($val) { ($val -replace "`r","").Trim() } else { '' }
                if ([string]::IsNullOrEmpty($val)) { return $null }
                return $val
            }
            throw "Get-KeychainEntry: 'secret-tool' failed (exit $rc)"
        }
        'windows' {
            # Read via advapi32 CredRead (same approach as keychain.sh PS inline block).
            $src = @'
[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
public static extern bool CredRead(string target, int type, int flags, out IntPtr credential);
[DllImport("advapi32.dll")]
public static extern void CredFree(IntPtr credential);
[StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
public struct CREDENTIAL {
    public int Flags; public int Type;
    public string TargetName; public string Comment;
    public long LastWritten; public int CredentialBlobSize;
    public IntPtr CredentialBlob; public int Persist;
    public int AttributeCount; public IntPtr Attributes;
    public string TargetAlias; public string UserName;
}
'@
            try {
                Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition $src -ErrorAction SilentlyContinue
            } catch {
                throw "Get-KeychainEntry: failed to load advapi32 bindings: $_"
            }
            $ptr = [IntPtr]::Zero
            if ([Win32.Cred]::CredRead($svc, 1, 0, [ref]$ptr)) {
                $c = [Runtime.InteropServices.Marshal]::PtrToStructure($ptr, [Type][Win32.Cred+CREDENTIAL])
                $val = ''
                if ($c.CredentialBlobSize -gt 0) {
                    $val = [Runtime.InteropServices.Marshal]::PtrToStringUni($c.CredentialBlob, $c.CredentialBlobSize / 2)
                }
                [Win32.Cred]::CredFree($ptr)
                if ([string]::IsNullOrEmpty($val)) { return $null }
                return $val
            }
            # CredRead returning false is "not found" (ERROR_NOT_FOUND = 1168).
            return $null
        }
        default {
            throw "Get-KeychainEntry: unsupported OS '$os'"
        }
    }
}

function Remove-KeychainEntry {
    $svc = Get-KeychainServiceName
    $acc = Get-KeychainAccountName
    $os  = Get-KeychainOS
    switch ($os) {
        'macos'             { security delete-generic-password -s $svc -a $acc 2>$null | Out-Null }
        { $_ -in 'linux','wsl' } { secret-tool clear service $svc account $acc 2>$null | Out-Null }
        'windows'           {
            $src = @'
[DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
public static extern bool CredDelete(string target, int type, int flags);
'@
            Add-Type -Namespace 'Win32' -Name 'CredDeleteApi' -MemberDefinition $src -ErrorAction SilentlyContinue
            [Win32.CredDeleteApi]::CredDelete($svc, 1, 0) | Out-Null
        }
    }
}

# ---------------------------------------------------------------------------
# Profile token storage — cross-platform plaintext file at
# XDG_CONFIG_HOME/juggernaut/bearer-token (or ~/.config/juggernaut/bearer-token).
# Mirrors Bash's profile_token_path / profile_token_store / profile_token_get /
# profile_token_delete. Test override via JUGGERNAUT_PROFILE_TOKEN_PATH.
# ---------------------------------------------------------------------------

function Get-ProfileTokenPath {
    if ($env:JUGGERNAUT_PROFILE_TOKEN_PATH) { return $env:JUGGERNAUT_PROFILE_TOKEN_PATH }
    $configRoot = if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME }
                 elseif ($env:HOME)         { Join-Path $env:HOME '.config' }
                 elseif ($env:USERPROFILE)  { Join-Path $env:USERPROFILE '.config' }
                 else                       { Join-Path ([Environment]::GetFolderPath('UserProfile')) '.config' }
    return Join-Path (Join-Path $configRoot 'juggernaut') 'bearer-token'
}

function Set-ProfileTokenEntry {
    param([Parameter(Mandatory)][string]$Key)
    $path = Get-ProfileTokenPath
    $dir  = Split-Path -Parent $path
    try {
        if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
        $tmp = Join-Path $dir (".bearer-token.tmp.{0}" -f [Guid]::NewGuid().ToString('N'))
        try {
            [IO.File]::WriteAllText($tmp, $Key, [Text.UTF8Encoding]::new($false))
            $chmod = Get-Command chmod -ErrorAction SilentlyContinue
            if ($chmod) { & $chmod.Source 600 -- $tmp 2>$null }
            Move-Item -Path $tmp -Destination $path -Force
        } catch {
            if (Test-Path $tmp) { Remove-Item $tmp -Force -ErrorAction SilentlyContinue }
            Write-Warning "Set-ProfileTokenEntry: write failed: $_"
            return $false
        }
        return $true
    } catch {
        Write-Warning "Set-ProfileTokenEntry: mkdir failed: $_"
        return $false
    }
}

function Get-ProfileTokenEntry {
    $path = Get-ProfileTokenPath
    if (-not (Test-Path $path)) { return $null }
    try {
        $val = [IO.File]::ReadAllText($path).Trim()
        if ([string]::IsNullOrEmpty($val)) { return $null }
        return $val
    } catch {
        throw "Get-ProfileTokenEntry: failed to read $path : $_"
    }
}

function Remove-ProfileTokenEntry {
    $path = Get-ProfileTokenPath
    if (Test-Path $path) {
        try { Remove-Item -Path $path -Force -ErrorAction Stop } catch {
            Write-Warning "Remove-ProfileTokenEntry: delete failed: $_"
        }
    }
}

# ---------------------------------------------------------------------------
# DPAPI-backed storage (Windows only). Used when a Bedrock API key exceeds
# the Credential Manager CredWrite blob cap (~1280 unicode chars / 2560 bytes).
# The DPAPI ciphertext only decrypts under the same Windows user account.
# ---------------------------------------------------------------------------

$script:DPAPIEntropyLabel = 'juggernaut-bedrock'

function Get-DPAPIHome {
    if ($env:JUGGERNAUT_HOME)    { return $env:JUGGERNAUT_HOME }
    if ($env:USERPROFILE)        { return $env:USERPROFILE }
    if ($env:HOME)               { return $env:HOME }
    return [Environment]::GetFolderPath('UserProfile')
}

function Get-DPAPIEntryPath {
    Join-Path (Get-DPAPIHome) '.juggernaut\bearer-token.dpapi.bin'
}

function Test-DPAPIAvailable {
    return ((Get-KeychainOS) -eq 'windows')
}

function Initialize-DPAPIAssembly {
    if (-not ('System.Security.Cryptography.ProtectedData' -as [type])) {
        # PS 5.1 ships the assembly but doesn't load it by default. PS 7 does.
        try { [Reflection.Assembly]::LoadWithPartialName('System.Security') | Out-Null } catch {}
    }
    return [bool]('System.Security.Cryptography.ProtectedData' -as [type])
}

function Set-DPAPIEntry {
    param([Parameter(Mandatory)][string]$Key)
    if ($env:JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL -eq '1') { return $false }
    if (-not (Test-DPAPIAvailable)) { return $false }
    if (-not (Initialize-DPAPIAssembly)) {
        Write-Warning 'Set-DPAPIEntry: System.Security.Cryptography.ProtectedData unavailable'
        return $false
    }
    $path = Get-DPAPIEntryPath
    $dir  = Split-Path -Parent $path
    try {
        if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
        $plain   = [Text.Encoding]::UTF8.GetBytes($Key)
        $entropy = [Text.Encoding]::UTF8.GetBytes($script:DPAPIEntropyLabel)
        $enc = [Security.Cryptography.ProtectedData]::Protect(
            $plain, $entropy, [Security.Cryptography.DataProtectionScope]::CurrentUser)
        [IO.File]::WriteAllBytes($path, $enc)
        return $true
    } catch {
        Write-Warning "Set-DPAPIEntry: protect/write failed: $_"
        return $false
    }
}

function Get-DPAPIEntry {
    # Returns the stored key as a non-empty string when the file exists AND
    # Unprotect succeeds. Returns $null when the file does not exist (silent).
    # Throws when the file exists but Unprotect fails — callers should surface
    # a loud error rather than silently falling through to another source.
    if (-not (Test-DPAPIAvailable)) { return $null }
    $path = Get-DPAPIEntryPath
    if (-not (Test-Path $path)) { return $null }
    if (-not (Initialize-DPAPIAssembly)) {
        throw "Get-DPAPIEntry: System.Security.Cryptography.ProtectedData unavailable"
    }
    try {
        $enc     = [IO.File]::ReadAllBytes($path)
        $entropy = [Text.Encoding]::UTF8.GetBytes($script:DPAPIEntropyLabel)
        $plain   = [Security.Cryptography.ProtectedData]::Unprotect(
            $enc, $entropy, [Security.Cryptography.DataProtectionScope]::CurrentUser)
        $val = [Text.Encoding]::UTF8.GetString($plain)
        if ([string]::IsNullOrEmpty($val)) { return $null }
        return $val
    } catch {
        throw "Get-DPAPIEntry: unprotect failed (file may be corrupt or from a different user): $_"
    }
}

function Remove-DPAPIEntry {
    $path = Get-DPAPIEntryPath
    if (Test-Path $path) {
        try { Remove-Item -Path $path -Force -ErrorAction Stop } catch {
            Write-Warning "Remove-DPAPIEntry: delete failed: $_"
        }
    }
}

# ---------------------------------------------------------------------------
# Top-level bearer-token abstraction. Callers don't need to know whether the
# value landed in Credential Manager or DPAPI — Save-/Read-/Remove-BearerToken
# pick the right backend and return a uniform result.
# ---------------------------------------------------------------------------

$script:BearerTokenCredManMaxChars = 1280

function Save-BearerToken {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)][string]$Key,
        [ValidateSet('auto','keychain','dpapi','profile')][string]$Mode = 'auto'
    )
    $os = Get-KeychainOS
    # profile: always write to profile token file regardless of platform.
    if ($Mode -eq 'profile') {
        if (Set-ProfileTokenEntry -Key $Key) {
            return @{ Ok = $true; Storage = 'profile'; Error = '' }
        }
        return @{ Ok = $false; Storage = 'none'; Error = 'profile token write failed' }
    }
    # Non-Windows: CredMan is 'keychain' (macOS Keychain, secret-tool). No DPAPI.
    if ($os -ne 'windows') {
        if ($Mode -eq 'dpapi') {
            return @{ Ok = $false; Storage = 'none'; Error = 'dpapi storage is Windows-only' }
        }
        if (Set-KeychainEntry -Key $Key) {
            return @{ Ok = $true; Storage = 'keychain'; Error = '' }
        }
        return @{ Ok = $false; Storage = 'none'; Error = 'keychain store failed' }
    }

    # Windows: auto decides between CredMan and DPAPI based on key length.
    $tryCredMan = switch ($Mode) {
        'auto'     { $Key.Length -le $script:BearerTokenCredManMaxChars }
        'keychain' { $true }
        'dpapi'    { $false }
    }
    $tryDpapi = switch ($Mode) {
        'auto'     { $true }
        'keychain' { $false }
        'dpapi'    { $true }
    }

    if ($tryCredMan) {
        if (Set-KeychainEntry -Key $Key) {
            # On success keep DPAPI file out of the way so reads are unambiguous.
            Remove-DPAPIEntry
            return @{ Ok = $true; Storage = 'keychain'; Error = '' }
        }
        if ($Mode -eq 'keychain') {
            return @{ Ok = $false; Storage = 'none'; Error = 'Credential Manager CredWrite failed (likely blob > 2560 bytes)' }
        }
        # auto: fall through to DPAPI
    }

    if ($tryDpapi) {
        if (Set-DPAPIEntry -Key $Key) {
            # Also clear any stale CredMan entry so reads pick the new DPAPI blob.
            Remove-KeychainEntry
            return @{ Ok = $true; Storage = 'dpapi'; Error = '' }
        }
        return @{ Ok = $false; Storage = 'none'; Error = 'DPAPI protect/write failed' }
    }

    return @{ Ok = $false; Storage = 'none'; Error = 'no storage backend attempted' }
}

function Read-BearerToken {
    # Return shape matches Save-BearerToken: { Value; Storage; Error }.
    # On Windows: DPAPI file first (no P/Invoke cost), then CredMan, then profile.
    # On Unix: keychain (secret-tool / security), then profile.
    $os = Get-KeychainOS
    if ($os -eq 'windows') {
        try {
            $v = Get-DPAPIEntry
            if (-not [string]::IsNullOrEmpty($v)) {
                return @{ Value = $v; Storage = 'dpapi'; Error = '' }
            }
        } catch {
            return @{ Value = $null; Storage = 'dpapi'; Error = "$_" }
        }
    }
    try {
        $v = Get-KeychainEntry
        if (-not [string]::IsNullOrEmpty($v)) {
            return @{ Value = $v; Storage = 'keychain'; Error = '' }
        }
    } catch {
        return @{ Value = $null; Storage = 'keychain'; Error = "$_" }
    }
    try {
        $v = Get-ProfileTokenEntry
        if (-not [string]::IsNullOrEmpty($v)) {
            return @{ Value = $v; Storage = 'profile'; Error = '' }
        }
    } catch {
        return @{ Value = $null; Storage = 'profile'; Error = "$_" }
    }
    return @{ Value = $null; Storage = 'none'; Error = '' }
}

function Remove-BearerToken {
    Remove-KeychainEntry
    if ((Get-KeychainOS) -eq 'windows') {
        Remove-DPAPIEntry
    }
    Remove-ProfileTokenEntry
}
