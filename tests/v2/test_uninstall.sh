#!/usr/bin/env bash
# tests/v2/test_uninstall.sh — v3 tests for commands/uninstall.sh.
# Covers: scope removal (user, project, both), --dry-run, --force, keychain.
# v3: uninstall does NOT touch shell profiles — those are cleaned by install.sh's wipe.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
mkdir -p "$TMP_HOME/.claude" "$TMP_WORK/.claude"

export HOME="$TMP_HOME"
export XDG_CONFIG_HOME="$TMP_HOME/.config"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export SHELL="/bin/bash"
export JUGGERNAUT_NO_TTY_PROMPTS=1
# Isolate keychain for CI: point at a guaranteed-absent service name.
_stamp="$(date +%s%N 2>/dev/null || date +%s)"
export JUGGERNAUT_KEYCHAIN_SERVICE="juggernaut-absent-uninstall-$$-${_stamp}"
EXPECTED_VERSION="$(tr -d '\r\n ' < "$REPO_ROOT/VERSION" 2>/dev/null || echo "3.0.0")"
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true

# shellcheck source=/dev/null
. "$REPO_ROOT/lib/schema.sh"
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/config_manager.sh"
# shellcheck source=/dev/null
. "$REPO_ROOT/lib/keychain.sh"
set +e

trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT

write_settings() {
  local path="$1" region="${2:-us-west-2}"
  local block
  block="$(J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION="$EXPECTED_VERSION" \
    J_AUTH_VALIDATED=true schema_new_juggernaut_block)"
  config_write_atomic "$path" \
    "$(config_merge_juggernaut_block '{}' "$block" "$(schema_derive_native_keys "$block")")"
}

run_uninstall() { bash "$REPO_ROOT/commands/uninstall.sh" "$@"; }

# ---------------------------------------------------------------------------
# Nothing to uninstall
# ---------------------------------------------------------------------------
section "nothing installed: clean exit"
output="$(run_uninstall --dry-run 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"Nothing to uninstall"* ]]; then pass
else fail "expected 'Nothing to uninstall' (rc=$rc): $output"; fi

# ---------------------------------------------------------------------------
# Dry-run leaves files untouched
# ---------------------------------------------------------------------------
section "dry-run shows changes and does not modify files"
write_settings "$TMP_HOME/.claude/settings.json"
output="$(run_uninstall --dry-run 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"[dry-run]"* && "$output" == *"settings.json"* && "$output" == *"No files were changed"* ]]; then pass
else fail "unexpected dry-run output (rc=$rc): $output"; fi
if config_has_juggernaut_block "$(cat "$TMP_HOME/.claude/settings.json")"; then pass
else fail "dry-run must not modify settings.json"; fi

# ---------------------------------------------------------------------------
# Real uninstall removes block, preserves unrelated keys
# ---------------------------------------------------------------------------
section "removes juggernaut block, preserves unrelated keys"
write_settings "$TMP_HOME/.claude/settings.json"
jq '. + {"permissions": {"allow": ["Bash"]}}' "$TMP_HOME/.claude/settings.json" \
  > "$TMP_HOME/.claude/settings.json.tmp" && mv "$TMP_HOME/.claude/settings.json.tmp" "$TMP_HOME/.claude/settings.json"
run_uninstall --force >/dev/null 2>&1
remaining="$(cat "$TMP_HOME/.claude/settings.json")"
if ! config_has_juggernaut_block "$remaining"; then pass
else fail "juggernaut block still present"; fi
if echo "$remaining" | jq -e '.permissions' >/dev/null 2>&1; then pass
else fail "unrelated key 'permissions' was removed"; fi
if echo "$remaining" | jq -e 'has("env") | not' >/dev/null 2>&1; then pass
else fail "native key .env was not cleaned up"; fi

# ---------------------------------------------------------------------------
# Default scope: remove from both scopes
# ---------------------------------------------------------------------------
section "default scope removes all scopes with a block"
write_settings "$TMP_HOME/.claude/settings.json"
write_settings "$TMP_WORK/.claude/settings.json" eu-west-1
output="$(cd "$TMP_WORK" && run_uninstall --force 2>&1)"; rc=$?
if [[ $rc -eq 0 \
      && "$output" == *"$TMP_HOME/.claude/settings.json"* \
      && "$output" == *"$TMP_WORK/.claude/settings.json"* ]]; then pass
