
# BEGIN: Claude Code Bedrock Configuration
# Auth mode: bedrock-api-key
# Region: eu-west-1
# Storage: profile
# EffortLevel: high
$env:AWS_REGION = 'eu-west-1'
$env:CLAUDE_CODE_USE_BEDROCK = '1'
$env:CLAUDE_CODE_MAX_OUTPUT_TOKENS = '32768'
$env:MAX_THINKING_TOKENS = '10000'
$env:ANTHROPIC_MODEL = 'global.anthropic.claude-sonnet-4-6'
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
$env:AWS_BEARER_TOKEN_BEDROCK = 'my-test-api-key-plaintext'
# END: Claude Code Bedrock Configuration
