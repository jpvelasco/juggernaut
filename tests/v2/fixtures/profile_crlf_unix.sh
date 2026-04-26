# Pre-existing user content

# BEGIN: Claude Code Bedrock Configuration
# Auth mode: iam
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true
export AWS_REGION="us-west-2"
export CLAUDE_CODE_USE_BEDROCK="1"
export CLAUDE_CODE_EFFORT_LEVEL="xhigh"
# END: Claude Code Bedrock Configuration

# Trailing user content
