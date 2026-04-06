#!/usr/bin/env bash

# Claude Code - Amazon Bedrock Setup Script
# Usage: ./setup-claude-bedrock.sh [bash|zsh|fish] [OPTIONS]
#
# Authentication modes:
#   --auth=iam       Use IAM/SSO credentials (default)
#   --auth=api-key   Use Bedrock API key (simpler, no AWS creds needed)

set -e

#───────────────────────────────────────────────────────────────────────────────
# Bash Version Check
#───────────────────────────────────────────────────────────────────────────────

if [[ -z "$BASH_VERSION" ]]; then
    echo "This script requires bash"
    exit 1
fi

if [[ "${BASH_VERSINFO[0]}" -lt 4 ]]; then
    echo "This script requires Bash 4.0 or later (found: $BASH_VERSION)"
    echo ""
    echo "Upgrade instructions:"
    echo "  macOS:  brew install bash"
    echo "  Ubuntu: sudo apt install bash"
    echo "  RHEL:   sudo yum install bash"
    exit 1
fi

#───────────────────────────────────────────────────────────────────────────────
# Configuration Registry
#───────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/bedrock-config.json"

declare -A SHELL_CONFIGS=(
    [bash]="$HOME/.bashrc"
    [zsh]="$HOME/.zshrc"
    [fish]="$HOME/.config/fish/config.fish"
)

declare -A SHELL_EXPORT_SYNTAX=(
    [bash]="export"
    [zsh]="export"
    [fish]="set -gx"
)

declare -A SHELL_DISPLAY_NAMES=(
    [bash]="Bash"
    [zsh]="Zsh"
    [fish]="Fish"
)

# Valid authentication modes
declare -a VALID_AUTH_MODES=(iam api-key)

# Valid storage modes
declare -a VALID_STORAGE_MODES=(profile keychain)

#───────────────────────────────────────────────────────────────────────────────
# JSON Config Loading (using jq or python fallback)
# All queries use jq-style dot notation: .key, .key.subkey
# Python fallback uses sys.argv to avoid shell injection risks
#───────────────────────────────────────────────────────────────────────────────

json_get() {
    local file=$1
    local query=$2

    if command -v jq >/dev/null 2>&1; then
        jq -r "$query" "$file" 2>/dev/null
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "
import json,sys,functools
data=json.load(open(sys.argv[1]))
keys=[k for k in sys.argv[2].split('.') if k]
print(functools.reduce(lambda d,k: d[k], keys, data))
" "$file" "$query" 2>/dev/null | tr -d '\r'
    else
        echo "Error: jq or python3 required" >&2
        return 1
    fi
}

json_get_keys() {
    local file=$1
    local query=$2

    if command -v jq >/dev/null 2>&1; then
        jq -r "$query | keys[]" "$file" 2>/dev/null
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "
import json,sys,functools
data=json.load(open(sys.argv[1]))
keys=[k for k in sys.argv[2].split('.') if k]
obj=functools.reduce(lambda d,k: d[k], keys, data)
print('\n'.join(obj.keys()))
" "$file" "$query" 2>/dev/null | tr -d '\r'
    else
        echo "Error: jq or python3 required" >&2
        return 1
    fi
}

json_get_array() {
    local file=$1
    local query=$2

    if command -v jq >/dev/null 2>&1; then
        jq -r "$query[]" "$file" 2>/dev/null
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "
import json,sys,functools
data=json.load(open(sys.argv[1]))
keys=[k for k in sys.argv[2].split('.') if k]
arr=functools.reduce(lambda d,k: d[k], keys, data)
print('\n'.join(str(x) for x in arr))
" "$file" "$query" 2>/dev/null | tr -d '\r'
    else
        echo "Error: jq or python3 required" >&2
        return 1
    fi
}

load_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        echo "Warning: Config file not found: $CONFIG_FILE" >&2
        echo "Using built-in defaults" >&2
        return 1
    fi

    # Load environment variables
    local keys
    keys=$(json_get_keys "$CONFIG_FILE" '.environment')
    if [[ -n "$keys" ]]; then
        while IFS= read -r key; do
            local value
            value=$(json_get "$CONFIG_FILE" ".environment.$key")
            BEDROCK_CONFIG["$key"]="$value"
            CONFIG_KEY_ORDER+=("$key")
        done <<< "$keys"
    fi

    # Load valid regions
    local regions
    regions=$(json_get_array "$CONFIG_FILE" '.regions')
    if [[ -n "$regions" ]]; then
        VALID_REGIONS=()
        while IFS= read -r region; do
            VALID_REGIONS+=("$region")
        done <<< "$regions"
    fi

    # Load defaults
    DEFAULT_REGION=$(json_get "$CONFIG_FILE" '.defaults.region')
    DEFAULT_AUTH=$(json_get "$CONFIG_FILE" '.defaults.auth_mode')
}

# Initialize config arrays
declare -A BEDROCK_CONFIG
declare -a CONFIG_KEY_ORDER=(AWS_REGION)  # AWS_REGION always first, rest loaded from JSON
declare -a VALID_REGIONS

# Load configuration from JSON
load_config

#───────────────────────────────────────────────────────────────────────────────
# Detection Functions
#───────────────────────────────────────────────────────────────────────────────

