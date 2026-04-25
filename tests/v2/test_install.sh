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
if [[ "$INSTALL_SH" == *'--configure'* && "$INSTALL_SH" == *'./juggernaut apply --v2'* && "$INSTALL_SH" != *'exec bash ./setup'* ]]; then
  pass
else
  fail "install.sh should install-only by default and configure only when explicit"
fi

section "install.ps1 user launcher"
for needle in '.local\bin' 'juggernaut.cmd' 'ExecutionPolicy Bypass' 'Set-ExecutionPolicy RemoteSigned -Scope CurrentUser' 'juggernaut doctor --v2'; do
  if [[ "$INSTALL_PS1" == *"$needle"* ]]; then pass; else fail "install.ps1 missing $needle"; fi
done
if [[ "$INSTALL_PS1" == *"If PowerShell blocks first run scripts, run:"* ]]; then
  pass
else
  fail "install.ps1 missing first-run execution-policy guidance"
fi
if [[ "$INSTALL_PS1" == *'[switch]$Configure'* && "$INSTALL_PS1" == *'juggernaut.ps1 apply --v2'* && "$INSTALL_PS1" != *'setup-claude-bedrock.ps1 @SetupArgs'* ]]; then
  pass
else
  fail "install.ps1 should install-only by default and configure only when explicit"
fi

section "fresh install doctor smoke"
TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK"' EXIT
mkdir -p "$TMP_HOME/.claude"

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
set +e

export J_AUTH_MODE=iam
export J_REGION=us-west-2
export J_EFFORT=xhigh
export J_STORAGE=profile
export J_USE_MANTLE=false
export J_OPUSPLAN=false
export J_SCOPE=user
export J_VERSION=2.1.4
export J_SHELL_FALLBACK_MODE=settings-only
BLOCK="$(schema_new_juggernaut_block)"
config_write_atomic "$TMP_HOME/.claude/settings.json" "$(config_merge_juggernaut_block '{}' "$BLOCK" "$(schema_derive_native_keys "$BLOCK")")"

chmod +x "$REPO_ROOT/juggernaut" "$REPO_ROOT/commands/doctor.sh"
if OUTPUT="$(cd "$TMP_WORK" && HOME="$TMP_HOME" AWS_PROFILE=juggernaut-test SHELL=/bin/bash "$REPO_ROOT/juggernaut" doctor --v2 2>&1)" &&
   [[ "$OUTPUT" == *"Status: OK"$'\n'"No issues found"* ]]; then
  pass
else
  fail "fresh install doctor smoke failed"
  printf '%s\n' "$OUTPUT" >&2
fi

echo
echo "install tests: $PASS passed, $FAIL failed"
exit "$FAIL"
