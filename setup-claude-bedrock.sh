#!/usr/bin/env bash

# Claude Code - Amazon Bedrock Setup Script
# Usage: ./setup-claude-bedrock.sh [bash|zsh|fish] [--dry-run] [--region=REGION]

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
# Configuration Registry (Associative Arrays)
#───────────────────────────────────────────────────────────────────────────────

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

# Valid AWS regions that support Bedrock
declare -a VALID_REGIONS=(
    us-east-1 us-east-2 us-west-2
    eu-west-1 eu-west-2 eu-west-3 eu-central-1 eu-central-2
    ap-southeast-1 ap-southeast-2 ap-southeast-3 ap-northeast-1 ap-northeast-2 ap-south-1
    sa-east-1 ca-central-1 me-south-1 me-central-1 il-central-1
)

# Bedrock configuration values
declare -A BEDROCK_CONFIG=(
    [CLAUDE_CODE_USE_BEDROCK]="1"
    [CLAUDE_CODE_MAX_OUTPUT_TOKENS]="16384"
    [MAX_THINKING_TOKENS]="1024"
    [ANTHROPIC_MODEL]="global.anthropic.claude-opus-4-5-20251101-v1:0"
    [ANTHROPIC_SMALL_FAST_MODEL]="global.anthropic.claude-sonnet-4-5-20250929-v1:0"
)

# Order matters for config file output
declare -a CONFIG_KEY_ORDER=(
    CLAUDE_CODE_USE_BEDROCK
    AWS_REGION
    CLAUDE_CODE_MAX_OUTPUT_TOKENS
    MAX_THINKING_TOKENS
    ANTHROPIC_MODEL
    ANTHROPIC_SMALL_FAST_MODEL
)

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

#───────────────────────────────────────────────────────────────────────────────
# Config Generation (Template Pattern)
#───────────────────────────────────────────────────────────────────────────────

generate_config_block() {
    local shell=$1
    local region=$2
    local syntax="${SHELL_EXPORT_SYNTAX[$shell]}"
    local config=""

    config+=$'\n'"# BEGIN: Claude Code Bedrock Configuration"$'\n'

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

remove_existing_config() {
    local config_file=$1

    # Remove config with markers (current format)
    sed_inplace '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$config_file"

    # Remove config without markers (legacy format)
    sed_inplace '/# Claude Code - Amazon Bedrock Configuration/,/ANTHROPIC_SMALL_FAST_MODEL/d' "$config_file"
}

show_help() {
    cat << 'EOF'
Claude Code - Amazon Bedrock Setup Script

Usage: ./setup-claude-bedrock.sh [SHELL] [OPTIONS]

Arguments:
  SHELL              Target shell: bash, zsh, or fish (auto-detected if omitted)

Options:
  --dry-run          Preview changes without modifying files
  --force, -f        Skip confirmation prompts
  --region=REGION    AWS region (default: us-west-2)
  --help, -h         Show this help message

Examples:
  ./setup-claude-bedrock.sh                    # Auto-detect shell
  ./setup-claude-bedrock.sh zsh                # Configure zsh
  ./setup-claude-bedrock.sh --dry-run          # Preview changes
  ./setup-claude-bedrock.sh --force            # No prompts
  ./setup-claude-bedrock.sh zsh --region=us-east-1
EOF
}

#───────────────────────────────────────────────────────────────────────────────
# Argument Parsing
#───────────────────────────────────────────────────────────────────────────────

DRY_RUN=false
FORCE=false
AWS_REGION="us-west-2"
SHELL_TYPE=""

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

    # Validate region
    if ! is_valid_region "$AWS_REGION"; then
        echo "Warning: '$AWS_REGION' may not be a valid Bedrock region"
        echo "Common Bedrock regions: us-east-1, us-west-2, eu-west-1, ap-northeast-1"

        if [[ "$DRY_RUN" == false ]]; then
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
    echo ""

    # Confirm with user (unless dry-run or force)
    if [[ "$DRY_RUN" == false && "$FORCE" == false ]]; then
        read -p "Configure $display_name for Bedrock? (y/n) " -n 1 -r
        echo
        [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Setup cancelled"; exit 0; }
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
    config_block=$(generate_config_block "$shell" "$AWS_REGION")

    if [[ "$DRY_RUN" == true ]]; then
        echo ""
        echo "Would append to $config_file:"
        echo "─────────────────────────────────────────"
        echo "$config_block"
        echo "─────────────────────────────────────────"
        echo ""
        echo "[DRY RUN] No changes made"
    else
        echo "$config_block" >> "$config_file"
        echo "Configuration added to $config_file"
        echo ""
        show_next_steps "$shell" "$config_file"
    fi
}

show_next_steps() {
    local shell=$1
    local config_file=$2

    echo "Next steps:"
    echo ""
    echo "1. Apply configuration:"
    if [[ "$shell" == "fish" ]]; then
        echo "   source ~/.config/fish/config.fish"
    else
        echo "   source $config_file"
    fi
    echo ""
    echo "2. Verify AWS credentials:"
    echo "   aws sts get-caller-identity"
    echo ""
    echo "3. Launch Claude Code:"
    echo "   claude"
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

    echo "Setting up Claude Code with Amazon Bedrock..."
    echo ""

    setup_shell "$SHELL_TYPE"
}

main "$@"
