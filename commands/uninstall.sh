#!/usr/bin/env bash
# commands/uninstall.sh - Juggernaut v3 uninstall subcommand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BEDROCK_CONFIG_PATH="${BEDROCK_CONFIG_PATH:-$SCRIPT_DIR/bedrock-config.json}"
export BEDROCK_CONFIG_PATH

DRY_RUN=false
FORCE=false
REQUESTED_SCOPE=""

for arg in "$@"; do
  case "$arg" in
    --dry-run)    DRY_RUN=true ;;
    --force|-f)   FORCE=true ;;
    --scope=*)    REQUESTED_SCOPE="${arg#--scope=}" ;;
    --version|-v) cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"; exit 0 ;;
    --help|-h)
      cat <<'EOF'
juggernaut uninstall - remove Juggernaut configuration

Usage: juggernaut uninstall [--scope=user|project] [--dry-run] [--force]

Options:
  --scope=user|project  Limit removal to one scope (default: all scopes with a block)
  --dry-run             Preview changes without writing files
  --force, -f           Skip confirmation prompt

Removes the Juggernaut block from settings.json, the keychain entry, and
the Claude launcher function block from shell profiles (~/.bashrc,
~/.zshrc, ~/.profile). Also removes a legacy ~/.local/bin/claude symlink
from earlier launcher iterations if present. Does not strip legacy
v1/v2 profile blocks from earlier versions — run the installer
(install.sh) for that full-wipe pass.
EOF
      exit 0 ;;
    *) echo "uninstall: unknown option '$arg'" >&2; exit 1 ;;
  esac
done

case "${REQUESTED_SCOPE:-}" in
  ""|user|project) ;;
  *) echo "uninstall: --scope must be 'user' or 'project' (got: '$REQUESTED_SCOPE')" >&2; exit 1 ;;
esac

. "$SCRIPT_DIR/lib/config_manager.sh"
. "$SCRIPT_DIR/lib/keychain.sh"

# ---------------------------------------------------------------------------
# Detect what's installed
# ---------------------------------------------------------------------------
user_path="$(config_user_settings_path)"
project_path="${PWD}/.claude/settings.json"

has_user=false
has_project=false

if [[ -f "$user_path" ]]; then
  _json="$(config_read "$user_path" 2>/dev/null || true)"
  [[ -n "$_json" ]] && config_has_juggernaut_block "$_json" && has_user=true
fi
if [[ -f "$project_path" ]]; then
  _json="$(config_read "$project_path" 2>/dev/null || true)"
  [[ -n "$_json" ]] && config_has_juggernaut_block "$_json" && has_project=true
fi

[[ "$REQUESTED_SCOPE" == "user" ]]    && has_project=false
[[ "$REQUESTED_SCOPE" == "project" ]] && has_user=false

has_keychain=false
if keychain_available; then
  _key="$(keychain_get 2>/dev/null || true)"
  [[ -n "$_key" ]] && has_keychain=true
fi

has_dpapi=false
dpapi_file=""
case "$(keychain_detect_os)" in
  gitbash|cygwin)
    dpapi_file="$(dpapi_path)"
    [[ -f "$dpapi_file" ]] && has_dpapi=true
    ;;
esac

has_profile_token=false
profile_token_file="$(profile_token_path)"
[[ -f "$profile_token_file" ]] && has_profile_token=true

_launcher_profile_candidates() {
  printf '%s\n' "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"
}

_profile_has_launcher_block() {
  local path="$1"
  [[ -f "$path" ]] || return 1
  grep -qE '^# BEGIN: Juggernaut Launcher' "$path" 2>/dev/null
}

launcher_profiles=()
while IFS= read -r _p; do
  if _profile_has_launcher_block "$_p"; then
    launcher_profiles+=("$_p")
  fi
done < <(_launcher_profile_candidates)

