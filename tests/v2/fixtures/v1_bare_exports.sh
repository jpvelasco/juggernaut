
# BEGIN: Claude Code Bedrock Configuration
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true
export AWS_REGION="us-west-2"
export CLAUDE_CODE_USE_BEDROCK="1"
export CLAUDE_CODE_MAX_OUTPUT_TOKENS="32768"
export MAX_THINKING_TOKENS="65536"
export ANTHROPIC_MODEL="global.anthropic.claude-sonnet-4-6"
export ANTHROPIC_DEFAULT_OPUS_MODEL="global.anthropic.claude-opus-4-7[1m]"
export ANTHROPIC_DEFAULT_SONNET_MODEL="global.anthropic.claude-sonnet-4-6"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="global.anthropic.claude-haiku-4-5-20251001-v1:0"
export CLAUDE_CODE_EFFORT_LEVEL="xhigh"
export ENABLE_PROMPT_CACHING_1H="1"
export DISABLE_TELEMETRY="1"
# shellcheck disable=SC2155
export AWS_BEARER_TOKEN_BEDROCK=$(echo "dummy-key-for-testing" 2>/dev/null)
# END: Claude Code Bedrock Configuration
