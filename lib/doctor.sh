#!/usr/bin/env bash
# lib/doctor.sh - read-only diagnostics for Juggernaut v3.

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
  local block="$1"
  local auth_mode storage probe_value probe_rc probe_error probe_source
  auth_mode="$(jq -r '.auth.mode // ""' <<<"$block")"
  storage="$(jq -r '.auth.storage // ""' <<<"$block")"
  [[ "$auth_mode" == "api-key" ]] && auth_mode="bedrock-api-key"
  probe_value=""
  probe_rc=1
  probe_error=""
  probe_source=""
  if [[ "$auth_mode" == "bedrock-api-key" ]]; then
    # Try DPAPI first (Windows long-key case), then keychain. Label the hit.
    # Capture exit code separately from `|| true` so we can distinguish
    # "not found" (1) from real errors (2).
    local dpapi_out dpapi_rc
    dpapi_out="$({ dpapi_get; printf '\x1e%s' "$?"; } 2>&1)"
    dpapi_rc="${dpapi_out##*$'\x1e'}"
    dpapi_out="${dpapi_out%$'\x1e'*}"
    if [[ "$dpapi_rc" -eq 0 && -n "$dpapi_out" ]]; then
      probe_value="$dpapi_out"
      probe_source="DPAPI file"
      probe_rc=0
    elif [[ "$dpapi_rc" -eq 2 ]]; then
      probe_error="$dpapi_out"
    fi
    if [[ -z "$probe_value" ]] && keychain_available 2>/dev/null; then
      probe_value="$({ keychain_get; printf '\x1e%s' "$?"; } 2>&1)"
      probe_rc="${probe_value##*$'\x1e'}"
      probe_value="${probe_value%$'\x1e'*}"
      if [[ "$probe_rc" -eq 0 && -n "$probe_value" ]]; then
        probe_source="system keychain"
      elif [[ "$probe_rc" -ne 0 && "$probe_rc" -ne 1 ]]; then
        probe_error="${probe_value%$'\n'}"
        probe_value=""
      else
        probe_value=""
      fi
    fi
    if [[ -z "$probe_value" ]]; then
      probe_value="$({ profile_token_get; printf '\x1e%s' "$?"; } 2>&1)"
      probe_rc="${probe_value##*$'\x1e'}"
      probe_value="${probe_value%$'\x1e'*}"
      if [[ "$probe_rc" -eq 0 && -n "$probe_value" ]]; then
        probe_source="profile token file"
      elif [[ "$probe_rc" -ne 0 && "$probe_rc" -ne 1 ]]; then
        probe_error="${probe_value%$'\n'}"
        probe_value=""
      else
        probe_value=""
      fi
    fi
    probe_error="${probe_error%$'\n'}"
  fi
  case "$auth_mode" in
    iam)
      doctor_kv "Auth" "IAM"
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_warn
        doctor_kv "Details" "AWS_BEARER_TOKEN_BEDROCK is set but auth mode is 'iam' — possible misconfiguration"
        doctor_kv "Fix" "run: juggernaut apply --auth=bedrock-api-key"
      elif [[ -n "${AWS_PROFILE:-}" ]]; then
        doctor_ok
        doctor_kv "Details" "AWS_PROFILE is set"
      elif [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
        doctor_ok
        doctor_kv "Details" "access key variables are set"
      else
        doctor_warn
        doctor_kv "Details" "no IAM credentials in environment"
      fi
      ;;
    bedrock-api-key)
      doctor_kv "Auth" "Bedrock API key"
      if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
        doctor_kv "Source" "AWS_BEARER_TOKEN_BEDROCK"
        doctor_ok
      elif [[ -n "$probe_value" ]]; then
        doctor_kv "Source" "$probe_source"
        doctor_kv "Storage" "$storage"
        doctor_ok
      else
        if [[ -n "$probe_error" ]]; then
          DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
          doctor_kv "Keychain/DPAPI" "WARN ($probe_error)"
        fi
        doctor_fail
        doctor_kv "Details" "no API key found in env, keychain, DPAPI file, or profile token file"
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
  local use_mantle mantle_url
  use_mantle="$(jq -r '.useMantle // false' <<<"$block")"
  mantle_url="$(jq -r '.mantle.baseUrl // ""' <<<"$block")"
  if [[ "$use_mantle" != "true" ]]; then
    doctor_kv "Status" "disabled (INFO)"
    return 0
  fi
  doctor_kv "Status" "enabled"
  [[ -n "$mantle_url" ]] && doctor_kv "URL" "$mantle_url"
  if [[ "$(jq -r '.env.CLAUDE_CODE_USE_MANTLE // ""' <<<"$block")" != "1" ]]; then
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Warning" "CLAUDE_CODE_USE_MANTLE=1 missing from env"
  fi
}

