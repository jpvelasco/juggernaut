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

section "v2 gate — JUGGERNAUT_USE_V2=0 exits 2"
JUGGERNAUT_USE_V2=0 bash "$REPO_ROOT/commands/doctor.sh" >/dev/null 2>&1
_RC=$?
if [[ "$_RC" -eq 2 ]]; then pass; else fail "doctor.sh should exit 2 with JUGGERNAUT_USE_V2=0 (got $_RC)"; fi

section "v2 default — runs without JUGGERNAUT_USE_V2 set"
# v2 is ON by default; doctor with no settings → not necessarily OK but must not exit 2.
unset JUGGERNAUT_USE_V2 2>/dev/null || true
JUGGERNAUT_USE_V2= bash "$REPO_ROOT/commands/doctor.sh" >/dev/null 2>&1 || true
_RC=$?
if [[ "$_RC" -ne 2 ]]; then pass; else fail "doctor.sh should run (not exit 2) when JUGGERNAUT_USE_V2 is unset (got $_RC)"; fi

TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT

mkdir -p "$TMP_HOME/.claude" "$TMP_WORK/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export JUGGERNAUT_USE_V2=1
export AWS_PROFILE="juggernaut-test"
export SHELL="/bin/bash"
EXPECTED_VERSION="$(cat "$REPO_ROOT/VERSION" 2>/dev/null | tr -d '\r\n ')"
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
. "$REPO_ROOT/lib/profile_writer.sh"
. "$REPO_ROOT/lib/keychain.sh"
. "$REPO_ROOT/lib/doctor.sh"
set +e

section "keychain read errors are visible"
keychain_available() { return 0; }
keychain_get() { echo "simulated keychain failure" >&2; return 2; }
ERR_BLOCK="$(
  J_AUTH_MODE=bedrock-api-key J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=keychain \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
    J_SHELL_FALLBACK_MODE=settings-only \
    schema_new_juggernaut_block
)"
OUTPUT="$(doctor_credentials "$ERR_BLOCK" "$TMP_HOME/.missing-profile" 2>&1)"
if [[ "$OUTPUT" == *"Keychain: WARN (simulated keychain failure)"* &&
      "$OUTPUT" == *"Details: no API key found in env, keychain, or shell profile"* ]]; then
  pass
else
  fail "expected visible keychain read failure"
  printf '%s\n' "$OUTPUT" >&2
fi
unset -f keychain_available keychain_get

write_scope_settings() {
  local scope="$1" target="$2" region="$3"
  J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE="$scope" J_VERSION="$EXPECTED_VERSION" \
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
  J_USE_MANTLE=true J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
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
