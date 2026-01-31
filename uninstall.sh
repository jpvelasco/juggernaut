#!/usr/bin/env bash

# Claude Code - Amazon Bedrock Uninstall Script
# Removes Juggernaut configuration from shell profiles
# Usage: ./uninstall.sh [bash|zsh|fish|all]

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

declare -A SHELL_CONFIGS=(
    [bash]="$HOME/.bashrc"
    [zsh]="$HOME/.zshrc"
    [fish]="$HOME/.config/fish/config.fish"
)

declare -A SHELL_DISPLAY_NAMES=(
    [bash]="Bash"
    [zsh]="Zsh"
    [fish]="Fish"
)

declare -a ALL_SHELLS=(bash zsh fish)

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

show_help() {
    cat << 'EOF'
Claude Code - Amazon Bedrock Uninstall Script

Usage: ./uninstall.sh [TARGET]

Arguments:
  TARGET    Shell to clean: bash, zsh, fish, or all (default: all)

Options:
  --help, -h    Show this help message

Examples:
  ./uninstall.sh          # Remove from all shells
  ./uninstall.sh zsh      # Remove from zsh only
  ./uninstall.sh bash     # Remove from bash only
EOF
}

#───────────────────────────────────────────────────────────────────────────────
# Core Functions
#───────────────────────────────────────────────────────────────────────────────

remove_config() {
    local shell=$1
    local config_file="${SHELL_CONFIGS[$shell]}"
    local display_name="${SHELL_DISPLAY_NAMES[$shell]}"

    # Check if config file exists
    if [[ ! -f "$config_file" ]]; then
        echo "Skip: $display_name config not found ($config_file)"
        return 0
    fi

    # Check if our config exists in the file
    if ! grep -q "CLAUDE_CODE_USE_BEDROCK" "$config_file" 2>/dev/null; then
        echo "Skip: No Bedrock config in $display_name ($config_file)"
        return 0
    fi

    # Remove config with markers (current format)
    sed_inplace '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$config_file"

    # Remove config without markers (legacy format)
    sed_inplace '/# Claude Code - Amazon Bedrock Configuration/,/ANTHROPIC_SMALL_FAST_MODEL/d' "$config_file"

    # Clean up multiple consecutive blank lines
    sed_inplace '/^$/N;/^\n$/d' "$config_file"

    echo "Done: Removed Bedrock config from $display_name ($config_file)"
}

uninstall_shell() {
    local target=$1

    if [[ "$target" == "all" ]]; then
        for shell in "${ALL_SHELLS[@]}"; do
            remove_config "$shell"
        done
    else
        remove_config "$target"
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Argument Parsing
#───────────────────────────────────────────────────────────────────────────────

TARGET="all"

parse_arguments() {
    for arg in "$@"; do
        case "$arg" in
            --help|-h)
                show_help
                exit 0
                ;;
            bash|zsh|fish|all)
                TARGET="$arg"
                ;;
            *)
                echo "Warning: Unknown argument '$arg' (ignored)"
                ;;
        esac
    done
}

#───────────────────────────────────────────────────────────────────────────────
# Main
#───────────────────────────────────────────────────────────────────────────────

main() {
    parse_arguments "$@"

    echo "Removing Claude Code Bedrock configuration..."
    echo ""

    uninstall_shell "$TARGET"

    echo ""
    echo "Next steps:"
    echo "1. Restart your terminal or source your shell config"
    echo "2. Claude Code will now use Anthropic's direct API (requires login)"
    echo ""
    echo "Uninstall complete!"
}

main "$@"
