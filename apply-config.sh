#!/usr/bin/env bash

# Apply Claude Code Bedrock Configuration to Current Session
# Usage: source apply-config.sh [--region=REGION] [--help]
#
# NOTE: This script must be sourced (not executed) for environment variables to persist.
# Works with both bash and zsh.

#───────────────────────────────────────────────────────────────────────────────
# Find Config File
#───────────────────────────────────────────────────────────────────────────────

# Get script directory (works when sourced)
if [[ -n "$BASH_SOURCE" ]]; then
    _JUGGERNAUT_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
elif [[ -n "$ZSH_VERSION" ]]; then
    _JUGGERNAUT_SCRIPT_DIR="$(cd "$(dirname "${(%):-%x}")" && pwd)"
else
    _JUGGERNAUT_SCRIPT_DIR="$(pwd)"
fi
_JUGGERNAUT_CONFIG_FILE="$_JUGGERNAUT_SCRIPT_DIR/bedrock-config.json"

#───────────────────────────────────────────────────────────────────────────────
# Argument Parsing
#───────────────────────────────────────────────────────────────────────────────

_JUGGERNAUT_REGION=""

for arg in "$@"; do
    case "$arg" in
        --region=*)
            _JUGGERNAUT_REGION="${arg#--region=}"
            ;;
        --help|-h)
            cat << 'EOF'
Apply Claude Code Bedrock Configuration

Usage: source apply-config.sh [--region=REGION]

Applies Claude Code Bedrock configuration to the current terminal session.
This script must be sourced (not executed) for environment variables to persist.

Options:
  --region=REGION    AWS region (default: from bedrock-config.json)
  --help, -h         Show this help message

Examples:
  source apply-config.sh
  source apply-config.sh --region=us-east-1
EOF
            return 0 2>/dev/null || exit 0
            ;;
    esac
done

#───────────────────────────────────────────────────────────────────────────────
# Load Configuration from JSON
#───────────────────────────────────────────────────────────────────────────────

if [[ ! -f "$_JUGGERNAUT_CONFIG_FILE" ]]; then
    echo "Error: Config file not found: $_JUGGERNAUT_CONFIG_FILE" >&2
    return 1 2>/dev/null || exit 1
fi

_JUGGERNAUT_JSON_CONTENT=$(cat "$_JUGGERNAUT_CONFIG_FILE")

# Parse JSON using jq or python fallback
_juggernaut_get_json_value() {
    local key=$1
    if command -v jq >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | jq -r "$key // empty"
    elif command -v python3 >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(eval('d$key') if eval('d$key') else '')" 2>/dev/null
    elif command -v python >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | python -c "import sys,json; d=json.load(sys.stdin); print(eval('d$key') if eval('d$key') else '')" 2>/dev/null
    else
        echo "Error: jq or python required to parse config" >&2
        return 1
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Apply Configuration
#───────────────────────────────────────────────────────────────────────────────

echo "Applying Claude Code Bedrock configuration..."
echo ""

# Load environment variables from config
export CLAUDE_CODE_USE_BEDROCK="$(_juggernaut_get_json_value '.environment.CLAUDE_CODE_USE_BEDROCK')"
export CLAUDE_CODE_MAX_OUTPUT_TOKENS="$(_juggernaut_get_json_value '.environment.CLAUDE_CODE_MAX_OUTPUT_TOKENS')"
export MAX_THINKING_TOKENS="$(_juggernaut_get_json_value '.environment.MAX_THINKING_TOKENS')"
export ANTHROPIC_MODEL="$(_juggernaut_get_json_value '.environment.ANTHROPIC_MODEL')"
export ANTHROPIC_SMALL_FAST_MODEL="$(_juggernaut_get_json_value '.environment.ANTHROPIC_SMALL_FAST_MODEL')"
export DISABLE_ERROR_REPORTING="$(_juggernaut_get_json_value '.environment.DISABLE_ERROR_REPORTING')"
export DISABLE_TELEMETRY="$(_juggernaut_get_json_value '.environment.DISABLE_TELEMETRY')"
export DISABLE_AUTOUPDATE="$(_juggernaut_get_json_value '.environment.DISABLE_AUTOUPDATE')"
export DISABLE_BUG_COMMAND="$(_juggernaut_get_json_value '.environment.DISABLE_BUG_COMMAND')"

# Region: command line overrides config default
if [[ -n "$_JUGGERNAUT_REGION" ]]; then
    export AWS_REGION="$_JUGGERNAUT_REGION"
else
    export AWS_REGION="$(_juggernaut_get_json_value '.defaults.region')"
fi

# Clean up temp variables and functions
unset _JUGGERNAUT_REGION _JUGGERNAUT_SCRIPT_DIR _JUGGERNAUT_CONFIG_FILE _JUGGERNAUT_JSON_CONTENT
unset -f _juggernaut_get_json_value

echo "Configuration applied:"
echo "  CLAUDE_CODE_USE_BEDROCK=$CLAUDE_CODE_USE_BEDROCK"
echo "  AWS_REGION=$AWS_REGION"
echo "  CLAUDE_CODE_MAX_OUTPUT_TOKENS=$CLAUDE_CODE_MAX_OUTPUT_TOKENS"
echo "  MAX_THINKING_TOKENS=$MAX_THINKING_TOKENS"
echo "  ANTHROPIC_MODEL=$ANTHROPIC_MODEL"
echo "  ANTHROPIC_SMALL_FAST_MODEL=$ANTHROPIC_SMALL_FAST_MODEL"
echo "  DISABLE_ERROR_REPORTING=$DISABLE_ERROR_REPORTING"
echo "  DISABLE_TELEMETRY=$DISABLE_TELEMETRY"
echo "  DISABLE_AUTOUPDATE=$DISABLE_AUTOUPDATE"
echo "  DISABLE_BUG_COMMAND=$DISABLE_BUG_COMMAND"
echo ""
echo "This configuration is active for the current terminal session only."
echo "To make it permanent, run: ./setup-claude-bedrock.sh"
