#!/usr/bin/env bash
# tests/v2/test_doctor.sh — v3 tests for commands/doctor.sh + lib/doctor.sh.
# Covers: scope detection, credentials check, region/model check, Mantle,
# opusplan drift diagnostic, and summary roll-up.

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

mkdir -p "$TMP_HOME/.claude" "$TMP_WORK/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export AWS_PROFILE="juggernaut-test"
export SHELL="/bin/bash"
EXPECTED_VERSION="$(tr -d '\r\n ' < "$REPO_ROOT/VERSION" 2>/dev/null || echo "3.0.0")"
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true

# Source library code for direct unit tests of doctor_credentials.
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/schema.sh"
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/config_manager.sh"
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/keychain.sh"
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/doctor.sh"
set +e

# ---------------------------------------------------------------------------
# doctor_credentials surfaces keychain read failures
# ---------------------------------------------------------------------------
section "keychain read errors are visible in doctor_credentials output"
# shellcheck disable=SC2317  # overrides invoked indirectly by doctor_credentials
keychain_available() { return 0; }
# shellcheck disable=SC2317
keychain_get() { echo "simulated keychain failure" >&2; return 2; }
ERR_BLOCK="$(
  J_AUTH_MODE=bedrock-api-key J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=keychain \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
    schema_new_juggernaut_block
)"
OUTPUT="$(doctor_credentials "$ERR_BLOCK" 2>&1)"
if [[ "$OUTPUT" == *"Keychain/DPAPI: WARN (simulated keychain failure)"* &&
      "$OUTPUT" == *"Details: no API key found in env, keychain, DPAPI file, or profile token file"* ]]; then
  pass
else
  fail "expected visible keychain read failure"
  printf '%s\n' "$OUTPUT" >&2
fi
unset -f keychain_available keychain_get

# ---------------------------------------------------------------------------
# Helper to write settings.json for a given scope/region.
# ---------------------------------------------------------------------------
write_scope_settings() {
  local scope="$1" target="$2" region="$3"
  local block
  block="$(J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE="$scope" J_VERSION="$EXPECTED_VERSION" \
    J_AUTH_VALIDATED=true \
    schema_new_juggernaut_block)"
  config_write_atomic "$target" "$(config_merge_juggernaut_block '{}' "$block" "$(schema_derive_native_keys "$block")")"
}

write_scope_settings user    "$TMP_HOME/.claude/settings.json" us-west-2
write_scope_settings project "$TMP_WORK/.claude/settings.json" eu-west-1

# ---------------------------------------------------------------------------
# End-to-end: section headers present
# ---------------------------------------------------------------------------
section "v3 doctor shows all v3 section headers"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
for header in "User Scope" "Project Scope" "Active Scope" "Credentials" "Region & Models" "Mantle" "Opusplan" "Summary"; do
  if [[ "$OUTPUT" == *"$header"* ]]; then pass; else fail "missing header: $header"; fi
done

# ---------------------------------------------------------------------------
# End-to-end: v3 removed sections must be gone
# ---------------------------------------------------------------------------
section "v3 doctor no longer shows profile-drift / shell-fallback sections"
if [[ "$OUTPUT" != *"Drift"* ]]; then pass; else fail "doctor still mentions 'Drift' (profile drift section should be gone)"; fi
if [[ "$OUTPUT" != *"shell fallback"* ]]; then pass; else fail "doctor still mentions 'shell fallback'"; fi
if [[ "$OUTPUT" != *"Profile:"* ]]; then pass; else fail "doctor still mentions 'Profile:' key"; fi

# ---------------------------------------------------------------------------
# Active scope detection
# ---------------------------------------------------------------------------
section "active scope = project when both scopes have a Juggernaut block and CWD has one"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Active Scope"$'\n'"project"* &&
      "$OUTPUT" == *"Region: eu-west-1 (OK)"* ]]; then
  pass
else
  fail "expected active scope=project and project region eu-west-1"
  printf '%s\n' "$OUTPUT" >&2
fi

section "--scope=user forces detail section to user settings"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" --scope=user 2>&1)"
if [[ "$OUTPUT" == *"Region: us-west-2 (OK)"* ]]; then
  pass
else
  fail "expected user-scope region us-west-2 when --scope=user"
  printf '%s\n' "$OUTPUT" >&2
fi

section "from HOME, project scope shows 'no Juggernaut config' and active=user"
OUTPUT="$(cd "$TMP_HOME" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"User Scope"* &&
      "$OUTPUT" == *"Project Scope"* &&
      "$OUTPUT" == *"Status: no Juggernaut config"* &&
      "$OUTPUT" == *"Active Scope"$'\n'"user"* ]]; then
  pass
