#!/usr/bin/env bash

# Apply Claude Code Bedrock Configuration to Current Session
# Usage: source apply-config.sh [--region=REGION] [--help]
#
# NOTE: This script must be sourced (not executed) for environment variables to persist.
# Works with both bash and zsh.

#───────────────────────────────────────────────────────────────────────────────
# Argument Parsing
#───────────────────────────────────────────────────────────────────────────────

_JUGGERNAUT_REGION="us-west-2"

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

# Export configuration (simple exports for bash/zsh compatibility)
export CLAUDE_CODE_USE_BEDROCK="1"
export AWS_REGION="$_JUGGERNAUT_REGION"
export CLAUDE_CODE_MAX_OUTPUT_TOKENS="16384"
export MAX_THINKING_TOKENS="1024"
export ANTHROPIC_MODEL="global.anthropic.claude-opus-4-5-20251101-v1:0"
export ANTHROPIC_SMALL_FAST_MODEL="global.anthropic.claude-sonnet-4-5-20250929-v1:0"
export DISABLE_ERROR_REPORTING="1"
export DISABLE_TELEMETRY="1"
export DISABLE_AUTOUPDATE="1"
export DISABLE_BUG_COMMAND="1"

# Clean up temp variable
unset _JUGGERNAUT_REGION

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
