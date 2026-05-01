#!/usr/bin/env bash
# lib/keychain.sh — OS keychain abstraction for Juggernaut v2.
# Extracted from setup-claude-bedrock.sh. setup-claude-bedrock.sh is unchanged.
# Requires: bash 4+.

set -euo pipefail

KEYCHAIN_SERVICE="juggernaut-bedrock"
KEYCHAIN_ACCOUNT="api-key"

# keychain_detect_os
# Same logic as detect_os in setup-claude-bedrock.sh, kept local to avoid
# depending on the v1 script being sourced.
keychain_detect_os() {
  case "$OSTYPE" in
    darwin*)      echo "macos" ;;
    linux*)
      [[ -f /proc/version ]] && grep -qi microsoft /proc/version 2>/dev/null \
        && echo "wsl" || echo "linux" ;;
    msys*|mingw*) echo "gitbash" ;;
    cygwin*)      echo "cygwin" ;;
    *)            echo "unknown" ;;
  esac
}

# keychain_available
# Returns 0 if the OS keychain tool is present, 1 otherwise.
keychain_available() {
  local os
  os="$(keychain_detect_os)"
  case "$os" in
    macos)               command -v security    >/dev/null 2>&1 ;;
    linux|wsl)           command -v secret-tool >/dev/null 2>&1 ;;
    gitbash|cygwin)      command -v cmdkey.exe  >/dev/null 2>&1 \
                           || command -v powershell.exe >/dev/null 2>&1 ;;
    *)                   return 1 ;;
  esac
}

# keychain_store <api_key>
# Writes the key to the OS credential store. Returns 1 on failure.
keychain_store() {
  local key="$1"
  local os
  [[ "${JUGGERNAUT_TEST_KEYCHAIN_FORCE_FAIL:-0}" == "1" ]] && return 1
  os="$(keychain_detect_os)"
  case "$os" in
    macos)
      security add-generic-password \
        -U -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w "$key" 2>/dev/null
      ;;
    linux|wsl)
      printf '%s' "$key" | secret-tool store \
        --label="Juggernaut Bedrock API Key" \
        service "$KEYCHAIN_SERVICE" account "$KEYCHAIN_ACCOUNT" 2>/dev/null
      ;;
    gitbash|cygwin)
      cmdkey.exe /delete:"$KEYCHAIN_SERVICE" >/dev/null 2>&1 || true
      cmdkey.exe /generic:"$KEYCHAIN_SERVICE" \
        /user:"$KEYCHAIN_ACCOUNT" /pass:"$key" >/dev/null 2>&1
      ;;
    *)
      echo "keychain_store: unsupported OS '$os'" >&2
      return 1
      ;;
  esac
}

