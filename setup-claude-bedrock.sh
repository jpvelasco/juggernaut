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

#───────────────────────────────────────────────────────────────────────────────
# JSON Config Loading (using jq or python fallback)
#───────────────────────────────────────────────────────────────────────────────

json_get() {
    local file=$1
    local query=$2

    if command -v jq >/dev/null 2>&1; then
        jq -r "$query" "$file" 2>/dev/null
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import json,sys; data=json.load(open('$file')); print(eval('data$query'))" 2>/dev/null
    elif command -v python >/dev/null 2>&1; then
        python -c "import json,sys; data=json.load(open('$file')); print(eval('data$query'))" 2>/dev/null
    else
        echo ""
    fi
}

json_get_keys() {
    local file=$1
    local query=$2

    if command -v jq >/dev/null 2>&1; then
        jq -r "$query | keys[]" "$file" 2>/dev/null
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import json; data=json.load(open('$file')); print('\n'.join(data${query}.keys()))" 2>/dev/null
    elif command -v python >/dev/null 2>&1; then
        python -c "import json; data=json.load(open('$file')); print('\n'.join(data${query}.keys()))" 2>/dev/null
    else
        echo ""
    fi
}

json_get_array() {
    local file=$1
    local query=$2

    if command -v jq >/dev/null 2>&1; then
        jq -r "$query[]" "$file" 2>/dev/null
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import json; data=json.load(open('$file')); print('\n'.join(data${query}))" 2>/dev/null
    elif command -v python >/dev/null 2>&1; then
        python -c "import json; data=json.load(open('$file')); print('\n'.join(data${query}))" 2>/dev/null
    else
        echo ""
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
    keys=$(json_get_keys "$CONFIG_FILE" '["environment"]')
    if [[ -n "$keys" ]]; then
        while IFS= read -r key; do
            local value
            value=$(json_get "$CONFIG_FILE" ".environment[\"$key\"]")
            BEDROCK_CONFIG["$key"]="$value"
            CONFIG_KEY_ORDER+=("$key")
        done <<< "$keys"
    fi

    # Load valid regions
    local regions
    regions=$(json_get_array "$CONFIG_FILE" '["regions"]')
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

#───────────────────────────────────────────────────────────────────────────────
# Config Generation (Template Pattern)
#───────────────────────────────────────────────────────────────────────────────

generate_config_block() {
    local shell=$1
    local region=$2
    local auth_mode=$3
    local api_key=$4
    local syntax="${SHELL_EXPORT_SYNTAX[$shell]}"
    local config=""

    config+=$'\n'"# BEGIN: Claude Code Bedrock Configuration"$'\n'
    config+="# Auth mode: $auth_mode"$'\n'

    for key in "${CONFIG_KEY_ORDER[@]}"; do
        local value
        if [[ "$key" == "AWS_REGION" ]]; then
            value="$region"
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
        if [[ "$shell" == "fish" ]]; then
            config+="$syntax AWS_BEARER_TOKEN_BEDROCK $api_key"$'\n'
        else
            config+="$syntax AWS_BEARER_TOKEN_BEDROCK=$api_key"$'\n'
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

    # Remove config without markers (legacy format)
    sed_inplace '/# Claude Code - Amazon Bedrock Configuration/,/ANTHROPIC_SMALL_FAST_MODEL/d' "$config_file"
}

# File locking to prevent concurrent modifications
LOCK_FD=""
LOCK_FILE=""

acquire_lock() {
    local config_file=$1
    LOCK_FILE="${config_file}.lock"

    # Check if flock is available (Linux, some macOS with homebrew)
    if command -v flock >/dev/null 2>&1; then
        exec {LOCK_FD}>"$LOCK_FILE"
        if ! flock -n "$LOCK_FD" 2>/dev/null; then
            echo "ERROR: Another instance is modifying $config_file" >&2
            echo "If no other setup is running, remove: $LOCK_FILE" >&2
            exit 1
        fi
    else
        # Fallback: use mkdir as atomic operation (portable)
        if ! mkdir "$LOCK_FILE" 2>/dev/null; then
            # Check if lock is stale (older than 5 minutes)
            if [[ -d "$LOCK_FILE" ]]; then
                local lock_age
                lock_age=$(( $(date +%s) - $(stat -f %m "$LOCK_FILE" 2>/dev/null || stat -c %Y "$LOCK_FILE" 2>/dev/null || echo 0) ))
                if [[ "$lock_age" -gt 300 ]]; then
                    echo "Warning: Removing stale lock file (age: ${lock_age}s)"
                    rmdir "$LOCK_FILE" 2>/dev/null || rm -rf "$LOCK_FILE"
                    mkdir "$LOCK_FILE" || { echo "ERROR: Cannot acquire lock"; exit 1; }
                else
                    echo "ERROR: Another instance is modifying $config_file" >&2
                    echo "If no other setup is running, remove: $LOCK_FILE" >&2
                    exit 1
                fi
            fi
        fi
    fi
}

release_lock() {
    if [[ -n "$LOCK_FILE" ]]; then
        if [[ -n "$LOCK_FD" ]]; then
            # flock mode: close file descriptor
            exec {LOCK_FD}>&- 2>/dev/null || true
        fi
        # Remove lock file/directory
        rmdir "$LOCK_FILE" 2>/dev/null || rm -f "$LOCK_FILE" 2>/dev/null || true
        LOCK_FILE=""
        LOCK_FD=""
    fi
}

# Ensure lock is released on exit
trap release_lock EXIT

show_help() {
    cat << 'EOF'
Claude Code - Amazon Bedrock Setup Script

Usage: ./setup-claude-bedrock.sh [SHELL] [OPTIONS]

Arguments:
  SHELL                  Target shell: bash, zsh, or fish (auto-detected if omitted)

Options:
  --auth=MODE            Authentication mode: iam (default) or api-key
  --bedrock-key=KEY      Bedrock API key (optional; prompts if not provided)
  --region=REGION        AWS region (default: us-west-2)
  --dry-run              Preview changes without modifying files
  --force, -f            Skip confirmation prompts
  --help, -h             Show this help message

Authentication Modes:
  iam        Use AWS IAM/SSO credentials (default)
             Requires: aws configure, SSO login, or IAM role

  api-key    Use Bedrock API key (simpler setup)
             Prompts securely if --bedrock-key not provided
             Get key from: AWS Console → Bedrock → API keys

Examples:
  # IAM/SSO authentication (default)
  ./setup-claude-bedrock.sh
  ./setup-claude-bedrock.sh zsh --region=us-east-1

  # API key authentication (interactive - recommended, more secure)
  ./setup-claude-bedrock.sh --auth=api-key

  # API key authentication (inline - for scripting/CI)
  ./setup-claude-bedrock.sh --auth=api-key --bedrock-key=br-xxxxxxxxxxxx

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
AWS_REGION="${DEFAULT_REGION:-us-west-2}"
SHELL_TYPE=""
AUTH_MODE="${DEFAULT_AUTH:-iam}"
BEDROCK_API_KEY=""

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
                ;;
            --bedrock-key=*)
                BEDROCK_API_KEY="${arg#--bedrock-key=}"
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

    # Prompt for API key if using api-key auth and key not provided
    if [[ "$AUTH_MODE" == "api-key" && -z "$BEDROCK_API_KEY" ]]; then
        if [[ "$DRY_RUN" == true ]]; then
            echo "[DRY RUN] Would prompt for Bedrock API key"
            BEDROCK_API_KEY="dry-run-placeholder"
        elif [[ ! -t 0 ]]; then
            # Non-interactive mode (stdin is not a terminal)
            echo "Error: --bedrock-key is required in non-interactive mode" >&2
            echo "" >&2
            echo "Usage: ./setup-claude-bedrock.sh --auth=api-key --bedrock-key=YOUR_KEY" >&2
            exit 1
        else
            echo "Get your Bedrock API key from:"
            echo "  AWS Console → Amazon Bedrock → API keys"
            echo ""
            read -s -p "Enter your Bedrock API key: " BEDROCK_API_KEY
            echo ""  # newline after hidden input

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
        # Show masked key for security
        local masked_key="${BEDROCK_API_KEY:0:8}...${BEDROCK_API_KEY: -4}"
        echo "API Key:  $masked_key"
    fi
    echo ""

    # Confirm with user (unless dry-run or force)
    if [[ "$DRY_RUN" == false && "$FORCE" == false ]]; then
        read -p "Configure $display_name for Bedrock? (y/n) " -n 1 -r
        echo
        [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Setup cancelled"; exit 0; }
    fi

    # Acquire lock before any file modifications
    if [[ "$DRY_RUN" == false ]]; then
        acquire_lock "$config_file"
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

    # Generate and apply configuration
    local config_block
    config_block=$(generate_config_block "$shell" "$AWS_REGION" "$AUTH_MODE" "$BEDROCK_API_KEY")

    if [[ "$DRY_RUN" == true ]]; then
        echo ""
        echo "Would append to $config_file:"
        echo "─────────────────────────────────────────"
        echo "$config_block"
        echo "─────────────────────────────────────────"
        echo ""
        echo "[DRY RUN] No changes made"
    else
        write_config_to_file "$config_file" "$config_block"
        echo "Configuration added to $config_file"
        echo ""
        show_next_steps "$shell" "$config_file" "$AUTH_MODE"
    fi
}

show_next_steps() {
    local shell=$1
    local config_file=$2
    local auth_mode=$3

    echo "Next steps:"
    echo ""
    echo "1. Apply configuration:"
    if [[ "$shell" == "fish" ]]; then
        echo "   source ~/.config/fish/config.fish"
    else
        echo "   source $config_file"
    fi
    echo ""

    if [[ "$auth_mode" == "api-key" ]]; then
        echo "2. Launch Claude Code:"
        echo "   claude"
        echo ""
        echo "   (No AWS credential setup needed - using API key)"
    else
        echo "2. Verify AWS credentials:"
        echo "   aws sts get-caller-identity"
        echo ""
        echo "3. Launch Claude Code:"
        echo "   claude"
    fi
    echo ""
    echo "Setup complete!"
}

#───────────────────────────────────────────────────────────────────────────────
# Main
#───────────────────────────────────────────────────────────────────────────────

main() {
    parse_arguments "$@"
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

    setup_shell "$SHELL_TYPE"
}

main "$@"
