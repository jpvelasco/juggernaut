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

# Feature flag gate — v2 commands are dormant until explicitly enabled.
if [[ "${JUGGERNAUT_USE_V2:-0}" != "1" ]]; then
  echo "Juggernaut v2 is not active. Use --v2 to enable v2 commands." >&2
  exit 0
fi

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
BEDROCK_CONFIG="${BEDROCK_CONFIG_PATH:-$REPO_ROOT/bedrock-config.json}"

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

if [[ ! -f "$BEDROCK_CONFIG" ]]; then
  echo "migrate: bedrock-config.json not found at $BEDROCK_CONFIG" >&2
  exit 1
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

# Portable in-place sed: macOS requires a backup extension argument.
_sed_inplace() {
  if sed --version 2>/dev/null | grep -q GNU; then
    sed -i "$@"
  else
    sed -i '' "$@"
  fi
}

FOUND=0
ERRORS=0
for profile in "${CANDIDATES[@]}"; do
  if migrator_has_v1_block "$profile"; then
    FOUND=$((FOUND + 1))
    echo "Found v1 block: $profile"

    if [[ "$DRY_RUN" == true ]]; then
      echo "[dry-run] Would migrate $profile → $SETTINGS_PATH"
      continue
    fi

    if migrator_run "$profile" "$SETTINGS_PATH" "$BEDROCK_CONFIG"; then
      if [[ "$CLEAN" == true ]]; then
        _sed_inplace '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/d' "$profile"
        echo "Removed v1 block from $profile (--clean)"
      fi
    else
      echo "migrate: failed to migrate $profile" >&2
      ERRORS=$((ERRORS + 1))
    fi
  fi
done

if (( FOUND == 0 )); then
  echo "No v1 profile blocks found. Nothing to migrate."
  exit 0
fi

if (( ERRORS > 0 )); then
  echo "Migration completed with $ERRORS error(s). Check output above." >&2
  exit 1
fi

echo "Migration done. Verify with: juggernaut doctor"
