#!/usr/bin/env bash
# commands/show.sh — Juggernaut v2 show subcommand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

v2_active="${JUGGERNAUT_USE_V2:-1}"
requested_scope=""
for arg in "$@"; do
  case "$arg" in
    --v2) v2_active=1 ;;
    --scope=*) requested_scope="${arg#--scope=}" ;;
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
  echo "juggernaut: invoke via the 'juggernaut' dispatcher (or set JUGGERNAUT_USE_V2=1)." >&2
  exit 2
fi

case "${requested_scope:-}" in
  ""|user|project) ;;
  *) echo "show: --scope must be 'user' or 'project' (got: '$requested_scope')" >&2; exit 1 ;;
esac

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

show_auth() {
  case "${1:-}" in
    api-key|bedrock-api-key) echo "Bedrock API key" ;;
    iam) echo "IAM" ;;
    *) show_text "${1:-}" ;;
  esac
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
    show_kv 2 "Status" "No active Juggernaut block"
    return 0
  fi

  local scope auth_mode region model effort opusplan use_mantle block_view
  block_view="$(show_block_view "$block")"
  IFS=$'\t' read -r scope auth_mode region model effort opusplan use_mantle <<< "$block_view"

  show_kv 2 "Scope" "$(show_text "${scope:-}")"
  show_kv 2 "Auth" "$(show_auth "${auth_mode:-}")"
  show_kv 2 "Region" "$(show_text "${region:-}")"
  show_kv 2 "Model" "$(show_text "${model:-}")"
  show_kv 2 "Effort" "$(show_text "${effort:-}")"
  show_kv 2 "Opus Plan" "$(show_state "${opusplan:-}")"
  show_kv 2 "Mantle" "$(show_state "${use_mantle:-}")"
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
  printf '  %s\n' "$(show_home_path "$path")"
  if [[ -z "$block" || "$block" == "null" ]]; then
    show_kv 4 "Region" "—"
    show_kv 4 "Model" "—"
    return 0
  fi

  local effective_view
  effective_view="$(printf '%s' "$block" | jq -r '[.auth.region // "", .model // ""] | @tsv')"
  IFS=$'\t' read -r region model <<< "$effective_view"

  show_kv 4 "Region" "$(show_text "${region:-}")"
  show_kv 4 "Model" "$(show_text "${model:-}")"
}

show_scope_config() {
  local scope="$1"
  local path="$2"
  local block="$3"
  local active="$4"
  local selected="$5"
  local title
  title="$(tr '[:lower:]' '[:upper:]' <<<"${scope:0:1}")${scope:1} Scope"
  if [[ "$active" == "true" && "$selected" == "true" ]]; then
    title+=" (active, selected)"
  elif [[ "$active" == "true" ]]; then
    title+=" (active)"
  elif [[ "$selected" == "true" ]]; then
    title+=" (selected)"
  fi

  echo "$title"
  printf '  %s\n' "$(show_home_path "$path")"
  if [[ -z "$block" || "$block" == "null" ]]; then
    show_kv 4 "Status" "No Juggernaut block"
    return 0
  fi

  local scope_meta auth_mode region model effort opusplan use_mantle block_view
  block_view="$(show_block_view "$block")"
  IFS=$'\t' read -r scope_meta auth_mode region model effort opusplan use_mantle <<< "$block_view"

  show_kv 4 "Scope" "$(show_text "${scope_meta:-}")"
  show_kv 4 "Auth" "$(show_auth "${auth_mode:-}")"
  show_kv 4 "Region" "$(show_text "${region:-}")"
  show_kv 4 "Model" "$(show_text "${model:-}")"
  show_kv 4 "Effort" "$(show_text "${effort:-}")"
  show_kv 4 "Opus Plan" "$(show_state "${opusplan:-}")"
  show_kv 4 "Mantle" "$(show_state "${use_mantle:-}")"
}

show_shell_fallback() {
  local block="$1"
  if [[ -z "$block" || "$block" == "null" ]]; then
    return 0
  fi

  local enabled storage fallback_view
  fallback_view="$(printf '%s' "$block" | jq -r '[
  (.shellFallback.enabled|tostring),
  (.auth.storage // "")
] | @tsv')"
  IFS=$'\t' read -r enabled storage <<< "$fallback_view"

  local shell_name shell_path
  shell_name="$(basename -- "${SHELL:-bash}")"
  shell_path="$(profile_writer_detect_shell_config_path "$shell_name")"

  echo "Shell Fallback"
  if [[ -n "$shell_path" ]]; then
    printf '  %s\n' "$(show_home_path "$shell_path")"
  fi
  show_kv 4 "Present" "$(show_bool "${enabled:-}")"
  if [[ "$enabled" == "true" ]]; then
    show_kv 4 "Storage" "$(show_text "${storage:-}")"
  fi
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

active_block="null"
active_scope=""
if [[ "$project_block" != "null" && -n "$project_block" ]]; then
  active_block="$project_block"
  active_scope="project"
elif [[ "$user_block" != "null" && -n "$user_block" ]]; then
  active_block="$user_block"
  active_scope="user"
fi

echo "Juggernaut show"
echo
echo "Scope Awareness"
if [[ -n "$requested_scope" ]]; then
  show_kv 2 "Selected Scope" "$requested_scope"
else
  show_kv 2 "Selected Scope" "not specified"
fi
if [[ -n "$active_scope" ]]; then
  show_kv 2 "Active Scope" "$active_scope takes precedence for this session"
else
  show_kv 2 "Active Scope" "No Juggernaut v2 block found"
fi
echo
show_scope_config "user" "$user_path" "$user_block" "$([[ "$active_scope" == "user" ]] && echo true || echo false)" "$([[ "${requested_scope:-}" == "user" ]] && echo true || echo false)"
echo
show_scope_config "project" "${project_path:-${PWD}/.claude/settings.json}" "$project_block" "$([[ "$active_scope" == "project" ]] && echo true || echo false)" "$([[ "${requested_scope:-}" == "project" ]] && echo true || echo false)"
if [[ "$active_block" != "null" && -n "$active_block" ]]; then
  echo
  show_shell_fallback "$active_block"
fi
