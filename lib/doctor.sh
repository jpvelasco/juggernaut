#!/usr/bin/env bash
# lib/doctor.sh - read-only diagnostics for Juggernaut v2.

set -euo pipefail

DOCTOR_FAILS=0
DOCTOR_WARNS=0

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
  case "$status" in
    FAIL) DOCTOR_FAILS=$((DOCTOR_FAILS + 1)) ;;
    WARN) DOCTOR_WARNS=$((DOCTOR_WARNS + 1)) ;;
  esac
  if [[ -n "$detail" ]]; then
    printf '  %-5s %s: %s\n' "$status" "$label" "$detail"
  else
    printf '  %-5s %s\n' "$status" "$label"
  fi
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

doctor_check_config() {
  local block="$1"
  local region model effort use_mantle mantle_url model_display

  region="$(jq -r '.auth.region // ""' <<<"$block")"
  model="$(jq -r '.model // ""' <<<"$block")"
  effort="$(jq -r '.effortLevel // ""' <<<"$block")"
  use_mantle="$(jq -r '.useMantle // false' <<<"$block")"
  mantle_url="$(jq -r '.mantle.baseUrl // ""' <<<"$block")"

  if [[ -n "$region" ]] && schema_is_supported_region "$region"; then
    doctor_status OK "region" "$region"
  else
    doctor_status FAIL "region" "$(doctor_text "$region") (unsupported)"
  fi

  if [[ -n "$model" ]]; then
    model_display="${model#global.anthropic.}"
    doctor_status OK "model" "$model_display"
  else
    doctor_status FAIL "model" "missing"
  fi

  if ! jq -e '.modelOverrides.opus and .modelOverrides.sonnet and .modelOverrides.haiku and .modelOverrides.subagent' <<<"$block" >/dev/null; then
    doctor_status WARN "overrides" "one or more model overrides missing"
  fi

  doctor_status OK "effort" "$(doctor_text "$effort")"

  if [[ "$use_mantle" == "true" ]]; then
    if [[ -n "$mantle_url" ]]; then
      doctor_status OK "mantle" "on  ($mantle_url)"
    else
      doctor_status OK "mantle" "on"
    fi
    if [[ "$(jq -r '.env.CLAUDE_CODE_USE_MANTLE // ""' <<<"$block")" != "1" ]]; then
      doctor_status WARN "mantle" "on  (CLAUDE_CODE_USE_MANTLE=1 missing from env)"
    fi
  else
    doctor_status OK "mantle" "off"
  fi
}

doctor_check_auth() {
  local block="$1" profile="$2"
  local auth_mode storage cred_detail
  auth_mode="$(jq -r '.auth.mode // ""' <<<"$block")"
  storage="$(jq -r '.auth.storage // ""' <<<"$block")"

  case "$auth_mode" in
    iam)
      if [[ -n "${AWS_PROFILE:-}" ]]; then
        cred_detail="AWS_PROFILE set"
      elif [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
        cred_detail="access key set"
      else
        cred_detail=""
      fi
      if [[ -n "$cred_detail" ]]; then
        doctor_status OK "auth" "iam  ($cred_detail)"
      else
        doctor_status WARN "auth" "iam  (no credentials in environment)"
      fi
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_status WARN "auth conflict" "AWS_BEARER_TOKEN_BEDROCK set while mode is iam"
      fi
      ;;
    api-key)
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_status OK "auth" "api-key  (env var set)"
      elif [[ "$storage" == "keychain" ]] && keychain_available 2>/dev/null && [[ -n "$(keychain_get 2>/dev/null || true)" ]]; then
        doctor_status OK "auth" "api-key  (keychain)"
      elif [[ -n "$profile" ]] && doctor_shell_has_key_assignment "$profile" "AWS_BEARER_TOKEN_BEDROCK"; then
        doctor_status OK "auth" "api-key  (shell profile)"
      else
        doctor_status FAIL "auth" "api-key  (no key in env, keychain, or profile)"
      fi
      ;;
    *)
      doctor_status FAIL "auth" "missing or unsupported mode"
      ;;
  esac
}

