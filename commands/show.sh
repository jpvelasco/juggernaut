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

Displays the current Juggernaut block, the effective user/project scopes, and
shell fallback details when present.
EOF
      exit 0
      ;;
    *)
      ;;
  esac
done

if [[ "$v2_active" != "1" ]]; then
  echo "show: v2 is not active yet. Run ./setup --v2 or set JUGGERNAUT_USE_V2=1 to continue." >&2
  exit 0
fi

. "$SCRIPT_DIR/lib/config_manager.sh"

if ! effective_json="$(config_load_effective)"; then
  echo "show: failed to load effective settings" >&2
  exit 1
fi

show_bool() {
  case "$1" in
    true) echo "yes" ;;
    false) echo "no" ;;
    *) echo "—" ;;
  esac
}

show_kv() {
  local indent="$1"
  local label="$2"
  local value="$3"
  printf '%*s%-20s %s\n' "$indent" "" "$label" "$value"
}

show_block_view() {
  local block="$1"
  printf '%s' "$block" | jq -r '[
    .meta.scope // "",
    .meta.version // "",
    .auth.mode // "",
    .auth.region // "",
    .auth.storage // "",
    .model // "",
    .effortLevel // "",
    (.opusplan|tostring),
    (.useMantle|tostring),
    (.mantle.baseUrl // ""),
    (.meta.lastUpdated // "")
  ] | @tsv'
}

show_current_block() {
  local path="$1"
  local block="$2"
  echo "Current Juggernaut block"
  if [[ -z "$block" || "$block" == "null" ]]; then
    show_kv 2 "Status" "not present"
    return 0
  fi

  local scope version auth_mode region storage model effort opusplan use_mantle mantle_url last_updated
  IFS=$'\t' read -r scope version auth_mode region storage model effort opusplan use_mantle mantle_url last_updated <<EOF
$(show_block_view "$block")
EOF

  show_kv 2 "Source" "$path"
  show_kv 2 "Scope" "$(show_text "${scope:-}")"
  show_kv 2 "Version" "$(show_text "${version:-}")"
  show_kv 2 "Auth mode" "$(show_text "${auth_mode:-}")"
  show_kv 2 "Region" "$(show_text "${region:-}")"
  show_kv 2 "Storage" "$(show_text "${storage:-}")"
  show_kv 2 "Model" "$(show_text "${model:-}")"
  show_kv 2 "Effort level" "$(show_text "${effort:-}")"
  show_kv 2 "Opus plan" "$(show_bool "${opusplan:-}")"
  show_kv 2 "Mantle" "$(show_bool "${use_mantle:-}")"
  if [[ -n "$mantle_url" ]]; then
    show_kv 2 "Mantle URL" "$mantle_url"
  fi
  if [[ -n "$last_updated" ]]; then
    show_kv 2 "Last updated" "$last_updated"
  fi
}

show_text() {
  local value="${1:-}"
  if [[ -n "$value" && "$value" != "null" ]]; then
    echo "$value"
  else
    echo "—"
  fi
}

show_scope_section() {
  local scope="$1"
  local path="$2"
  local block="$3"
  local auth_mode region storage model effort use_mantle
  echo "  $scope"
  if [[ -z "$block" || "$block" == "null" ]]; then
    show_kv 4 "Status" "not present"
    return 0
  fi

  IFS=$'\t' read -r auth_mode region storage model effort use_mantle <<EOF
$(printf '%s' "$block" | jq -r '[.auth.mode // "", .auth.region // "", .auth.storage // "", .model // "", .effortLevel // "", (.useMantle|tostring)] | @tsv')
EOF

  show_kv 4 "Source" "$path"
  show_kv 4 "Auth mode" "$(show_text "${auth_mode:-}")"
  show_kv 4 "Region" "$(show_text "${region:-}")"
  show_kv 4 "Storage" "$(show_text "${storage:-}")"
  show_kv 4 "Model" "$(show_text "${model:-}")"
  show_kv 4 "Effort level" "$(show_text "${effort:-}")"
  show_kv 4 "Mantle" "$(show_bool "${use_mantle:-}")"
}

show_shell_fallback() {
  local block="$1"
  if [[ -z "$block" || "$block" == "null" ]]; then
    return 0
  fi

  local enabled mode count
  IFS=$'\t' read -r enabled mode count <<EOF
$(printf '%s' "$block" | jq -r '[
  (.shellFallback.enabled|tostring),
  (.shellFallback.mode // ""),
  ((.shellFallback.lastWrittenProfiles // []) | length | tostring)
] | @tsv')
EOF

  if [[ "$enabled" != "true" && "$count" == "0" ]]; then
    return 0
  fi

  echo "Shell fallback"
  show_kv 2 "Enabled" "$(show_bool "${enabled:-}")"
  show_kv 2 "Mode" "$(show_text "${mode:-}")"
  if [[ "$count" == "0" ]]; then
    show_kv 2 "Last written profiles" "none"
    return 0
  fi

  show_kv 2 "Last written profiles" "${count} item(s)"
  printf '%s' "$block" | jq -r '.shellFallback.lastWrittenProfiles[]?' | while IFS= read -r profile; do
    [[ -n "$profile" ]] && show_kv 4 "-" "$profile"
  done
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
show_current_block "$active_path" "$active_block"
echo
echo "Effective config"
show_scope_section "User scope" "$user_path" "$user_block"
if [[ "$project_json" != "null" ]]; then
  show_scope_section "Project scope" "$project_path" "$project_block"
fi
if [[ "$active_block" != "null" && -n "$active_block" ]]; then
  echo
  show_shell_fallback "$active_block"
fi
