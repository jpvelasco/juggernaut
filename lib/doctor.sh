#!/usr/bin/env bash
# lib/doctor.sh - read-only diagnostics for Juggernaut v2.

set -euo pipefail

DOCTOR_FAILS=0
DOCTOR_WARNS=0
DOCTOR_INDENT=2

doctor_home_path() {
  local path="${1:-}"
  local home="${HOME:-}"
  if [[ -n "$home" && "$path" == "$home"* ]]; then
    local suffix="${path#"$home"}"
    suffix="${suffix//\\//}"
    printf '~%s\n' "$suffix"
  else
    printf '%s\n' "$path"
  fi
}

doctor_text() {
  local value="${1:-}"
  if [[ -n "$value" && "$value" != "null" ]]; then
    printf '%s\n' "$value"
  else
    printf '%s\n' "-"
  fi
}

doctor_status() {
  local status="$1" label="$2" detail="${3:-}"
  local indent="${DOCTOR_INDENT:-2}"
  case "$status" in
    FAIL) DOCTOR_FAILS=$((DOCTOR_FAILS + 1)) ;;
    WARN) DOCTOR_WARNS=$((DOCTOR_WARNS + 1)) ;;
  esac
  if [[ -n "$detail" ]]; then
    printf '%*s%-5s %s: %s\n' "$indent" "" "$status" "$label" "$detail"
  else
    printf '%*s%-5s %s\n' "$indent" "" "$status" "$label"
  fi
}

doctor_subsection() {
  printf '  %s\n' "$1"
}

doctor_bool_state() {
  case "${1:-}" in
    true) echo "enabled" ;;
    false) echo "disabled" ;;
    *) echo "-" ;;
  esac
}

doctor_scope_title() {
  local scope="$1" active="$2" selected="$3"
  local label
  label="$(tr '[:lower:]' '[:upper:]' <<<"${scope:0:1}")${scope:1} Scope"
  if [[ "$active" == "true" && "$selected" == "true" ]]; then
    printf '%s (active, selected)\n' "$label"
  elif [[ "$active" == "true" ]]; then
    printf '%s (active)\n' "$label"
  elif [[ "$selected" == "true" ]]; then
    printf '%s (selected)\n' "$label"
  else
    printf '%s\n' "$label"
  fi
}

doctor_profile_path() {
  local shell_name
  shell_name="$(basename -- "${SHELL:-bash}")"
  profile_writer_detect_shell_config_path "$shell_name"
}

