#!/usr/bin/env bash
# tests/v2/test_schema.sh — unit tests for lib/schema.sh
# Run: bash tests/v2/test_schema.sh  (exits nonzero on failure)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=../../lib/schema.sh
source "$REPO_ROOT/lib/schema.sh"

export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }

section() { echo; echo "== $1 =="; }

# --- schema_new_juggernaut_block defaults ---
section "New block with defaults"
block="$(schema_new_juggernaut_block)"

# Check managed-by
[[ "$(echo "$block" | jq -r '.meta.managedBy')" == "juggernaut" ]] && pass || fail "managedBy should be juggernaut"
[[ "$(echo "$block" | jq -r '.schemaVersion')" == "1" ]] && pass || fail "schemaVersion should be 1"
[[ "$(echo "$block" | jq -r '.useMantle')" == "true" ]] && pass || fail "useMantle default should be true"
[[ "$(echo "$block" | jq -r '.auth.region')" == "us-east-1" ]] && pass || fail "default region should be us-east-1"
[[ "$(echo "$block" | jq -r '.effortLevel')" == "xhigh" ]] && pass || fail "default effort should be xhigh"

# Mantle env rule: useMantle=true sets CLAUDE_CODE_USE_MANTLE=1
[[ "$(echo "$block" | jq -r '.env.CLAUDE_CODE_USE_MANTLE // ""')" == "1" ]] && pass || fail "useMantle=true should set CLAUDE_CODE_USE_MANTLE=1"

# No mantle base URL by default → key absent
[[ "$(echo "$block" | jq 'has("env") and (.env | has("ANTHROPIC_BEDROCK_MANTLE_BASE_URL"))')" == "false" ]] && pass || fail "absent mantle baseUrl should not write env key"

# --- useMantle=false scrubs both env keys ---
section "useMantle=false"
J_USE_MANTLE=false block2="$(schema_new_juggernaut_block)"
[[ "$(echo "$block2" | jq '.env | has("CLAUDE_CODE_USE_MANTLE")')" == "false" ]] && pass || fail "useMantle=false should omit CLAUDE_CODE_USE_MANTLE"
[[ "$(echo "$block2" | jq '.env | has("ANTHROPIC_BEDROCK_MANTLE_BASE_URL")')" == "false" ]] && pass || fail "useMantle=false should omit ANTHROPIC_BEDROCK_MANTLE_BASE_URL"

# --- Mantle base URL when provided ---
section "mantle baseUrl"
J_USE_MANTLE=true J_MANTLE_BASE_URL="https://mantle.example.com" block3="$(schema_new_juggernaut_block)"
[[ "$(echo "$block3" | jq -r '.env.ANTHROPIC_BEDROCK_MANTLE_BASE_URL')" == "https://mantle.example.com" ]] && pass || fail "baseUrl should be written to env"
[[ "$(echo "$block3" | jq -r '.mantle.baseUrl')" == "https://mantle.example.com" ]] && pass || fail "baseUrl should be stored in juggernaut.mantle.baseUrl"

# --- opusplan flips ANTHROPIC_MODEL to literal "opusplan" ---
section "opusplan special literal"
J_OPUSPLAN=true block4="$(schema_new_juggernaut_block)"
[[ "$(echo "$block4" | jq -r '.env.ANTHROPIC_MODEL')" == "opusplan" ]] && pass || fail "opusplan=true should set ANTHROPIC_MODEL=opusplan"

# --- Region override ---
section "Region override preserves us-west-2"
J_REGION=us-west-2 block5="$(schema_new_juggernaut_block)"
[[ "$(echo "$block5" | jq -r '.auth.region')" == "us-west-2" ]] && pass || fail "region override should apply to auth.region"
[[ "$(echo "$block5" | jq -r '.env.AWS_REGION')" == "us-west-2" ]] && pass || fail "region override should apply to env.AWS_REGION"

# --- Validation ---
section "schema_validate accepts default block"
schema_validate "$block" && pass || fail "default block should validate"

section "schema_validate rejects bad auth.mode"
bad1="$(echo "$block" | jq '.auth.mode = "sso"')"
schema_validate "$bad1" 2>/dev/null && fail "bad auth.mode should reject" || pass

section "schema_validate rejects bad effortLevel"
bad2="$(echo "$block" | jq '.effortLevel = "insane"')"
schema_validate "$bad2" 2>/dev/null && fail "bad effortLevel should reject" || pass

section "schema_validate rejects missing managedBy"
bad3="$(echo "$block" | jq '.meta.managedBy = "someoneElse"')"
schema_validate "$bad3" 2>/dev/null && fail "wrong managedBy should reject" || pass

section "schema_validate rejects unsupported region"
bad4="$(echo "$block" | jq '.auth.region = "mars-central-1"')"
schema_validate "$bad4" 2>/dev/null && fail "bad region should reject" || pass

# --- Derive native keys ---
section "schema_derive_native_keys"
native="$(schema_derive_native_keys "$block")"
[[ "$(echo "$native" | jq 'has("env") and has("model") and has("modelOverrides")')" == "true" ]] && pass || fail "native keys should include env/model/modelOverrides"
[[ "$(echo "$native" | jq -r '.env.CLAUDE_CODE_USE_BEDROCK')" == "1" ]] && pass || fail "native env should carry CLAUDE_CODE_USE_BEDROCK"

echo
echo "schema.sh tests: $PASS passed, $FAIL failed"
exit $FAIL
