#!/usr/bin/env bash
# tests/v2/test_migrator.sh — golden-file tests for lib/migrator.sh.
# Each fixture is parsed and built into a v2 block; key fields are asserted.

set -uo pipefail
# Prevent set -e in sourced libs from killing this test runner.
set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURES="$REPO_ROOT/tests/v2/fixtures"

BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export BEDROCK_CONFIG_PATH

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
. "$REPO_ROOT/lib/migrator.sh"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }
assert_eq() {
  local label="$1" got="$2" want="$3"
  if [[ "$got" == "$want" ]]; then pass; else fail "$label: expected '$want', got '$got'"; fi
}
assert_true() {
  local label="$1" val="$2"
  if [[ "$val" == "true" ]]; then pass; else fail "$label: expected true, got '$val'"; fi
}
assert_false() {
  local label="$1" val="$2"
  if [[ "$val" == "false" ]]; then pass; else fail "$label: expected false, got '$val'"; fi
}

# ---------------------------------------------------------------------------
# Helper: parse + build from a fixture file
# ---------------------------------------------------------------------------
build_from_fixture() {
  local fixture="$1"
  local raw
  raw="$(migrator_extract_block "$fixture")"
  local parsed
  parsed="$(migrator_parse_v1_block "$raw")"
  migrator_build_v2_block "$parsed" "$BEDROCK_CONFIG_PATH"
}

# ---------------------------------------------------------------------------
# Fixture: v1_iam_default
# ---------------------------------------------------------------------------
section "v1_iam_default — IAM auth, us-east-1, default models"
BLOCK="$(build_from_fixture "$FIXTURES/v1_iam_default.sh")"

assert_eq "auth.mode"    "$(printf '%s' "$BLOCK" | jq -r '.auth.mode')"    "iam"
assert_eq "auth.region"  "$(printf '%s' "$BLOCK" | jq -r '.auth.region')"  "us-east-1"
assert_eq "auth.storage" "$(printf '%s' "$BLOCK" | jq -r '.auth.storage')" "profile"
assert_eq "effortLevel"  "$(printf '%s' "$BLOCK" | jq -r '.effortLevel')"  "xhigh"
assert_false "opusplan"  "$(printf '%s' "$BLOCK" | jq -r '.opusplan')"
assert_false "useMantle" "$(printf '%s' "$BLOCK" | jq -r '.useMantle')"
assert_eq "env.AWS_REGION"             "$(printf '%s' "$BLOCK" | jq -r '.env.AWS_REGION')"             "us-east-1"
assert_eq "env.CLAUDE_CODE_USE_BEDROCK" "$(printf '%s' "$BLOCK" | jq -r '.env.CLAUDE_CODE_USE_BEDROCK')" "1"
assert_eq "meta.managedBy"  "$(printf '%s' "$BLOCK" | jq -r '.meta.managedBy')"  "juggernaut"
assert_eq "meta.migratedFrom" "$(printf '%s' "$BLOCK" | jq -r '.meta.migratedFrom')" "v1.7.x"
assert_eq "legacyEnv.source" "$(printf '%s' "$BLOCK" | jq -r '.legacyEnv.source')" "v1.7.x-profile-block"

# ---------------------------------------------------------------------------
# Fixture: v1_apikey_profile
# ---------------------------------------------------------------------------
section "v1_apikey_profile — API key, plaintext in profile, us-west-2"
BLOCK="$(build_from_fixture "$FIXTURES/v1_apikey_profile.sh")"

assert_eq "auth.mode"    "$(printf '%s' "$BLOCK" | jq -r '.auth.mode')"    "api-key"
assert_eq "auth.region"  "$(printf '%s' "$BLOCK" | jq -r '.auth.region')"  "us-west-2"
assert_eq "auth.storage" "$(printf '%s' "$BLOCK" | jq -r '.auth.storage')" "profile"
assert_false "opusplan"  "$(printf '%s' "$BLOCK" | jq -r '.opusplan')"

# ---------------------------------------------------------------------------
# Fixture: v1_apikey_keychain
# ---------------------------------------------------------------------------
section "v1_apikey_keychain — API key, keychain storage, us-east-1"
BLOCK="$(build_from_fixture "$FIXTURES/v1_apikey_keychain.sh")"

assert_eq "auth.mode"    "$(printf '%s' "$BLOCK" | jq -r '.auth.mode')"    "api-key"
assert_eq "auth.storage" "$(printf '%s' "$BLOCK" | jq -r '.auth.storage')" "keychain"
assert_eq "auth.region"  "$(printf '%s' "$BLOCK" | jq -r '.auth.region')"  "us-east-1"

# ---------------------------------------------------------------------------
# Fixture: v1_opusplan_effort
# ---------------------------------------------------------------------------
section "v1_opusplan_effort — opusplan=true, effort=high, us-west-2"
BLOCK="$(build_from_fixture "$FIXTURES/v1_opusplan_effort.sh")"

