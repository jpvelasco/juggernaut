#!/usr/bin/env bash
# tests/v2/test_launcher.sh - verify the Juggernaut claude launcher function.
# The v3.0.1 launcher is a bracketed shell function written to the user's
# shell profile (not a standalone script). These tests exercise the function
# itself plus the install.sh profile-writing helper.

set -uo pipefail
set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
INSTALL_SH="$REPO_ROOT/install.sh"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# make_stub_install DIR KEYCHAIN_BEHAVIOR
# Populates DIR with a minimal INSTALL_DIR layout: a lib/keychain.sh that
# defines keychain_get according to KEYCHAIN_BEHAVIOR:
#   hit      -> prints 'token-from-stub-keychain', returns 0
#   miss     -> returns 1
#   error    -> writes to stderr, returns 2
#   absent   -> no keychain.sh file present
make_stub_install() {
  local dir="$1" behavior="$2"
  mkdir -p "$dir/lib"
  case "$behavior" in
    absent) return 0 ;;
    hit)
      cat > "$dir/lib/keychain.sh" <<'KC'
#!/usr/bin/env bash
keychain_get() { printf 'token-from-stub-keychain'; return 0; }
KC
      ;;
    miss)
      cat > "$dir/lib/keychain.sh" <<'KC'
#!/usr/bin/env bash
keychain_get() { return 1; }
KC
      ;;
    error)
      cat > "$dir/lib/keychain.sh" <<'KC'
#!/usr/bin/env bash
keychain_get() { echo "simulated tool error" >&2; return 2; }
KC
      ;;
  esac
}

# make_stub_claude DIR [NAME]
# Writes a stub claude binary that echoes env + argv then exits 42.
make_stub_claude() {
  local dir="$1" name="${2:-claude}"
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

# write_launcher_block PROFILE INSTALL_DIR
# Writes the same launcher block that install.sh would write, but against
# a custom INSTALL_DIR (so the function sources our stubbed keychain).
write_launcher_block() {
  local profile="$1" install_dir="$2"
  cat >> "$profile" <<LAUNCHER

# BEGIN: Juggernaut Launcher
# Juggernaut claude launcher - injects AWS_BEARER_TOKEN_BEDROCK from the OS
# keychain before exec'ing the real claude binary. Silent on success.
claude() {
  if [ -z "\${AWS_BEARER_TOKEN_BEDROCK:-}" ]; then
    if [ -r "$install_dir/lib/keychain.sh" ]; then
      # shellcheck disable=SC1091
      _juggernaut_token=\$(. "$install_dir/lib/keychain.sh"; keychain_get 2>/dev/null) || _juggernaut_token=''
      if [ -n "\$_juggernaut_token" ]; then
        export AWS_BEARER_TOKEN_BEDROCK="\$_juggernaut_token"
      fi
      unset _juggernaut_token
    fi
  fi
  command claude "\$@"
}
# END: Juggernaut Launcher
LAUNCHER
}

# run_claude PROFILE STUB_DIR [ENV_VARS...] -- args...
# Sources PROFILE in a fresh bash, puts STUB_DIR at the head of PATH so
# `command claude` resolves to the stub, and calls claude with the given
# args. Extra env vars can be passed via FOO=bar pairs before the --.
run_claude() {
  local profile="$1" stub_dir="$2"; shift 2
  local env_pairs=()
  while [[ $# -gt 0 && "$1" != "--" ]]; do
    env_pairs+=("$1")
    shift
  done
  [[ "${1:-}" == "--" ]] && shift
  env -i HOME="$HOME" PATH="$stub_dir:/usr/bin:/bin" "${env_pairs[@]}" \
    bash --noprofile --norc -c 'source "$1"; shift; claude "$@"' -- "$profile" "$@"
}

# ---------------------------------------------------------------------------
section "install.sh exists"
if [[ -f "$INSTALL_SH" ]]; then pass
else fail "install.sh not found at $INSTALL_SH"; fi

# ---------------------------------------------------------------------------
section "env preset - pass through unchanged"
PROFILE="$TMP/profile1"
: > "$PROFILE"
INSTALL1="$TMP/install1"; make_stub_install "$INSTALL1" hit
STUB1="$TMP/stub1"; make_stub_claude "$STUB1"
write_launcher_block "$PROFILE" "$INSTALL1"

OUT="$(run_claude "$PROFILE" "$STUB1" AWS_BEARER_TOKEN_BEDROCK="pre-existing-token" -- --foo bar 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER=pre-existing-token"* ]] && [[ "$OUT" == *"STUB_ARGS=--foo bar"* ]]; then
  pass
else
  fail "env-preset: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain hit - token injected"
PROFILE="$TMP/profile2"
: > "$PROFILE"
INSTALL2="$TMP/install2"; make_stub_install "$INSTALL2" hit
STUB2="$TMP/stub2"; make_stub_claude "$STUB2"
write_launcher_block "$PROFILE" "$INSTALL2"

OUT="$(run_claude "$PROFILE" "$STUB2" -- arg1 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER=token-from-stub-keychain"* ]]; then
  pass
