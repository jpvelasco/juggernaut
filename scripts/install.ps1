$ErrorActionPreference = "Stop"

$AuthMode = "bedrock-" + "api-key"

Write-Output @"
Juggernaut v5 is installed from npm only.

Install Claude Code with Anthropic's installer:
  curl -fsSL https://claude.ai/install.sh | bash

Install Juggernaut:
  npm install -g juggernaut-bedrock

Then configure Bedrock:
  juggernaut apply --auth=$AuthMode
"@

exit 1
