#!/usr/bin/env bash
# tests/v2/test_install.sh - static acceptance checks for installer robustness.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0
FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

INSTALL_SH="$(cat "$REPO_ROOT/install.sh")"
INSTALL_PS1="$(cat "$REPO_ROOT/install.ps1")"

section "install.sh executable permissions"
for needle in 'chmod +x' 'commands/*.sh' 'lib/*.sh' 'juggernaut' 'setup'; do
  if [[ "$INSTALL_SH" == *"$needle"* ]]; then pass; else fail "install.sh missing $needle"; fi
done

section "install.sh user launcher"
if [[ "$INSTALL_SH" == *'.local/bin'* && "$INSTALL_SH" == *'ln -sfn'* && "$INSTALL_SH" == *'juggernaut doctor --v2'* ]]; then
  pass
else
  fail "install.sh missing ~/.local/bin symlink or verification message"
fi

section "install.ps1 user launcher"
for needle in '.local\bin' 'juggernaut.cmd' 'ExecutionPolicy Bypass' 'Set-ExecutionPolicy RemoteSigned -Scope CurrentUser' 'juggernaut doctor --v2'; do
  if [[ "$INSTALL_PS1" == *"$needle"* ]]; then pass; else fail "install.ps1 missing $needle"; fi
done

echo
echo "install tests: $PASS passed, $FAIL failed"
exit "$FAIL"
