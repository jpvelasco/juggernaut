#!/usr/bin/env bash
# commands/show.sh - Juggernaut v3 show subcommand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

requested_scope=""
for arg in "$@"; do
  case "$arg" in
    --scope=*) requested_scope="${arg#--scope=}" ;;
    --version|-v)
      cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"
      exit 0
      ;;
    --help|-h)
      cat <<'EOF'
juggernaut show - print the current Juggernaut configuration

Usage: juggernaut show [--scope=user|project]

Displays the current Juggernaut block and the effective user/project scopes.
EOF
      exit 0
      ;;
    *)
      ;;
  esac
done

case "${requested_scope:-}" in
  ""|user|project) ;;
  *) echo "show: --scope must be 'user' or 'project' (got: '$requested_scope')" >&2; exit 1 ;;
esac

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

active_scope=""
if [[ "$project_block" != "null" && -n "$project_block" ]]; then
  active_scope="project"
elif [[ "$user_block" != "null" && -n "$user_block" ]]; then
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
  show_kv 2 "Active Scope" "No Juggernaut block found"
fi
echo
show_scope_config "user" "$user_path" "$user_block" "$([[ "$active_scope" == "user" ]] && echo true || echo false)" "$([[ "${requested_scope:-}" == "user" ]] && echo true || echo false)"
echo
show_scope_config "project" "${project_path:-${PWD}/.claude/settings.json}" "$project_block" "$([[ "$active_scope" == "project" ]] && echo true || echo false)" "$([[ "${requested_scope:-}" == "project" ]] && echo true || echo false)"
