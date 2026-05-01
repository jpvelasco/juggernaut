#!/usr/bin/env bash
# tests/v2/test_upgrade_banner.sh — tests for lib/upgrade_banner.sh

set -uo pipefail
set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
RELEASE_VERSION="$(tr -d '\r\n ' < "$REPO_ROOT/VERSION" 2>/dev/null || echo 'test-version')"
export BEDROCK_CONFIG_PATH

. "$REPO_ROOT/lib/profile_paths.sh"
. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
. "$REPO_ROOT/lib/migrator.sh"
. "$REPO_ROOT/lib/upgrade_banner.sh"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }
assert_eq() {
  local label="$1" got="$2" want="$3"
  if [[ "$got" == "$want" ]]; then pass; else fail "$label: expected '$want', got '$got'"; fi
}
assert_true()  { local l="$1" v="$2"; if [[ "$v" == "true" ]];  then pass; else fail "$l: expected true, got '$v'"; fi; }
assert_false() { local l="$1" v="$2"; if [[ "$v" == "false" ]]; then pass; else fail "$l: expected false, got '$v'"; fi; }

# ---------------------------------------------------------------------------
# Setup helpers
# ---------------------------------------------------------------------------
make_v1_profile() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  cat > "$path" << 'PROFILE_EOF'
# existing content
# BEGIN: Claude Code Bedrock Configuration
export AWS_REGION="us-west-2"
export CLAUDE_CODE_USE_BEDROCK="1"
# END: Claude Code Bedrock Configuration
PROFILE_EOF
}

make_v2_settings() {
  local path="$1"
  local region="${2:-us-west-2}"
  local block
  mkdir -p "$(dirname "$path")"
  block="$(J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$RELEASE_VERSION" \
    J_SHELL_FALLBACK_MODE=settings-only schema_new_juggernaut_block)"
  config_write_atomic "$path" \
    "$(config_merge_juggernaut_block '{}' "$block" "$(schema_derive_native_keys "$block")")"
}

# ---------------------------------------------------------------------------
# State detection: no v1, no v2
# ---------------------------------------------------------------------------
section "detect_state — clean (no v1, no v2)"
TMP_HOME="$(mktemp -d)"
OLD_HOME="$HOME"
export HOME="$TMP_HOME"
STATE="$(upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
export HOME="$OLD_HOME"
rm -rf "$TMP_HOME"

assert_false "no-v1/has_v1"         "$(printf '%s' "$STATE" | jq -r '.has_v1')"
assert_false "no-v1/has_v2_settings" "$(printf '%s' "$STATE" | jq -r '.has_v2_settings')"

# ---------------------------------------------------------------------------
# State detection: v1 profile present, no v2
# ---------------------------------------------------------------------------
section "detect_state — v1 profile present, no v2 settings"
TMP_HOME="$(mktemp -d)"
OLD_HOME="$HOME"
export HOME="$TMP_HOME"
make_v1_profile "$TMP_HOME/.bashrc"
STATE="$(upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
export HOME="$OLD_HOME"
rm -rf "$TMP_HOME"

assert_true  "v1-only/has_v1"          "$(printf '%s' "$STATE" | jq -r '.has_v1')"
assert_false "v1-only/has_v2_settings"  "$(printf '%s' "$STATE" | jq -r '.has_v2_settings')"
V1_COUNT="$(printf '%s' "$STATE" | jq '.v1_profiles | length')"
if [[ "$V1_COUNT" -ge 1 ]]; then pass; else fail "v1-only: expected at least 1 v1_profile, got $V1_COUNT"; fi

# ---------------------------------------------------------------------------
# State detection: v2 settings present, no v1
# ---------------------------------------------------------------------------
section "detect_state — v2 settings present, no v1"
TMP_HOME="$(mktemp -d)"
OLD_HOME="$HOME"
export HOME="$TMP_HOME"
mkdir -p "$TMP_HOME/.claude"
make_v2_settings "$TMP_HOME/.claude/settings.json"
STATE="$(upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
export HOME="$OLD_HOME"
rm -rf "$TMP_HOME"

assert_false "v2-only/has_v1"         "$(printf '%s' "$STATE" | jq -r '.has_v1')"
assert_true  "v2-only/has_v2_settings" "$(printf '%s' "$STATE" | jq -r '.has_v2_settings')"

