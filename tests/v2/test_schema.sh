#!/usr/bin/env bash
# tests/v2/test_schema.sh — unit tests for lib/schema.sh
# Run: bash tests/v2/test_schema.sh  (exits nonzero on failure)

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=../../lib/schema.sh
source "$REPO_ROOT/lib/schema.sh"

export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
# Assertion helpers: using `if/else` keeps exit codes explicit (avoids SC2015 A && B || C pitfalls).
assert_eq()      { if [[ "$1" == "$2" ]]; then pass; else fail "$3 (expected '$2', got '$1')"; fi; }
assert_cmd()     { if "$@" >/dev/null 2>&1; then pass; else fail "command failed: $*"; fi; }
assert_not_cmd() { if ! "$@" >/dev/null 2>&1; then pass; else fail "command unexpectedly succeeded: $*"; fi; }

section() { echo; echo "== $1 =="; }

section "New block with defaults"
block="$(schema_new_juggernaut_block)"
assert_eq "$(echo "$block" | jq -r '.meta.managedBy')"    "juggernaut" "managedBy should be juggernaut"
assert_eq "$(echo "$block" | jq -r '.schemaVersion')"     "1"          "schemaVersion should be 1"
assert_eq "$(echo "$block" | jq -r '.useMantle')"         "true"       "useMantle default should be true"
assert_eq "$(echo "$block" | jq -r '.auth.region')"       "us-east-1"  "default region should be us-east-1"
assert_eq "$(echo "$block" | jq -r '.effortLevel')"       "xhigh"      "default effort should be xhigh"
assert_eq "$(echo "$block" | jq -r '.env.CLAUDE_CODE_USE_MANTLE')" "1" "useMantle=true should set CLAUDE_CODE_USE_MANTLE=1"
assert_eq "$(echo "$block" | jq 'has("env") and (.env | has("ANTHROPIC_BEDROCK_MANTLE_BASE_URL"))')" "false" \
  "absent mantle baseUrl should not write env key"

section "useMantle=false"
export J_USE_MANTLE=false
block2="$(schema_new_juggernaut_block)"
unset J_USE_MANTLE
assert_eq "$(echo "$block2" | jq '.env | has("CLAUDE_CODE_USE_MANTLE")')"            "false" "useMantle=false should omit CLAUDE_CODE_USE_MANTLE"
assert_eq "$(echo "$block2" | jq '.env | has("ANTHROPIC_BEDROCK_MANTLE_BASE_URL")')" "false" "useMantle=false should omit mantle base URL"

section "mantle baseUrl"
export J_USE_MANTLE=true J_MANTLE_BASE_URL="https://mantle.example.com"
block3="$(schema_new_juggernaut_block)"
unset J_USE_MANTLE J_MANTLE_BASE_URL
assert_eq "$(echo "$block3" | jq -r '.env.ANTHROPIC_BEDROCK_MANTLE_BASE_URL')" "https://mantle.example.com" "baseUrl should be written to env"
assert_eq "$(echo "$block3" | jq -r '.mantle.baseUrl')"                       "https://mantle.example.com" "baseUrl stored in juggernaut.mantle.baseUrl"

section "opusplan special literal"
export J_OPUSPLAN=true
block4="$(schema_new_juggernaut_block)"
unset J_OPUSPLAN
assert_eq "$(echo "$block4" | jq -r '.env.ANTHROPIC_MODEL')" "opusplan" "opusplan=true should set ANTHROPIC_MODEL=opusplan"

section "Region override preserves us-west-2"
export J_REGION=us-west-2
block5="$(schema_new_juggernaut_block)"
unset J_REGION
assert_eq "$(echo "$block5" | jq -r '.auth.region')"    "us-west-2" "region override should apply to auth.region"
assert_eq "$(echo "$block5" | jq -r '.env.AWS_REGION')" "us-west-2" "region override should apply to env.AWS_REGION"

section "schema_validate — positive and negative cases"
assert_cmd schema_validate "$block"
assert_not_cmd schema_validate "$(echo "$block" | jq '.auth.mode = "sso"')"
assert_not_cmd schema_validate "$(echo "$block" | jq '.effortLevel = "insane"')"
assert_not_cmd schema_validate "$(echo "$block" | jq '.meta.managedBy = "someoneElse"')"
assert_not_cmd schema_validate "$(echo "$block" | jq '.auth.region = "mars-central-1"')"
assert_not_cmd schema_validate "$(echo "$block" | jq '.auth.region = ""')"

section "schema_derive_native_keys"
native="$(schema_derive_native_keys "$block")"
assert_eq "$(echo "$native" | jq 'has("env") and has("model") and has("modelOverrides")')" "true" "native keys should include env/model/modelOverrides"
assert_eq "$(echo "$native" | jq -r '.env.CLAUDE_CODE_USE_BEDROCK')"                       "1"    "native env should carry CLAUDE_CODE_USE_BEDROCK"
assert_eq "$(echo "$native" | jq -r '.env.AWS_REGION')"                                    "us-east-1" "native env should carry AWS_REGION derived from juggernaut.auth.region"

echo
echo "schema.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
