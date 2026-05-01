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

# Subcommand safety — must be invoked via the juggernaut dispatcher.
if [[ "${JUGGERNAUT_USE_V2:-1}" != "1" ]]; then
  echo "juggernaut: invoke via the 'juggernaut' dispatcher (or set JUGGERNAUT_USE_V2=1)." >&2
  exit 2
fi

. "$REPO_ROOT/lib/schema.sh"
. "$REPO_ROOT/lib/config_manager.sh"
. "$REPO_ROOT/lib/migrator.sh"

ROLLBACK=false
CLEAN=false
DRY_RUN=false
YES=false
SCOPE="user"

for arg in "$@"; do
  case "$arg" in
    --rollback) ROLLBACK=true ;;
    --clean)    CLEAN=true ;;
    --dry-run)  DRY_RUN=true ;;
    --yes|--force|-f) YES=true ;;
    --project)  SCOPE="project" ;;
    --help|-h)
      cat <<'EOF'
juggernaut migrate - migrate v1 profile block to settings.json

Usage: juggernaut migrate [--dry-run] [--clean] [--rollback] [--project] [--yes]

Options:
  --dry-run   Show what would be done without writing anything
  --clean     Remove profile block(s) after a successful migration
  --rollback  Restore most recent settings.json backup
  --project   Migrate to project scope (./.claude/settings.json)
  --yes       Confirm destructive cleanup prompts
EOF
      exit 0 ;;
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

if [[ "$CLEAN" == true && "$DRY_RUN" != true && "$YES" != true ]]; then
  if [[ -t 0 ]]; then
    read -r -p "Remove migrated v1 profile blocks after migration? [y/N] " _answer
    case "$_answer" in
      y|Y|yes|YES) ;;
      *) echo "migrate: cleanup skipped. Re-run with --clean --yes to confirm." >&2; CLEAN=false ;;
    esac
  else
    echo "migrate: --clean requires confirmation. Re-run with --clean --yes." >&2
    exit 1
  fi
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
