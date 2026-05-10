#!/usr/bin/env bash
# tests/v2/test_show.sh — v3 tests for commands/show.sh.
# Covers: human-readable output, active/selected scope hints, --scope flag.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT

mkdir -p "$TMP_HOME/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export SHELL="/bin/zsh"
EXPECTED_VERSION="$(tr -d '\r\n ' < "$REPO_ROOT/VERSION" 2>/dev/null || echo "3.0.0")"

# shellcheck source=/dev/null
. "$REPO_ROOT/lib/schema.sh"
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/config_manager.sh"
set +e

# ---------------------------------------------------------------------------
# Human-readable IAM output
# ---------------------------------------------------------------------------
section "IAM + user scope — human-readable output"
USER_BLOCK="$(J_AUTH_MODE=iam J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=keychain \
  J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true schema_new_juggernaut_block)"
config_write_atomic "$TMP_HOME/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$USER_BLOCK" "$(schema_derive_native_keys "$USER_BLOCK")")"

OUTPUT="$(bash "$REPO_ROOT/commands/show.sh" 2>&1)"
if [[ "$OUTPUT" == *"Scope Awareness"* &&
      "$OUTPUT" == *"Active Scope: user takes precedence for this session"* &&
      "$OUTPUT" == *"User Scope (active)"* &&
      "$OUTPUT" == *"Scope: user"* &&
      "$OUTPUT" == *"Auth: IAM"* &&
      "$OUTPUT" == *"Region: us-west-2"* &&
      "$OUTPUT" == *"Project Scope"* &&
      "$OUTPUT" == *"Status: No Juggernaut block"* ]]; then
  pass
else
  fail "expected show output to describe IAM + user scope"
  printf '%s\n' "$OUTPUT" >&2
fi

# v3: no "Shell Fallback" section anywhere in output.
if [[ "$OUTPUT" != *"Shell Fallback"* ]]; then pass
else fail "v3 show output should NOT include a Shell Fallback section"; fi

# ---------------------------------------------------------------------------
# Bedrock API-key + opusplan + Mantle
# ---------------------------------------------------------------------------
section "Bedrock API-key + opusplan + Mantle"
USER_BLOCK="$(J_AUTH_MODE=bedrock-api-key J_REGION=eu-west-1 J_EFFORT=xhigh J_STORAGE=keychain \
  J_USE_MANTLE=true J_OPUSPLAN=true J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true schema_new_juggernaut_block)"
config_write_atomic "$TMP_HOME/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$USER_BLOCK" "$(schema_derive_native_keys "$USER_BLOCK")")"

export SHELL="/bin/bash"
OUTPUT="$(bash "$REPO_ROOT/commands/show.sh" 2>&1)"
if [[ "$OUTPUT" == *"User Scope (active)"* &&
      "$OUTPUT" == *"Auth: Bedrock API key"* &&
      "$OUTPUT" == *"Region: eu-west-1"* &&
      "$OUTPUT" == *"Opus Plan: enabled"* &&
      "$OUTPUT" == *"Mantle: enabled"* ]]; then
  pass
else
  fail "expected Bedrock API key + opusplan + Mantle output"
  printf '%s\n' "$OUTPUT" >&2
fi

# ---------------------------------------------------------------------------
# Both scopes — show with --scope=user, project still marked active
# ---------------------------------------------------------------------------
section "both scopes — --scope=user shows selected hint while project stays active"
mkdir -p "$TMP_WORK/.claude"
PROJECT_BLOCK="$(J_AUTH_MODE=iam J_REGION=ap-southeast-1 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=project J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true schema_new_juggernaut_block)"
config_write_atomic "$TMP_WORK/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$PROJECT_BLOCK" "$(schema_derive_native_keys "$PROJECT_BLOCK")")"

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

# ---------------------------------------------------------------------------
# Help text
# ---------------------------------------------------------------------------
section "show --help exits 0, does not mention legacy flags"
help_out="$(bash "$REPO_ROOT/commands/show.sh" --help 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then pass; else fail "show --help should exit 0 (got $rc)"; fi
if [[ "$help_out" == *"--scope"* ]]; then pass; else fail "show --help should mention --scope"; fi
if [[ "$help_out" != *"--legacy-v1"* && "$help_out" != *"shell-fallback"* ]]; then pass
else fail "show --help should NOT mention legacy flags"; fi

# ---------------------------------------------------------------------------
# Unknown option → non-zero with message
# ---------------------------------------------------------------------------
section "unknown option exits non-zero with usage hint"
OUTPUT="$(bash "$REPO_ROOT/commands/show.sh" --not-a-real-flag 2>&1)"
RC=$?
if [[ "$RC" -ne 0 && "$OUTPUT" == *"unknown option"* && "$OUTPUT" == *"--not-a-real-flag"* ]]; then
  pass
else
  fail "show unknown option should exit non-zero and mention the flag (got RC=$RC); output: $OUTPUT"
fi

echo
echo "show.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