else fail "expected both scopes removed (rc=$rc): $output"; fi
if config_has_juggernaut_block "$(cat "$TMP_HOME/.claude/settings.json")"; then
  fail "user block still present after default-scope uninstall"
else pass; fi
if config_has_juggernaut_block "$(cat "$TMP_WORK/.claude/settings.json")"; then
  fail "project block still present after default-scope uninstall"
else pass; fi

# ---------------------------------------------------------------------------
# --scope=user removes only user scope
# ---------------------------------------------------------------------------
section "--scope=user removes only user scope"
write_settings "$TMP_HOME/.claude/settings.json"
write_settings "$TMP_WORK/.claude/settings.json" eu-west-1
(cd "$TMP_WORK" && run_uninstall --scope=user --force >/dev/null 2>&1)
if config_has_juggernaut_block "$(cat "$TMP_HOME/.claude/settings.json")"; then
  fail "user block should have been removed"
else pass; fi
if config_has_juggernaut_block "$(cat "$TMP_WORK/.claude/settings.json")"; then pass
else fail "project block was incorrectly removed"; fi

# Clean state before next test.
rm -f "$TMP_HOME/.claude/settings.json" "$TMP_WORK/.claude/settings.json"

# ---------------------------------------------------------------------------
# Uninstall does NOT touch shell profiles (v3 behavior)
# ---------------------------------------------------------------------------
section "uninstall does not touch shell profiles in v3"
write_settings "$TMP_HOME/.claude/settings.json"
printf '# existing content\n\n# BEGIN: Juggernaut\nexport AWS_REGION="us-west-2"\n# END: Juggernaut\n' > "$TMP_HOME/.bashrc"
run_uninstall --force >/dev/null 2>&1
if grep -q "BEGIN: Juggernaut" "$TMP_HOME/.bashrc"; then pass
else fail "v3 uninstall should NOT strip profile blocks (install.sh --wipe does that)"; fi
rm -f "$TMP_HOME/.bashrc"

# ---------------------------------------------------------------------------
# Idempotent: second call is a no-op
# ---------------------------------------------------------------------------
section "idempotent: second call is a no-op"
output="$(run_uninstall --force 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"Nothing to uninstall"* ]]; then pass
else fail "second uninstall not idempotent (rc=$rc): $output"; fi

# ---------------------------------------------------------------------------
# Profile token file is removed
# ---------------------------------------------------------------------------
section "removes profile token file"
mkdir -p "$TMP_HOME/.config/juggernaut"
printf 'br-profile-token' > "$TMP_HOME/.config/juggernaut/bearer-token"
output="$(run_uninstall --force 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"Removed profile token file"* && ! -f "$TMP_HOME/.config/juggernaut/bearer-token" ]]; then pass
else fail "profile token file should be removed (rc=$rc): $output"; fi

# ---------------------------------------------------------------------------
# Invalid scope rejected
# ---------------------------------------------------------------------------
section "rejects invalid scope"
output="$(run_uninstall --scope=invalid 2>&1)"; rc=$?
if [[ $rc -ne 0 && "$output" == *"scope"* ]]; then pass
else fail "expected error for invalid scope (rc=$rc): $output"; fi

# ---------------------------------------------------------------------------
# Help text
# ---------------------------------------------------------------------------
section "uninstall --help exits 0 and omits legacy flags"
help_out="$(run_uninstall --help 2>&1)"; rc=$?
if [[ $rc -eq 0 ]]; then pass; else fail "uninstall --help should exit 0 (got $rc)"; fi
if [[ "$help_out" == *"--scope"* && "$help_out" == *"--dry-run"* ]]; then pass
else fail "uninstall --help should mention --scope and --dry-run"; fi
if [[ "$help_out" != *"--legacy-v1"* ]]; then pass
else fail "uninstall --help should NOT mention --legacy-v1"; fi

echo
echo "uninstall.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
