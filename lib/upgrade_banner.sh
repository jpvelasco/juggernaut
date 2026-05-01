#!/usr/bin/env bash
# lib/upgrade_banner.sh — Upgrade/migration state detection and user-facing banner.
# Sourced by install.sh and the juggernaut dispatcher.
# Requires: lib/profile_paths.sh, lib/migrator.sh, lib/config_manager.sh, jq.

set -euo pipefail

# ---------------------------------------------------------------------------
# upgrade_banner_detect_state [settings_path]
# Emits a JSON object describing the current upgrade/migration state:
#   has_v1         — true if any candidate profile has an active v1 block
#   v1_profiles    — array of paths that have a v1 block
#   has_v2_settings — true if settings_path contains a managed juggernaut block
#   installed_version — version string from $INSTALL_DIR/VERSION (may be "")
#   release_version   — version string from VERSION file next to this script
#   is_upgrade        — true when installed != release and installed is non-empty
#   migration_declined — true if all detected v1 profiles have a MigrationDeclined marker
# ---------------------------------------------------------------------------
upgrade_banner_detect_state() {
  local settings_path="${1:-$HOME/.claude/settings.json}"

  # Detect v1 profile blocks using the canonical candidate list.
  local v1_profiles=()
  local migration_declined=true
  local any_v1=false
  while IFS= read -r candidate; do
    [[ -z "$candidate" ]] && continue
    if [[ -f "$candidate" ]] && tr -d '\r' < "$candidate" 2>/dev/null \
       | grep -q "# BEGIN: Claude Code Bedrock Configuration"; then
      v1_profiles+=("$candidate")
      any_v1=true
      # A profile counts as "active v1" only if MigrationDeclined is absent
      # (unless JUGGERNAUT_FORCE_MIGRATION_PROMPT=1 forces re-detection).
      if [[ "${JUGGERNAUT_FORCE_MIGRATION_PROMPT:-}" == "1" ]] \
         || ! tr -d '\r' < "$candidate" 2>/dev/null | grep -q "^# MigrationDeclined:"; then
        migration_declined=false
      fi
    fi
  done < <(profile_paths_v1_candidates)

  # If no profiles have an active (non-declined) v1 block, treat as no v1.
  local has_v1=false
  if $any_v1 && ! $migration_declined; then
    has_v1=true
  fi

  # Detect v2 settings.json.
  local has_v2_settings=false
  if [[ -s "$settings_path" ]]; then
    local managed
    managed="$(jq -r '.juggernaut.meta.managedBy // ""' "$settings_path" 2>/dev/null || true)"
    [[ "$managed" == "juggernaut" ]] && has_v2_settings=true
  fi

  # Version strings.
  local install_dir="${JUGGERNAUT_DIR:-$HOME/.juggernaut}"
  local installed_version=""
  if [[ -f "$install_dir/VERSION" ]]; then
    installed_version="$(tr -d '\r\n ' < "$install_dir/VERSION" 2>/dev/null || true)"
  fi
  local release_version=""
  local _self_dir
  _self_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local _repo_root
  _repo_root="$(cd "$_self_dir/.." && pwd)"
  if [[ -f "$_repo_root/VERSION" ]]; then
    release_version="$(tr -d '\r\n ' < "$_repo_root/VERSION" 2>/dev/null || true)"
  fi

  local is_upgrade=false
  if [[ -n "$installed_version" && -n "$release_version" && "$installed_version" != "$release_version" ]]; then
    is_upgrade=true
  fi

  # Build JSON output.
  local v1_json
  v1_json="$(printf '%s\n' "${v1_profiles[@]+"${v1_profiles[@]}"}" | jq -Rn '[inputs]')"

  jq -n \
    --argjson has_v1        "$has_v1" \
    --argjson v1_profiles   "$v1_json" \
    --argjson has_v2        "$has_v2_settings" \
    --arg installed         "$installed_version" \
    --arg release           "$release_version" \
    --argjson is_upgrade    "$is_upgrade" \
    --argjson declined      "$migration_declined" \
    '{
      has_v1:              $has_v1,
      v1_profiles:         $v1_profiles,
      has_v2_settings:     $has_v2,
      installed_version:   $installed,
      release_version:     $release,
      is_upgrade:          $is_upgrade,
      migration_declined:  $declined
    }'
}

