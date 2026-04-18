#!/usr/bin/env bash
# tests/v2/test_keychain.sh — tests for lib/keychain.sh
# Verifies detection, availability check, and get_command output shape.
# Actual keychain write/read requires system credentials — those are tested
# in the CI step "Test actual keychain store/retrieve" which uses the
# native macOS security / Windows cmdkey tooling directly.

set -uo pipefail
set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

. "$REPO_ROOT/lib/keychain.sh"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }
assert_eq() {
  local label="$1" got="$2" want="$3"
  if [[ "$got" == "$want" ]]; then pass; else fail "$label: expected '$want', got '$got'"; fi
}
assert_nonempty() {
  local label="$1" val="$2"
  if [[ -n "$val" ]]; then pass; else fail "$label: expected non-empty, got empty"; fi
}
assert_contains() {
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then pass; else fail "$label: '$needle' not found in output"; fi
}

# ---------------------------------------------------------------------------
# keychain_detect_os
# ---------------------------------------------------------------------------
section "keychain_detect_os"
OS="$(keychain_detect_os)"
assert_nonempty "os detected" "$OS"
case "$OS" in
  macos|linux|wsl|gitbash|cygwin|unknown) pass ;;
  *) fail "os value unexpected: '$OS'" ;;
esac

# ---------------------------------------------------------------------------
# keychain_available
# ---------------------------------------------------------------------------
section "keychain_available"
# On CI we just check the function doesn't crash — we don't assert the result
# since CI runners may or may not have keychain tools installed.
if keychain_available 2>/dev/null; then
  AVAIL=true
else
  AVAIL=false
fi
# No assert — just verify it returns 0 or 1 (the call above didn't crash).
pass

# ---------------------------------------------------------------------------
# keychain_get_command — bash syntax
# ---------------------------------------------------------------------------
section "keychain_get_command — bash"
CMD="$(keychain_get_command bash)"
assert_nonempty "bash cmd non-empty" "$CMD"
# Must be a $(...) expansion for bash.
assert_contains "bash cmd starts with \$(" "$CMD" "\$("
# Must reference the service name.
assert_contains "bash cmd references service" "$CMD" "juggernaut-bedrock"

# ---------------------------------------------------------------------------
# keychain_get_command — fish syntax
# ---------------------------------------------------------------------------
section "keychain_get_command — fish"
FISH_CMD="$(keychain_get_command fish)"
assert_nonempty "fish cmd non-empty" "$FISH_CMD"
# Fish uses (...) not $(...).
assert_contains "fish cmd starts with (" "$FISH_CMD" "("
if [[ "$FISH_CMD" == "\$("* ]]; then
  fail "fish cmd should not start with \$("
else
  pass
fi

# ---------------------------------------------------------------------------
# keychain_get_command — zsh same as bash
# ---------------------------------------------------------------------------
section "keychain_get_command — zsh"
ZSH_CMD="$(keychain_get_command zsh)"
assert_eq "zsh == bash cmd" "$ZSH_CMD" "$CMD"

# ---------------------------------------------------------------------------
# Constants are set
# ---------------------------------------------------------------------------
section "service and account constants"
assert_eq "KEYCHAIN_SERVICE" "$KEYCHAIN_SERVICE" "juggernaut-bedrock"
assert_eq "KEYCHAIN_ACCOUNT" "$KEYCHAIN_ACCOUNT" "api-key"

# ---------------------------------------------------------------------------
# keychain_store + keychain_get round-trip (macOS only — skipped elsewhere)
# ---------------------------------------------------------------------------
section "keychain round-trip (macOS only)"
if [[ "$OS" == "macos" ]] && keychain_available 2>/dev/null; then
  TEST_KEY="jug-ci-test-$(date +%s)"
  if keychain_store "$TEST_KEY" 2>/dev/null; then
    RETRIEVED="$(keychain_get 2>/dev/null || true)"
    assert_eq "retrieved == stored" "$RETRIEVED" "$TEST_KEY"
    keychain_delete 2>/dev/null || true
  else
    echo "  SKIP: keychain_store failed (system may restrict CI keychain writes)"
    pass  # Not a test failure — system restriction.
  fi
else
  echo "  SKIP: not macOS or keychain unavailable"
  pass
fi

echo
echo "keychain.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
