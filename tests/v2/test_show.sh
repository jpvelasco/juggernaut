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

section "v2 gate — JUGGERNAUT_USE_V2=0 exits 2"
JUGGERNAUT_USE_V2=0 bash "$REPO_ROOT/commands/show.sh" >/dev/null 2>&1
_RC=$?
if [[ "$_RC" -eq 2 ]]; then pass; else fail "show.sh should exit 2 with JUGGERNAUT_USE_V2=0 (got $_RC)"; fi

section "human-readable output"
TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT

mkdir -p "$TMP_HOME/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export JUGGERNAUT_USE_V2=1
export SHELL="/bin/zsh"
EXPECTED_VERSION="$(cat "$REPO_ROOT/VERSION" 2>/dev/null | tr -d '\r\n ')"

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
set +e

J_AUTH_MODE=iam J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=keychain \
  J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_SHELL_FALLBACK_MODE=both \
  USER_BLOCK="$(schema_new_juggernaut_block)"
config_write_atomic "$TMP_HOME/.claude/settings.json" "$(config_merge_juggernaut_block '{}' "$USER_BLOCK" "$(schema_derive_native_keys "$USER_BLOCK")")"

OUTPUT="$(bash "$REPO_ROOT/commands/show.sh" 2>&1)"
if [[ "$OUTPUT" == *"Scope Awareness"* &&
      "$OUTPUT" == *"Active Scope: user takes precedence for this session"* &&
      "$OUTPUT" == *"User Scope (active)"* &&
      "$OUTPUT" == *"Scope: user"* &&
      "$OUTPUT" == *"Auth: IAM"* &&
      "$OUTPUT" == *"Region: us-west-2"* &&
      "$OUTPUT" == *"Project Scope"* &&
      "$OUTPUT" == *"Status: No Juggernaut block"* &&
      "$OUTPUT" == *"Shell Fallback"* &&
      "$OUTPUT" == *"Present: yes"* &&
      "$OUTPUT" == *"Storage: keychain"* ]]; then
  pass
else
  fail "expected show output to match the calm layout"
  printf '%s\n' "$OUTPUT" >&2
fi

section "human-readable output without shell fallback"
J_AUTH_MODE=api-key J_REGION=eu-west-1 J_EFFORT=xhigh J_STORAGE=keychain \
  J_USE_MANTLE=true J_OPUSPLAN=true J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_SHELL_FALLBACK_MODE=settings-only \
  USER_BLOCK="$(schema_new_juggernaut_block)"
config_write_atomic "$TMP_HOME/.claude/settings.json" "$(config_merge_juggernaut_block '{}' "$USER_BLOCK" "$(schema_derive_native_keys "$USER_BLOCK")")"

export SHELL="/bin/bash"
OUTPUT="$(bash "$REPO_ROOT/commands/show.sh" 2>&1)"
if [[ "$OUTPUT" == *"User Scope (active)"* &&
      "$OUTPUT" == *"Auth: Bedrock API key"* &&
      "$OUTPUT" == *"Region: eu-west-1"* &&
      "$OUTPUT" == *"Opus Plan: enabled"* &&
      "$OUTPUT" == *"Mantle: enabled"* &&
      "$OUTPUT" == *"Shell Fallback"* &&
      "$OUTPUT" == *"Present: no"* &&
      "$OUTPUT" != *"Storage: keychain"* ]]; then
  pass
else
  fail "expected disabled shell fallback output to omit storage"
  printf '%s\n' "$OUTPUT" >&2
fi

section "shows both scopes when both exist"
mkdir -p "$TMP_WORK/.claude"
J_AUTH_MODE=iam J_REGION=ap-southeast-1 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=project J_VERSION="$EXPECTED_VERSION" \
  J_SHELL_FALLBACK_MODE=settings-only \
  PROJECT_BLOCK="$(schema_new_juggernaut_block)"
config_write_atomic "$TMP_WORK/.claude/settings.json" "$(config_merge_juggernaut_block '{}' "$PROJECT_BLOCK" "$(schema_derive_native_keys "$PROJECT_BLOCK")")"

OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/show.sh" --scope=user 2>&1)"
if [[ "$OUTPUT" == *"Selected Scope: user"* &&
      "$OUTPUT" == *"Active Scope: project takes precedence for this session"* &&
      "$OUTPUT" == *"User Scope (selected)"* &&
      "$OUTPUT" == *"Project Scope (active)"* &&
      "$OUTPUT" == *"Region: eu-west-1"* &&
      "$OUTPUT" == *"Region: ap-southeast-1"* ]]; then
  pass
else
  fail "expected show to print both scopes and mark selected/active"
  printf '%s\n' "$OUTPUT" >&2
fi

echo
echo "show.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
