#!/usr/bin/env bash
# tests/v2/test_install.sh — v3 installer acceptance tests.
# v3 installer is a destructive wipe-and-reinstall: strips profile blocks,
# removes the juggernaut key from settings.json, deletes the keychain entry,
# clones a fresh tree, installs a launcher. It does NOT auto-apply.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

INSTALL_SH="$(cat "$REPO_ROOT/install.sh")"
INSTALL_PS1="$(cat "$REPO_ROOT/install.ps1")"

# ---------------------------------------------------------------------------
# Static checks: v3 installer contract
# ---------------------------------------------------------------------------
section "install.sh mentions wipe-and-reinstall behavior"
for needle in 'wipe-and-reinstall' 'Pre-wipe summary' 'BEGIN: Juggernaut' 'BEGIN: Claude Code Bedrock Configuration' 'juggernaut-bedrock' '--dry-run' 'juggernaut apply --auth=iam'; do
  if [[ "$INSTALL_SH" == *"$needle"* ]]; then pass; else fail "install.sh missing '$needle'"; fi
done

section "install.sh does NOT auto-apply or mention legacy flags"
for needle in '--configure' '--legacy-v1' '--v2' 'setup-claude-bedrock' 'JUGGERNAUT_USE_V2'; do
  if [[ "$INSTALL_SH" != *"$needle"* ]]; then pass; else fail "install.sh still mentions '$needle'"; fi
done

section "install.sh sets up user launcher + --ref flag"
if [[ "$INSTALL_SH" == *'.local/bin'* && "$INSTALL_SH" == *'ln -sfn'* ]]; then pass
else fail "install.sh missing ~/.local/bin symlink"; fi
if [[ "$INSTALL_SH" == *'--ref'* && "$INSTALL_SH" == *'git clone --branch "$REF"'* ]]; then pass
else fail "install.sh should support installing an explicit branch/ref for PR testing"; fi

section "install.sh executable bits and chmod suppress on Windows"
for needle in 'chmod +x' 'commands/*.sh' 'lib/*.sh' '2>/dev/null || true'; do
  if [[ "$INSTALL_SH" == *"$needle"* ]]; then pass; else fail "install.sh missing '$needle'"; fi
done

section "install.ps1 v3 wipe-and-reinstall"
for needle in 'Pre-wipe summary' 'BEGIN: Juggernaut' 'BEGIN: Claude Code Bedrock Configuration' 'juggernaut-bedrock' '-DryRun'; do
  if [[ "$INSTALL_PS1" == *"$needle"* ]]; then pass; else fail "install.ps1 missing '$needle'"; fi
done

section "install.ps1 does NOT auto-apply or mention legacy flags"
for needle in '-Configure' '-LegacyV1' 'setup-claude-bedrock.ps1' 'JUGGERNAUT_USE_V2'; do
  if [[ "$INSTALL_PS1" != *"$needle"* ]]; then pass; else fail "install.ps1 still mentions '$needle'"; fi
done

section "install.ps1 user launcher + ExecutionPolicy guidance"
for needle in '.local\bin' 'juggernaut.cmd' 'ExecutionPolicy'; do
  if [[ "$INSTALL_PS1" == *"$needle"* ]]; then pass; else fail "install.ps1 missing '$needle'"; fi
done

section "install.ps1 supports -Ref for branch/sha installs"
if [[ "$INSTALL_PS1" == *'[string]$Ref'* && "$INSTALL_PS1" == *'JUGGERNAUT_REF'* ]]; then pass
else fail "install.ps1 should accept -Ref"; fi

# ---------------------------------------------------------------------------
# Runtime: fixture-backed install scenarios
# ---------------------------------------------------------------------------
TMP_REMOTE="$(mktemp -d)"
TMP_SRC="$(mktemp -d)"
TMP_INSTALL_HOME="$(mktemp -d)"
trap 'rm -rf "$TMP_REMOTE" "$TMP_SRC" "$TMP_INSTALL_HOME"' EXIT

