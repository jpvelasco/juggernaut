
# BEGIN: Claude Code Bedrock Configuration
# Auth mode: bedrock-api-key
# Region: us-east-1
# Storage: keychain
# EffortLevel: xhigh
$env:AWS_REGION = 'us-east-1'
$env:CLAUDE_CODE_USE_BEDROCK = '1'
$env:CLAUDE_CODE_MAX_OUTPUT_TOKENS = '32768'
$env:MAX_THINKING_TOKENS = '65536'
$env:ANTHROPIC_MODEL = 'global.anthropic.claude-sonnet-4-6'
$env:ANTHROPIC_DEFAULT_OPUS_MODEL = 'global.anthropic.claude-opus-4-7[1m]'
$env:ANTHROPIC_DEFAULT_SONNET_MODEL = 'global.anthropic.claude-sonnet-4-6'
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = 'global.anthropic.claude-haiku-4-5-20251001-v1:0'
$env:CLAUDE_CODE_SUBAGENT_MODEL = 'global.anthropic.claude-haiku-4-5-20251001-v1:0'
$env:CLAUDE_CODE_EFFORT_LEVEL = 'xhigh'
$env:ENABLE_PROMPT_CACHING_1H = '1'
$env:DISABLE_ERROR_REPORTING = '1'
$env:DISABLE_TELEMETRY = '1'
$env:DISABLE_AUTOUPDATE = '1'
$env:DISABLE_BUG_COMMAND = '1'
function Get-JuggernautBedrockApiKey {
    $src = @"
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
"@
    Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition $src -ErrorAction SilentlyContinue
    $ptr = [IntPtr]::Zero
    if ([Win32.Cred]::CredRead('juggernaut-bedrock', 1, 0, [ref]$ptr)) {
        try {
            $c = [Runtime.InteropServices.Marshal]::PtrToStructure($ptr, [Type][Win32.Cred+CREDENTIAL])
            if ($c.CredentialBlobSize -gt 0) {
                return [Runtime.InteropServices.Marshal]::PtrToStringUni($c.CredentialBlob, $c.CredentialBlobSize / 2)
            }
        } finally {
            [Win32.Cred]::CredFree($ptr)
        }
    }
    return ''
}
$env:AWS_BEARER_TOKEN_BEDROCK = Get-JuggernautBedrockApiKey
# END: Claude Code Bedrock Configuration