# ---------------------------------------------------------------------------
# State detection: MigrationDeclined marker suppresses v1
# ---------------------------------------------------------------------------
section "detect_state — MigrationDeclined marker suppresses v1 detection"
TMP_HOME="$(mktemp -d)"
OLD_HOME="$HOME"
export HOME="$TMP_HOME"
make_v1_profile "$TMP_HOME/.bashrc"
# Append decline marker.
printf '\n# MigrationDeclined: 2024-01-01\n' >> "$TMP_HOME/.bashrc"
STATE="$(upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
assert_false "declined/has_v1" "$(printf '%s' "$STATE" | jq -r '.has_v1')"

# JUGGERNAUT_FORCE_MIGRATION_PROMPT=1 re-enables detection.
STATE2="$(JUGGERNAUT_FORCE_MIGRATION_PROMPT=1 upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
assert_true  "declined-force/has_v1" "$(printf '%s' "$STATE2" | jq -r '.has_v1')"

export HOME="$OLD_HOME"
rm -rf "$TMP_HOME"

# ---------------------------------------------------------------------------
# State detection: is_upgrade flag
# ---------------------------------------------------------------------------
section "detect_state — is_upgrade when versions differ"
TMP_HOME="$(mktemp -d)"
OLD_HOME="$HOME"
export HOME="$TMP_HOME"

# Simulate an install dir with a different version.
TMP_INSTALL_DIR="$(mktemp -d)"
echo "1.0.0" > "$TMP_INSTALL_DIR/VERSION"
OLD_JUG_DIR="${JUGGERNAUT_DIR:-}"
export JUGGERNAUT_DIR="$TMP_INSTALL_DIR"

STATE="$(upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
assert_true  "is_upgrade/true"     "$(printf '%s' "$STATE" | jq -r '.is_upgrade')"
assert_eq    "is_upgrade/installed" "$(printf '%s' "$STATE" | jq -r '.installed_version')" "1.0.0"

if [[ -n "$OLD_JUG_DIR" ]]; then export JUGGERNAUT_DIR="$OLD_JUG_DIR"; else unset JUGGERNAUT_DIR; fi
export HOME="$OLD_HOME"
rm -rf "$TMP_HOME" "$TMP_INSTALL_DIR"

# ---------------------------------------------------------------------------
# State detection: no is_upgrade when versions match
# ---------------------------------------------------------------------------
section "detect_state — no is_upgrade when versions match"
TMP_HOME="$(mktemp -d)"
OLD_HOME="$HOME"
export HOME="$TMP_HOME"

TMP_INSTALL_DIR="$(mktemp -d)"
echo "$RELEASE_VERSION" > "$TMP_INSTALL_DIR/VERSION"
OLD_JUG_DIR="${JUGGERNAUT_DIR:-}"
export JUGGERNAUT_DIR="$TMP_INSTALL_DIR"

STATE="$(upgrade_banner_detect_state "$TMP_HOME/.claude/settings.json" 2>/dev/null)"
assert_false "no-upgrade/false" "$(printf '%s' "$STATE" | jq -r '.is_upgrade')"

if [[ -n "$OLD_JUG_DIR" ]]; then export JUGGERNAUT_DIR="$OLD_JUG_DIR"; else unset JUGGERNAUT_DIR; fi
export HOME="$OLD_HOME"
rm -rf "$TMP_HOME" "$TMP_INSTALL_DIR"

# ---------------------------------------------------------------------------
# upgrade_banner_print: no-op when no v1 and no upgrade
# ---------------------------------------------------------------------------
section "upgrade_banner_print — silent when no banner needed"
CLEAN_STATE='{"has_v1":false,"v1_profiles":[],"has_v2_settings":false,"installed_version":"","release_version":"test","is_upgrade":false,"migration_declined":true}'
BANNER_OUT="$(upgrade_banner_print "$CLEAN_STATE" 2>&1)"
if [[ -z "$BANNER_OUT" ]]; then pass; else fail "expected no output for clean state, got: $BANNER_OUT"; fi

# ---------------------------------------------------------------------------
# upgrade_banner_print: emits banner when v1 detected
# ---------------------------------------------------------------------------
section "upgrade_banner_print — emits banner when v1 detected"
V1_STATE='{"has_v1":true,"v1_profiles":["/home/user/.bashrc"],"has_v2_settings":false,"installed_version":"","release_version":"2.3.0","is_upgrade":false,"migration_declined":false}'
BANNER_OUT="$(upgrade_banner_print "$V1_STATE" 2>&1)"
if [[ "$BANNER_OUT" == *"v1 configuration detected"* ]]; then pass; else fail "expected v1 banner, got: $BANNER_OUT"; fi
if [[ "$BANNER_OUT" == *".bashrc"* ]]; then pass; else fail "expected profile path in banner, got: $BANNER_OUT"; fi