git -C "$TMP_SRC" init -q
git -C "$TMP_SRC" config user.email test@example.invalid
git -C "$TMP_SRC" config user.name "Juggernaut Test"
mkdir -p "$TMP_SRC/commands" "$TMP_SRC/lib"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/juggernaut"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/commands/apply.sh"
printf '#!/usr/bin/env bash\n' > "$TMP_SRC/lib/schema.sh"
printf '9.9.9\n' > "$TMP_SRC/VERSION"
chmod +x "$TMP_SRC/juggernaut" "$TMP_SRC/commands/apply.sh" "$TMP_SRC/lib/schema.sh"
git -C "$TMP_SRC" add .
git -C "$TMP_SRC" commit -q -m "fixture"
git -C "$TMP_SRC" tag v9.9.9
git clone --bare -q "$TMP_SRC" "$TMP_REMOTE/repo.git"

# ---------------------------------------------------------------------------
# --dry-run prints pre-wipe summary and writes nothing
# ---------------------------------------------------------------------------
section "--dry-run prints summary and writes nothing"
DRY_HOME="$(mktemp -d)"
mkdir -p "$DRY_HOME/.claude"
printf '{"juggernaut":{"meta":{"managedBy":"juggernaut"}}}\n' > "$DRY_HOME/.claude/settings.json"
printf '# BEGIN: Juggernaut\nexport FOO=1\n# END: Juggernaut\n' > "$DRY_HOME/.bashrc"
BEFORE_SETTINGS_MD5="$(md5sum "$DRY_HOME/.claude/settings.json" 2>/dev/null | awk '{print $1}')"
BEFORE_BASHRC_MD5="$(md5sum "$DRY_HOME/.bashrc" 2>/dev/null | awk '{print $1}')"
OUT="$(HOME="$DRY_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" JUGGERNAUT_DIR="$DRY_HOME/.juggernaut" \
  bash "$REPO_ROOT/install.sh" --version v9.9.9 --dry-run 2>&1)"
RC=$?
if [[ "$RC" -eq 0 && "$OUT" == *"Pre-wipe summary"* && "$OUT" == *"strip Juggernaut/v1 block"* && "$OUT" == *"remove 'juggernaut' key"* && "$OUT" == *"dry-run"* ]]; then
  pass
else
  fail "dry-run should emit summary and exit 0 (rc=$RC)"
  printf '%s\n' "$OUT" >&2
fi
AFTER_SETTINGS_MD5="$(md5sum "$DRY_HOME/.claude/settings.json" 2>/dev/null | awk '{print $1}')"
AFTER_BASHRC_MD5="$(md5sum "$DRY_HOME/.bashrc" 2>/dev/null | awk '{print $1}')"
if [[ "$BEFORE_SETTINGS_MD5" == "$AFTER_SETTINGS_MD5" && "$BEFORE_BASHRC_MD5" == "$AFTER_BASHRC_MD5" ]]; then pass
else fail "dry-run must not modify files"; fi
if [[ ! -d "$DRY_HOME/.juggernaut" ]]; then pass
else fail "dry-run must not clone into JUGGERNAUT_DIR"; fi
rm -rf "$DRY_HOME"

# ---------------------------------------------------------------------------
# Full install wipes profile block + settings.json juggernaut key
# ---------------------------------------------------------------------------
section "full install wipes profile block and settings.json juggernaut key, does not auto-apply"
WIPE_HOME="$(mktemp -d)"
mkdir -p "$WIPE_HOME/.claude"
printf '{"juggernaut":{"meta":{"managedBy":"juggernaut"}},"permissions":{"allow":["Bash"]}}\n' > "$WIPE_HOME/.claude/settings.json"
printf '# keep this\n# BEGIN: Juggernaut\nexport FOO=1\n# END: Juggernaut\n# keep that\n' > "$WIPE_HOME/.bashrc"
OUT="$(HOME="$WIPE_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" JUGGERNAUT_DIR="$WIPE_HOME/.juggernaut" \
  bash "$REPO_ROOT/install.sh" --version v9.9.9 2>&1)"
RC=$?
if [[ "$RC" -eq 0 && -d "$WIPE_HOME/.juggernaut" ]]; then pass
else fail "install should succeed and clone into \$JUGGERNAUT_DIR (rc=$RC)"; printf '%s\n' "$OUT" >&2; fi

if ! grep -q "BEGIN: Juggernaut" "$WIPE_HOME/.bashrc"; then pass
else fail "install should strip profile Juggernaut block"; fi
if grep -q "keep this" "$WIPE_HOME/.bashrc" && grep -q "keep that" "$WIPE_HOME/.bashrc"; then pass
else fail "install should preserve unrelated profile content"; fi