detect_os() {
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

detect_shell() {
    if [[ -n "$ZSH_VERSION" ]]; then
        echo "zsh"
    elif [[ -n "$BASH_VERSION" ]]; then
        echo "bash"
    elif [[ -n "$FISH_VERSION" ]]; then
        echo "fish"
    else
        basename "$SHELL"
    fi
}

is_valid_shell() {
    local shell=$1
    [[ -n "${SHELL_CONFIGS[$shell]}" ]]
}

is_valid_region() {
    local region=$1
    local r
    for r in "${VALID_REGIONS[@]}"; do
        [[ "$r" == "$region" ]] && return 0
    done
    return 1
}

is_valid_auth_mode() {
    local mode=$1
    local m
    for m in "${VALID_AUTH_MODES[@]}"; do
        [[ "$m" == "$mode" ]] && return 0
    done
    return 1
}

is_valid_storage_mode() {
    local mode=$1
    local m
    for m in "${VALID_STORAGE_MODES[@]}"; do
        [[ "$m" == "$mode" ]] && return 0
    done
    return 1
}

#───────────────────────────────────────────────────────────────────────────────
# Keychain Functions (OS-specific secure credential storage)
#───────────────────────────────────────────────────────────────────────────────

KEYCHAIN_SERVICE="juggernaut-bedrock"
KEYCHAIN_ACCOUNT="api-key"

# Detect if keychain is available on this system
keychain_available() {
    local os=$(detect_os)
    case "$os" in
        macos)
            command -v security >/dev/null 2>&1
            ;;
        linux|wsl)
            command -v secret-tool >/dev/null 2>&1
            ;;
        gitbash|cygwin)
            # Windows - check for cmdkey (built-in) or PowerShell
            command -v cmdkey.exe >/dev/null 2>&1 || command -v powershell.exe >/dev/null 2>&1
            ;;
        *)
            return 1
            ;;
    esac
}

# Store API key in system keychain
keychain_store() {
    local key=$1
    local os=$(detect_os)

    case "$os" in
        macos)
            # Delete existing entry first (ignore errors)
            security delete-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" 2>/dev/null || true
            # Add new entry
            security add-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w "$key" 2>/dev/null
            ;;
        linux|wsl)
            # secret-tool will overwrite existing entry
            echo -n "$key" | secret-tool store --label="Juggernaut Bedrock API Key" \
                service "$KEYCHAIN_SERVICE" account "$KEYCHAIN_ACCOUNT" 2>/dev/null
            ;;
        gitbash|cygwin)
            # Use cmdkey for Windows Credential Manager
            cmdkey.exe /delete:"$KEYCHAIN_SERVICE" >/dev/null 2>&1 || true
            cmdkey.exe /generic:"$KEYCHAIN_SERVICE" /user:"$KEYCHAIN_ACCOUNT" /pass:"$key" >/dev/null 2>&1
            ;;
        *)
            return 1
            ;;
    esac
}

# Retrieve API key from system keychain
keychain_get() {
    local os=$(detect_os)

    case "$os" in
        macos)
            security find-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" -w 2>/dev/null
            ;;
        linux|wsl)
            secret-tool lookup service "$KEYCHAIN_SERVICE" account "$KEYCHAIN_ACCOUNT" 2>/dev/null
            ;;
        gitbash|cygwin)
            # Use PowerShell with .NET CredentialManager to read from Windows Credential Manager
            powershell.exe -NoProfile -Command "
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
            " 2>/dev/null | tr -d '\r'
            ;;
        *)
            return 1
            ;;
    esac
}

# Delete API key from system keychain
keychain_delete() {
    local os=$(detect_os)

    case "$os" in
        macos)
            security delete-generic-password -s "$KEYCHAIN_SERVICE" -a "$KEYCHAIN_ACCOUNT" 2>/dev/null || true
            ;;
        linux|wsl)
            secret-tool clear service "$KEYCHAIN_SERVICE" account "$KEYCHAIN_ACCOUNT" 2>/dev/null || true
            ;;
        gitbash|cygwin)
            cmdkey.exe /delete:"$KEYCHAIN_SERVICE" >/dev/null 2>&1 || true
            ;;
    esac
}

# Generate the shell command to retrieve key from keychain
keychain_get_command() {
    local shell=$1
    local os=$(detect_os)
    local cmd=""

    case "$os" in
        macos)
            cmd="security find-generic-password -s '$KEYCHAIN_SERVICE' -a '$KEYCHAIN_ACCOUNT' -w 2>/dev/null"
            ;;
        linux|wsl)
            cmd="secret-tool lookup service '$KEYCHAIN_SERVICE' account '$KEYCHAIN_ACCOUNT' 2>/dev/null"
            ;;
        gitbash|cygwin)
            cmd="powershell.exe -NoProfile -Command \"Add-Type -Namespace 'Win32' -Name 'Cred' -MemberDefinition '[DllImport(\\\"advapi32.dll\\\", SetLastError=true, CharSet=CharSet.Unicode)] public static extern bool CredRead(string t, int ty, int f, out IntPtr c); [DllImport(\\\"advapi32.dll\\\")] public static extern void CredFree(IntPtr c); [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)] public struct CREDENTIAL { public int Flags; public int Type; public string TargetName; public string Comment; public long LastWritten; public int CredentialBlobSize; public IntPtr CredentialBlob; public int Persist; public int AttributeCount; public IntPtr Attributes; public string TargetAlias; public string UserName; }'; \\\$p=[IntPtr]::Zero; if([Win32.Cred]::CredRead('$KEYCHAIN_SERVICE',1,0,[ref]\\\$p)){ \\\$c=[Runtime.InteropServices.Marshal]::PtrToStructure(\\\$p,[Type][Win32.Cred+CREDENTIAL]); if(\\\$c.CredentialBlobSize -gt 0){[Runtime.InteropServices.Marshal]::PtrToStringUni(\\\$c.CredentialBlob,\\\$c.CredentialBlobSize/2)}; [Win32.Cred]::CredFree(\\\$p) }\" 2>/dev/null | tr -d '\\r'"
            ;;
    esac

    # Format for shell syntax
    if [[ "$shell" == "fish" ]]; then
        echo "($cmd)"
    else
        echo "\$($cmd)"
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Config Generation (Template Pattern)
#───────────────────────────────────────────────────────────────────────────────

