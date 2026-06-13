#!/usr/bin/env bash
# Pull Codacy server tool configs into .codacy/ and install tool runtimes.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CODACY_CLI="${CODACY_CLI:-codacy-cli}"
if ! command -v "$CODACY_CLI" &>/dev/null; then
  CODACY_CLI="bash <(curl -Ls https://raw.githubusercontent.com/codacy/codacy-cli-v2/main/codacy-cli.sh)"
fi

if [[ -z "${CODACY_API_TOKEN:-}" ]]; then
  echo "CODACY_API_TOKEN is required to sync rules from Codacy cloud." >&2
  echo "Set it in WSL: export CODACY_API_TOKEN=..." >&2
  exit 1
fi

echo "==> Resetting Codacy config from server (jpvelasco/juggernaut)"
eval "$CODACY_CLI config reset \
  --api-token \"\$CODACY_API_TOKEN\" \
  --provider gh \
  --organization jpvelasco \
  --repository juggernaut"

bash "$ROOT/scripts/codacy/patch-eslint.sh"

echo "==> Installing Codacy CLI tool runtimes"
eval "$CODACY_CLI install"

echo "==> Sync complete. Commit .codacy/ changes, then run: make codacy"