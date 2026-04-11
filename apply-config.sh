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
if [[ -n "${BASH_SOURCE[0]+x}" ]]; then
    _JUGGERNAUT_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
elif [[ -n "$ZSH_VERSION" ]]; then
    # ${(%):-%x} is zsh syntax for current script path
    # shellcheck disable=SC2296
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
        --version|-v)
            cat "$_JUGGERNAUT_SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"
            # exit 0 is fallback when script is executed, not sourced
            # shellcheck disable=SC2317
            return 0 2>/dev/null || exit 0
            ;;
        --help|-h)
            cat << 'EOF'
Apply Claude Code Bedrock Configuration

Usage: source apply-config.sh [--region=REGION]

Applies Claude Code Bedrock configuration to the current terminal session.
This script must be sourced (not executed) for environment variables to persist.

Options:
  --region=REGION    AWS region (default: from bedrock-config.json)
  --version, -v      Show version
  --help, -h         Show this help message

Examples:
  source apply-config.sh
  source apply-config.sh --region=us-east-1
EOF
            # exit 0 is fallback when script is executed, not sourced
            # shellcheck disable=SC2317
            return 0 2>/dev/null || exit 0
            ;;
    esac
done

#───────────────────────────────────────────────────────────────────────────────
# Load Configuration from JSON
#───────────────────────────────────────────────────────────────────────────────

if [[ ! -f "$_JUGGERNAUT_CONFIG_FILE" ]]; then
    echo "Error: Config file not found: $_JUGGERNAUT_CONFIG_FILE" >&2
    # exit 1 is fallback when script is executed, not sourced
    # shellcheck disable=SC2317
    return 1 2>/dev/null || exit 1
fi

_JUGGERNAUT_JSON_CONTENT=$(cat "$_JUGGERNAUT_CONFIG_FILE")

# Parse JSON using jq or python3 fallback
# Python fallback uses sys.argv to avoid eval() and shell injection risks
_juggernaut_get_json_value() {
    local key=$1
    if command -v jq >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | jq -r "$key // empty" | tr -d '\r'
    elif command -v python3 >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | python3 -c "
import sys,json,functools
d=json.load(sys.stdin)
keys=[k for k in sys.argv[1].split('.') if k]
val=functools.reduce(lambda d,k: d[k], keys, d)
print('' if val is None else val)
" "$key" 2>/dev/null | tr -d '\r'
    else
        echo "Error: jq or python3 required to parse config" >&2
        return 1
    fi
}

# Helper to get all keys from a JSON object
_juggernaut_get_json_keys() {
    local key=$1
    if command -v jq >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | jq -r "$key | keys[]" 2>/dev/null | tr -d '\r'
    elif command -v python3 >/dev/null 2>&1; then
        echo "$_JUGGERNAUT_JSON_CONTENT" | python3 -c "
import sys,json,functools
d=json.load(sys.stdin)
keys=[k for k in sys.argv[1].split('.') if k]
obj=functools.reduce(lambda d,k: d[k], keys, d)
print('\n'.join(obj.keys()))
" "$key" 2>/dev/null | tr -d '\r'
    else
        echo "Error: jq or python3 required to parse config" >&2
        return 1
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Apply Configuration
#───────────────────────────────────────────────────────────────────────────────

echo "Applying Claude Code Bedrock configuration..."
echo ""

# Dynamically load and export all environment variables from config
_JUGGERNAUT_ENV_KEYS=$(_juggernaut_get_json_keys '.environment')
if [[ -n "$_JUGGERNAUT_ENV_KEYS" ]]; then
    while IFS= read -r _JUGGERNAUT_KEY; do
        _JUGGERNAUT_VAL="$(_juggernaut_get_json_value ".environment.$_JUGGERNAUT_KEY")"
        if [[ -z "$_JUGGERNAUT_VAL" ]]; then
            echo "Warning: empty value for $_JUGGERNAUT_KEY in config" >&2
        fi
        export "$_JUGGERNAUT_KEY=$_JUGGERNAUT_VAL"
    done <<< "$_JUGGERNAUT_ENV_KEYS"
else
    echo "Error: Could not load environment variables from config" >&2
    # exit 1 is fallback when script is executed, not sourced
    # shellcheck disable=SC2317
    return 1 2>/dev/null || exit 1
fi

# Region: command line overrides config default
if [[ -n "$_JUGGERNAUT_REGION" ]]; then
    export AWS_REGION="$_JUGGERNAUT_REGION"
else
    _JUGGERNAUT_DEFAULT_REGION="$(_juggernaut_get_json_value '.defaults.region')"
    export AWS_REGION="$_JUGGERNAUT_DEFAULT_REGION"
fi

# Display applied configuration
echo "Configuration applied:"
echo "  AWS_REGION=$AWS_REGION"
while IFS= read -r _JUGGERNAUT_KEY; do
    if [[ -n "$ZSH_VERSION" ]]; then
        # ${(P)...} is zsh indirect expansion
        # shellcheck disable=SC2296
        echo "  ${_JUGGERNAUT_KEY}=${(P)_JUGGERNAUT_KEY}"
    else
        echo "  ${_JUGGERNAUT_KEY}=${!_JUGGERNAUT_KEY}"
    fi
done <<< "$_JUGGERNAUT_ENV_KEYS"

# Clean up temp variables and functions
unset _JUGGERNAUT_REGION _JUGGERNAUT_SCRIPT_DIR _JUGGERNAUT_CONFIG_FILE _JUGGERNAUT_JSON_CONTENT
unset _JUGGERNAUT_ENV_KEYS _JUGGERNAUT_KEY _JUGGERNAUT_VAL
unset -f _juggernaut_get_json_value _juggernaut_get_json_keys

echo ""
echo "This configuration is active for the current terminal session only."
echo "To make it permanent, run: ./setup-claude-bedrock.sh"