generate_config_block() {
    local shell=$1
    local region=$2
    local auth_mode=$3
    local api_key=$4
    local storage_mode=$5
    local syntax="${SHELL_EXPORT_SYNTAX[$shell]}"
    local config=""

    config+=$'\n'"# BEGIN: Claude Code Bedrock Configuration"$'\n'
    config+="# Auth mode: $auth_mode"$'\n'
    if [[ -n "$CUSTOM_MODEL" ]]; then
        config+="# Model: $CUSTOM_MODEL"$'\n'
    fi
    if [[ -n "$CUSTOM_FAST_MODEL" ]]; then
        config+="# FastModel: $CUSTOM_FAST_MODEL"$'\n'
    fi
    if [[ -n "$CUSTOM_OPUS_MODEL" ]]; then
        config+="# OpusModel: $CUSTOM_OPUS_MODEL"$'\n'
    fi
    if [[ -n "$CUSTOM_SONNET_MODEL" ]]; then
        config+="# SonnetModel: $CUSTOM_SONNET_MODEL"$'\n'
    fi
    if [[ -n "$CUSTOM_HAIKU_MODEL" ]]; then
        config+="# HaikuModel: $CUSTOM_HAIKU_MODEL"$'\n'
    fi
    if [[ "$storage_mode" == "keychain" ]]; then
        config+="# Storage: keychain (encrypted)"$'\n'
    fi

    # Unset conflicting auth variables to prevent credential conflicts
    if [[ "$auth_mode" == "api-key" ]]; then
        # Using API key - unset AWS credentials/profile that might cause confusion
        if [[ "$shell" == "fish" ]]; then
            config+="set -e AWS_ACCESS_KEY_ID 2>/dev/null"$'\n'
            config+="set -e AWS_SECRET_ACCESS_KEY 2>/dev/null"$'\n'
            config+="set -e AWS_SESSION_TOKEN 2>/dev/null"$'\n'
            config+="set -e AWS_PROFILE 2>/dev/null"$'\n'
        else
            config+="unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_PROFILE 2>/dev/null || true"$'\n'
        fi
    else
        # Using IAM/SSO - unset API key that might interfere
        if [[ "$shell" == "fish" ]]; then
            config+="set -e AWS_BEARER_TOKEN_BEDROCK 2>/dev/null"$'\n'
        else
            config+="unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true"$'\n'
        fi
    fi

    for key in "${CONFIG_KEY_ORDER[@]}"; do
        local value
        if [[ "$key" == "AWS_REGION" ]]; then
            value="$region"
        elif [[ "$key" == "ANTHROPIC_MODEL" && -n "$CUSTOM_MODEL" ]]; then
            value="$CUSTOM_MODEL"
        elif [[ "$key" == "ANTHROPIC_SMALL_FAST_MODEL" && -n "$CUSTOM_FAST_MODEL" ]]; then
            value="$CUSTOM_FAST_MODEL"
        elif [[ "$key" == "ANTHROPIC_DEFAULT_OPUS_MODEL" && -n "$CUSTOM_OPUS_MODEL" ]]; then
            value="$CUSTOM_OPUS_MODEL"
        elif [[ "$key" == "ANTHROPIC_DEFAULT_SONNET_MODEL" && -n "$CUSTOM_SONNET_MODEL" ]]; then
            value="$CUSTOM_SONNET_MODEL"
        elif [[ "$key" == "ANTHROPIC_DEFAULT_HAIKU_MODEL" && -n "$CUSTOM_HAIKU_MODEL" ]]; then
            value="$CUSTOM_HAIKU_MODEL"
        else
            value="${BEDROCK_CONFIG[$key]}"
        fi

        if [[ "$shell" == "fish" ]]; then
            config+="$syntax $key $value"$'\n'
        else
            config+="$syntax $key=$value"$'\n'
        fi
    done

    # Add API key if using api-key auth mode
    if [[ "$auth_mode" == "api-key" && -n "$api_key" ]]; then
        if [[ "$storage_mode" == "keychain" ]]; then
            # Retrieve from keychain at shell startup
            local keychain_cmd
            keychain_cmd=$(keychain_get_command "$shell")
            if [[ "$shell" == "fish" ]]; then
                config+="$syntax AWS_BEARER_TOKEN_BEDROCK $keychain_cmd"$'\n'
            else
                config+="$syntax AWS_BEARER_TOKEN_BEDROCK=$keychain_cmd"$'\n'
            fi
        else
            # Store directly in profile (legacy behavior)
            if [[ "$shell" == "fish" ]]; then
                config+="$syntax AWS_BEARER_TOKEN_BEDROCK $api_key"$'\n'
            else
                config+="$syntax AWS_BEARER_TOKEN_BEDROCK=$api_key"$'\n'
            fi
        fi
    fi

    config+="# END: Claude Code Bedrock Configuration"$'\n'

    echo "$config"
}

#───────────────────────────────────────────────────────────────────────────────
# Utility Functions
#───────────────────────────────────────────────────────────────────────────────

sed_inplace() {
    if [[ "$OSTYPE" == darwin* ]]; then
        sed -i '' "$@"
    else
        sed -i "$@"
    fi
}

backup_config_file() {
    local config_file=$1
    local backup_file="${config_file}.backup.$(date +%Y%m%d_%H%M%S)"

    if [[ -f "$config_file" ]]; then
        if cp "$config_file" "$backup_file" 2>/dev/null; then
            echo "Backup created: $backup_file"
            return 0
        else
            echo "Warning: Could not create backup at $backup_file" >&2
            return 1
        fi
    fi
    return 0
}

write_config_to_file() {
    local config_file=$1
    local config_block=$2

    if ! echo "$config_block" >> "$config_file" 2>/dev/null; then
        echo "" >&2
        echo "ERROR: Cannot write to $config_file" >&2
        echo "Possible causes:" >&2
        echo "  - File or directory is read-only" >&2
        echo "  - Insufficient permissions" >&2
        echo "  - Disk is full" >&2
        echo "" >&2
        echo "Try running with appropriate permissions or check disk space." >&2
        exit 1
    fi
}

remove_existing_config() {
    local config_file=$1

    # Remove config with markers (current format)
    sed_inplace '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$config_file"

    # Remove config without markers (legacy format - match from header to last known env var)
    sed_inplace '/# Claude Code - Amazon Bedrock Configuration/,/ANTHROPIC_SMALL_FAST_MODEL\|ANTHROPIC_MODEL/d' "$config_file"
}

# Detect existing auth mode from config file
# Returns: "iam", "api-key", or empty string if not found
detect_existing_auth_mode() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        grep "^# Auth mode:" "$config_file" 2>/dev/null | head -1 | sed 's/^# Auth mode: //'
    fi
}

