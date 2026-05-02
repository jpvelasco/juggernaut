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

Removes the Juggernaut block from settings.json and the keychain entry.
Shell-profile blocks are not touched here; in v3 Juggernaut does not write
to shell profiles. Run the installer (install.sh) for a full wipe that
includes legacy profile blocks from earlier versions.
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

if [[ "$has_user" == "false" && "$has_project" == "false" && "$has_keychain" == "false" ]]; then
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

if [[ "$DRY_RUN" == "true" ]]; then
  echo "No files were changed."
else
  echo ""
  echo "Uninstall complete."
fi
