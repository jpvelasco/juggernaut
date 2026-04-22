#!/usr/bin/env bash
# commands/uninstall.sh — Juggernaut v2 uninstall subcommand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BEDROCK_CONFIG_PATH="${BEDROCK_CONFIG_PATH:-$SCRIPT_DIR/bedrock-config.json}"
export BEDROCK_CONFIG_PATH

v2_active="${JUGGERNAUT_USE_V2:-0}"
DRY_RUN=false
FORCE=false
REQUESTED_SCOPE=""

for arg in "$@"; do
  case "$arg" in
    --v2)         v2_active=1 ;;
    --dry-run)    DRY_RUN=true ;;
    --force|-f)   FORCE=true ;;
    --scope=*)    REQUESTED_SCOPE="${arg#--scope=}" ;;
    --version|-v) cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"; exit 0 ;;
    --help|-h)
      cat <<'EOF'
juggernaut uninstall - remove Juggernaut v2 configuration

Usage: juggernaut uninstall [--scope=user|project] [--dry-run] [--force]

Options:
  --scope=user|project  Limit removal to one scope (default: all scopes with a block)
  --dry-run             Preview changes without writing files
  --force, -f           Skip confirmation prompt
EOF
      exit 0 ;;
    *) echo "uninstall: unknown option '$arg'" >&2; exit 1 ;;
  esac
done

if [[ "$v2_active" != "1" ]]; then
  echo "Juggernaut v2 is not active. Use --v2 to enable v2 commands." >&2
  exit 0
fi

case "${REQUESTED_SCOPE:-}" in
  ""|user|project) ;;
  *) echo "uninstall: --scope must be 'user' or 'project' (got: '$REQUESTED_SCOPE')" >&2; exit 1 ;;
esac

. "$SCRIPT_DIR/lib/config_manager.sh"
. "$SCRIPT_DIR/lib/profile_writer.sh"
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

# Apply explicit scope filter
[[ "$REQUESTED_SCOPE" == "user" ]]    && has_project=false
[[ "$REQUESTED_SCOPE" == "project" ]] && has_user=false

# Scan all known shell profiles for the marker block
profile_targets=()
for _p in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.config/fish/config.fish"; do
  profile_writer_has_block "$_p" && profile_targets+=("$_p")
done

# Detect keychain
has_keychain=false
if keychain_available; then
  _key="$(keychain_get 2>/dev/null || true)"
  [[ -n "$_key" ]] && has_keychain=true
fi

if [[ "$has_user" == "false" && "$has_project" == "false" \
      && ${#profile_targets[@]} -eq 0 && "$has_keychain" == "false" ]]; then
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
  for _p in "${profile_targets[@]}"; do
    printf '  - Juggernaut block from %s\n' "$_p"
  done
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

for _p in "${profile_targets[@]}"; do
  if [[ "$DRY_RUN" == "true" ]]; then printf '[dry-run] Would remove Juggernaut block from %s\n' "$_p"
  else profile_writer_remove_block "$_p"; printf 'Removed Juggernaut block from %s\n' "$_p"; fi
done

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