# Detect existing custom model from config file
detect_existing_model() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        grep "^# Model:" "$config_file" 2>/dev/null | head -1 | sed 's/^# Model: //'
    fi
}

# Detect existing storage mode from config file
# Returns: "keychain" or "profile" (absence of marker = profile)
detect_existing_storage_mode() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        if grep -q "^# Storage: keychain" "$config_file" 2>/dev/null; then
            echo "keychain"
            return
        fi
    fi
    echo "profile"
}

# Detect existing custom fast model from config file
detect_existing_fast_model() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        grep "^# FastModel:" "$config_file" 2>/dev/null | head -1 | sed 's/^# FastModel: //'
    fi
}

# Detect existing custom opus model from config file
detect_existing_opus_model() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        grep "^# OpusModel:" "$config_file" 2>/dev/null | head -1 | sed 's/^# OpusModel: //'
    fi
}

# Detect existing custom sonnet model from config file
detect_existing_sonnet_model() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        grep "^# SonnetModel:" "$config_file" 2>/dev/null | head -1 | sed 's/^# SonnetModel: //'
    fi
}

# Detect existing custom haiku model from config file
detect_existing_haiku_model() {
    local config_file=$1
    if [[ -f "$config_file" ]]; then
        grep "^# HaikuModel:" "$config_file" 2>/dev/null | head -1 | sed 's/^# HaikuModel: //'
    fi
}

# Validate model ID format
validate_model_id() {
    local model_id=$1
    local model_type=$2  # "model" or "fast-model"

    # "default" is a special value to reset to bedrock-config.json
    if [[ "$model_id" == "default" ]]; then
        return 0
    fi

    # Non-empty check
    if [[ -z "$model_id" ]]; then
        echo "Error: --$model_type model ID cannot be empty" >&2
        return 1
    fi

    # Basic format check (Bedrock model ID patterns)
    if [[ ! "$model_id" =~ ^([a-z][-a-z0-9]*\.)?anthropic\. ]]; then
        echo "Warning: '$model_id' doesn't match expected Bedrock model ID format" >&2
        echo "Expected patterns: anthropic.claude-*, global.anthropic.claude-*, us.anthropic.claude-*" >&2
    fi

    return 0
}

# Warn user about custom model usage
warn_custom_model() {
    local model_id=$1
    local model_type=$2

    echo ""
    echo "⚠️  Custom $model_type model: $model_id"
    echo "   Cannot validate without working AWS credentials."
    echo "   Ensure this model is available in your Bedrock region."
    echo ""

    if [[ "$FORCE" != true && "$DRY_RUN" != true ]]; then
        read -p "Continue with custom model? (y/n) " -n 1 -r
        echo
        [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Setup cancelled"; exit 0; }
    fi
}

show_help() {
    cat << 'EOF'
Claude Code - Amazon Bedrock Setup Script

Usage: ./setup-claude-bedrock.sh [SHELL] [OPTIONS]

Arguments:
  SHELL                  Target shell: bash, zsh, or fish (auto-detected if omitted)

Options:
  --auth=MODE            Authentication mode: iam (default) or api-key
  --bedrock-key=KEY      Bedrock API key (optional; prompts if not provided)
  --preserve-key         Reuse existing API key from environment (no prompt)
  --storage=MODE         Where to store API key: profile or keychain
                         Default: keychain on macOS/Windows, profile on Linux
  --region=REGION        AWS region (default: us-west-2)
  --model=ID             Custom primary model (use "default" to reset)
  --fast-model=ID        Custom fast model (use "default" to reset)
  --opus-model=ID        Custom opus model (use "default" to reset)
  --sonnet-model=ID      Custom sonnet model (use "default" to reset)
  --haiku-model=ID       Custom haiku model (use "default" to reset)
  --global               Use global inference profiles (default)
  --model-prefix=PREFIX  Inference profile prefix (e.g., us, eu, ap)
  --dry-run              Preview changes without modifying files
  --force, -f            Skip confirmation prompts
  --version, -v          Show version
  --help, -h             Show this help message

Authentication Modes:
  iam        Use AWS IAM/SSO credentials (default)
             Requires: aws configure, SSO login, or IAM role

  api-key    Use Bedrock API key (simpler setup)
             Prompts securely if --bedrock-key not provided
             Get key from: AWS Console → Bedrock → API keys

Storage Modes:
  profile    Store API key directly in shell profile
             Key is plaintext but protected by file permissions
             Default on Linux (keychain requires libsecret-tools)

  keychain   Store API key in system keychain (more secure)
             macOS: Keychain Access (default)
             Linux: Secret Service (GNOME Keyring / KWallet)
             Windows: Credential Manager (default)

Examples:
  # IAM/SSO authentication (default)
  ./setup-claude-bedrock.sh
  ./setup-claude-bedrock.sh zsh --region=us-east-1

  # API key authentication (interactive - recommended, more secure)
  ./setup-claude-bedrock.sh --auth=api-key

  # API key with secure keychain storage (most secure)
  ./setup-claude-bedrock.sh --auth=api-key --storage=keychain

  # API key authentication (inline - for scripting/CI)
  ./setup-claude-bedrock.sh --auth=api-key --bedrock-key=br-xxxxxxxxxxxx

  # Update config while preserving existing API key
  ./setup-claude-bedrock.sh --auth=api-key --preserve-key

  # Custom model IDs
  ./setup-claude-bedrock.sh --model=anthropic.claude-3-opus-20240229-v1:0
  ./setup-claude-bedrock.sh --fast-model=anthropic.claude-3-haiku-20240307-v1:0

  # Reset custom model back to bedrock-config.json default
  ./setup-claude-bedrock.sh --model=default

  # Preview changes
  ./setup-claude-bedrock.sh --dry-run
  ./setup-claude-bedrock.sh --auth=api-key --bedrock-key=br-xxx --dry-run
EOF
}

#───────────────────────────────────────────────────────────────────────────────
# Argument Parsing
#───────────────────────────────────────────────────────────────────────────────

DRY_RUN=false
FORCE=false
PRESERVE_KEY=false
AWS_REGION="${DEFAULT_REGION:-us-west-2}"
SHELL_TYPE=""
AUTH_MODE=""                 # Don't set default yet - may be detected from existing config
AUTH_MODE_EXPLICIT=false     # Track if user explicitly set --auth flag
STORAGE_MODE="profile"
STORAGE_MODE_EXPLICIT=false  # Track if user explicitly set --storage flag
BEDROCK_API_KEY=""
CUSTOM_MODEL=""
CUSTOM_FAST_MODEL=""
MODEL_EXPLICIT=false
FAST_MODEL_EXPLICIT=false
CUSTOM_OPUS_MODEL=""
CUSTOM_SONNET_MODEL=""
CUSTOM_HAIKU_MODEL=""
OPUS_MODEL_EXPLICIT=false
SONNET_MODEL_EXPLICIT=false
HAIKU_MODEL_EXPLICIT=false
MODEL_PREFIX=""
MODEL_PREFIX_EXPLICIT=false
USE_GLOBAL=false

parse_arguments() {
    for arg in "$@"; do
        case "$arg" in
            --dry-run)
                DRY_RUN=true
                ;;
            --force|-f)
                FORCE=true
                ;;
            --region=*)
                AWS_REGION="${arg#--region=}"
                ;;
            --auth=*)
                AUTH_MODE="${arg#--auth=}"
                AUTH_MODE_EXPLICIT=true   # User explicitly set auth mode
                ;;
            --bedrock-key=*)
                BEDROCK_API_KEY="${arg#--bedrock-key=}"
                ;;
            --preserve-key)
                PRESERVE_KEY=true
                ;;
            --storage=*)
                STORAGE_MODE="${arg#--storage=}"
                STORAGE_MODE_EXPLICIT=true
                ;;
            --model=*)
                CUSTOM_MODEL="${arg#--model=}"
                MODEL_EXPLICIT=true
                ;;
            --fast-model=*)
                CUSTOM_FAST_MODEL="${arg#--fast-model=}"
                FAST_MODEL_EXPLICIT=true
                ;;
            --opus-model=*)
                CUSTOM_OPUS_MODEL="${arg#--opus-model=}"
                OPUS_MODEL_EXPLICIT=true
                ;;
            --sonnet-model=*)
                CUSTOM_SONNET_MODEL="${arg#--sonnet-model=}"
                SONNET_MODEL_EXPLICIT=true
                ;;
            --haiku-model=*)
                CUSTOM_HAIKU_MODEL="${arg#--haiku-model=}"
                HAIKU_MODEL_EXPLICIT=true
                ;;
            --global)
                USE_GLOBAL=true
                ;;
            --model-prefix=*)
                MODEL_PREFIX="${arg#--model-prefix=}"
                MODEL_PREFIX_EXPLICIT=true
                ;;
            --version|-v)
                cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"
                exit 0
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            bash|zsh|fish)
                SHELL_TYPE="$arg"
                ;;
            *)
                echo "Warning: Unknown argument '$arg' (ignored)"
                ;;
        esac
    done

    # Auto-detect shell if not specified
    if [[ -z "$SHELL_TYPE" ]]; then
        SHELL_TYPE=$(detect_shell)
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Validation
#───────────────────────────────────────────────────────────────────────────────

