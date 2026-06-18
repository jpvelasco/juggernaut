$ErrorActionPreference = "Stop"

Write-Output @"
Juggernaut v5 is installed from npm only.

Install Claude Code with Anthropic's installer:
  curl -fsSL https://claude.ai/install.sh | bash

Install Juggernaut:
  npm install -g juggernaut-bedrock

Then configure Bedrock:
  juggernaut apply --auth=bedrock-api-key
"@

exit 1