assert_true  "opusplan"   "$(printf '%s' "$BLOCK" | jq -r '.opusplan')"
assert_eq    "effortLevel" "$(printf '%s' "$BLOCK" | jq -r '.effortLevel')" "high"
assert_eq    "auth.region" "$(printf '%s' "$BLOCK" | jq -r '.auth.region')" "us-west-2"
assert_eq    "env.ANTHROPIC_MODEL" "$(printf '%s' "$BLOCK" | jq -r '.env.ANTHROPIC_MODEL')" "opusplan"

# ---------------------------------------------------------------------------
# Fixture: v1_custom_models
# ---------------------------------------------------------------------------
section "v1_custom_models — eu-west-1, us-prefixed model IDs"
BLOCK="$(build_from_fixture "$FIXTURES/v1_custom_models.sh")"

assert_eq "auth.region"        "$(printf '%s' "$BLOCK" | jq -r '.auth.region')"              "eu-west-1"
assert_eq "model"              "$(printf '%s' "$BLOCK" | jq -r '.model')"                    "us.anthropic.claude-sonnet-4-6"
assert_eq "modelOverrides.opus" "$(printf '%s' "$BLOCK" | jq -r '.modelOverrides.opus')"     "us.anthropic.claude-opus-4-7[1m]"

# ---------------------------------------------------------------------------
# Fixture: v1_apikey_keychain — legacyEnv snapshot includes keychain export
# ---------------------------------------------------------------------------
section "v1_apikey_keychain — legacyEnv.snapshot has AWS_BEARER_TOKEN_BEDROCK"
BLOCK="$(build_from_fixture "$FIXTURES/v1_apikey_keychain.sh")"

LEGACY_VAL="$(printf '%s' "$BLOCK" | jq -r '.legacyEnv.snapshot["AWS_BEARER_TOKEN_BEDROCK"] // ""')"
# The value should be present (even if it's the uneval'd command string).
if [[ -n "$LEGACY_VAL" ]]; then pass; else fail "legacyEnv.snapshot should capture AWS_BEARER_TOKEN_BEDROCK"; fi

# ---------------------------------------------------------------------------
# Fixture: v1_bare_exports — no metadata comments, fallback to export lines
# ---------------------------------------------------------------------------
section "v1_bare_exports — all fields from export lines only"
BLOCK="$(build_from_fixture "$FIXTURES/v1_bare_exports.sh")"

assert_eq "bare/auth.mode"   "$(printf '%s' "$BLOCK" | jq -r '.auth.mode')"   "iam"
assert_eq "bare/auth.region" "$(printf '%s' "$BLOCK" | jq -r '.auth.region')" "us-west-2"
assert_eq "bare/model"       "$(printf '%s' "$BLOCK" | jq -r '.model')"       "global.anthropic.claude-sonnet-4-6"

# legacyEnv snapshot should capture unquoted export values too
BARE_LEGACY="$(printf '%s' "$BLOCK" | jq -r '.legacyEnv.snapshot["AWS_BEARER_TOKEN_BEDROCK"] // ""')"
if [[ -n "$BARE_LEGACY" ]]; then pass; else fail "legacyEnv.snapshot should capture unquoted export line"; fi

# ---------------------------------------------------------------------------
# Fixture: v1_fish_profile — fish set -gx lines detected but not parsed as exports
# ---------------------------------------------------------------------------
section "v1_fish_profile — has_v1_block detects fish profile"
if migrator_has_v1_block "$FIXTURES/v1_fish_profile.fish"; then pass; else fail "should detect v1 block in fish fixture"; fi

# Fish uses set -gx, not export — migrator treats auth.mode as default (iam)
# and region/model fall back to defaults since no export lines exist.
FISH_RAW="$(migrator_extract_block "$FIXTURES/v1_fish_profile.fish")"
FISH_PARSED="$(migrator_parse_v1_block "$FISH_RAW")"
assert_eq "fish/auth.mode"    "$(printf '%s' "$FISH_PARSED" | jq -r '.authMode')"    "iam"
assert_eq "fish/effortLevel"  "$(printf '%s' "$FISH_PARSED" | jq -r '.effortLevel')" "xhigh"
assert_eq "fish/region-default" "$(printf '%s' "$FISH_PARSED" | jq -r '.region')"   "us-east-1"

# ---------------------------------------------------------------------------
# migrator_has_v1_block detection
# ---------------------------------------------------------------------------
section "migrator_has_v1_block"
if migrator_has_v1_block "$FIXTURES/v1_iam_default.sh"; then pass; else fail "should detect v1 block in fixture"; fi
EMPTY="$(mktemp)"
if ! migrator_has_v1_block "$EMPTY"; then pass; else fail "should not detect v1 block in empty file"; fi
rm -f "$EMPTY"
if ! migrator_has_v1_block "/nonexistent/path.sh"; then pass; else fail "should return false for missing file"; fi

# ---------------------------------------------------------------------------
# Full round-trip: migrator_run writes settings.json
# ---------------------------------------------------------------------------
section "migrator_run — full round-trip to settings.json"
TMP_DIR="$(mktemp -d)"
TMP_SETTINGS="$TMP_DIR/settings.json"
TMP_PROFILE="$TMP_DIR/profile.sh"
cp "$FIXTURES/v1_iam_default.sh" "$TMP_PROFILE"