# ---------------------------------------------------------------------------
# upgrade_banner_print: emits version diff when is_upgrade
# ---------------------------------------------------------------------------
section "upgrade_banner_print — version diff in upgrade banner"
UPGRADE_STATE='{"has_v1":false,"v1_profiles":[],"has_v2_settings":true,"installed_version":"2.2.5","release_version":"2.3.0","is_upgrade":true,"migration_declined":false}'
BANNER_OUT="$(upgrade_banner_print "$UPGRADE_STATE" 2>&1)"
if [[ "$BANNER_OUT" == *"2.2.5"* && "$BANNER_OUT" == *"2.3.0"* ]]; then pass
else fail "expected version diff in upgrade banner, got: $BANNER_OUT"; fi

# ---------------------------------------------------------------------------
# upgrade_banner_confirm: no v1 → always proceeds (return 0)
# ---------------------------------------------------------------------------
section "upgrade_banner_confirm — returns 0 when has_v1=false"
NO_V1_STATE='{"has_v1":false,"v1_profiles":[],"has_v2_settings":true,"installed_version":"","release_version":"2.3.0","is_upgrade":false,"migration_declined":false}'
upgrade_banner_confirm "$NO_V1_STATE" false false; RC=$?
if [[ $RC -eq 0 ]]; then pass; else fail "expected 0 for no-v1 state, got $RC"; fi

# ---------------------------------------------------------------------------
# upgrade_banner_confirm: --yes flag → proceeds (return 0)
# ---------------------------------------------------------------------------
section "upgrade_banner_confirm — --yes proceeds"
V1_STATE_NOOPT='{"has_v1":true,"v1_profiles":["/tmp/.bashrc"],"has_v2_settings":false,"installed_version":"","release_version":"2.3.0","is_upgrade":false,"migration_declined":false}'
upgrade_banner_confirm "$V1_STATE_NOOPT" true false; RC=$?
if [[ $RC -eq 0 ]]; then pass; else fail "expected 0 with --yes, got $RC"; fi

# ---------------------------------------------------------------------------
# upgrade_banner_confirm: --legacy-v1 flag → legacy mode (return 2)
# ---------------------------------------------------------------------------
section "upgrade_banner_confirm — --legacy-v1 returns 2"
upgrade_banner_confirm "$V1_STATE_NOOPT" false true; RC=$?
if [[ $RC -eq 2 ]]; then pass; else fail "expected 2 with --legacy-v1, got $RC"; fi

# ---------------------------------------------------------------------------
# upgrade_banner_confirm: non-TTY without flags → abort (return 1)
# ---------------------------------------------------------------------------
section "upgrade_banner_confirm — non-TTY without --yes/--legacy-v1 aborts"
ABORT_OUT="$(JUGGERNAUT_NO_TTY_PROMPTS=1 upgrade_banner_confirm "$V1_STATE_NOOPT" false false 2>&1)"; RC=$?
if [[ $RC -eq 1 ]]; then pass; else fail "expected 1 for non-TTY no-flags, got $RC"; fi
if [[ "$ABORT_OUT" == *"--yes"* || "$ABORT_OUT" == *"--legacy-v1"* ]]; then pass
else fail "expected abort message with flag hints, got: $ABORT_OUT"; fi

# ---------------------------------------------------------------------------
# Sentinel: mark and suppress
# ---------------------------------------------------------------------------
section "upgrade_banner_suppress_sentinel — mark and check"
TMP_INSTALL_DIR="$(mktemp -d)"
if upgrade_banner_suppress_sentinel "$TMP_INSTALL_DIR" "2.3.0"; then
  fail "sentinel should not exist before mark"
else
  pass
fi
upgrade_banner_mark_seen "$TMP_INSTALL_DIR" "2.3.0"
if upgrade_banner_suppress_sentinel "$TMP_INSTALL_DIR" "2.3.0"; then
  pass
else
  fail "sentinel should exist after mark"
fi
# Different version is not suppressed.
if upgrade_banner_suppress_sentinel "$TMP_INSTALL_DIR" "2.4.0"; then
  fail "sentinel for different version should not exist"
else
  pass
fi
rm -rf "$TMP_INSTALL_DIR"

echo
echo "upgrade_banner.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
