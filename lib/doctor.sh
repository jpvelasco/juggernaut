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

doctor_section() { printf '\n%s\n' "$1"; }
doctor_kv()      { printf '%s: %s\n' "$1" "$2"; }

doctor_kv_inline() {
  local label="$1" value="$2" status="$3"
  case "$status" in
    FAIL) DOCTOR_FAILS=$((DOCTOR_FAILS + 1)) ;;
    WARN) DOCTOR_WARNS=$((DOCTOR_WARNS + 1)) ;;
  esac
  printf '%s: %s (%s)\n' "$label" "$value" "$status"
}

doctor_ok()   { printf 'Status: OK\n'; }
doctor_warn() { DOCTOR_WARNS=$((DOCTOR_WARNS + 1)); printf 'Status: WARN\n'; }
doctor_fail() { DOCTOR_FAILS=$((DOCTOR_FAILS + 1)); printf 'Status: FAIL\n'; }

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
        gsub(/^"/, "", value); gsub(/"$/, "", value)
        print value; exit
      }
    }
    $1 == "set" && $2 == "-gx" && $3 == key {
      value = $4
      gsub(/^"/, "", value); gsub(/"$/, "", value)
      print value; exit
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

doctor_scope_block() {
  local path="$1" settings="$2"
  printf '%s\n' "$(doctor_home_path "$path")"
  if [[ ! -f "$path" ]]; then
    doctor_kv "Status" "not found"
    return 0
  fi
  if [[ -z "$settings" ]]; then
    doctor_fail
    doctor_kv "Details" "not valid JSON"
    return 0
  fi
  if ! config_has_juggernaut_block "$settings"; then
    doctor_kv "Status" "no Juggernaut config"
    return 0
  fi
  local block
  block="$(config_get_juggernaut_block "$settings")"
  if schema_validate "$block" >/dev/null 2>&1; then
    doctor_ok
    doctor_kv "Juggernaut block" "present and valid"
  else
    doctor_fail
    doctor_kv "Juggernaut block" "present but invalid"
  fi
}

doctor_credentials() {
  local block="$1" profile="$2"
  local auth_mode storage
  auth_mode="$(jq -r '.auth.mode // ""' <<<"$block")"
  storage="$(jq -r '.auth.storage // ""' <<<"$block")"
  [[ "$auth_mode" == "api-key" ]] && auth_mode="bedrock-api-key"
  case "$auth_mode" in
    iam)
      doctor_kv "Auth" "IAM"
      if [[ -n "${AWS_PROFILE:-}" ]]; then
        doctor_ok
        doctor_kv "Details" "AWS_PROFILE is set"
      elif [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
        doctor_ok
        doctor_kv "Details" "access key variables are set"
      else
        doctor_warn
        doctor_kv "Details" "no IAM credentials in environment"
      fi
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_warn
        doctor_kv "Details" "AWS_BEARER_TOKEN_BEDROCK is set but auth mode is 'iam' — possible misconfiguration"
        doctor_kv "Fix" "run: juggernaut apply --v2 (auto-corrects to bedrock-api-key)"
      fi
      ;;
    bedrock-api-key)
      doctor_kv "Auth" "Bedrock API key"
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_kv "Source" "AWS_BEARER_TOKEN_BEDROCK"
        doctor_ok
      elif [[ "$storage" == "keychain" ]] && keychain_available 2>/dev/null && [[ -n "$(keychain_get 2>/dev/null || true)" ]]; then
        doctor_kv "Source" "system keychain"
        doctor_ok
      elif [[ -n "$profile" ]] && doctor_shell_has_key_assignment "$profile" "AWS_BEARER_TOKEN_BEDROCK"; then
        doctor_kv "Source" "shell profile"
        doctor_ok
      else
        doctor_fail
        doctor_kv "Details" "no API key found in env, keychain, or shell profile"
      fi
      ;;
    *)
      doctor_fail
      doctor_kv "Details" "missing or unsupported auth mode"
      ;;
  esac
}