else
  fail "expected active=user and project with no-config status from HOME"
  printf '%s\n' "$OUTPUT" >&2
fi

# ---------------------------------------------------------------------------
# Summary: no issues on a fresh apply
# ---------------------------------------------------------------------------
section "fresh user+project apply → Status: OK, No issues found"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Status: OK"$'\n'"No issues found"* ]]; then
  pass
else
  fail "expected clean summary"
  printf '%s\n' "$OUTPUT" >&2
fi

# ---------------------------------------------------------------------------
# Fresh install guidance must use explicit auth mode.
# ---------------------------------------------------------------------------
section "fresh install without config recommends explicit auth mode"
NO_CONFIG_HOME="$(mktemp -d)"
mkdir -p "$NO_CONFIG_HOME/.claude"
printf '{}\n' > "$NO_CONFIG_HOME/.claude/settings.json"
OUTPUT="$(HOME="$NO_CONFIG_HOME" bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" != *"Run 'juggernaut apply'"* &&
      "$OUTPUT" == *"juggernaut apply --auth=iam"* &&
      "$OUTPUT" == *"juggernaut apply --auth=bedrock-api-key"* ]]; then
  pass
else
  fail "expected doctor to recommend explicit apply auth modes"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$NO_CONFIG_HOME"

# ---------------------------------------------------------------------------
# Bedrock API-key auth: bearer-token source detected
# ---------------------------------------------------------------------------
section "bedrock API-key auth reports bearer-token source"
API_HOME="$(mktemp -d)"
mkdir -p "$API_HOME/.claude"
API_BLOCK="$(J_AUTH_MODE=bedrock-api-key J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=true J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true \
  schema_new_juggernaut_block)"
config_write_atomic "$API_HOME/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$API_BLOCK" "$(schema_derive_native_keys "$API_BLOCK")")"
OUTPUT="$(HOME="$API_HOME" AWS_BEARER_TOKEN_BEDROCK=br-test bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Auth: Bedrock API key"* &&
      "$OUTPUT" == *"Source: AWS_BEARER_TOKEN_BEDROCK"* ]]; then
  pass
else
  fail "expected bearer-token credentials output"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$API_HOME"

# ---------------------------------------------------------------------------
# Bedrock API-key auth: Linux profile token storage detected
# ---------------------------------------------------------------------------
section "bedrock API-key auth reports profile token source"
API_HOME="$(mktemp -d)"
mkdir -p "$API_HOME/.claude" "$API_HOME/.config/juggernaut"
API_BLOCK="$(J_AUTH_MODE=bedrock-api-key J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=true J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true \
  schema_new_juggernaut_block)"
config_write_atomic "$API_HOME/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$API_BLOCK" "$(schema_derive_native_keys "$API_BLOCK")")"
printf 'br-profile-token' > "$API_HOME/.config/juggernaut/bearer-token"
OUTPUT="$(HOME="$API_HOME" XDG_CONFIG_HOME="$API_HOME/.config" bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Auth: Bedrock API key"* &&
      "$OUTPUT" == *"Source: profile token file"* &&
      "$OUTPUT" == *"Status: OK"* ]]; then
  pass
else
  fail "expected profile-token credentials output"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$API_HOME"

# ---------------------------------------------------------------------------
# IAM auth with AWS_BEARER_TOKEN_BEDROCK set → warning
# ---------------------------------------------------------------------------
section "iam auth + AWS_BEARER_TOKEN_BEDROCK set → warning"
IAM_HOME="$(mktemp -d)"
mkdir -p "$IAM_HOME/.claude"
write_scope_settings user "$IAM_HOME/.claude/settings.json" us-west-2
OUTPUT="$(HOME="$IAM_HOME" AWS_BEARER_TOKEN_BEDROCK=br-test bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"AWS_BEARER_TOKEN_BEDROCK is set but auth mode is 'iam'"* ]]; then
  pass
else
  fail "expected IAM/bearer-token mismatch warning"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$IAM_HOME"

# ---------------------------------------------------------------------------
# Opusplan drift diagnostic
# ---------------------------------------------------------------------------
section "opusplan enabled and env matches → Status: OK (no drift)"
OP_HOME="$(mktemp -d)"
mkdir -p "$OP_HOME/.claude"
OP_BLOCK="$(J_AUTH_MODE=iam J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=false J_OPUSPLAN=true J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true \
  schema_new_juggernaut_block)"
