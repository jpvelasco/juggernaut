#!/usr/bin/env bash
# tests/v2/test_uninstall.sh — integration tests for commands/uninstall.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

# ── v2 gate ──────────────────────────────────────────────────────────────────
section "v2 gate"
output="$(bash "$REPO_ROOT/commands/uninstall.sh" 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"Juggernaut v2 is not active"* ]]; then pass
else fail "expected inactive msg (rc=$rc): $output"; fi

# ── Setup fixtures ────────────────────────────────────────────────────────────
TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
mkdir -p "$TMP_HOME/.claude" "$TMP_WORK/.claude"

export HOME="$TMP_HOME"
export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"
export JUGGERNAUT_USE_V2=1
export SHELL="/bin/bash"
unset AWS_BEARER_TOKEN_BEDROCK 2>/dev/null || true

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
. "$REPO_ROOT/lib/keychain.sh"
set +e

# Stash and clear any real keychain entry so tests run in isolation.
_saved_keychain_key="$(keychain_get 2>/dev/null || true)"
if [[ -n "$_saved_keychain_key" ]]; then keychain_delete 2>/dev/null || true; fi
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"; if [[ -n "$_saved_keychain_key" ]]; then keychain_store "$_saved_keychain_key" 2>/dev/null || true; fi' EXIT

write_settings() {
  local path="$1" region="${2:-us-west-2}"
  local block
  block="$(J_AUTH_MODE=iam J_REGION="$region" J_EFFORT=xhigh J_STORAGE=profile \
    J_USE_MANTLE=false J_OPUSPLAN=false J_SCOPE=user J_VERSION=2.1.4 \
    J_SHELL_FALLBACK_MODE=settings-only schema_new_juggernaut_block)"
  config_write_atomic "$path" "$(config_merge_juggernaut_block '{}' "$block" "$(schema_derive_native_keys "$block")")"
}

run_uninstall() { bash "$REPO_ROOT/commands/uninstall.sh" "$@"; }

# ── Nothing to uninstall ──────────────────────────────────────────────────────
section "nothing installed: clean exit"
output="$(run_uninstall --dry-run 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"Nothing to uninstall"* ]]; then pass
else fail "expected 'Nothing to uninstall' (rc=$rc): $output"; fi

# ── Dry-run: user scope ───────────────────────────────────────────────────────
section "dry-run shows what would change and leaves files untouched"
write_settings "$TMP_HOME/.claude/settings.json"
output="$(run_uninstall --dry-run 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"[dry-run]"* && "$output" == *"settings.json"* && "$output" == *"No files were changed"* ]]; then pass
else fail "unexpected dry-run output (rc=$rc): $output"; fi
if config_has_juggernaut_block "$(cat "$TMP_HOME/.claude/settings.json")"; then pass
else fail "dry-run must not modify settings.json"; fi

# ── Real uninstall: removes block, preserves unrelated keys ──────────────────
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
if echo "$remaining" | jq -e '.env // empty' | grep -qv null 2>/dev/null; then
  fail "native key .env was not cleaned up"
else pass; fi

# ── Default scope: both scopes ────────────────────────────────────────────────
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

# ── Explicit scope: only requested scope removed ──────────────────────────────
section "--scope=user removes only user scope"
write_settings "$TMP_HOME/.claude/settings.json"
write_settings "$TMP_WORK/.claude/settings.json" eu-west-1
(cd "$TMP_WORK" && run_uninstall --scope=user --force >/dev/null 2>&1)
if config_has_juggernaut_block "$(cat "$TMP_HOME/.claude/settings.json")"; then
  fail "user block should have been removed"
else pass; fi
if config_has_juggernaut_block "$(cat "$TMP_WORK/.claude/settings.json")"; then pass
else fail "project block was incorrectly removed"; fi

# ── Profile block removal ─────────────────────────────────────────────────────
section "removes profile block, leaves non-block content"
write_settings "$TMP_HOME/.claude/settings.json"
printf '# existing content\n\n%s\nexport AWS_REGION="us-west-2"\n%s\n' \
  "# BEGIN: Claude Code Bedrock Configuration" \
  "# END: Claude Code Bedrock Configuration" > "$TMP_HOME/.bashrc"
output="$(run_uninstall --force 2>&1)"; rc=$?
if [[ $rc -eq 0 ]] && ! grep -q "BEGIN: Claude Code Bedrock Configuration" "$TMP_HOME/.bashrc"; then pass
else fail "profile block still present (rc=$rc): $output"; fi
if grep -q "existing content" "$TMP_HOME/.bashrc"; then pass
else fail "non-block content was removed from profile"; fi

# ── Idempotent ────────────────────────────────────────────────────────────────
section "idempotent: second call is a no-op"
output="$(run_uninstall --force 2>&1)"; rc=$?
if [[ $rc -eq 0 && "$output" == *"Nothing to uninstall"* ]]; then pass
else fail "second uninstall not idempotent (rc=$rc): $output"; fi

# ── Invalid scope rejected ────────────────────────────────────────────────────
section "rejects invalid scope"
output="$(run_uninstall --scope=invalid 2>&1)"; rc=$?
if [[ $rc -ne 0 && "$output" == *"scope"* ]]; then pass
else fail "expected error for invalid scope (rc=$rc): $output"; fi

echo
echo "uninstall.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