else
  fail "keychain-hit: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain miss (rc=1) - launched with env unset"
PROFILE="$TMP/profile3"
: > "$PROFILE"
INSTALL3="$TMP/install3"; make_stub_install "$INSTALL3" miss
STUB3="$TMP/stub3"; make_stub_claude "$STUB3"
write_launcher_block "$PROFILE" "$INSTALL3"

OUT="$(run_claude "$PROFILE" "$STUB3" -- 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER_LEN=0"* ]]; then
  pass
else
  fail "keychain-miss: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain error (rc=2) - fall through, still launch"
PROFILE="$TMP/profile4"
: > "$PROFILE"
INSTALL4="$TMP/install4"; make_stub_install "$INSTALL4" error
STUB4="$TMP/stub4"; make_stub_claude "$STUB4"
write_launcher_block "$PROFILE" "$INSTALL4"

OUT="$(run_claude "$PROFILE" "$STUB4" -- 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER_LEN=0"* ]]; then
  pass
else
  fail "keychain-error: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "keychain lib absent - launched with env unset"
PROFILE="$TMP/profile5"
: > "$PROFILE"
INSTALL5="$TMP/install5"; make_stub_install "$INSTALL5" absent
STUB5="$TMP/stub5"; make_stub_claude "$STUB5"
write_launcher_block "$PROFILE" "$INSTALL5"

OUT="$(run_claude "$PROFILE" "$STUB5" -- 2>&1)"
RC=$?
if [[ "$RC" == "42" ]] && [[ "$OUT" == *"STUB_BEARER_LEN=0"* ]]; then
  pass
else
  fail "keychain-absent: rc=$RC output=$OUT"
fi

# ---------------------------------------------------------------------------
section "argv passthrough - exact match"
PROFILE="$TMP/profile6"
: > "$PROFILE"
INSTALL6="$TMP/install6"; make_stub_install "$INSTALL6" hit
STUB6="$TMP/stub6"; make_stub_claude "$STUB6"
write_launcher_block "$PROFILE" "$INSTALL6"

OUT="$(run_claude "$PROFILE" "$STUB6" -- --one two 'three four' --five=six 2>&1)"
if [[ "$OUT" == *"STUB_ARGS=--one two three four --five=six"* ]]; then
  pass
else
  fail "argv-passthrough: output=$OUT"
fi

# ---------------------------------------------------------------------------
section "install.sh writes exactly one launcher block"
# Full installer end-to-end is too heavy for unit tests (clones the repo).
# Instead, extract and evaluate just the install_launcher_profile_block
# function to verify the idempotency contract.
FAKE_HOME="$TMP/home-install"
mkdir -p "$FAKE_HOME"
touch "$FAKE_HOME/.bashrc"

# Extract the function body from install.sh so we test the real writer.
# Bounded by the function declaration and the '# END install_launcher_profile_block'
# sentinel comment — simple closing-brace matching breaks on the nested `}`
# inside the heredoc that defines the claude() function in the block.
INSTALL_FN="$TMP/install_fn.sh"
awk '
  /^install_launcher_profile_block\(\)/ { capture = 1 }
  capture { print }
  capture && /^# END install_launcher_profile_block$/ { exit }
' "$INSTALL_SH" > "$INSTALL_FN"

(
  HOME="$FAKE_HOME"
  INSTALL_DIR="$TMP/install-fake"
  make_stub_install "$INSTALL_DIR" hit
  # shellcheck disable=SC1090
  . "$INSTALL_FN"
  install_launcher_profile_block >/dev/null
  install_launcher_profile_block >/dev/null   # second call must be idempotent
)

begin_count="$(grep -c '^# BEGIN: Juggernaut Launcher' "$FAKE_HOME/.bashrc" 2>/dev/null || echo 0)"
end_count="$(grep -c '^# END: Juggernaut Launcher' "$FAKE_HOME/.bashrc" 2>/dev/null || echo 0)"
if [[ "$begin_count" == "1" && "$end_count" == "1" ]]; then
  pass
else
  fail "idempotency: begin=$begin_count end=$end_count"
fi

# ---------------------------------------------------------------------------
section "fish config gets fish-syntax launcher block"
FISH_HOME="$TMP/home-fish"
mkdir -p "$FISH_HOME/.config/fish"
touch "$FISH_HOME/.config/fish/config.fish"