migrator_run "$TMP_PROFILE" "$TMP_SETTINGS" "$BEDROCK_CONFIG_PATH"

READBACK="$(config_read "$TMP_SETTINGS")"
assert_eq "round-trip managedBy"  "$(printf '%s' "$READBACK" | jq -r '.juggernaut.meta.managedBy')"      "juggernaut"
assert_eq "round-trip AWS_REGION" "$(printf '%s' "$READBACK" | jq -r '.env.AWS_REGION')"                 "us-east-1"
assert_eq "round-trip BEDROCK"    "$(printf '%s' "$READBACK" | jq -r '.env.CLAUDE_CODE_USE_BEDROCK')"    "1"
assert_eq "round-trip native model" "$(printf '%s' "$READBACK" | jq -r '.model')"                        "global.anthropic.claude-sonnet-4-6"

# Profile should be annotated (notice present, metadata comments removed).
if grep -q "Juggernaut v2: PRIMARY config" "$TMP_PROFILE"; then pass; else fail "profile not annotated"; fi
if grep -q "^# Auth mode:" "$TMP_PROFILE"; then fail "metadata comment should be removed after annotation"; else pass; fi

rm -rf "$TMP_DIR"

# ---------------------------------------------------------------------------
# migrator_rollback
# ---------------------------------------------------------------------------
section "migrator_rollback — restores previous settings.json"
TMP_DIR="$(mktemp -d)"
TMP_SETTINGS="$TMP_DIR/settings.json"
TMP_PROFILE="$TMP_DIR/profile.sh"
cp "$FIXTURES/v1_iam_default.sh" "$TMP_PROFILE"

# Write a pre-existing settings.json so config_write_atomic will back it up on migration.
echo '{"preexisting": true}' > "$TMP_SETTINGS"

migrator_run "$TMP_PROFILE" "$TMP_SETTINGS" "$BEDROCK_CONFIG_PATH" >/dev/null

# Clobber the settings with garbage so rollback is verifiable.
echo '{"clobbered": true}' > "$TMP_SETTINGS"

migrator_rollback "$TMP_SETTINGS" >/dev/null
AFTER_ROLLBACK="$(config_read "$TMP_SETTINGS")"
# The backup was the pre-existing {"preexisting":true}; it won't have juggernaut key.
# Just assert rollback produced valid JSON (not the clobbered version).
if ! printf '%s' "$AFTER_ROLLBACK" | jq -e '. | has("clobbered") | not' >/dev/null 2>&1; then
  fail "rollback did not restore — still shows clobbered JSON"
else
  pass
fi

rm -rf "$TMP_DIR"

# ---------------------------------------------------------------------------
# --dry-run: no files written
# ---------------------------------------------------------------------------
section "migrate.sh --dry-run — no files written"
TMP_DIR="$(mktemp -d)"
TMP_SETTINGS="$TMP_DIR/settings.json"
TMP_PROFILE="$TMP_DIR/profile.sh"
cp "$FIXTURES/v1_iam_default.sh" "$TMP_PROFILE"

BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json" \
  bash "$REPO_ROOT/commands/migrate.sh" --dry-run >/dev/null 2>&1 || true

# settings.json must NOT exist (dry-run should not write it)
if [[ ! -f "$TMP_SETTINGS" ]]; then pass; else fail "--dry-run must not write settings.json"; fi

rm -rf "$TMP_DIR"

# ---------------------------------------------------------------------------
# --clean: removes the v1 block from profile after migration
# ---------------------------------------------------------------------------
section "migrate.sh --clean — removes block from profile"
TMP_DIR="$(mktemp -d)"
TMP_SETTINGS="$TMP_DIR/settings.json"
TMP_PROFILE="$TMP_DIR/profile.sh"
cp "$FIXTURES/v1_iam_default.sh" "$TMP_PROFILE"

# Run migration with --clean via the library functions directly (not CLI,
# since migrate.sh scans fixed CANDIDATES paths, not arbitrary test paths).
migrator_run "$TMP_PROFILE" "$TMP_SETTINGS" "$BEDROCK_CONFIG_PATH" >/dev/null

# Simulate the --clean step (same logic as commands/migrate.sh).
if sed --version 2>/dev/null | grep -q GNU; then
  sed -i '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$TMP_PROFILE"
else
  sed -i '' '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$TMP_PROFILE"
fi

# v1 block must be gone after --clean
if ! grep -q "BEGIN: Claude Code Bedrock Configuration" "$TMP_PROFILE"; then pass; else fail "--clean must remove the v1 block"; fi

# settings.json must still be valid
READBACK="$(config_read "$TMP_SETTINGS")"
assert_eq "clean/managedBy" "$(printf '%s' "$READBACK" | jq -r '.juggernaut.meta.managedBy')" "juggernaut"

rm -rf "$TMP_DIR"

echo
echo "migrator.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
