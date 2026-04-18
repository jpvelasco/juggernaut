# lib/keychain.ps1 — OS keychain abstraction for Juggernaut v2 (PowerShell).
# Mirror of lib/keychain.sh. Requires PowerShell 5.1+.

$script:KeychainService = 'juggernaut-bedrock'
$script:KeychainAccount = 'api-key'

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
    $svc = $script:KeychainService
    $acc = $script:KeychainAccount
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
            Add-Type -AssemblyName System.Runtime.InteropServices -ErrorAction SilentlyContinue
            $cred = New-Object PSCredential($acc, (ConvertTo-SecureString $Key -AsPlainText -Force))
            try {
                [void][Windows.Security.Credentials.PasswordVault]
            } catch {}
            # Prefer cmdkey (always available on Windows) for simplicity.
            cmdkey /delete:$svc 2>$null | Out-Null
            cmdkey /generic:$svc /user:$acc /pass:$Key 2>$null | Out-Null
            return $LASTEXITCODE -eq 0
        }
        default { return $false }
    }
}

function Get-KeychainEntry {
    $svc = $script:KeychainService
    $acc = $script:KeychainAccount
    $os  = Get-KeychainOS
    switch ($os) {
        'macos' {
            $val = security find-generic-password -s $svc -a $acc -w 2>$null
            return ($val -replace "`r","").Trim()
        }
        { $_ -in 'linux','wsl' } {
            $val = secret-tool lookup service $svc account $acc 2>$null
            return ($val -replace "`r","").Trim()
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
            Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition $src -ErrorAction SilentlyContinue
            $ptr = [IntPtr]::Zero
            if ([Win32.Cred]::CredRead($svc, 1, 0, [ref]$ptr)) {
                $c = [Runtime.InteropServices.Marshal]::PtrToStructure($ptr, [Type][Win32.Cred+CREDENTIAL])
                $val = ''
                if ($c.CredentialBlobSize -gt 0) {
                    $val = [Runtime.InteropServices.Marshal]::PtrToStringUni($c.CredentialBlob, $c.CredentialBlobSize / 2)
                }
                [Win32.Cred]::CredFree($ptr)
                return $val
            }
            return ''
        }
        default { return '' }
    }
}

function Remove-KeychainEntry {
    $svc = $script:KeychainService
    $acc = $script:KeychainAccount
    $os  = Get-KeychainOS
    switch ($os) {
        'macos'             { security delete-generic-password -s $svc -a $acc 2>$null | Out-Null }
        { $_ -in 'linux','wsl' } { secret-tool clear service $svc account $acc 2>$null | Out-Null }
        'windows'           { cmdkey /delete:$svc 2>$null | Out-Null }
    }
}

# Get-KeychainRetrievalExpression <shell>
# Returns the shell expression to embed in a profile for runtime key retrieval.
function Get-KeychainRetrievalExpression {
    param([Parameter(Mandatory)][string]$Shell)
    $svc = $script:KeychainService
    $acc = $script:KeychainAccount
    $os  = Get-KeychainOS
    $cmd = switch ($os) {
        'macos'             { "security find-generic-password -s '$svc' -a '$acc' -w 2>/dev/null" }
        { $_ -in 'linux','wsl' } { "secret-tool lookup service '$svc' account '$acc' 2>/dev/null" }
        'windows'           {
            # Same single-line PS block used in keychain.sh.
            "powershell.exe -NoProfile -Command `"Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition '[DllImport(\`\`\`\"advapi32.dll\`\`\`\", SetLastError=true, CharSet=CharSet.Unicode)] public static extern bool CredRead(string t, int ty, int f, out IntPtr c); [DllImport(\`\`\`\"advapi32.dll\`\`\`\")] public static extern void CredFree(IntPtr c); [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] public struct CREDENTIAL { public int Flags; public int Type; public string TargetName; public string Comment; public long LastWritten; public int CredentialBlobSize; public IntPtr CredentialBlob; public int Persist; public int AttributeCount; public IntPtr Attributes; public string TargetAlias; public string UserName; }'; `$p=[IntPtr]::Zero; if([Win32.Cred]::CredRead('$svc',1,0,[ref]`$p)){ `$c=[Runtime.InteropServices.Marshal]::PtrToStructure(`$p,[Type][Win32.Cred+CREDENTIAL]); if(`$c.CredentialBlobSize -gt 0){[Runtime.InteropServices.Marshal]::PtrToStringUni(`$c.CredentialBlob,`$c.CredentialBlobSize/2)}; [Win32.Cred]::CredFree(`$p) }`" 2>/dev/null | tr -d '``r'"
        }
        default { "echo ''" }
    }
    if ($Shell -eq 'fish') { return "($cmd)" }
    return "`$($cmd)"
}
