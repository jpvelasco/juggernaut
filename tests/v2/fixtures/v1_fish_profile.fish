
# BEGIN: Claude Code Bedrock Configuration
# Auth mode: iam
# EffortLevel: xhigh
set -e AWS_BEARER_TOKEN_BEDROCK 2>/dev/null
set -gx AWS_REGION us-east-1
set -gx CLAUDE_CODE_USE_BEDROCK 1
set -gx CLAUDE_CODE_MAX_OUTPUT_TOKENS 32768
set -gx MAX_THINKING_TOKENS 65536
set -gx ANTHROPIC_MODEL global.anthropic.claude-sonnet-4-6
set -gx ANTHROPIC_DEFAULT_OPUS_MODEL global.anthropic.claude-opus-4-7[1m]
set -gx ANTHROPIC_DEFAULT_SONNET_MODEL global.anthropic.claude-sonnet-4-6
set -gx ANTHROPIC_DEFAULT_HAIKU_MODEL global.anthropic.claude-haiku-4-5-20251001-v1:0
set -gx CLAUDE_CODE_SUBAGENT_MODEL global.anthropic.claude-haiku-4-5-20251001-v1:0
set -gx CLAUDE_CODE_EFFORT_LEVEL xhigh
set -gx ENABLE_PROMPT_CACHING_1H 1
set -gx DISABLE_ERROR_REPORTING 1
set -gx DISABLE_TELEMETRY 1
set -gx DISABLE_AUTOUPDATE 1
set -gx DISABLE_BUG_COMMAND 1
# END: Claude Code Bedrock Configuration
