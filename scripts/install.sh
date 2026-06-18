#!/usr/bin/env bash
set -euo pipefail

cat >&2 <<'EOF'
Juggernaut v5 is installed from npm only.

Install Claude Code with Anthropic's installer:
  curl -fsSL https://claude.ai/install.sh | bash

Install Juggernaut:
  npm install -g juggernaut-bedrock

Then configure Bedrock:
  juggernaut apply --auth=bedrock-api-key
EOF

exit 1