validate_inputs() {
    # Validate shell
    if ! is_valid_shell "$SHELL_TYPE"; then
        echo "Unsupported shell: $SHELL_TYPE"
        echo "Supported shells: bash, zsh, fish"
        exit 1
    fi

    # Validate auth mode
    if ! is_valid_auth_mode "$AUTH_MODE"; then
        echo "Invalid auth mode: $AUTH_MODE"
        echo "Valid modes: iam, api-key"
        exit 1
    fi

    # Validate storage mode
    if ! is_valid_storage_mode "$STORAGE_MODE"; then
        echo "Invalid storage mode: $STORAGE_MODE"
        echo "Valid modes: profile, keychain"
        exit 1
    fi

    # Check keychain availability when using keychain storage
    if [[ "$STORAGE_MODE" == "keychain" ]]; then
        if ! keychain_available; then
            local os=$(detect_os)
            echo "Error: Keychain storage not available on this system" >&2
            echo "" >&2
            case "$os" in
                linux|wsl)
                    echo "Install libsecret tools:" >&2
                    echo "  Ubuntu/Debian: sudo apt install libsecret-tools" >&2
                    echo "  Fedora/RHEL:   sudo dnf install libsecret" >&2
                    echo "  Arch:          sudo pacman -S libsecret" >&2
                    ;;
                macos)
                    echo "macOS Keychain should be available by default." >&2
                    echo "Ensure 'security' command exists: which security" >&2
                    ;;
                *)
                    echo "Keychain storage requires OS-specific tools." >&2
                    ;;
            esac
            echo "" >&2
            echo "Alternatively, use --storage=profile (default)" >&2
            exit 1
        fi
    fi

    # Handle API key for api-key auth mode
    if [[ "$AUTH_MODE" == "api-key" && -z "$BEDROCK_API_KEY" ]]; then
        if [[ "$PRESERVE_KEY" == true ]]; then
            # Reuse existing key from environment
            if [[ -n "$AWS_BEARER_TOKEN_BEDROCK" ]]; then
                BEDROCK_API_KEY="$AWS_BEARER_TOKEN_BEDROCK"
                echo "Using existing API key from environment"
            else
                echo "Error: --preserve-key specified but AWS_BEARER_TOKEN_BEDROCK is not set" >&2
                echo "Run setup without --preserve-key to enter a new key" >&2
                exit 1
            fi
        elif [[ "$AUTH_MODE_EXPLICIT" != true ]]; then
            # Auth mode was auto-detected from existing config — try to reuse key automatically
            if [[ -n "$AWS_BEARER_TOKEN_BEDROCK" ]]; then
                BEDROCK_API_KEY="$AWS_BEARER_TOKEN_BEDROCK"
                echo "Using existing API key from environment"
            elif [[ "$STORAGE_MODE" == "keychain" ]] && keychain_available; then
                local keychain_key
                keychain_key=$(keychain_get)
                if [[ -n "$keychain_key" ]]; then
                    BEDROCK_API_KEY="$keychain_key"
                    echo "Using existing API key from keychain"
                fi
            fi
            # If still empty, fall through to prompt below
            if [[ -z "$BEDROCK_API_KEY" ]]; then
                if [[ "$DRY_RUN" == true ]]; then
                    echo "[DRY RUN] Would prompt for Bedrock API key"
                    BEDROCK_API_KEY="dry-run-placeholder"
                elif [[ ! -t 0 ]]; then
                    echo "Error: --bedrock-key or --preserve-key is required in non-interactive mode" >&2
                    echo "" >&2
                    echo "Usage: ./setup-claude-bedrock.sh --auth=api-key --bedrock-key=YOUR_KEY" >&2
                    echo "   or: ./setup-claude-bedrock.sh --auth=api-key --preserve-key" >&2
                    exit 1
                else
                    echo "Get your Bedrock API key from:"
                    echo "  AWS Console → Amazon Bedrock → API keys"
                    echo ""
                    read -s -p "Enter your Bedrock API key: " BEDROCK_API_KEY
                    echo ""
                    BEDROCK_API_KEY="${BEDROCK_API_KEY%"${BEDROCK_API_KEY##*[![:space:]]}"}"
                    BEDROCK_API_KEY="${BEDROCK_API_KEY#"${BEDROCK_API_KEY%%[![:space:]]*}"}"
                    if [[ -z "$BEDROCK_API_KEY" ]]; then
                        echo "Error: API key cannot be empty"
                        exit 1
                    fi
                fi
            fi
        elif [[ "$DRY_RUN" == true ]]; then
            echo "[DRY RUN] Would prompt for Bedrock API key"
            BEDROCK_API_KEY="dry-run-placeholder"
        elif [[ ! -t 0 ]]; then
            # Non-interactive mode (stdin is not a terminal)
            echo "Error: --bedrock-key or --preserve-key is required in non-interactive mode" >&2
            echo "" >&2
            echo "Usage: ./setup-claude-bedrock.sh --auth=api-key --bedrock-key=YOUR_KEY" >&2
            echo "   or: ./setup-claude-bedrock.sh --auth=api-key --preserve-key" >&2
            exit 1
        else
            echo "Get your Bedrock API key from:"
            echo "  AWS Console → Amazon Bedrock → API keys"
            echo ""
            read -s -p "Enter your Bedrock API key: " BEDROCK_API_KEY
            echo ""  # newline after hidden input

            # Strip any trailing whitespace/carriage returns (common from copy-paste)
            BEDROCK_API_KEY="${BEDROCK_API_KEY%"${BEDROCK_API_KEY##*[![:space:]]}"}"
            BEDROCK_API_KEY="${BEDROCK_API_KEY#"${BEDROCK_API_KEY%%[![:space:]]*}"}"

            if [[ -z "$BEDROCK_API_KEY" ]]; then
                echo "Error: API key cannot be empty"
                exit 1
            fi
        fi
    fi

    # Validate region
    if ! is_valid_region "$AWS_REGION"; then
        echo "Warning: '$AWS_REGION' may not be a valid Bedrock region"
        echo "Common Bedrock regions: us-east-1, us-west-2, eu-west-1, ap-northeast-1"

        if [[ "$DRY_RUN" == false && "$FORCE" == false ]]; then
            read -p "Continue anyway? (y/n) " -n 1 -r
            echo
            [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Setup cancelled"; exit 1; }
        fi
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Setup Logic
#───────────────────────────────────────────────────────────────────────────────

setup_shell() {
    local shell=$1
    local config_file="${SHELL_CONFIGS[$shell]}"
    local display_name="${SHELL_DISPLAY_NAMES[$shell]}"
    local os=$(detect_os)

    # Create parent directory for fish
    if [[ "$shell" == "fish" && "$DRY_RUN" == false ]]; then
        mkdir -p "$(dirname "$config_file")"
    fi

    # Create config file if it doesn't exist
    if [[ ! -f "$config_file" && "$DRY_RUN" == false ]]; then
        touch "$config_file"
    fi

    echo "Detected: $display_name on $os"
    echo "Target:   $config_file"
    echo "Region:   $AWS_REGION"
    echo "Auth:     $AUTH_MODE"
    if [[ "$AUTH_MODE" == "api-key" ]]; then
        # Show masked key for security (guard against short keys)
        local masked_key
        if [[ ${#BEDROCK_API_KEY} -gt 12 ]]; then
            masked_key="${BEDROCK_API_KEY:0:8}...${BEDROCK_API_KEY: -4}"
        else
            masked_key="****"
        fi
        echo "API Key:  $masked_key"
        echo "Storage:  $STORAGE_MODE"
    fi
    echo ""

    # Confirm with user (unless dry-run or force)
    if [[ "$DRY_RUN" == false && "$FORCE" == false ]]; then
        read -p "Configure $display_name for Bedrock? (y/n) " -n 1 -r
        echo
        [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Setup cancelled"; exit 0; }
    fi

    # Backup existing config file before any modifications
    if [[ -f "$config_file" && "$DRY_RUN" == false ]]; then
        backup_config_file "$config_file"
    elif [[ -f "$config_file" && "$DRY_RUN" == true ]]; then
        echo "[DRY RUN] Would create backup of $config_file"
    fi

    # Check for existing configuration
    if [[ -f "$config_file" ]] && grep -q "CLAUDE_CODE_USE_BEDROCK" "$config_file" 2>/dev/null; then
        echo "Existing configuration found"

        if [[ "$DRY_RUN" == true ]]; then
            echo "[DRY RUN] Would remove existing configuration"
        elif [[ "$FORCE" == true ]]; then
            remove_existing_config "$config_file"
            echo "Removed existing configuration"
        else
            read -p "Replace existing configuration? (y/n) " -n 1 -r
            echo
            [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Setup cancelled"; exit 0; }

            remove_existing_config "$config_file"
            echo "Removed existing configuration"
        fi
    fi

    # Store API key in keychain if using keychain storage
    if [[ "$AUTH_MODE" == "api-key" && "$STORAGE_MODE" == "keychain" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            echo "[DRY RUN] Would store API key in system keychain"
        else
            if keychain_store "$BEDROCK_API_KEY"; then
                echo "API key stored in system keychain"
            else
                echo "Error: Failed to store API key in keychain" >&2
                exit 1
            fi
        fi
    fi

    # Generate and apply configuration
    local config_block
    config_block=$(generate_config_block "$shell" "$AWS_REGION" "$AUTH_MODE" "$BEDROCK_API_KEY" "$STORAGE_MODE")

    if [[ "$DRY_RUN" == true ]]; then
        echo ""
        echo "Would append to $config_file:"
        echo "─────────────────────────────────────────"
        echo "$config_block"
        echo "─────────────────────────────────────────"
        echo ""
        if [[ "$STORAGE_MODE" == "keychain" ]]; then
            echo "[DRY RUN] API key would be stored in system keychain (encrypted)"
        fi
        echo "[DRY RUN] No changes made"
    else
        write_config_to_file "$config_file" "$config_block"
        echo "Configuration added to $config_file"
    fi
}

show_next_steps() {
    local auth_mode=$1
    local current_shell
    current_shell=$(basename "$SHELL")
    local config_file="${SHELL_CONFIGS[$current_shell]}"

    echo ""
    echo "Setup complete!"
    echo ""
    echo "To apply configuration:"
    echo ""
    if [[ "$current_shell" == "fish" ]]; then
        echo "   source ~/.config/fish/config.fish"
    else
        echo "   source $config_file"
    fi
    echo ""
    echo "Or restart your terminal."
    echo ""

    if [[ "$auth_mode" == "api-key" ]]; then
        echo "Then launch Claude Code:"
        echo "   claude"
    else
        echo "Then verify AWS credentials and launch:"
        echo "   aws sts get-caller-identity"
        echo "   claude"
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Main
#───────────────────────────────────────────────────────────────────────────────

main() {
    parse_arguments "$@"

    # Detect existing auth mode if user didn't explicitly specify
    if [[ "$AUTH_MODE_EXPLICIT" != true ]]; then
        local config_file="${SHELL_CONFIGS[$SHELL_TYPE]}"
        if [[ -f "$config_file" ]] && grep -q "CLAUDE_CODE_USE_BEDROCK" "$config_file" 2>/dev/null; then
            local existing_auth
            existing_auth=$(detect_existing_auth_mode "$config_file")
            if [[ -n "$existing_auth" ]]; then
                AUTH_MODE="$existing_auth"
                echo "Preserving existing auth mode: $AUTH_MODE"
            fi
        fi
    fi

    # Apply default if still unset
    if [[ -z "$AUTH_MODE" ]]; then
        AUTH_MODE="${DEFAULT_AUTH:-iam}"
    fi

    # Detect existing custom models if user didn't explicitly specify
    local config_file="${SHELL_CONFIGS[$SHELL_TYPE]}"
    if [[ "$MODEL_EXPLICIT" != true && -f "$config_file" ]]; then
        local existing_model
        existing_model=$(detect_existing_model "$config_file")
        if [[ -n "$existing_model" ]]; then
            CUSTOM_MODEL="$existing_model"
            echo "Preserving existing custom model: $CUSTOM_MODEL"
        fi
    fi

    if [[ "$FAST_MODEL_EXPLICIT" != true && -f "$config_file" ]]; then
        local existing_fast_model
        existing_fast_model=$(detect_existing_fast_model "$config_file")
        if [[ -n "$existing_fast_model" ]]; then
            CUSTOM_FAST_MODEL="$existing_fast_model"
            echo "Preserving existing custom fast model: $CUSTOM_FAST_MODEL"
        fi
    fi

    if [[ "$OPUS_MODEL_EXPLICIT" != true && -f "$config_file" ]]; then
        local existing_opus_model
        existing_opus_model=$(detect_existing_opus_model "$config_file")
        if [[ -n "$existing_opus_model" ]]; then
            CUSTOM_OPUS_MODEL="$existing_opus_model"
            echo "Preserving existing custom opus model: $CUSTOM_OPUS_MODEL"
        fi
    fi

    if [[ "$SONNET_MODEL_EXPLICIT" != true && -f "$config_file" ]]; then
        local existing_sonnet_model
        existing_sonnet_model=$(detect_existing_sonnet_model "$config_file")
        if [[ -n "$existing_sonnet_model" ]]; then
            CUSTOM_SONNET_MODEL="$existing_sonnet_model"
            echo "Preserving existing custom sonnet model: $CUSTOM_SONNET_MODEL"
        fi
    fi

    if [[ "$HAIKU_MODEL_EXPLICIT" != true && -f "$config_file" ]]; then
        local existing_haiku_model
        existing_haiku_model=$(detect_existing_haiku_model "$config_file")
        if [[ -n "$existing_haiku_model" ]]; then
            CUSTOM_HAIKU_MODEL="$existing_haiku_model"
            echo "Preserving existing custom haiku model: $CUSTOM_HAIKU_MODEL"
        fi
    fi

    # Detect existing storage mode if user didn't explicitly specify
    if [[ "$STORAGE_MODE_EXPLICIT" != true && -f "$config_file" ]]; then
        local existing_storage
        existing_storage=$(detect_existing_storage_mode "$config_file")
        if [[ "$existing_storage" == "keychain" ]]; then
            STORAGE_MODE="keychain"
            echo "Preserving existing storage mode: keychain"
        fi
    fi

    # Platform-aware storage default for new installs (macOS/Windows default to keychain)
    if [[ "$STORAGE_MODE_EXPLICIT" != true && "$STORAGE_MODE" == "profile" ]]; then
        local os=$(detect_os)
        if [[ "$os" == "macos" || "$os" == "gitbash" || "$os" == "cygwin" ]]; then
            # Only change default if there's no existing config (new install)
            if ! grep -q "CLAUDE_CODE_USE_BEDROCK" "$config_file" 2>/dev/null; then
                if keychain_available; then
                    STORAGE_MODE="keychain"
                fi
            fi
        fi
    fi

    # Offer to migrate plaintext API keys to keychain on macOS/Windows
    if [[ "$AUTH_MODE" == "api-key" && "$STORAGE_MODE_EXPLICIT" != true && "$STORAGE_MODE" == "profile" ]]; then
        local os=$(detect_os)
        if [[ "$os" == "macos" || "$os" == "gitbash" || "$os" == "cygwin" ]]; then
            if [[ -f "$config_file" ]] && grep -q "CLAUDE_CODE_USE_BEDROCK" "$config_file" 2>/dev/null; then
                if ! grep -q "# Storage: keychain" "$config_file" 2>/dev/null && keychain_available; then
                    local keychain_name
                    case "$os" in
                        macos) keychain_name="macOS Keychain" ;;
                        *)     keychain_name="Windows Credential Manager" ;;
                    esac
                    if [[ "$FORCE" == true ]]; then
                        STORAGE_MODE="keychain"
                        echo "Migrating API key to $keychain_name (more secure)"
                    elif [[ "$DRY_RUN" == false && -t 0 ]]; then
                        echo ""
                        echo "Your API key is stored in plaintext in your shell profile."
                        read -p "Move to $keychain_name for better security? (y/n) " -n 1 -r
                        echo
                        if [[ $REPLY =~ ^[Yy]$ ]]; then
                            STORAGE_MODE="keychain"
                            echo "Migrating API key to $keychain_name"
                        fi
                    fi
                fi
            fi
        fi
    fi

    # Handle "default" reset value - clears custom model
    if [[ "$CUSTOM_MODEL" == "default" ]]; then
        CUSTOM_MODEL=""
        echo "Resetting primary model to default from bedrock-config.json"
    fi
    if [[ "$CUSTOM_FAST_MODEL" == "default" ]]; then
        CUSTOM_FAST_MODEL=""
        echo "Resetting fast model to default from bedrock-config.json"
    fi
    if [[ "$CUSTOM_OPUS_MODEL" == "default" ]]; then
        CUSTOM_OPUS_MODEL=""
        echo "Resetting opus model to default from bedrock-config.json"
    fi
    if [[ "$CUSTOM_SONNET_MODEL" == "default" ]]; then
        CUSTOM_SONNET_MODEL=""
        echo "Resetting sonnet model to default from bedrock-config.json"
    fi
    if [[ "$CUSTOM_HAIKU_MODEL" == "default" ]]; then
        CUSTOM_HAIKU_MODEL=""
        echo "Resetting haiku model to default from bedrock-config.json"
    fi

    # Validate and warn for custom models
    if [[ -n "$CUSTOM_MODEL" ]]; then
        validate_model_id "$CUSTOM_MODEL" "model" || exit 1
        warn_custom_model "$CUSTOM_MODEL" "primary"
    fi

    if [[ -n "$CUSTOM_FAST_MODEL" ]]; then
        validate_model_id "$CUSTOM_FAST_MODEL" "fast-model" || exit 1
        warn_custom_model "$CUSTOM_FAST_MODEL" "fast"
    fi

    if [[ -n "$CUSTOM_OPUS_MODEL" && "$CUSTOM_OPUS_MODEL" != "default" ]]; then
        validate_model_id "$CUSTOM_OPUS_MODEL" "opus-model" || exit 1
        warn_custom_model "$CUSTOM_OPUS_MODEL" "opus"
    fi
    if [[ -n "$CUSTOM_SONNET_MODEL" && "$CUSTOM_SONNET_MODEL" != "default" ]]; then
        validate_model_id "$CUSTOM_SONNET_MODEL" "sonnet-model" || exit 1
        warn_custom_model "$CUSTOM_SONNET_MODEL" "sonnet"
    fi
    if [[ -n "$CUSTOM_HAIKU_MODEL" && "$CUSTOM_HAIKU_MODEL" != "default" ]]; then
        validate_model_id "$CUSTOM_HAIKU_MODEL" "haiku-model" || exit 1
        warn_custom_model "$CUSTOM_HAIKU_MODEL" "haiku"
    fi

    # Apply --global or --model-prefix to all model env vars
    if [[ "$USE_GLOBAL" == true ]]; then
        MODEL_PREFIX="global"
    fi

    if [[ -n "$MODEL_PREFIX" ]]; then
        local transform_keys=(
            ANTHROPIC_MODEL ANTHROPIC_SMALL_FAST_MODEL
            ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL
        )
        for key in "${transform_keys[@]}"; do
            if [[ -n "${BEDROCK_CONFIG[$key]}" ]]; then
                BEDROCK_CONFIG["$key"]=$(echo "${BEDROCK_CONFIG[$key]}" | sed "s/^[^.]*\./${MODEL_PREFIX}./")
            fi
        done
    fi

    validate_inputs

    if [[ "$DRY_RUN" == true ]]; then
        echo "[DRY RUN] No changes will be made"
        echo ""
    fi

    if [[ "$AUTH_MODE" == "api-key" ]]; then
        echo "Setting up Claude Code with Amazon Bedrock (API key auth)..."
    else
        echo "Setting up Claude Code with Amazon Bedrock (IAM/SSO auth)..."
    fi
    echo ""

    # Detect current shell
    local current_shell
    current_shell=$(basename "$SHELL")

    # Configure the specified shell
    setup_shell "$SHELL_TYPE"

    # If current shell differs from specified, configure it too
    if [[ "$current_shell" != "$SHELL_TYPE" && -n "${SHELL_CONFIGS[$current_shell]}" ]]; then
        echo ""
        echo "────────────────────────────────────────────────────────────"
        echo "Also configuring your current shell ($current_shell)..."
        echo "────────────────────────────────────────────────────────────"
        echo ""
        # Use FORCE for the second shell to avoid double prompting
        local orig_force=$FORCE
        FORCE=true
        setup_shell "$current_shell"
        FORCE=$orig_force
    fi

    # Show next steps (always for current shell, which is now guaranteed to be configured)
    if [[ "$DRY_RUN" == false ]]; then
        show_next_steps "$AUTH_MODE"
    fi
}

main "$@"
