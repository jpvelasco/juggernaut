#!/usr/bin/env bash
# tests/v2/test_launcher.sh — verify bin/claude behavior.
# Covers env preservation, keychain injection, fall-through on miss/error,
# missing-upstream error path, recursion guard, and argv passthrough.

set -uo pipefail
set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LAUNCHER="$REPO_ROOT/bin/claude"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

make_stub_claude() {
  local dir="$1" name="${2:-claude.exe}"
  mkdir -p "$dir"
  cat > "$dir/$name" << 'STUB'
#!/usr/bin/env bash
printf 'STUB_BEARER_LEN=%d\n' "${#AWS_BEARER_TOKEN_BEDROCK}"
printf 'STUB_BEARER=%s\n' "${AWS_BEARER_TOKEN_BEDROCK:-}"
printf 'STUB_ARGS=%s\n' "$*"
exit 42
STUB
  chmod +x "$dir/$name"
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ---------------------------------------------------------------------------
section "launcher exists and is executable"
if [[ -x "$LAUNCHER" ]]; then
  pass
else
  fail "bin/claude not executable at $LAUNCHER"
fi

# ---------------------------------------------------------------------------
section "env preset — pass through unchanged"
STUB_DIR="$TMP/stub1"
make_stub_claude "$STUB_DIR"
OUT="$(AWS_BEARER_TOKEN_BEDROCK="pre-existing-token" \
       JUGGERNAUT_CLAUDE_BIN="$STUB_DIR/claude.exe" \
       bash "$LAUNCHER" --foo bar 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER=pre-existing-token"* ]] && [[ "$OUT" == *"STUB_ARGS=--foo bar"* ]]; then
  pass
else
  fail "env-preset: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain hit — token injected"
# Stub out keychain_get inside an overridden keychain.sh via LD_PRELOAD-style
# trick? Simpler: set JUGGERNAUT_KEYCHAIN_SERVICE to a name we then create
# via the real keychain. But CI runners may lack keychain tooling, so we use
# a different approach: shadow lib/keychain.sh by pointing $INSTALL_DIR at a
# temp tree that contains only a stub keychain.sh.
STUB_INSTALL="$TMP/stub-install"
mkdir -p "$STUB_INSTALL/lib" "$STUB_INSTALL/bin"
cat > "$STUB_INSTALL/lib/keychain.sh" << 'STUBKC'
#!/usr/bin/env bash
keychain_get() { printf 'token-from-stub-keychain'; return 0; }
STUBKC
cp "$LAUNCHER" "$STUB_INSTALL/bin/claude"
chmod +x "$STUB_INSTALL/bin/claude"

STUB_DIR2="$TMP/stub2"
make_stub_claude "$STUB_DIR2"
unset AWS_BEARER_TOKEN_BEDROCK
OUT="$(JUGGERNAUT_CLAUDE_BIN="$STUB_DIR2/claude.exe" \
       bash "$STUB_INSTALL/bin/claude" arg1 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER=token-from-stub-keychain"* ]]; then
  pass
else
  fail "keychain-hit: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain miss (rc=1) — launched with env unset"
cat > "$STUB_INSTALL/lib/keychain.sh" << 'STUBKC'
#!/usr/bin/env bash
keychain_get() { return 1; }
STUBKC
unset AWS_BEARER_TOKEN_BEDROCK
OUT="$(JUGGERNAUT_CLAUDE_BIN="$STUB_DIR2/claude.exe" \
       bash "$STUB_INSTALL/bin/claude" 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER_LEN=0"* ]]; then
  pass
else
  fail "keychain-miss: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain error (rc=2) — fall through, still launch"
cat > "$STUB_INSTALL/lib/keychain.sh" << 'STUBKC'
#!/usr/bin/env bash
keychain_get() { echo "simulated tool error" >&2; return 2; }
STUBKC
unset AWS_BEARER_TOKEN_BEDROCK
OUT="$(JUGGERNAUT_CLAUDE_BIN="$STUB_DIR2/claude.exe" \
       bash "$STUB_INSTALL/bin/claude" 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER_LEN=0"* ]]; then
  pass
else
  fail "keychain-error: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "no upstream binary — exit 127"
EMPTY_DIR="$TMP/empty_path"
mkdir -p "$EMPTY_DIR"
# Put a harmless non-claude executable so PATH isn't completely empty.
cat > "$EMPTY_DIR/not-claude" << 'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$EMPTY_DIR/not-claude"
unset AWS_BEARER_TOKEN_BEDROCK
unset JUGGERNAUT_CLAUDE_BIN
# Use /usr/bin on PATH so bash itself can still find dirname/cd/basename.
OUT="$(PATH="$EMPTY_DIR:/usr/bin" bash "$LAUNCHER" 2>&1)"
RC=$?
if [[ "$RC" == "127" ]] && [[ "$OUT" == *"no upstream binary"* ]]; then
  pass
else
  fail "no-upstream: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "argv passthrough — exact match"
STUB_DIR3="$TMP/stub3"
make_stub_claude "$STUB_DIR3"
unset AWS_BEARER_TOKEN_BEDROCK
OUT="$(AWS_BEARER_TOKEN_BEDROCK="x" \
       JUGGERNAUT_CLAUDE_BIN="$STUB_DIR3/claude.exe" \
       bash "$LAUNCHER" --one two 'three four' --five=six 2>&1)"
if [[ "$OUT" == *"STUB_ARGS=--one two three four --five=six"* ]]; then
  pass
else
  fail "argv-passthrough: output=$OUT"
fi

# ---------------------------------------------------------------------------
section "recursion guard — does not invoke itself"
# Create a PATH where the launcher symlink appears first, plus a real stub
# later. The launcher should skip the self-symlink and pick up the stub.
RECUR_DIR="$TMP/recur"
mkdir -p "$RECUR_DIR"
ln -sfn "$LAUNCHER" "$RECUR_DIR/claude"
STUB_DIR4="$TMP/stub4"
make_stub_claude "$STUB_DIR4"
unset AWS_BEARER_TOKEN_BEDROCK
unset JUGGERNAUT_CLAUDE_BIN
OUT="$(AWS_BEARER_TOKEN_BEDROCK="x" \
       PATH="$RECUR_DIR:$STUB_DIR4:/usr/bin" \
       bash "$LAUNCHER" --version 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_ARGS=--version"* ]]; then
  pass
else
  fail "recursion-guard: rc=$RC output=$OUT"
fi

echo
echo "launcher.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
