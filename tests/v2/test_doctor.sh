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

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
set +e

write_scope_settings() {
  local scope="$1" target="$2" region="$3"
  J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE="$scope" J_VERSION=2.0.0 \
    J_SHELL_FALLBACK_MODE=settings-only \
    BLOCK="$(schema_new_juggernaut_block)"
  config_write_atomic "$target" "$(config_merge_juggernaut_block '{}' "$BLOCK" "$(schema_derive_native_keys "$BLOCK")")"
}

write_scope_settings user "$TMP_HOME/.claude/settings.json" us-west-2
write_scope_settings project "$TMP_WORK/.claude/settings.json" eu-west-1

section "shows both scopes and explicit selected scope"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" --scope=user 2>&1)"
if [[ "$OUTPUT" == *"Scope Awareness"* &&
      "$OUTPUT" == *"selected scope: user"* &&
      "$OUTPUT" == *"active scope: project takes precedence for this session"* &&
      "$OUTPUT" == *"User Scope (selected)"* &&
      "$OUTPUT" == *"Project Scope (active)"* &&
      "$OUTPUT" == *"region: us-west-2"* &&
      "$OUTPUT" == *"region: eu-west-1"* ]]; then
  pass
else
  fail "expected both user and project scope output"
  printf '%s\n' "$OUTPUT" >&2
fi

section "reports native drift per scope"
tmp_json="$TMP_HOME/.claude/settings.json.tmp"
jq '.model = "drifted-model"' "$TMP_HOME/.claude/settings.json" > "$tmp_json"
mv "$tmp_json" "$TMP_HOME/.claude/settings.json"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" --scope=user 2>&1)"
if [[ "$OUTPUT" == *"settings native keys: differ from juggernaut block"* ]]; then
  pass
else
  fail "expected settings native drift warning"
  printf '%s\n' "$OUTPUT" >&2
fi

section "malformed settings fails without mutation"
printf '{' > "$TMP_WORK/.claude/settings.json"
if OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"; then
  fail "doctor should exit non-zero for malformed settings"
  printf '%s\n' "$OUTPUT" >&2
else
  if [[ "$OUTPUT" == *"Project Scope"* && "$OUTPUT" == *"settings.json: not valid JSON"* ]]; then
    pass
  else
    fail "expected malformed project settings failure"
    printf '%s\n' "$OUTPUT" >&2
  fi
fi

echo
echo "doctor.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