if command -v jq >/dev/null 2>&1; then
  if jq -e '.juggernaut | not' "$WIPE_HOME/.claude/settings.json" >/dev/null 2>&1; then pass
  else fail "install should remove .juggernaut from settings.json"; fi
  if jq -e '.permissions.allow' "$WIPE_HOME/.claude/settings.json" >/dev/null 2>&1; then pass
  else fail "install should preserve unrelated settings.json keys"; fi
fi

# v3: install must NOT auto-apply — no fresh juggernaut block present
if command -v jq >/dev/null 2>&1; then
  if ! jq -e '.juggernaut // empty' "$WIPE_HOME/.claude/settings.json" >/dev/null 2>&1; then pass
  else fail "install must not auto-apply a juggernaut block"; fi
fi

# Post-install message tells user to run apply explicitly.
if [[ "$OUT" == *"juggernaut apply --auth=iam"* && "$OUT" == *"No configuration has been written"* ]]; then pass
else fail "install should tell user to run apply explicitly"; fi

rm -rf "$WIPE_HOME"

# ---------------------------------------------------------------------------
# --ref installs the requested branch
# ---------------------------------------------------------------------------
section "--ref installs the requested branch"
git -C "$TMP_SRC" checkout -q -b fixture-ref
printf 'fixture-ref\n' > "$TMP_SRC/VERSION"
git -C "$TMP_SRC" add VERSION
git -C "$TMP_SRC" commit -q -m "fixture ref"
git -C "$TMP_SRC" push -q "$TMP_REMOTE/repo.git" fixture-ref

REF_HOME="$(mktemp -d)"
if OUT="$(HOME="$REF_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" JUGGERNAUT_DIR="$REF_HOME/.juggernaut" \
    bash "$REPO_ROOT/install.sh" --ref fixture-ref 2>&1)" &&
   [[ "$OUT" == *"Installing Juggernaut fixture-ref"* ]] &&
   [[ "$(tr -d '\r\n ' < "$REF_HOME/.juggernaut/VERSION")" == "fixture-ref" ]]; then
  pass
else
  fail "--ref should clone and install the requested branch"
  printf '%s\n' "$OUT" >&2
fi
rm -rf "$REF_HOME"

# ---------------------------------------------------------------------------
# Dirty install backup
# ---------------------------------------------------------------------------
section "dirty existing install is backed up before clone"
git clone --branch v9.9.9 -q "$TMP_REMOTE/repo.git" "$TMP_INSTALL_HOME/.juggernaut"
printf '# local edit\n' >> "$TMP_INSTALL_HOME/.juggernaut/lib/schema.sh"
if OUT="$(HOME="$TMP_INSTALL_HOME" JUGGERNAUT_REPO_URL="$TMP_REMOTE/repo.git" JUGGERNAUT_DIR="$TMP_INSTALL_HOME/.juggernaut" \
    bash "$REPO_ROOT/install.sh" --version v9.9.9 2>&1)" &&
   [[ "$OUT" == *"Existing installation has local changes"* ]] &&
   [[ "$OUT" == *"Backup created:"* ]] &&
   [[ -d "$TMP_INSTALL_HOME/.juggernaut" ]] &&
   [[ -z "$(git -C "$TMP_INSTALL_HOME/.juggernaut" status --short)" ]]; then
  shopt -s nullglob
  BACKUPS=("$TMP_INSTALL_HOME"/.juggernaut.backup.*)
  shopt -u nullglob
  if [[ "${#BACKUPS[@]}" -ge 1 && -f "${BACKUPS[0]}/lib/schema.sh" ]] &&
     grep -q '# local edit' "${BACKUPS[0]}/lib/schema.sh"; then
    pass
  else
    fail "dirty install backup should preserve the edited install tree"
    printf '%s\n' "$OUT" >&2
  fi
else
  fail "dirty install should be backed up and replaced by a clean clone"
  printf '%s\n' "$OUT" >&2
fi

# ---------------------------------------------------------------------------
# --help
# ---------------------------------------------------------------------------
section "install.sh --help exits 0"
HELP="$(bash "$REPO_ROOT/install.sh" --help 2>&1)"; RC=$?
if [[ $RC -eq 0 && "$HELP" == *"--dry-run"* && "$HELP" == *"--ref"* ]]; then pass
else fail "install --help should exit 0 and document --dry-run/--ref"; fi

echo
echo "install tests: $PASS passed, $FAIL failed"
exit "$FAIL"
