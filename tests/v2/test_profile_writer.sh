#!/usr/bin/env bash
# tests/v2/test_profile_writer.sh — integration tests for lib/profile_writer.sh
# Focus: CRLF robustness on Windows Git Bash + basic LF parity.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURES="$REPO_ROOT/tests/v2/fixtures"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

. "$REPO_ROOT/lib/profile_writer.sh"
set +e

section "profile_writer_has_block — LF fixture"
TMP="$(mktemp)"
cp "$FIXTURES/profile_crlf_unix.sh" "$TMP"
if profile_writer_has_block "$TMP"; then pass; else fail "LF fixture: block not detected"; fi
rm -f "$TMP"

section "profile_writer_has_block — CRLF fixture"
TMP="$(mktemp)"
cp "$FIXTURES/profile_crlf.sh" "$TMP"
# Confirm fixture actually has CR bytes (guards against autocrlf normalization).
# Use tr (not grep) because Git Bash grep treats input as text and can match
# \r inconsistently on Windows.
CR_COUNT="$(tr -cd '\r' < "$TMP" | wc -c)"
if [[ "$CR_COUNT" -gt 0 ]]; then
  pass
else
  fail "CRLF fixture lost its CR bytes on checkout — check .gitattributes"
fi
if profile_writer_has_block "$TMP"; then
  pass
else
  fail "CRLF fixture: block not detected"
fi
rm -f "$TMP"

section "profile_writer_remove_block — CRLF fixture strips markers + body"
TMP="$(mktemp)"
cp "$FIXTURES/profile_crlf.sh" "$TMP"
if profile_writer_remove_block "$TMP"; then pass; else fail "remove_block returned non-zero"; fi
if grep -q "BEGIN: Claude Code Bedrock Configuration" "$TMP"; then
  fail "remove_block left BEGIN marker behind"
else
  pass
fi
if grep -q "END: Claude Code Bedrock Configuration" "$TMP"; then
  fail "remove_block left END marker behind"
else
  pass
fi
if grep -q "Pre-existing user content" "$TMP"; then
  pass
else
  fail "remove_block destroyed pre-existing user content"
fi
if grep -q "Trailing user content" "$TMP"; then
  pass
else
  fail "remove_block destroyed trailing user content"
fi
rm -f "$TMP"

section "profile_writer_remove_block — LF fixture preserves surrounding content"
TMP="$(mktemp)"
cp "$FIXTURES/profile_crlf_unix.sh" "$TMP"
profile_writer_remove_block "$TMP" || fail "LF remove_block returned non-zero"
if grep -q "BEGIN: Claude Code Bedrock Configuration" "$TMP"; then
  fail "remove_block left BEGIN marker behind (LF)"
else
  pass
fi
if grep -q "Pre-existing user content" "$TMP" && grep -q "Trailing user content" "$TMP"; then
  pass
else
  fail "remove_block destroyed user content (LF)"
fi
rm -f "$TMP"

echo
echo "Results: $PASS passed, $FAIL failed"
[[ "$FAIL" -eq 0 ]] || exit 1
