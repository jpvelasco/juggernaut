#!/usr/bin/env bash

# Apply Claude Code Bedrock Configuration to Current Session
# Usage: source apply-config.sh [--region=REGION] [--help]
#
# NOTE: This script must be sourced (not executed) for environment variables to persist.

#───────────────────────────────────────────────────────────────────────────────
# Bash Check (non-fatal for sourcing)
#───────────────────────────────────────────────────────────────────────────────

if [[ -z "$BASH_VERSION" && -z "$ZSH_VERSION" ]]; then
    echo "This script works best with bash or zsh"
fi

#───────────────────────────────────────────────────────────────────────────────
# Configuration
#───────────────────────────────────────────────────────────────────────────────

# Bedrock configuration values (single source of truth)
declare -A BEDROCK_CONFIG=(
    [CLAUDE_CODE_USE_BEDROCK]="1"
    [CLAUDE_CODE_MAX_OUTPUT_TOKENS]="16384"
    [MAX_THINKING_TOKENS]="1024"
    [ANTHROPIC_MODEL]="global.anthropic.claude-opus-4-5-20251101-v1:0"
    [ANTHROPIC_SMALL_FAST_MODEL]="global.anthropic.claude-sonnet-4-5-20250929-v1:0"
)

#───────────────────────────────────────────────────────────────────────────────
# Argument Parsing
#───────────────────────────────────────────────────────────────────────────────

AWS_REGION="us-west-2"

for arg in "$@"; do
    case "$arg" in
        --region=*)
            AWS_REGION="${arg#--region=}"
            ;;
        --help|-h)
            cat << 'EOF'
Apply Claude Code Bedrock Configuration

Usage: source apply-config.sh [--region=REGION]

Applies Claude Code Bedrock configuration to the current terminal session.
This script must be sourced (not executed) for environment variables to persist.

Options:
  --region=REGION    AWS region (default: us-west-2)
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
# Apply Configuration
#───────────────────────────────────────────────────────────────────────────────

echo "Applying Claude Code Bedrock configuration..."
echo ""

# Export all config values
for key in "${!BEDROCK_CONFIG[@]}"; do
    export "$key"="${BEDROCK_CONFIG[$key]}"
done

# Export region separately (not in the static config)
export AWS_REGION="$AWS_REGION"

echo "Configuration applied:"
echo "  CLAUDE_CODE_USE_BEDROCK=$CLAUDE_CODE_USE_BEDROCK"
echo "  AWS_REGION=$AWS_REGION"
echo "  CLAUDE_CODE_MAX_OUTPUT_TOKENS=$CLAUDE_CODE_MAX_OUTPUT_TOKENS"
echo "  MAX_THINKING_TOKENS=$MAX_THINKING_TOKENS"
echo "  ANTHROPIC_MODEL=$ANTHROPIC_MODEL"
echo "  ANTHROPIC_SMALL_FAST_MODEL=$ANTHROPIC_SMALL_FAST_MODEL"
echo ""
echo "This configuration is active for the current terminal session only."
echo "To make it permanent, run: ./setup-claude-bedrock.sh"
