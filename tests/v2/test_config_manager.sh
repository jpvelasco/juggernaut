#!/usr/bin/env bash
# tests/v2/test_config_manager.sh — unit tests for lib/config_manager.sh

set -uo pipefail

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
assert_eq() { if [[ "$1" == "$2" ]]; then pass; else fail "$3 (expected '$2', got '$1')"; fi; }
assert_cmd() { if "$@" >/dev/null 2>&1; then pass; else fail "command failed: $*"; fi; }
assert_not_cmd() { if ! "$@" >/dev/null 2>&1; then pass; else fail "command unexpectedly succeeded: $*"; fi; }
section() { echo; echo "== $1 =="; }

TMPDIR_LOCAL="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_LOCAL"' EXIT

TARGET="$TMPDIR_LOCAL/settings.json"

section "Write-read round trip on fresh file"
export J_AUTH_VALIDATED=true
block="$(schema_new_juggernaut_block)"
unset J_AUTH_VALIDATED
native="$(schema_derive_native_keys "$block")"
existing="$(config_read "$TARGET")"
merged="$(config_merge_juggernaut_block "$existing" "$block" "$native")"
config_write_atomic "$TARGET" "$merged"
read_back="$(config_read "$TARGET")"
assert_eq "$(echo "$read_back" | jq -r '.juggernaut.meta.managedBy')" "juggernaut" "round-trip should preserve managedBy"
assert_eq "$(echo "$read_back" | jq -r '.env.CLAUDE_CODE_USE_BEDROCK')" "1" "round-trip should preserve native env"
assert_cmd config_has_juggernaut_block "$read_back"

section "Merge preserves unrelated user keys"
user_content='{"theme":"dark","permissions":{"allow":["npm"]}}'
echo "$user_content" >"$TARGET"
existing="$(config_read "$TARGET")"
merged="$(config_merge_juggernaut_block "$existing" "$block" "$native")"
config_write_atomic "$TARGET" "$merged"
read_back="$(config_read "$TARGET")"
assert_eq "$(echo "$read_back" | jq -r '.theme')"                     "dark"       "theme key should be preserved"
assert_eq "$(echo "$read_back" | jq -r '.permissions.allow[0]')"      "npm"        "permissions should be preserved"
assert_eq "$(echo "$read_back" | jq -r '.juggernaut.meta.managedBy')" "juggernaut" "juggernaut block should be present"

section "Backup created and rotated at 5"
for _ in 1 2 3 4 5 6 7; do
  sleep 1
  config_write_atomic "$TARGET" "$merged"
done
backup_count="$(find "$TMPDIR_LOCAL" -maxdepth 1 -name 'settings.json.backup.*' | wc -l | tr -d ' ')"
if [[ "$backup_count" -le 5 ]]; then pass; else fail "expected ≤5 backups, got $backup_count"; fi

section "Remove leaves unrelated user keys"
read_back="$(config_read "$TARGET")"
stripped="$(config_remove_juggernaut_block "$read_back")"
assert_eq "$(echo "$stripped" | jq 'has("juggernaut")')" "false" "juggernaut key should be removed"
assert_eq "$(echo "$stripped" | jq 'has("env")')"        "false" "env key should be removed"
assert_eq "$(echo "$stripped" | jq -r '.theme')"         "dark"  "theme should still be present after remove"

section "Refuse to write invalid JSON"
assert_not_cmd config_write_atomic "$TARGET" "not json at all"

section "config_exists semantics"
assert_cmd config_exists "$TARGET"
assert_not_cmd config_exists "/nonexistent/path"

section "Locking: serializes two writers (smoke)"
LOCK_TARGET="$TMPDIR_LOCAL/locked.json"
echo '{}' > "$LOCK_TARGET"
slow_writer() {
  config_with_lock "$LOCK_TARGET" bash -c '
    sleep 0.3
    echo "{\"who\":\"'"$1"'\"}" > "'"$LOCK_TARGET"'"
  '
}
slow_writer first &
P1=$!
slow_writer second &
P2=$!
wait $P1 $P2
# Just verify lock didn't corrupt the file.
if jq -e . "$LOCK_TARGET" >/dev/null 2>&1; then pass; else fail "file corrupted under concurrent writers"; fi

echo
echo "config_manager.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