# keychain_get
# Prints the stored key to stdout. Exit codes:
#   0 — found, value printed to stdout
#   1 — not found (silent)
#   2 — tool error (human-readable message on stderr)
keychain_get() {
  local os rc value
  os="$(keychain_detect_os)"
  case "$os" in
    macos)
      # `security` exits 44 when the item is not found, 0 on success, other
      # non-zero codes on tool errors (missing binary, locked keychain).
      value="$(security find-generic-password \
        -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w 2>/dev/null)"
      rc=$?
      case "$rc" in
        0) printf '%s' "$value"; return 0 ;;
        44) return 1 ;;
        *) echo "keychain_get: macOS 'security' failed (exit $rc)" >&2; return 2 ;;
      esac
      ;;
    linux|wsl)
      # `secret-tool lookup` prints nothing and exits 0 when the attribute
      # tuple isn't present, making "not found" indistinguishable from
      # "found empty". Treat empty output as not-found. Non-zero exit that
      # isn't a missing attribute (e.g. D-Bus error, missing binary) → 2.
      if ! command -v secret-tool >/dev/null 2>&1; then
        echo "keychain_get: 'secret-tool' not found on PATH" >&2
        return 2
      fi
      value="$(secret-tool lookup \
        service "$KEYCHAIN_SERVICE" account "$KEYCHAIN_ACCOUNT" 2>/dev/null)"
      rc=$?
      if [[ "$rc" -eq 0 && -n "$value" ]]; then
        printf '%s' "$value"; return 0
      fi
      if [[ "$rc" -eq 0 || "$rc" -eq 1 ]]; then
        return 1
      fi
      echo "keychain_get: 'secret-tool' failed (exit $rc)" >&2
      return 2
      ;;
    gitbash|cygwin)
      if ! command -v powershell.exe >/dev/null 2>&1; then
        echo "keychain_get: 'powershell.exe' not found on PATH" >&2
        return 2
      fi
      value="$(powershell.exe -NoProfile -Command "
        Add-Type -Namespace 'Win32' -Name 'Credential' -MemberDefinition '
          [DllImport(\"advapi32.dll\", SetLastError = true, CharSet = CharSet.Unicode)]
          public static extern bool CredRead(string target, int type, int flags, out IntPtr credential);
          [DllImport(\"advapi32.dll\")]
          public static extern void CredFree(IntPtr credential);
          [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
          public struct CREDENTIAL {
            public int Flags; public int Type;
            public string TargetName; public string Comment;
            public long LastWritten; public int CredentialBlobSize;
            public IntPtr CredentialBlob; public int Persist;
            public int AttributeCount; public IntPtr Attributes;
            public string TargetAlias; public string UserName;
          }
        '
        \$ptr = [IntPtr]::Zero
        if ([Win32.Credential]::CredRead('$KEYCHAIN_SERVICE', 1, 0, [ref]\$ptr)) {
          \$cred = [Runtime.InteropServices.Marshal]::PtrToStructure(\$ptr, [Type][Win32.Credential+CREDENTIAL])
          if (\$cred.CredentialBlobSize -gt 0) {
            [Runtime.InteropServices.Marshal]::PtrToStringUni(\$cred.CredentialBlob, \$cred.CredentialBlobSize / 2)
          }
          [Win32.Credential]::CredFree(\$ptr)
        }
      " 2>/dev/null | tr -d '\r')"
      rc=$?
      if [[ "$rc" -ne 0 ]]; then
        echo "keychain_get: 'powershell.exe' failed (exit $rc)" >&2
        return 2
      fi
      if [[ -n "$value" ]]; then
        printf '%s' "$value"; return 0
      fi
      return 1
      ;;
    *)
      echo "keychain_get: unsupported OS '$os'" >&2
      return 2
      ;;
  esac
}

# keychain_delete
# Removes the stored key. Silent if not found.
keychain_delete() {
  local os
  os="$(keychain_detect_os)"
  case "$os" in
    macos)
      security delete-generic-password \
        -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" 2>/dev/null || true
      ;;
    linux|wsl)
      secret-tool clear \
        service "$KEYCHAIN_SERVICE" account "$KEYCHAIN_ACCOUNT" 2>/dev/null || true
      ;;
    gitbash|cygwin)
      cmdkey.exe /delete:"$KEYCHAIN_SERVICE" >/dev/null 2>&1 || true
      ;;
  esac
}

# keychain_get_command <shell>
# Prints the shell expression used at profile startup to retrieve the key.
# Output is ready to embed verbatim in an export/set -gx line.
keychain_get_command() {
  local shell="$1"
  local os cmd
  os="$(keychain_detect_os)"

  case "$os" in
    macos)
      cmd="security find-generic-password -s '$KEYCHAIN_SERVICE' -a '$KEYCHAIN_ACCOUNT' -w 2>/dev/null"
      ;;
    linux|wsl)
      cmd="secret-tool lookup service '$KEYCHAIN_SERVICE' account '$KEYCHAIN_ACCOUNT' 2>/dev/null"
      ;;
    gitbash|cygwin)
      # Single-line PowerShell command that reads from Windows Credential Manager.
      cmd="powershell.exe -NoProfile -Command \"Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition '[DllImport(\\\"advapi32.dll\\\", SetLastError=true, CharSet=CharSet.Unicode)] public static extern bool CredRead(string t, int ty, int f, out IntPtr c); [DllImport(\\\"advapi32.dll\\\")] public static extern void CredFree(IntPtr c); [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] public struct CREDENTIAL { public int Flags; public int Type; public string TargetName; public string Comment; public long LastWritten; public int CredentialBlobSize; public IntPtr CredentialBlob; public int Persist; public int AttributeCount; public IntPtr Attributes; public string TargetAlias; public string UserName; }'; \\\$p=[IntPtr]::Zero; if([Win32.Cred]::CredRead('$KEYCHAIN_SERVICE',1,0,[ref]\\\$p)){ \\\$c=[Runtime.InteropServices.Marshal]::PtrToStructure(\\\$p,[Type][Win32.Cred+CREDENTIAL]); if(\\\$c.CredentialBlobSize -gt 0){[Runtime.InteropServices.Marshal]::PtrToStringUni(\\\$c.CredentialBlob,\\\$c.CredentialBlobSize/2)}; [Win32.Cred]::CredFree(\\\$p) }\" 2>/dev/null | tr -d '\\r'"
      ;;
    *)
      cmd="echo ''"
      ;;
  esac

  if [[ "$shell" == "fish" ]]; then
    printf '(%s)' "$cmd"
  else
    printf '$(%s)' "$cmd"
  fi
}
