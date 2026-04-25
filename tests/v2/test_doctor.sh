#!/usr/bin/env bash
# tests/v2/test_doctor.sh - integration checks for commands/doctor.sh.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0
FAIL=0

fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

section "v2 gate"
if output="$(bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"; then
  if [[ "$output" == *"Juggernaut v2 is not active. Use --v2 to enable v2 commands."* ]]; then
    pass
  else
    fail "expected inactive message, got: $output"
  fi
else
  fail "doctor.sh should exit 0 when v2 is inactive"
fi

TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT

mkdir -p "$TMP_HOME/.claude" "$TMP_WORK/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export JUGGERNAUT_USE_V2=1
export AWS_PROFILE="juggernaut-test"
export SHELL="/bin/bash"
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
set +e

write_scope_settings() {
  local scope="$1" target="$2" region="$3"
  J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE="$scope" J_VERSION=2.2.2 \
    J_SHELL_FALLBACK_MODE=settings-only \
    BLOCK="$(schema_new_juggernaut_block)"
  config_write_atomic "$target" "$(config_merge_juggernaut_block '{}' "$BLOCK" "$(schema_derive_native_keys "$BLOCK")")"
}

write_scope_settings user    "$TMP_HOME/.claude/settings.json" us-west-2
write_scope_settings project "$TMP_WORK/.claude/settings.json" eu-west-1

section "shows both scopes with section headers"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"User Scope"* &&
      "$OUTPUT" == *"Project Scope"* &&
      "$OUTPUT" == *"Active Scope"* &&
      "$OUTPUT" == *"Credentials"* &&
      "$OUTPUT" == *"Region & Models"* &&
      "$OUTPUT" == *"Drift"* &&
      "$OUTPUT" == *"Summary"* ]]; then
  pass
else
  fail "missing expected section headers"
  printf '%s\n' "$OUTPUT" >&2
fi

section "shows active scope and both paths"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Active Scope"$'\n'"project"* &&
      "$OUTPUT" == *"~/.claude/settings.json"* &&
      "$OUTPUT" == *"Region: eu-west-1 (OK)"* ]]; then
  pass
else
  fail "expected active scope=project and project region"
  printf '%s\n' "$OUTPUT" >&2
fi

section "honours --scope flag for detail sections"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" --scope=user 2>&1)"
if [[ "$OUTPUT" == *"Region: us-west-2 (OK)"* ]]; then
  pass
else
  fail "expected user scope region us-west-2 when --scope=user"
  printf '%s\n' "$OUTPUT" >&2
fi

section "no issues on a fresh apply"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Status: OK"$'\n'"No issues found"* ]]; then
  pass
else
  fail "expected clean summary"
  printf '%s\n' "$OUTPUT" >&2
fi

section "bedrock API-key auth reports bearer-token source without IAM warning"
API_HOME="$(mktemp -d)"
mkdir -p "$API_HOME/.claude"
J_AUTH_MODE=bedrock-api-key J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=true J_OPUSPLAN=false J_SCOPE=user J_VERSION=2.2.2 \
  J_SHELL_FALLBACK_MODE=settings-only \
  API_BLOCK="$(schema_new_juggernaut_block)"
config_write_atomic "$API_HOME/.claude/settings.json" "$(config_merge_juggernaut_block '{}' "$API_BLOCK" "$(schema_derive_native_keys "$API_BLOCK")")"
OUTPUT="$(HOME="$API_HOME" AWS_BEARER_TOKEN_BEDROCK=br-test AWS_PROFILE=also-set bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Auth: Bedrock API key"* &&
      "$OUTPUT" == *"Source: AWS_BEARER_TOKEN_BEDROCK"* &&
      "$OUTPUT" == *"Reason: Bedrock API key detected"* &&
      "$OUTPUT" != *"is set while auth mode is iam"* ]]; then
  pass
else
  fail "expected bearer-token credentials output without IAM warning"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$API_HOME"

section "reports native drift"
tmp_json="$TMP_HOME/.claude/settings.json.tmp"
jq '.model = "drifted-model"' "$TMP_HOME/.claude/settings.json" > "$tmp_json"
mv "$tmp_json" "$TMP_HOME/.claude/settings.json"
# drift check uses active scope (project) by default; switch to user to see user drift
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" --scope=user 2>&1)"
if [[ "$OUTPUT" == *"Settings native keys: WARN"* ]]; then
  pass
else
  fail "expected drift warning for user scope"
  printf '%s\n' "$OUTPUT" >&2
fi

section "malformed settings fails without mutation"
printf '{' > "$TMP_WORK/.claude/settings.json"
if OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"; then
  fail "doctor should exit non-zero for malformed settings"
  printf '%s\n' "$OUTPUT" >&2
else
  if [[ "$OUTPUT" == *"Project Scope"* && "$OUTPUT" == *"not valid JSON"* ]]; then
    pass
  else
    fail "expected malformed project settings failure"
    printf '%s\n' "$OUTPUT" >&2
  fi
fi

echo
echo "doctor.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
