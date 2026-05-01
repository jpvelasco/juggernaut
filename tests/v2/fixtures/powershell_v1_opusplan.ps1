
# BEGIN: Claude Code Bedrock Configuration
# Auth mode: iam
# Region: us-west-2
# Storage: profile
# EffortLevel: high
# OpusPlan: true
$env:AWS_REGION = 'us-west-2'
$env:CLAUDE_CODE_USE_BEDROCK = '1'
$env:CLAUDE_CODE_MAX_OUTPUT_TOKENS = '32768'
$env:MAX_THINKING_TOKENS = '10000'
$env:ANTHROPIC_MODEL = 'opusplan'
$env:ANTHROPIC_DEFAULT_OPUS_MODEL = 'global.anthropic.claude-opus-4-7[1m]'
$env:ANTHROPIC_DEFAULT_SONNET_MODEL = 'global.anthropic.claude-sonnet-4-6'
$env:ANTHROPIC_DEFAULT_HAIKU_MODEL = 'global.anthropic.claude-haiku-4-5-20251001-v1:0'
$env:CLAUDE_CODE_SUBAGENT_MODEL = 'global.anthropic.claude-haiku-4-5-20251001-v1:0'
$env:CLAUDE_CODE_EFFORT_LEVEL = 'high'
$env:ENABLE_PROMPT_CACHING_1H = '1'
$env:DISABLE_ERROR_REPORTING = '1'
$env:DISABLE_TELEMETRY = '1'
$env:DISABLE_AUTOUPDATE = '1'
$env:DISABLE_BUG_COMMAND = '1'
# END: Claude Code Bedrock Configuration
