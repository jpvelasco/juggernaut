#!/usr/bin/env bash
# tests/v2/test_config_manager.sh — unit tests for lib/config_manager.sh
# Exercises read/merge/write round-trips with tmp paths.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=../../lib/schema.sh
source "$REPO_ROOT/lib/schema.sh"
# shellcheck source=../../lib/config_manager.sh
source "$REPO_ROOT/lib/config_manager.sh"

export BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

TMPDIR_LOCAL="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_LOCAL"' EXIT

TARGET="$TMPDIR_LOCAL/settings.json"

# --- Round-trip on empty target ---
section "Write-read round trip on fresh file"
block="$(schema_new_juggernaut_block)"
native="$(schema_derive_native_keys "$block")"

existing="$(config_read "$TARGET")"
merged="$(config_merge_juggernaut_block "$existing" "$block" "$native")"
config_write_atomic "$TARGET" "$merged"

read_back="$(config_read "$TARGET")"
[[ "$(echo "$read_back" | jq -r '.juggernaut.meta.managedBy')" == "juggernaut" ]] && pass || fail "round-trip should preserve managedBy"
[[ "$(echo "$read_back" | jq -r '.env.CLAUDE_CODE_USE_BEDROCK')" == "1" ]] && pass || fail "round-trip should preserve native env"

config_has_juggernaut_block "$read_back" && pass || fail "has_juggernaut_block should return 0"

# --- Merge preserves user keys ---
section "Merge preserves unrelated user keys"
user_content='{"theme":"dark","permissions":{"allow":["npm"]}}'
echo "$user_content" >"$TARGET"
existing="$(config_read "$TARGET")"
merged="$(config_merge_juggernaut_block "$existing" "$block" "$native")"
config_write_atomic "$TARGET" "$merged"

read_back="$(config_read "$TARGET")"
[[ "$(echo "$read_back" | jq -r '.theme')" == "dark" ]] && pass || fail "theme key should be preserved"
[[ "$(echo "$read_back" | jq -r '.permissions.allow[0]')" == "npm" ]] && pass || fail "permissions should be preserved"
[[ "$(echo "$read_back" | jq -r '.juggernaut.meta.managedBy')" == "juggernaut" ]] && pass || fail "juggernaut block should be present"

# --- Backup created and rotated ---
section "Backup created and rotated at 5"
for i in 1 2 3 4 5 6 7; do
  # Tweak timestamp to force unique backups
  sleep 1
  config_write_atomic "$TARGET" "$merged"
done
backup_count="$(find "$TMPDIR_LOCAL" -maxdepth 1 -name 'settings.json.backup.*' | wc -l | tr -d ' ')"
[[ "$backup_count" -le 5 ]] && pass || fail "expected ≤5 backups, got $backup_count"

# --- Remove juggernaut block leaves user keys alone ---
section "Remove leaves unrelated user keys"
read_back="$(config_read "$TARGET")"
stripped="$(config_remove_juggernaut_block "$read_back")"
[[ "$(echo "$stripped" | jq 'has("juggernaut")')" == "false" ]] && pass || fail "juggernaut key should be removed"
[[ "$(echo "$stripped" | jq 'has("env")')" == "false" ]] && pass || fail "env key should be removed"
[[ "$(echo "$stripped" | jq -r '.theme')" == "dark" ]] && pass || fail "theme should still be present after remove"

# --- Invalid JSON rejected ---
section "Refuse to write invalid JSON"
set +e
config_write_atomic "$TARGET" "not json at all" 2>/dev/null
rc=$?
set -e
[[ "$rc" -ne 0 ]] && pass || fail "invalid JSON write should fail"

# --- config_exists ---
section "config_exists semantics"
config_exists "$TARGET" && pass || fail "real file should be considered existing"
config_exists "/nonexistent/path" && fail "nonexistent path should not be existing" || pass

echo
echo "config_manager.sh tests: $PASS passed, $FAIL failed"
exit $FAIL
