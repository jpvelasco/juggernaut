#!/usr/bin/env bash
# commands/doctor.sh - Juggernaut v2 doctor subcommand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BEDROCK_CONFIG_PATH="${BEDROCK_CONFIG_PATH:-$SCRIPT_DIR/bedrock-config.json}"
export BEDROCK_CONFIG_PATH

v2_active="${JUGGERNAUT_USE_V2:-0}"
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
juggernaut doctor - check Juggernaut v2 configuration

Usage: juggernaut doctor [--scope=user|project]

Checks user and project settings.json files, credentials, model/region settings,
Mantle status, and drift between settings.json and the shell fallback.
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

case "${requested_scope:-}" in
  ""|user|project) ;;
  *) echo "doctor: --scope must be 'user' or 'project' (got: '$requested_scope')" >&2; exit 1 ;;
esac

. "$SCRIPT_DIR/lib/config_manager.sh"
. "$SCRIPT_DIR/lib/schema.sh"
. "$SCRIPT_DIR/lib/profile_writer.sh"
. "$SCRIPT_DIR/lib/keychain.sh"
. "$SCRIPT_DIR/lib/doctor.sh"

read_settings_or_empty() {
  local path="$1"
  [[ -f "$path" ]] || return 0
  config_read "$path" 2>/dev/null || return 1
}

user_path="$(config_user_settings_path)"
project_path="$(config_project_settings_path "$PWD" 2>/dev/null || true)"
[[ -n "$project_path" ]] || project_path="$PWD/.claude/settings.json"

user_settings=""
project_settings=""
if ! user_settings="$(read_settings_or_empty "$user_path")"; then
  user_settings=""
fi
if ! project_settings="$(read_settings_or_empty "$project_path")"; then
  project_settings=""
fi

user_has_block=false
project_has_block=false
if [[ -n "$user_settings" ]] && config_has_juggernaut_block "$user_settings"; then
  user_has_block=true
fi
if [[ -n "$project_settings" ]] && config_has_juggernaut_block "$project_settings"; then
  project_has_block=true
fi

active_scope=""
if [[ "$project_has_block" == "true" ]]; then
  active_scope="project"
elif [[ "$user_has_block" == "true" ]]; then
  active_scope="user"
fi

profile_path="$(doctor_profile_path)"

printf 'Juggernaut doctor\n'

# ── User Scope ───────────────────────────────────────────────────────────────
doctor_section "User Scope"
doctor_scope_block "$user_path" "$user_settings"

# ── Project Scope ─────────────────────────────────────────────────────────────
doctor_section "Project Scope"
doctor_scope_block "$project_path" "$project_settings"

# ── Active Scope ──────────────────────────────────────────────────────────────
doctor_section "Active Scope"
if [[ -n "$active_scope" ]]; then
  printf '%s\n' "$active_scope"
else
  DOCTOR_FAILS=$((DOCTOR_FAILS + 1))
  printf 'none (no Juggernaut v2 block found)\n'
fi

# Resolve which block to use for the detailed checks below.
# Honour --scope if given; otherwise use the active scope.
check_scope="${requested_scope:-$active_scope}"
check_settings=""
if [[ "$check_scope" == "user" ]]; then
  check_settings="$user_settings"
elif [[ "$check_scope" == "project" ]]; then
  check_settings="$project_settings"
fi

if [[ -n "$check_settings" ]] && config_has_juggernaut_block "$check_settings"; then
  check_block="$(config_get_juggernaut_block "$check_settings")"

  # ── Credentials ─────────────────────────────────────────────────────────────
  doctor_section "Credentials"
  doctor_credentials "$check_block" "$profile_path"

  # ── Region & Models ──────────────────────────────────────────────────────────
  doctor_section "Region & Models"
  doctor_region_models "$check_block"

  # ── Mantle ───────────────────────────────────────────────────────────────────
  doctor_section "Mantle"
  doctor_mantle "$check_block"

  # ── Drift ────────────────────────────────────────────────────────────────────
  doctor_section "Drift"
  doctor_drift "$check_settings" "$check_block" "$profile_path"
fi

doctor_summary
