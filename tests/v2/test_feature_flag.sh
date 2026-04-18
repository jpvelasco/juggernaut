#!/usr/bin/env bash
# tests/v2/test_feature_flag.sh — the --v2 / JUGGERNAUT_USE_V2 flag is dormant plumbing.
# Confirms it is accepted, announced, does not alter v1 dry-run output, and does not leak
# into paths that shouldn't see it.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

section "--v2 flag is accepted by ./setup without altering exit code"
# Use help to exercise the arg parser without running a real setup.
if bash "$REPO_ROOT/setup" --v2 --help >/dev/null 2>&1; then pass; else fail "./setup --v2 --help should exit 0"; fi

section "--v2 announces itself to stderr"
announce="$(bash "$REPO_ROOT/setup" --v2 --help 2>&1 1>/dev/null | head -1)"
if [[ "$announce" == *"Juggernaut v2.0 enabled"* && "$announce" == *"currently dormant"* ]]; then
  pass
else
  fail "expected v2 announce on stderr (got: '$announce')"
fi

section "JUGGERNAUT_USE_V2=1 env var also triggers announce"
announce_env="$(JUGGERNAUT_USE_V2=1 bash "$REPO_ROOT/setup" --help 2>&1 1>/dev/null | head -1)"
if [[ "$announce_env" == *"Juggernaut v2.0 enabled"* ]]; then pass; else fail "env var should trigger announce (got: '$announce_env')"; fi

section "Default run (no flag) does NOT announce v2"
clean_stderr="$(bash "$REPO_ROOT/setup" --help 2>&1 1>/dev/null)"
if [[ "$clean_stderr" != *"Juggernaut v2"* ]]; then pass; else fail "v1 default run should be silent about v2"; fi

section "--v2 is stripped before delegation (help output byte-identical)"
with_v2="$(bash "$REPO_ROOT/setup" --v2 --help 2>/dev/null)"
without_v2="$(bash "$REPO_ROOT/setup" --help 2>/dev/null)"
if [[ "$with_v2" == "$without_v2" ]]; then pass; else fail "--v2 should not change help output"; fi

section "--v2 does NOT alter v1 dry-run stdout (behavioral isolation)"
# Run v1 apply in dry-run twice, once with --v2, once without. The user-visible
# stdout must be byte-identical — v2 plumbing lives in env/stderr only.
dry_no="$(bash "$REPO_ROOT/setup-claude-bedrock.sh" bash --dry-run --force --auth=iam --region=us-west-2 2>/dev/null)"
dry_v2="$(bash "$REPO_ROOT/setup-claude-bedrock.sh" bash --v2 --dry-run --force --auth=iam --region=us-west-2 2>/dev/null)"
if [[ "$dry_no" == "$dry_v2" ]]; then
  pass
else
  fail "--v2 altered v1 dry-run stdout (diff below)"
  diff <(printf '%s\n' "$dry_no") <(printf '%s\n' "$dry_v2") | head -20 >&2
fi

section "JUGGERNAUT_USE_V2=1 env var also preserves v1 dry-run stdout"
dry_env="$(JUGGERNAUT_USE_V2=1 bash "$REPO_ROOT/setup-claude-bedrock.sh" bash --dry-run --force --auth=iam --region=us-west-2 2>/dev/null)"
if [[ "$dry_no" == "$dry_env" ]]; then
  pass
else
  fail "JUGGERNAUT_USE_V2=1 altered v1 dry-run stdout"
fi

section "Flag is accepted by setup-claude-bedrock.sh arg parser"
# --dry-run + --help-ish path: use --version, which exits 0 from the inner parser too.
if bash "$REPO_ROOT/setup-claude-bedrock.sh" bash --v2 --version >/dev/null 2>&1; then pass; else fail "setup-claude-bedrock.sh should accept --v2"; fi

echo
echo "feature-flag tests: $PASS passed, $FAIL failed"
exit "$FAIL"