# ---------------------------------------------------------------------------
# upgrade_banner_print <state_json>
# Prints the banner to stderr. No-op if state does not require a banner.
# ---------------------------------------------------------------------------
upgrade_banner_print() {
  local state="$1"

  local has_v1 is_upgrade installed release
  has_v1="$(printf '%s' "$state" | jq -r '.has_v1')"
  is_upgrade="$(printf '%s' "$state" | jq -r '.is_upgrade')"
  installed="$(printf '%s' "$state" | jq -r '.installed_version')"
  release="$(printf '%s' "$state" | jq -r '.release_version')"

  [[ "$has_v1" == "true" || "$is_upgrade" == "true" ]] || return 0

  printf '\n' >&2
  printf '╔══════════════════════════════════════════════════════════════╗\n' >&2
  if [[ "$is_upgrade" == "true" && -n "$installed" ]]; then
    printf '  Juggernaut: upgrading %s → %s\n' "$installed" "$release" >&2
  else
    printf '  Juggernaut %s\n' "$release" >&2
  fi

  if [[ "$has_v1" == "true" ]]; then
    printf '\n' >&2
    printf '  v1 configuration detected in your shell profile.\n' >&2
    printf '  This release migrates your settings to settings.json (v2).\n' >&2
    printf '\n' >&2
    local profiles
    profiles="$(printf '%s' "$state" | jq -r '.v1_profiles[]')"
    while IFS= read -r p; do
      printf '    %s\n' "$p" >&2
    done <<< "$profiles"
    printf '\n' >&2
    printf '  Continue?  y = migrate to v2   n = exit\n' >&2
    printf '  Keep v1?   pass --legacy-v1 to stay on v1 for now\n' >&2
  fi
  printf '╚══════════════════════════════════════════════════════════════╝\n' >&2
  printf '\n' >&2
}

# ---------------------------------------------------------------------------
# upgrade_banner_confirm <state_json> <yes_flag> <legacy_v1_flag>
# Returns:
#   0 — proceed with v2 migration
#   1 — abort (non-TTY, no --yes or --legacy-v1)
#   2 — proceed in legacy-v1 mode
# ---------------------------------------------------------------------------
upgrade_banner_confirm() {
  local state="$1"
  local yes_flag="${2:-false}"
  local legacy_flag="${3:-false}"

  local has_v1
  has_v1="$(printf '%s' "$state" | jq -r '.has_v1')"

  # No v1 block — nothing to confirm.
  [[ "$has_v1" == "true" ]] || return 0

  # --legacy-v1 accepted explicitly.
  [[ "$legacy_flag" == "true" ]] && return 2

  # --yes accepted explicitly.
  [[ "$yes_flag" == "true" ]] && return 0

  # Non-TTY without explicit flags → abort.
  if [[ "${JUGGERNAUT_NO_TTY_PROMPTS:-0}" == "1" || ! -t 0 ]]; then
    printf 'juggernaut: non-TTY install with v1 configuration detected.\n' >&2
    printf 'Pass --yes to migrate to v2, or --legacy-v1 to keep v1.\n' >&2
    return 1
  fi

  # Interactive prompt.
  local answer
  read -r -p 'Migrate to v2? [Y/n/legacy-v1] ' answer </dev/tty
  case "${answer,,}" in
    ''|y|yes)      return 0 ;;
    legacy*|l)     return 2 ;;
    *)             return 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# upgrade_banner_suppress_sentinel <install_dir> <version>
# Returns 0 if the banner has already been shown for this version (sentinel file exists).
# ---------------------------------------------------------------------------
upgrade_banner_suppress_sentinel() {
  local install_dir="$1"
  local version="$2"
  [[ -f "$install_dir/.juggernaut_banner_seen.$version" ]]
}

# upgrade_banner_mark_seen <install_dir> <version>
upgrade_banner_mark_seen() {
  local install_dir="$1"
  local version="$2"
  touch "$install_dir/.juggernaut_banner_seen.$version" 2>/dev/null || true
}