FISH_INSTALL_DIR="$TMP/install-fish"
make_stub_install "$FISH_INSTALL_DIR" hit

INSTALL_FN_FISH="$TMP/install_fn_fish.sh"
awk '
  /^install_launcher_profile_block\(\)/ { capture = 1 }
  capture { print }
  capture && /^# END install_launcher_profile_block$/ { exit }
' "$INSTALL_SH" > "$INSTALL_FN_FISH"

(
  HOME="$FISH_HOME"
  INSTALL_DIR="$FISH_INSTALL_DIR"
  # shellcheck disable=SC1090
  . "$INSTALL_FN_FISH"
  install_launcher_profile_block >/dev/null
)

FISH_CFG="$FISH_HOME/.config/fish/config.fish"
if grep -q '^# BEGIN: Juggernaut Launcher' "$FISH_CFG"; then
  pass
else
  fail "fish config.fish should contain launcher block"
fi

# Fish syntax: must use `function claude` not `claude()`
if grep -q '^function claude$' "$FISH_CFG"; then
  pass
else
  fail "fish launcher must use 'function claude' syntax (not bash 'claude()')"
fi

# Fish syntax: must use `command claude \$argv` not `command claude "\$@"`
if grep -q 'command claude \$argv' "$FISH_CFG"; then
  pass
else
  fail "fish launcher must use 'command claude \$argv' (not bash '\$@')"
fi

# Fish syntax: must not contain bash function syntax
if ! grep -q 'claude()' "$FISH_CFG"; then
  pass
else
  fail "fish launcher must not contain bash function syntax 'claude()'"
fi

# Idempotency: running install again should produce exactly one launcher block
(
  HOME="$FISH_HOME"
  INSTALL_DIR="$FISH_INSTALL_DIR"
  # shellcheck disable=SC1090
  . "$INSTALL_FN_FISH"
  install_launcher_profile_block >/dev/null
)

fish_begin_count="$(grep -c '^# BEGIN: Juggernaut Launcher' "$FISH_CFG" 2>/dev/null || echo 0)"
fish_end_count="$(grep -c '^# END: Juggernaut Launcher' "$FISH_CFG" 2>/dev/null || echo 0)"
if [[ "$fish_begin_count" == "1" && "$fish_end_count" == "1" ]]; then
  pass
else
  fail "fish idempotency: begin=$fish_begin_count end=$fish_end_count"
fi

# ---------------------------------------------------------------------------
section "uninstall strips fish launcher block"
# Verify uninstall.sh's _launcher_profile_candidates includes fish config.
UNINSTALL_SH="$REPO_ROOT/commands/uninstall.sh"
if grep -q '\.config/fish/config\.fish' "$UNINSTALL_SH"; then
  pass
else
  fail "uninstall.sh _launcher_profile_candidates should include ~/.config/fish/config.fish"
fi

# Actually strip the block from the fish config we just wrote.
STRIP_FN_FISH="$TMP/strip_fn_fish.sh"
awk '
  /^_remove_launcher_block\(\)/ { capture = 1 }
  capture { print }
  capture && /^# END _remove_launcher_block$/ { exit }
' "$UNINSTALL_SH" > "$STRIP_FN_FISH"
(
  # shellcheck disable=SC1090
  . "$STRIP_FN_FISH"
  _remove_launcher_block "$FISH_CFG" >/dev/null
)
if ! grep -qE '^# (BEGIN|END): Juggernaut Launcher' "$FISH_CFG"; then
  pass
else
  fail "strip: fish launcher markers still present after uninstall"
fi

# ---------------------------------------------------------------------------
section "uninstall strips launcher block"
UNINSTALL_SH="$REPO_ROOT/commands/uninstall.sh"
if [[ ! -f "$UNINSTALL_SH" ]]; then
  fail "uninstall.sh not found at $UNINSTALL_SH"
else
  # FAKE_HOME still has a launcher block from the previous section.
  # Reuse it, run the strip helper directly from uninstall.sh.
  STRIP_FN="$TMP/strip_fn.sh"
  awk '
    /^_remove_launcher_block\(\)/ { capture = 1 }
    capture { print }
    capture && /^# END _remove_launcher_block$/ { exit }
  ' "$UNINSTALL_SH" > "$STRIP_FN"
  (
    # shellcheck disable=SC1090
    . "$STRIP_FN"
    _remove_launcher_block "$FAKE_HOME/.bashrc" >/dev/null
  )
  if ! grep -qE '^# (BEGIN|END): Juggernaut Launcher' "$FAKE_HOME/.bashrc"; then
    pass
  else
    fail "strip: markers still present in profile"
  fi
fi

echo
echo "launcher.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