doctor_region_models() {
  local block="$1"
  local region model effort
  region="$(jq -r '.auth.region // ""' <<<"$block")"
  model="$(jq -r '.model // ""' <<<"$block")"
  effort="$(jq -r '.effortLevel // ""' <<<"$block")"
  if [[ -n "$region" ]] && schema_is_supported_region "$region"; then
    doctor_kv_inline "Region" "$region" "OK"
  else
    doctor_kv_inline "Region" "${region:--}" "FAIL"
  fi
  if [[ -n "$model" ]]; then
    doctor_kv_inline "Model" "$model" "OK"
  else
    doctor_kv_inline "Model" "-" "FAIL"
  fi
  doctor_kv "Effort" "${effort:--}"
  if ! jq -e '.modelOverrides.opus and .modelOverrides.sonnet and .modelOverrides.haiku and .modelOverrides.subagent' <<<"$block" >/dev/null; then
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Overrides" "WARN (one or more missing)"
  fi
}

doctor_mantle() {
  local block="$1"
  local use_mantle mantle_url auth_mode
  use_mantle="$(jq -r '.useMantle // false' <<<"$block")"
  mantle_url="$(jq -r '.mantle.baseUrl // ""' <<<"$block")"
  auth_mode="$(jq -r '.auth.mode // ""' <<<"$block")"
  [[ "$auth_mode" == "api-key" ]] && auth_mode="bedrock-api-key"
  if [[ "$use_mantle" != "true" ]]; then
    doctor_kv "Status" "disabled"
    return 0
  fi
  doctor_kv "Status" "enabled"
  if [[ "$auth_mode" == "bedrock-api-key" ]]; then
    doctor_kv "Reason" "Bedrock API key detected"
  fi
  [[ -n "$mantle_url" ]] && doctor_kv "URL" "$mantle_url"
  if [[ "$(jq -r '.env.CLAUDE_CODE_USE_MANTLE // ""' <<<"$block")" != "1" ]]; then
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Warning" "CLAUDE_CODE_USE_MANTLE=1 missing from env"
  fi
}

doctor_drift() {
  local settings="$1" block="$2" profile="$3"
  local expected
  expected="$(schema_derive_native_keys "$block")"
  if jq -en --argjson settings "$settings" --argjson expected "$expected" '
    (($settings.model // null) == ($expected.model // null))
    and (($settings.modelOverrides // {}) == ($expected.modelOverrides // {}))
    and (all((($expected.env // {}) | keys[]); (($settings.env // {})[.] // null) == $expected.env[.]))
  ' >/dev/null; then
    doctor_kv "Settings native keys" "OK (in sync)"
  else
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Settings native keys" "WARN (differ from juggernaut block)"
  fi
  local enabled mode
  enabled="$(jq -r '.shellFallback.enabled // false' <<<"$block")"
  mode="$(jq -r '.shellFallback.mode // "both"' <<<"$block")"
  if [[ "$enabled" != "true" || "$mode" == "settings-only" ]]; then
    doctor_kv "Settings vs Shell Fallback" "OK (no fallback configured)"
    return 0
  fi
  if [[ -z "$profile" || ! -f "$profile" ]] || ! profile_writer_has_block "$profile"; then
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Settings vs Shell Fallback" "WARN (expected but not found)"
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
    doctor_kv "Settings vs Shell Fallback" "OK (no drift detected)"
  else
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Settings vs Shell Fallback" "WARN ($mismatches differing value(s))"
  fi
}

doctor_summary() {
  doctor_section "Summary"
  if (( DOCTOR_FAILS > 0 )); then
    printf 'Status: FAIL\n'
    printf '%d failure(s), %d warning(s)\n' "$DOCTOR_FAILS" "$DOCTOR_WARNS"
    echo "Run 'juggernaut apply' to fix configuration issues."
    return 1
  fi
  if (( DOCTOR_WARNS > 0 )); then
    printf 'Status: WARN\n'
    printf '%d warning(s)\n' "$DOCTOR_WARNS"
    return 0
  fi
  printf 'Status: OK\n'
  echo "No issues found"
  return 0
}
