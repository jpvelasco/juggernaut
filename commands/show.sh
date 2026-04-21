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
  echo "Juggernaut v2 is not active. Use --v2 to enable v2 commands." >&2
  exit 0
fi

. "$SCRIPT_DIR/lib/config_manager.sh"
. "$SCRIPT_DIR/lib/profile_writer.sh"

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

show_state() {
  case "$1" in
    true) echo "enabled" ;;
    false) echo "disabled" ;;
    *) echo "—" ;;
  esac
}

show_kv() {
  local indent="$1"
  local label="$2"
  local value="$3"
  printf '%*s%s: %s\n' "$indent" "" "$label" "$value"
}

show_block_view() {
  local block="$1"
  printf '%s' "$block" | jq -r '[
    .meta.scope // "",
    .auth.mode // "",
    .auth.region // "",
    .model // "",
    .effortLevel // "",
    (.opusplan|tostring),
    (.useMantle|tostring)
  ] | @tsv'
}

show_current_block() {
  local block="$1"
  echo "Current Juggernaut Block"
  if [[ -z "$block" || "$block" == "null" ]]; then
    show_kv 0 "Status" "No active Juggernaut block"
    return 0
  fi

  local scope auth_mode region model effort opusplan use_mantle block_view
  block_view="$(show_block_view "$block")"
  IFS=$'\t' read -r scope auth_mode region model effort opusplan use_mantle <<< "$block_view"

  show_kv 0 "Scope" "$(show_text "${scope:-}")"
  show_kv 0 "Auth" "$(show_text "${auth_mode:-}")"
  show_kv 0 "Region" "$(show_text "${region:-}")"
  show_kv 0 "Model" "$(show_text "${model:-}")"
  show_kv 0 "Effort" "$(show_text "${effort:-}")"
  show_kv 0 "Opus Plan" "$(show_state "${opusplan:-}")"
  show_kv 0 "Mantle" "$(show_state "${use_mantle:-}")"
}

show_text() {
  local value="${1:-}"
  if [[ -n "$value" && "$value" != "null" ]]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "—"
  fi
}

show_home_path() {
  local path="$1"
  local home="${HOME:-}"
  if [[ -n "$home" && "$path" == "$home"* ]]; then
    local suffix="${path#"$home"}"
    suffix="${suffix//\\//}"
    printf '~%s\n' "$suffix"
  else
    printf '%s\n' "$path"
  fi
}

show_effective_config() {
  local path="$1"
  local block="$2"
  local region model
  echo "Effective Config"
  printf '%s\n' "$(show_home_path "$path")"
  if [[ -z "$block" || "$block" == "null" ]]; then
    show_kv 0 "Region" "—"
    show_kv 0 "Model" "—"
    return 0
  fi

  local effective_view
  effective_view="$(printf '%s' "$block" | jq -r '[.auth.region // "", .model // ""] | @tsv')"
  IFS=$'\t' read -r region model <<< "$effective_view"

  show_kv 0 "Region" "$(show_text "${region:-}")"
  show_kv 0 "Model" "$(show_text "${model:-}")"
}

show_shell_fallback() {
  local block="$1"
  if [[ -z "$block" || "$block" == "null" ]]; then
    return 0
  fi

  local enabled storage count fallback_view
  fallback_view="$(printf '%s' "$block" | jq -r '[
  (.shellFallback.enabled|tostring),
  (.auth.storage // ""),
  ((.shellFallback.lastWrittenProfiles // []) | length | tostring)
] | @tsv')"
  IFS=$'\t' read -r enabled storage count <<< "$fallback_view"

  if [[ "$enabled" != "true" && "$count" == "0" ]]; then
    return 0
  fi

  local shell_name shell_path
  shell_name="$(basename -- "${SHELL:-bash}")"
  shell_path="$(profile_writer_detect_shell_config_path "$shell_name")"

  echo "Shell Fallback"
  if [[ -n "$shell_path" ]]; then
    show_home_path "$shell_path"
  fi
  show_kv 0 "Present" "$(show_bool "${enabled:-}")"
  show_kv 0 "Storage" "$(show_text "${storage:-}")"
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
  active_path="${project_path:-${PWD}/.claude/settings.json}"
  active_block="$project_block"
elif [[ "$user_block" != "null" && -n "$user_block" ]]; then
  active_path="$user_path"
  active_block="$user_block"
fi

echo "Juggernaut show"
show_current_block "$active_block"
show_effective_config "$active_path" "$active_block"
if [[ "$active_block" != "null" && -n "$active_block" ]]; then
  show_shell_fallback "$active_block"
fi
