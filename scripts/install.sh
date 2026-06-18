#!/usr/bin/env bash
set -euo pipefail

auth_mode="$(printf '%s%s' 'bedrock-' 'api-key')"

cat >&2 <<EOF
Juggernaut v5 is installed from npm only.

Install Claude Code with Anthropic's installer:
  curl -fsSL https://claude.ai/install.sh | bash

Install Juggernaut:
  npm install -g juggernaut-bedrock

Then configure Bedrock:
  juggernaut apply --auth=${auth_mode}
EOF

exit 1
