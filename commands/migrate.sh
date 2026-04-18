#!/usr/bin/env bash
# commands/migrate.sh — juggernaut migrate subcommand.
# Usage:
#   migrate                  Detect v1 block in each shell profile and migrate to settings.json.
#   migrate --rollback       Restore most recent settings.json backup.
#   migrate --clean          Remove profile block(s) after a successful migration.
#   migrate --dry-run        Show what would be done without writing anything.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
. "$REPO_ROOT/lib/migrator.sh"

ROLLBACK=false
CLEAN=false
DRY_RUN=false
SCOPE="user"

for arg in "$@"; do
  case "$arg" in
    --rollback) ROLLBACK=true ;;
    --clean)    CLEAN=true ;;
    --dry-run)  DRY_RUN=true ;;
    --project)  SCOPE="project" ;;
    *) echo "migrate: unknown option '$arg'" >&2; exit 1 ;;
  esac
done

SETTINGS_PATH="$(config_resolve_target "$SCOPE")"

# ---------------------------------------------------------------------------
# Rollback path
# ---------------------------------------------------------------------------
if [[ "$ROLLBACK" == true ]]; then
  if [[ "$DRY_RUN" == true ]]; then
    echo "[dry-run] Would rollback $SETTINGS_PATH to most recent backup"
    exit 0
  fi
  migrator_rollback "$SETTINGS_PATH"
  exit 0
fi

# ---------------------------------------------------------------------------
# Detect v1 profile blocks across all standard shell profiles
# ---------------------------------------------------------------------------
declare -a CANDIDATES=(
  "$HOME/.bashrc"
  "$HOME/.bash_profile"
  "$HOME/.zshrc"
  "$HOME/.config/fish/config.fish"
  "$HOME/.profile"
)

FOUND=0
for profile in "${CANDIDATES[@]}"; do
  if migrator_has_v1_block "$profile"; then
    FOUND=$((FOUND + 1))
    echo "Found v1 block: $profile"

    if [[ "$DRY_RUN" == true ]]; then
      echo "[dry-run] Would migrate $profile → $SETTINGS_PATH"
      continue
    fi

    migrator_run "$profile" "$SETTINGS_PATH" "${BEDROCK_CONFIG_PATH:-$REPO_ROOT/bedrock-config.json}"

    if [[ "$CLEAN" == true ]]; then
      # Remove the entire v1 block from the profile.
      sed_inplace() {
        # Portable in-place: macOS sed requires a backup extension argument.
        if sed --version 2>/dev/null | grep -q GNU; then
          sed -i "$@"
        else
          sed -i '' "$@"
        fi
      }
      sed_inplace '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$profile"
      echo "Removed v1 block from $profile (--clean)"
    fi
  fi
done

if (( FOUND == 0 )); then
  echo "No v1 profile blocks found. Nothing to migrate."
  exit 0
fi

echo "Migration done. Verify with: juggernaut doctor"