doctor_check_drift() {
  local settings="$1" block="$2" profile="$3"
  local expected

  expected="$(schema_derive_native_keys "$block")"
  if jq -en --argjson settings "$settings" --argjson expected "$expected" '
    (($settings.model // null) == ($expected.model // null))
    and (($settings.modelOverrides // {}) == ($expected.modelOverrides // {}))
    and (all((($expected.env // {}) | keys[]); (($settings.env // {})[.] // null) == $expected.env[.]))
  ' >/dev/null; then
    doctor_status OK "drift" "in sync"
  else
    doctor_status WARN "drift" "native keys differ from juggernaut block"
  fi

  local enabled mode
  enabled="$(jq -r '.shellFallback.enabled // false' <<<"$block")"
  mode="$(jq -r '.shellFallback.mode // "both"' <<<"$block")"

  [[ "$enabled" == "true" && "$mode" != "settings-only" ]] || return 0

  if [[ -z "$profile" || ! -f "$profile" ]] || ! profile_writer_has_block "$profile"; then
    doctor_status WARN "shell" "fallback expected but not found"
    return 0
  fi

  local mismatches=0 key exp_val actual
  for key in AWS_REGION ANTHROPIC_MODEL ANTHROPIC_DEFAULT_OPUS_MODEL ANTHROPIC_DEFAULT_SONNET_MODEL ANTHROPIC_DEFAULT_HAIKU_MODEL CLAUDE_CODE_SUBAGENT_MODEL CLAUDE_CODE_EFFORT_LEVEL CLAUDE_CODE_USE_MANTLE ANTHROPIC_BEDROCK_MANTLE_BASE_URL; do
    exp_val="$(jq -r --arg key "$key" '.env[$key] // ""' <<<"$block")"
    [[ -z "$exp_val" ]] && continue
    actual="$(doctor_shell_value "$profile" "$key" || true)"
    [[ "$actual" != "$exp_val" ]] && mismatches=$((mismatches + 1))
  done

  if (( mismatches == 0 )); then
    doctor_status OK "shell" "$(doctor_home_path "$profile") in sync"
  else
    doctor_status WARN "shell" "$(doctor_home_path "$profile") has $mismatches differing value(s)"
  fi
}

doctor_check_scope() {
  local scope="$1" path="$2" settings="$3" active="$4" selected="$5" profile="$6"
  doctor_scope_title "$scope" "$active" "$selected"
  printf '  %s\n' "$(doctor_home_path "$path")"

  if [[ ! -f "$path" ]]; then
    doctor_status INFO "status" "not found"
    return 0
  fi

  if [[ -z "$settings" ]]; then
    doctor_status FAIL "status" "not valid JSON"
    return 0
  fi

  if ! config_has_juggernaut_block "$settings"; then
    doctor_status WARN "status" "no Juggernaut block found"
    return 0
  fi

  local block
  block="$(config_get_juggernaut_block "$settings")"
  if ! schema_validate "$block" >/dev/null 2>&1; then
    doctor_status FAIL "status" "block schema invalid"
  fi

  doctor_check_config "$block"
  doctor_check_auth "$block" "$profile"
  doctor_check_drift "$settings" "$block" "$profile"
}

doctor_summary() {
  echo
  if (( DOCTOR_FAILS > 0 )); then
    echo "Summary: $DOCTOR_FAILS failure(s), $DOCTOR_WARNS warning(s)"
    echo "  Run 'juggernaut apply' to fix configuration issues."
    return 1
  fi
  if (( DOCTOR_WARNS > 0 )); then
    echo "Summary: $DOCTOR_WARNS warning(s)"
    return 0
  fi
  echo "Summary: no issues found"
  return 0
}
