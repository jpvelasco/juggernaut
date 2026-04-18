
# BEGIN: Claude Code Bedrock Configuration
# Auth mode: iam
# Model: us.anthropic.claude-sonnet-4-6
# OpusModel: us.anthropic.claude-opus-4-7[1m]
# SonnetModel: us.anthropic.claude-sonnet-4-6
# HaikuModel: us.anthropic.claude-haiku-4-5-20251001-v1:0
# 1MContext: true
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true
export AWS_REGION="eu-west-1"
export CLAUDE_CODE_USE_BEDROCK="1"
export CLAUDE_CODE_MAX_OUTPUT_TOKENS="32768"
export MAX_THINKING_TOKENS="65536"
export ANTHROPIC_MODEL="us.anthropic.claude-sonnet-4-6"
export ANTHROPIC_DEFAULT_OPUS_MODEL="us.anthropic.claude-opus-4-7[1m]"
export ANTHROPIC_DEFAULT_SONNET_MODEL="us.anthropic.claude-sonnet-4-6"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="us.anthropic.claude-haiku-4-5-20251001-v1:0"
export CLAUDE_CODE_SUBAGENT_MODEL="global.anthropic.claude-haiku-4-5-20251001-v1:0"
export ANTHROPIC_DEFAULT_OPUS_MODEL_NAME="Opus 4.7 (New flagship - 1M context)"
export ANTHROPIC_DEFAULT_SONNET_MODEL_NAME="Sonnet 4.6 (Recommended)"
export ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME="Haiku 4.5 (Fast)"
export ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION="Most capable model yet - 1M context, high-res vision (up to 2576px / ~3.75MP), xhigh effort by default, stronger agentic reasoning and self-verification."
export ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION="Best balance of speed and intelligence"
export ANTHROPIC_DEFAULT_HAIKU_MODEL_DESCRIPTION="Fastest model for everyday tasks and subagents"
export ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES="effort,max_effort,xhigh_effort,thinking,adaptive_thinking,interleaved_thinking"
export ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES="effort,thinking,adaptive_thinking,interleaved_thinking"
export CLAUDE_CODE_EFFORT_LEVEL="xhigh"
export CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS="1"
export ENABLE_PROMPT_CACHING_1H="1"
export DISABLE_ERROR_REPORTING="1"
export DISABLE_TELEMETRY="1"
export DISABLE_AUTOUPDATE="1"
export DISABLE_BUG_COMMAND="1"
# END: Claude Code Bedrock Configuration
