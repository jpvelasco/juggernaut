#!/usr/bin/env bash
# tests/v2/test_show.sh — integration checks for commands/show.sh.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0
FAIL=0

fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

section "v2 gate"
if output="$(bash "$REPO_ROOT/commands/show.sh" 2>&1)"; then
  if [[ "$output" == *"Juggernaut v2 is not active. Use --v2 to enable v2 commands."* ]]; then
    pass
  else
    fail "expected inactive message, got: $output"
  fi
else
  fail "show.sh should exit 0 when v2 is inactive"
fi

section "human-readable output"
TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT

mkdir -p "$TMP_HOME/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export JUGGERNAUT_USE_V2=1
export SHELL="/bin/zsh"

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
set +e

J_AUTH_MODE=iam J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=keychain \
  J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION=2.0.0 \
  J_SHELL_FALLBACK_MODE=both \
  USER_BLOCK="$(schema_new_juggernaut_block)"
config_write_atomic "$TMP_HOME/.claude/settings.json" "$(config_merge_juggernaut_block '{}' "$USER_BLOCK" "$(schema_derive_native_keys "$USER_BLOCK")")"

OUTPUT="$(bash "$REPO_ROOT/commands/show.sh" 2>&1)"
if [[ "$OUTPUT" == $'Juggernaut show\nCurrent Juggernaut Block\nScope: user\nAuth: iam\nRegion: us-west-2\nModel: global.anthropic.claude-sonnet-4-6\nEffort: xhigh\nOpus Plan: disabled\nMantle: disabled\nEffective Config\n~/.claude/settings.json\nRegion: us-west-2\nModel: global.anthropic.claude-sonnet-4-6\nShell Fallback\n~/.zshrc\nPresent: yes\nStorage: keychain' ]]; then
  pass
else
  fail "expected show output to match the calm layout"
  printf '%s\n' "$OUTPUT" >&2
fi

echo
echo "show.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
