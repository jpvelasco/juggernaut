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
if [[ "$INSTALL_SH" == *'--ref'* && "$INSTALL_SH" == *'JUGGERNAUT_REF'* && "$INSTALL_SH" == *'git clone --branch "$REF"'* ]]; then
  pass
else
  fail "install.sh should support installing an explicit branch/ref for PR testing"
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
if [[ "$INSTALL_PS1" == *'[switch]$Configure'* && "$INSTALL_PS1" == *'Convert-InstallerApplyArgs'* && "$INSTALL_PS1" == *'commands\apply.ps1'* && "$INSTALL_PS1" != *'juggernaut.ps1 apply --v2'* && "$INSTALL_PS1" != *'exit $LASTEXITCODE'* && "$INSTALL_PS1" != *'setup-claude-bedrock.ps1 @SetupArgs'* ]]; then
  pass
else
  fail "install.ps1 should configure without routing through an exit-based dispatcher"
fi
if [[ "$INSTALL_PS1" == *'[string]$Ref'* && "$INSTALL_PS1" == *'JUGGERNAUT_REF'* && "$INSTALL_PS1" == *'git clone --branch $Ref'* ]]; then
  pass
else
  fail "install.ps1 should support installing an explicit branch/ref for PR testing"
fi

section "dirty existing install is backed up before fresh clone"
for needle in 'JUGGERNAUT_REPO_URL' 'install_tree_dirty' 'backup_existing_install' '.backup.' 'Backup created:'; do
  if [[ "$INSTALL_SH" == *"$needle"* ]]; then pass; else fail "install.sh missing dirty install backup behavior: $needle"; fi
done
for needle in 'JUGGERNAUT_REPO_URL' 'Test-InstallTreeDirty' 'Backup-ExistingInstall' '.backup.' 'Backup created:'; do
  if [[ "$INSTALL_PS1" == *"$needle"* ]]; then pass; else fail "install.ps1 missing dirty install backup behavior: $needle"; fi
done

TMP_REMOTE="$(mktemp -d)"
TMP_SRC="$(mktemp -d)"
TMP_INSTALL_HOME="$(mktemp -d)"
trap 'rm -rf "$TMP_HOME" "$TMP_WORK" "$TMP_REMOTE" "$TMP_SRC" "$TMP_INSTALL_HOME"' EXIT

git -C "$TMP_SRC" init -q
git -C "$TMP_SRC" config user.email test@example.invalid
git -C "$TMP_SRC" config user.name "Juggernaut Test"
mkdir -p "$TMP_SRC/commands" "$TMP_SRC/lib"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/juggernaut"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/setup"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/commands/apply.sh"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/lib/schema.sh"
printf '9.9.9\n' > "$TMP_SRC/VERSION"
chmod +x "$TMP_SRC/juggernaut" "$TMP_SRC/setup" "$TMP_SRC/commands/apply.sh" "$TMP_SRC/lib/schema.sh"
git -C "$TMP_SRC" add .
git -C "$TMP_SRC" commit -q -m "fixture"
git -C "$TMP_SRC" tag v9.9.9
git clone --bare -q "$TMP_SRC" "$TMP_REMOTE/repo.git"
git clone --branch v9.9.9 -q "$TMP_REMOTE/repo.git" "$TMP_INSTALL_HOME/.juggernaut"
printf '# local edit\n' >> "$TMP_INSTALL_HOME/.juggernaut/lib/schema.sh"

if OUTPUT="$(HOME="$TMP_INSTALL_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" bash "$REPO_ROOT/install.sh" --version v9.9.9 2>&1)" &&
   [[ "$OUTPUT" == *"Existing installation has local changes"* ]] &&
   [[ "$OUTPUT" == *"Backup created:"* ]] &&
   [[ -d "$TMP_INSTALL_HOME/.juggernaut" ]] &&
   [[ -z "$(git -C "$TMP_INSTALL_HOME/.juggernaut" status --short)" ]]; then
  shopt -s nullglob
  BACKUPS=("$TMP_INSTALL_HOME"/.juggernaut.backup.*)
  shopt -u nullglob
  if [[ "${#BACKUPS[@]}" -eq 1 && -f "${BACKUPS[0]}/lib/schema.sh" ]] &&
     grep -q '# local edit' "${BACKUPS[0]}/lib/schema.sh"; then
    pass
  else
    fail "dirty install backup should preserve the edited install tree"
    printf '%s\n' "$OUTPUT" >&2
  fi
else
  fail "dirty install should be backed up and replaced by a clean clone"
  printf '%s\n' "$OUTPUT" >&2
fi

section "--ref installs the requested branch"
TMP_REF_HOME="$(mktemp -d)"
git -C "$TMP_SRC" checkout -q -b fixture-ref
printf 'fixture-ref\n' > "$TMP_SRC/VERSION"
git -C "$TMP_SRC" add VERSION
git -C "$TMP_SRC" commit -q -m "fixture ref"
git -C "$TMP_SRC" push -q "$TMP_REMOTE/repo.git" fixture-ref

if OUTPUT="$(HOME="$TMP_REF_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" bash "$REPO_ROOT/install.sh" --ref fixture-ref 2>&1)" &&
   [[ "$OUTPUT" == *"Installing Juggernaut fixture-ref"* ]] &&
   [[ "$(tr -d '\r\n ' < "$TMP_REF_HOME/.juggernaut/VERSION")" == "fixture-ref" ]]; then
  pass