has_launcher=false
[[ ${#launcher_profiles[@]} -gt 0 ]] && has_launcher=true

# Legacy v3.0.x-dev symlink cleanup: earlier launcher iterations placed a
# symlink at ~/.local/bin/claude. Newer installs use a shell function, but
# we still remove the old symlink if we find one (never remove a regular
# file — that's likely Anthropic's claude binary).
legacy_symlink="$HOME/.local/bin/claude"
has_legacy_symlink=false
[[ -L "$legacy_symlink" ]] && has_legacy_symlink=true

if [[ "$has_user" == "false" && "$has_project" == "false" \
      && "$has_keychain" == "false" && "$has_dpapi" == "false" \
      && "$has_profile_token" == "false" \
      && "$has_launcher" == "false" && "$has_legacy_symlink" == "false" ]]; then
  echo "Nothing to uninstall."
  exit 0
fi

# ---------------------------------------------------------------------------
# Confirmation (skipped for --force and --dry-run)
# ---------------------------------------------------------------------------
if [[ "$FORCE" != "true" && "$DRY_RUN" != "true" ]]; then
  echo "The following will be removed:"
  [[ "$has_user"    == "true" ]] && printf '  - Juggernaut block from %s\n' "$user_path"
  [[ "$has_project" == "true" ]] && printf '  - Juggernaut block from %s\n' "$project_path"
  [[ "$has_keychain" == "true" ]] && printf '  - Keychain entry: %s/%s\n' "$KEYCHAIN_SERVICE" "$KEYCHAIN_ACCOUNT"
  [[ "$has_dpapi" == "true" ]]    && printf '  - DPAPI file: %s\n' "$dpapi_file"
  [[ "$has_profile_token" == "true" ]] && printf '  - Profile token file: %s\n' "$profile_token_file"
  for _lp in "${launcher_profiles[@]+"${launcher_profiles[@]}"}"; do
    printf '  - Launcher block from %s\n' "$_lp"
  done
  [[ "$has_legacy_symlink" == "true" ]] && printf '  - Legacy launcher symlink: %s\n' "$legacy_symlink"
  echo ""
  read -r -p "Proceed? [y/N] " _answer
  case "$_answer" in
    [Yy]|[Yy][Ee][Ss]) ;;
    *) echo "Aborted."; exit 0 ;;
  esac
fi

# ---------------------------------------------------------------------------
# Execute
# ---------------------------------------------------------------------------
_remove_settings() {
  local path="$1"
  local json
  json="$(config_read "$path")"
  local cleaned
  cleaned="$(config_remove_juggernaut_block "$json")"
  config_write_atomic "$path" "$cleaned"
  printf 'Removed Juggernaut block from %s\n' "$path"
}

if [[ "$has_user" == "true" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then printf '[dry-run] Would remove Juggernaut block from %s\n' "$user_path"
  else _remove_settings "$user_path"; fi
fi

if [[ "$has_project" == "true" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then printf '[dry-run] Would remove Juggernaut block from %s\n' "$project_path"
  else _remove_settings "$project_path"; fi
fi

if [[ "$has_keychain" == "true" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[dry-run] Would remove keychain entry: %s/%s\n' "$KEYCHAIN_SERVICE" "$KEYCHAIN_ACCOUNT"
  else
    keychain_delete
    printf 'Removed keychain entry: %s/%s\n' "$KEYCHAIN_SERVICE" "$KEYCHAIN_ACCOUNT"
  fi
fi

if [[ "$has_dpapi" == "true" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[dry-run] Would remove DPAPI file: %s\n' "$dpapi_file"
  else
    dpapi_delete
    printf 'Removed DPAPI file: %s\n' "$dpapi_file"
  fi
fi

if [[ "$has_profile_token" == "true" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[dry-run] Would remove profile token file: %s\n' "$profile_token_file"
  else
    profile_token_delete
    printf 'Removed profile token file: %s\n' "$profile_token_file"
  fi
fi

_remove_launcher_block() {
  local path="$1"
  local tmp
  tmp="$(mktemp "${path}.launcherXXXXXX")"
  awk '
    BEGIN { skip = 0 }
    /^# BEGIN: Juggernaut Launcher/ { skip = 1; next }
    /^# END: Juggernaut Launcher/   { skip = 0; next }
    skip == 0 { print }
  ' "$path" > "$tmp" && mv "$tmp" "$path"
  printf 'Removed launcher block from %s\n' "$path"
}
# END _remove_launcher_block

for _lp in "${launcher_profiles[@]+"${launcher_profiles[@]}"}"; do
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[dry-run] Would remove launcher block from %s\n' "$_lp"
  else
    _remove_launcher_block "$_lp"
  fi
done

if [[ "$has_legacy_symlink" == "true" ]]; then
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '[dry-run] Would remove legacy launcher symlink: %s\n' "$legacy_symlink"
  else
    rm -f "$legacy_symlink"
    printf 'Removed legacy launcher symlink: %s\n' "$legacy_symlink"
  fi
fi

if [[ "$DRY_RUN" == "true" ]]; then
  echo "No files were changed."
else
  echo ""
  echo "Uninstall complete."
fi