doctor_shell_value() {
  local profile="$1" key="$2"
  [[ -f "$profile" ]] || return 1
  awk -v begin="$PROFILE_WRITER_BEGIN_MARKER" -v end="$PROFILE_WRITER_END_MARKER" '
    $0 == begin { in_block=1; next }
    $0 == end { in_block=0; next }
    in_block { print }
  ' "$profile" | awk -v key="$key" '
    $1 == "export" {
      prefix = key "="
      if (index($2, prefix) == 1) {
        value = substr($2, length(prefix) + 1)
        gsub(/^"/, "", value)
        gsub(/"$/, "", value)
        print value
        exit
      }
    }
    $1 == "set" && $2 == "-gx" && $3 == key {
      value = $4
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
  '
}

doctor_shell_has_key_assignment() {
  local profile="$1" key="$2"
  [[ -f "$profile" ]] || return 1
  awk -v begin="$PROFILE_WRITER_BEGIN_MARKER" -v end="$PROFILE_WRITER_END_MARKER" '
    $0 == begin { in_block=1; next }
    $0 == end { in_block=0; next }
    in_block { print }
  ' "$profile" | grep -Eq "(export[[:space:]]+$key=|set[[:space:]]+-gx[[:space:]]+$key[[:space:]])"
}

doctor_check_native_drift() {
  local settings="$1" block="$2"
  local expected
  expected="$(schema_derive_native_keys "$block")"
  if jq -e --argjson settings "$settings" --argjson expected "$expected" '
    (($settings.model // null) == ($expected.model // null))
    and (($settings.modelOverrides // {}) == ($expected.modelOverrides // {}))
    and (all((($expected.env // {}) | keys[]); (($settings.env // {})[.] // null) == $expected.env[.]))
  ' >/dev/null; then
    doctor_status OK "settings native keys" "match juggernaut block"
  else
    doctor_status WARN "settings native keys" "differ from juggernaut block"
  fi
}

doctor_check_shell_drift() {
  local block="$1" profile="$2"
  local enabled mode
  enabled="$(jq -r '.shellFallback.enabled // false' <<<"$block")"
  mode="$(jq -r '.shellFallback.mode // "both"' <<<"$block")"

  if [[ "$enabled" != "true" || "$mode" == "settings-only" ]]; then
    doctor_status OK "shell fallback" "not required for this scope"
    return 0
  fi

  if [[ -z "$profile" || ! -f "$profile" ]] || ! profile_writer_has_block "$profile"; then
    doctor_status WARN "shell fallback" "expected but not found"
    return 0
  fi

  local mismatches=0 key expected actual
  for key in AWS_REGION ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL CLAUDE_CODE_USE_MANTLE ANTHROPIC_BEDROCK_MANTLE_BASE_URL; do
    expected="$(jq -r --arg key "$key" '.env[$key] // ""' <<<"$block")"
    [[ -z "$expected" ]] && continue
    actual="$(doctor_shell_value "$profile" "$key" || true)"
    if [[ "$actual" != "$expected" ]]; then
      mismatches=$((mismatches + 1))
    fi
  done

  if (( mismatches == 0 )); then
    doctor_status OK "shell fallback" "$(doctor_home_path "$profile") matches settings.json"
  else
    doctor_status WARN "shell fallback" "$(doctor_home_path "$profile") has $mismatches differing value(s)"
  fi
}

doctor_check_auth() {
  local block="$1" profile="$2"
  local auth_mode storage
  auth_mode="$(jq -r '.auth.mode // ""' <<<"$block")"
  storage="$(jq -r '.auth.storage // ""' <<<"$block")"

  case "$auth_mode" in
    iam)
      doctor_status OK "auth mode" "iam"
      if [[ -n "${AWS_PROFILE:-}" ]]; then
        doctor_status OK "IAM env" "AWS_PROFILE is set"
      elif [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
        doctor_status OK "IAM env" "access key variables are set"
      else
        doctor_status WARN "IAM env" "no IAM environment variables detected"
      fi
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_status WARN "API key env" "AWS_BEARER_TOKEN_BEDROCK is set while auth mode is iam"
      fi
      ;;
    api-key)
      doctor_status OK "auth mode" "api-key"
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_status OK "API key source" "AWS_BEARER_TOKEN_BEDROCK is set"
      elif [[ "$storage" == "keychain" ]] && keychain_available 2>/dev/null && [[ -n "$(keychain_get 2>/dev/null || true)" ]]; then
        doctor_status OK "API key source" "keychain entry present"
      elif [[ -n "$profile" ]] && doctor_shell_has_key_assignment "$profile" "AWS_BEARER_TOKEN_BEDROCK"; then
        doctor_status OK "API key source" "shell fallback contains API key assignment"
      else
        doctor_status FAIL "API key source" "no API key found in env, keychain, or shell fallback"
      fi
      ;;
    *)
      doctor_status FAIL "auth mode" "missing or unsupported"
      ;;
  esac
}

doctor_check_region_models_mantle() {
  local block="$1"
  local region model effort use_mantle mantle_url
  region="$(jq -r '.auth.region // ""' <<<"$block")"
  model="$(jq -r '.model // ""' <<<"$block")"
  effort="$(jq -r '.effortLevel // ""' <<<"$block")"
  use_mantle="$(jq -r '.useMantle // false' <<<"$block")"
  mantle_url="$(jq -r '.mantle.baseUrl // ""' <<<"$block")"

  if [[ -n "$region" ]] && schema_is_supported_region "$region"; then
    doctor_status OK "region" "$region"
  else
    doctor_status FAIL "region" "$(doctor_text "$region") is not supported"
  fi

  if [[ -n "$model" ]]; then
    doctor_status OK "model" "$model"
  else
    doctor_status FAIL "model" "missing"
  fi

  if jq -e '.modelOverrides.opus and .modelOverrides.sonnet and .modelOverrides.haiku and .modelOverrides.subagent' <<<"$block" >/dev/null; then
    doctor_status OK "overrides" "opus, sonnet, haiku, subagent present"
  else
    doctor_status WARN "overrides" "one or more model overrides are missing"
  fi

  doctor_status OK "effort" "$(doctor_text "$effort")"
  doctor_status OK "mantle" "$(doctor_bool_state "$use_mantle")"
  if [[ "$use_mantle" == "true" ]]; then
    if [[ "$(jq -r '.env.CLAUDE_CODE_USE_MANTLE // ""' <<<"$block")" == "1" ]]; then
      doctor_status OK "mantle env" "CLAUDE_CODE_USE_MANTLE=1"
    else
      doctor_status WARN "mantle env" "missing CLAUDE_CODE_USE_MANTLE=1"
    fi
  fi
  if [[ "$use_mantle" == "true" && -n "$mantle_url" ]]; then
    doctor_status INFO "mantle URL" "$mantle_url"
  fi
}

doctor_check_scope() {
  local scope="$1" path="$2" settings="$3" active="$4" selected="$5" profile="$6"
  doctor_scope_title "$scope" "$active" "$selected"
  DOCTOR_INDENT=2
  doctor_status INFO "path" "$(doctor_home_path "$path")"

  if [[ ! -f "$path" ]]; then
    doctor_status INFO "settings.json" "not found"
    return 0
  fi

  doctor_subsection "Settings"
  DOCTOR_INDENT=4
  if [[ -z "$settings" ]]; then
    doctor_status FAIL "settings.json" "not valid JSON"
    DOCTOR_INDENT=2
    return 0
  fi
  doctor_status OK "settings.json" "valid JSON"

  if ! config_has_juggernaut_block "$settings"; then
    doctor_status WARN "juggernaut block" "missing"
    DOCTOR_INDENT=2
    return 0
  fi

  local block
  block="$(config_get_juggernaut_block "$settings")"
  if schema_validate "$block" >/dev/null 2>&1; then
    doctor_status OK "juggernaut block" "present, schema valid"
  else
    doctor_status FAIL "juggernaut block" "present, schema invalid"
  fi

  doctor_subsection "Configuration"
  DOCTOR_INDENT=4
  doctor_check_region_models_mantle "$block"

  doctor_subsection "Auth"
  DOCTOR_INDENT=4
  doctor_check_auth "$block" "$profile"

  doctor_subsection "Drift"
  DOCTOR_INDENT=4
  doctor_check_native_drift "$settings" "$block"
  doctor_check_shell_drift "$block" "$profile"
  DOCTOR_INDENT=2
}

doctor_summary() {
  echo "Summary"
  if (( DOCTOR_FAILS > 0 )); then
    doctor_status INFO "result" "$DOCTOR_FAILS failure(s), $DOCTOR_WARNS warning(s)"
    echo "  Next  Fix the failed checks above, then rerun doctor."
    return 1
  fi
  if (( DOCTOR_WARNS > 0 )); then
    doctor_status INFO "result" "$DOCTOR_WARNS warning(s)"
    echo "  Next  Review the warnings above; rerun apply with --scope=user or --scope=project if you want to refresh a scope."
    return 0
  fi
  doctor_status INFO "result" "No issues found"
  return 0
}