doctor_top_level_model() {
  local settings="$1"
  local settings_model
  settings_model="$(jq -r '.model // ""' <<<"$settings")"
  if [[ "$settings_model" == "opusplan" ]]; then
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Top-level model" 'WARN ("opusplan" is not a Bedrock model ID)'
    doctor_kv "Details" "Claude Code will send this to Bedrock and hang on retries"
    doctor_kv "Fix" "run: juggernaut apply"
  fi
}

doctor_opusplan() {
  local settings="$1" block="$2"
  local opusplan_on settings_model env_model
  opusplan_on="$(jq -r '.juggernaut.opusplan // false' <<<"$settings")"
  if [[ "$opusplan_on" != "true" ]]; then
    doctor_kv "Status" "disabled"
    return 0
  fi
  doctor_kv "Status" "enabled"
  # Expected: both .env.ANTHROPIC_MODEL in the block and the top-level .env.ANTHROPIC_MODEL
  # in settings.json should read "opusplan".
  settings_model="$(jq -r '.env.ANTHROPIC_MODEL // ""' <<<"$settings")"
  env_model="$(jq -r '.env.ANTHROPIC_MODEL // ""' <<<"$block")"
  if [[ "$settings_model" == "opusplan" && "$env_model" == "opusplan" ]]; then
    doctor_ok
  else
    DOCTOR_WARNS=$((DOCTOR_WARNS + 1))
    doctor_kv "Warning" "ANTHROPIC_MODEL mismatch (settings.env='$settings_model', block.env='$env_model'; expected 'opusplan')"
    doctor_kv "Fix" "run: juggernaut apply --opusplan"
  fi
}

doctor_launcher() {
  local block="$1"
  local use_bedrock auth_mode env_token
  use_bedrock="$(jq -r '.env.CLAUDE_CODE_USE_BEDROCK // ""' <<<"$block")"
  auth_mode="$(jq -r '.auth.mode // ""' <<<"$block")"
  [[ "$auth_mode" == "api-key" ]] && auth_mode="bedrock-api-key"
  env_token="${AWS_BEARER_TOKEN_BEDROCK:-}"

  # Detect the launcher by scanning the user's shell profiles for the
  # marker block. A shell function takes precedence over on-PATH binaries
  # in interactive shells, so this is the installed state we care about.
  local -a launcher_profiles=()
  local _candidate
  for _candidate in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
    if [[ -f "$_candidate" ]] && \
       grep -qE '^# BEGIN: Juggernaut Launcher' "$_candidate" 2>/dev/null; then
      launcher_profiles+=("$_candidate")
    fi
  done
  local has_launcher=false
  [[ ${#launcher_profiles[@]} -gt 0 ]] && has_launcher=true

  # The launcher is only needed for bedrock-api-key auth (injects a bearer
  # token from the OS keychain). IAM auth signs requests with AWS_PROFILE or
  # access keys and never reads AWS_BEARER_TOKEN_BEDROCK.
  if [[ "$auth_mode" != "bedrock-api-key" ]]; then
    doctor_kv "Status" "not applicable (IAM auth does not use a bearer token)"
    return 0
  fi
  # Guard against a config where api-key is declared but CLAUDE_CODE_USE_BEDROCK
  # was not written into env (would indicate a broken apply).
  if [[ "$use_bedrock" != "1" ]]; then
    doctor_kv "Status" "not applicable (CLAUDE_CODE_USE_BEDROCK not set)"
    return 0
  fi

  if [[ -n "$env_token" ]]; then
    doctor_ok
    doctor_kv "Source" "AWS_BEARER_TOKEN_BEDROCK already in env"
    if [[ "$has_launcher" == "true" ]]; then
      doctor_kv "Launcher" "$(doctor_home_path "${launcher_profiles[0]}") (also installed)"
    fi
    return 0
  fi

  if [[ "$has_launcher" == "true" ]]; then
    doctor_ok
    doctor_kv "Launcher" "$(doctor_home_path "${launcher_profiles[0]}")"
    doctor_kv "Source" "OS keychain via launcher function"
    return 0
  fi

  doctor_warn
  doctor_kv "Launcher" "not installed (no shell profile contains the launcher block)"
  doctor_kv "Details" "claude will hang on launch — no bearer token in env and no launcher to inject it"
  doctor_kv "Fix" "re-run the installer (install.sh) or set AWS_BEARER_TOKEN_BEDROCK in the environment"
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
