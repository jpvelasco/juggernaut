#!/usr/bin/env bash
# commands/show.sh — Juggernaut v2 show subcommand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

v2_active="${JUGGERNAUT_USE_V2:-0}"
for arg in "$@"; do
  case "$arg" in
    --v2) v2_active=1 ;;
    --version|-v)
      cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"
      exit 0
      ;;
    --help|-h)
      cat <<'EOF'
juggernaut show — print the current Juggernaut configuration

Usage: juggernaut show

Displays the active Juggernaut block plus a side-by-side summary of user and
project scope settings when present.
EOF
      exit 0
      ;;
    *)
      ;;
  esac
done

if [[ "$v2_active" != "1" ]]; then
  echo "show: v2 is not active. Pass --v2 to ./setup or set JUGGERNAUT_USE_V2=1." >&2
  exit 0
fi

. "$SCRIPT_DIR/lib/config_manager.sh"

if ! effective_json="$(config_load_effective)"; then
  echo "show: failed to load effective settings" >&2
  exit 1
fi

show_blank() {
  echo "  not present"
}

show_bool() {
  case "$1" in
    true) echo "yes" ;;
    false) echo "no" ;;
    *) echo "—" ;;
  esac
}

show_block() {
  local title="$1"
  local path="$2"
  local block="$3"
  echo "$title"
  if [[ -z "$block" || "$block" == "null" ]]; then
    echo "  not present"
    return 0
  fi

  local auth_mode region storage model effort use_mantle mantle_url last_updated opusplan
  IFS=$'\t' read -r auth_mode region storage model effort use_mantle mantle_url last_updated opusplan <<EOF
$(printf '%s' "$block" | jq -r '[.auth.mode // "", .auth.region // "", .auth.storage // "", .model // "", .effortLevel // "", (.useMantle|tostring), (.mantle.baseUrl // ""), (.meta.lastUpdated // ""), (.opusplan|tostring)] | @tsv')
EOF

  echo "  File: $path"
  echo "  Auth: ${auth_mode:-—}"
  echo "  Region: ${region:-—}"
  echo "  Storage: ${storage:-—}"
  echo "  Model: ${model:-—}"
  echo "  Effort: ${effort:-—}"
  echo "  Opus plan: ${opusplan:-—}"
  echo "  Mantle: $(show_bool "${use_mantle:-}")"
  if [[ -n "$mantle_url" ]]; then
    echo "  Mantle URL: $mantle_url"
  fi
  if [[ -n "$last_updated" ]]; then
    echo "  Last updated: $last_updated"
  fi
}

show_scope_row() {
  local scope="$1"
  local block="$2"
  local auth_mode region storage model effort use_mantle
  if [[ -z "$block" || "$block" == "null" ]]; then
    printf '  %-8s %-7s %-12s %-9s %-36s %-7s %-7s\n' "$scope" "—" "—" "—" "—" "—" "—"
    return 0
  fi

  IFS=$'\t' read -r auth_mode region storage model effort use_mantle <<EOF
$(printf '%s' "$block" | jq -r '[.auth.mode // "", .auth.region // "", .auth.storage // "", .model // "", .effortLevel // "", (.useMantle|tostring)] | @tsv')
EOF

  printf '  %-8s %-7s %-12s %-9s %-36s %-7s %-7s\n' \
    "$scope" "${auth_mode:-—}" "${region:-—}" "${storage:-—}" "${model:-—}" "${effort:-—}" "$(show_bool "${use_mantle:-}")"
}

user_path="$(config_user_settings_path)"
project_path=""
project_block=""
user_block=""

user_json="$(printf '%s' "$effective_json" | jq -c '.user.settings')"
project_json="$(printf '%s' "$effective_json" | jq -c '.project.settings // null')"

user_block="$(printf '%s' "$user_json" | jq -c '.juggernaut // null')"
if [[ "$project_json" != "null" ]]; then
  project_path="$(printf '%s' "$effective_json" | jq -r '.project.path // ""')"
  project_block="$(printf '%s' "$project_json" | jq -c '.juggernaut // null')"
fi

active_path="—"
active_block="null"
if [[ "$project_block" != "null" && -n "$project_block" ]]; then
  active_path="${project_path:-$PWD/.claude/settings.json}"
  active_block="$project_block"
elif [[ "$user_block" != "null" && -n "$user_block" ]]; then
  active_path="$user_path"
  active_block="$user_block"
fi

echo "Juggernaut show"
echo
show_block "Current block" "$active_path" "$active_block"
echo
echo "Effective config"
printf '  %-8s %-7s %-12s %-9s %-36s %-7s %-7s\n' "Scope" "Auth" "Region" "Storage" "Model" "Effort" "Mantle"
show_scope_row "User" "$user_block"
if [[ "$project_json" != "null" ]]; then
  show_scope_row "Project" "$project_block"
fi
