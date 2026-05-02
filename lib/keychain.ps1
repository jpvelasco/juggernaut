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
            Add-Type -Namespace 'Win32' -Name 'CredWriteApi' -MemberDefinition $src -ErrorAction SilentlyContinue
            [Win32.CredWriteApi]::CredDelete($svc, 1, 0) | Out-Null

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
                return [Win32.CredWriteApi]::CredWrite([ref]$cred, 0)
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