config_write_atomic "$OP_HOME/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$OP_BLOCK" "$(schema_derive_native_keys "$OP_BLOCK")")"
OUTPUT="$(HOME="$OP_HOME" bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"Opusplan"* &&
      "$OUTPUT" == *"Status: enabled"* &&
      "$OUTPUT" != *"ANTHROPIC_MODEL mismatch"* ]]; then
  pass
else
  fail "expected opusplan OK when env.ANTHROPIC_MODEL matches"
  printf '%s\n' "$OUTPUT" >&2
fi

section "opusplan enabled but env.ANTHROPIC_MODEL overridden → WARN with fix hint"
# Tamper with top-level env.ANTHROPIC_MODEL to simulate external override.
tmp_json="$OP_HOME/.claude/settings.json.tmp"
jq '.env.ANTHROPIC_MODEL = "global.anthropic.claude-sonnet-4-6"' \
  "$OP_HOME/.claude/settings.json" > "$tmp_json"
mv "$tmp_json" "$OP_HOME/.claude/settings.json"
OUTPUT="$(HOME="$OP_HOME" bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *"ANTHROPIC_MODEL mismatch"* &&
      "$OUTPUT" == *"Fix"*"juggernaut apply --opusplan"* ]]; then
  pass
else
  fail "expected opusplan drift warning with fix hint"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$OP_HOME"

# ---------------------------------------------------------------------------
# Malformed settings
# ---------------------------------------------------------------------------
section "malformed project settings → non-zero exit and 'not valid JSON'"
printf '{' > "$TMP_WORK/.claude/settings.json"
OUTPUT="$(cd "$TMP_WORK" && bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
RC=$?
if [[ "$RC" -ne 0 && "$OUTPUT" == *"Project Scope"* && "$OUTPUT" == *"not valid JSON"* ]]; then
  pass
else
  fail "expected non-zero exit and 'not valid JSON' for malformed settings"
  printf '%s\n' "$OUTPUT" >&2
fi

# ---------------------------------------------------------------------------
# Top-level .model poisoning (opusplan in wrong place)
# ---------------------------------------------------------------------------
section "top-level .model='opusplan' → WARN + fix hint (independent of block.opusplan)"
POISON_HOME="$(mktemp -d)"
mkdir -p "$POISON_HOME/.claude"
POISON_BLOCK="$(J_AUTH_MODE=iam J_REGION=us-west-2 J_EFFORT=xhigh J_STORAGE=profile \
  J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
  J_AUTH_VALIDATED=true \
  schema_new_juggernaut_block)"
config_write_atomic "$POISON_HOME/.claude/settings.json" \
  "$(config_merge_juggernaut_block '{}' "$POISON_BLOCK" "$(schema_derive_native_keys "$POISON_BLOCK")")"
tmp_json="$POISON_HOME/.claude/settings.json.tmp"
jq '.model = "opusplan"' "$POISON_HOME/.claude/settings.json" > "$tmp_json"
mv "$tmp_json" "$POISON_HOME/.claude/settings.json"
OUTPUT="$(HOME="$POISON_HOME" bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" == *'Top-level model: WARN'* &&
      "$OUTPUT" == *'"opusplan" is not a Bedrock model ID'* &&
      "$OUTPUT" == *"Fix: run: juggernaut apply"* ]]; then
  pass
else
  fail "expected top-level .model=opusplan to warn with fix hint"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$POISON_HOME"

section "top-level .model healthy (bedrock ID) → no top-level warning"
CLEAN_HOME="$(mktemp -d)"
mkdir -p "$CLEAN_HOME/.claude"
write_scope_settings user "$CLEAN_HOME/.claude/settings.json" us-west-2
OUTPUT="$(HOME="$CLEAN_HOME" bash "$REPO_ROOT/commands/doctor.sh" 2>&1)"
if [[ "$OUTPUT" != *"Top-level model: WARN"* ]]; then
  pass
else
  fail "healthy top-level .model should not trigger the poisoning warning"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$CLEAN_HOME"

# ---------------------------------------------------------------------------
# --help / --version
# ---------------------------------------------------------------------------
section "doctor --help exits 0 and mentions v3"
HELP_OUT="$(bash "$REPO_ROOT/commands/doctor.sh" --help 2>&1)"
RC=$?
if [[ "$RC" -eq 0 && "$HELP_OUT" == *"Juggernaut v3"* ]]; then pass; else fail "doctor --help should exit 0 and mention 'Juggernaut v3'"; fi
if [[ "$HELP_OUT" != *"JUGGERNAUT_USE_V2"* ]]; then pass; else fail "doctor --help should not mention JUGGERNAUT_USE_V2"; fi

echo
echo "doctor.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