else
  fail "--ref should clone and install the requested branch"
  printf '%s\n' "$OUTPUT" >&2
fi

git -C "$TMP_SRC" checkout -q -b fixture-ref-2
printf 'fixture-ref-2\n' > "$TMP_SRC/VERSION"
git -C "$TMP_SRC" add VERSION
git -C "$TMP_SRC" commit -q -m "fixture ref 2"
git -C "$TMP_SRC" push -q "$TMP_REMOTE/repo.git" fixture-ref-2
if OUTPUT="$(HOME="$TMP_REF_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" bash "$REPO_ROOT/install.sh" --ref fixture-ref-2 2>&1)" &&
   [[ "$OUTPUT" == *"Updating existing installation"* ]] &&
   [[ "$(tr -d '\r\n ' < "$TMP_REF_HOME/.juggernaut/VERSION")" == "fixture-ref-2" ]]; then
  pass
else
  fail "--ref should update a clean existing install to the requested fetched ref"
  printf '%s\n' "$OUTPUT" >&2
fi
rm -rf "$TMP_REF_HOME"

section "piped installer --ref --configure writes Bedrock API-key settings"
TMP_SCENARIO_SRC="$(mktemp -d)"
TMP_SCENARIO_REMOTE="$(mktemp -d)"
TMP_SCENARIO_HOME="$(mktemp -d)"
git clone -q "$REPO_ROOT" "$TMP_SCENARIO_SRC"
git -C "$TMP_SCENARIO_SRC" checkout -q -b scenario-ref
git -C "$TMP_SCENARIO_SRC" config user.email test@example.invalid
git -C "$TMP_SCENARIO_SRC" config user.name "Juggernaut Test"
printf 'scenario-ref\n' > "$TMP_SCENARIO_SRC/VERSION"
git -C "$TMP_SCENARIO_SRC" add VERSION
git -C "$TMP_SCENARIO_SRC" commit -q -m "scenario ref"
git clone --bare -q "$TMP_SCENARIO_SRC" "$TMP_SCENARIO_REMOTE/repo.git"

OUTPUT="$(
  HOME="$TMP_SCENARIO_HOME" JUGGERNAUT_REPO_URL="$TMP_SCENARIO_REMOTE/repo.git" \
    bash -s -- --ref scenario-ref --configure --auth=bedrock-api-key \
      --bedrock-key=br-ci-token --storage=profile --no-shell-fallback \
      < "$REPO_ROOT/install.sh" 2>&1
)"
RC=$?
if [[ "$RC" -eq 0 &&
      "$OUTPUT" == *"Installing Juggernaut scenario-ref"* &&
      "$OUTPUT" != *"unknown option '--ref'"* &&
      "$OUTPUT" != *"unknown option '--auth'"* &&
      "$(tr -d '\r\n ' < "$TMP_SCENARIO_HOME/.juggernaut/VERSION")" == "scenario-ref" &&
      "$(jq -r '.juggernaut.auth.mode' "$TMP_SCENARIO_HOME/.claude/settings.json")" == "bedrock-api-key" &&
      "$(jq -r '.juggernaut.auth.storage' "$TMP_SCENARIO_HOME/.claude/settings.json")" == "profile" ]]; then
  pass
else
  fail "piped installer should install requested ref and pass configure args to apply"
  printf '%s\n' "$OUTPUT" >&2
fi

if DOCTOR_OUTPUT="$(cd "$TMP_SCENARIO_HOME" && HOME="$TMP_SCENARIO_HOME" AWS_BEARER_TOKEN_BEDROCK=br-ci-token "$TMP_SCENARIO_HOME/.local/bin/juggernaut" doctor --v2 2>&1)" &&
   [[ "$DOCTOR_OUTPUT" == *"Auth: Bedrock API key"* &&
      "$DOCTOR_OUTPUT" == *"Status: OK"$'\n'"No issues found"* &&
      "$DOCTOR_OUTPUT" != *"ParserError"* &&
      "$DOCTOR_OUTPUT" != *"Unexpected token"* ]]; then
  pass
else
  fail "installed launcher doctor should pass after piped API-key configure"
  printf '%s\n' "$DOCTOR_OUTPUT" >&2
fi
rm -rf "$TMP_SCENARIO_SRC" "$TMP_SCENARIO_REMOTE" "$TMP_SCENARIO_HOME"

section "fresh install doctor smoke"
TMP_HOME="$(mktemp -d)"
TMP_WORK="$(mktemp -d)"
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
J_VERSION="$(tr -d '\r\n ' < "$REPO_ROOT/VERSION" 2>/dev/null)"
export J_VERSION
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

section "symlinked launcher resolves install dir"
TMP_BIN="$TMP_HOME/.local/bin"
mkdir -p "$TMP_BIN"
ln -sfn "$REPO_ROOT/juggernaut" "$TMP_BIN/juggernaut"
if OUTPUT="$(cd "$TMP_WORK" && HOME="$TMP_HOME" AWS_PROFILE=juggernaut-test SHELL=/bin/bash "$TMP_BIN/juggernaut" doctor --v2 2>&1)" &&
   [[ "$OUTPUT" == *"Status: OK"$'\n'"No issues found"* ]]; then
  pass
else
  fail "symlinked launcher should resolve commands from install dir"
  printf '%s\n' "$OUTPUT" >&2
fi

echo
echo "install tests: $PASS passed, $FAIL failed"
exit "$FAIL"
