#!/usr/bin/env bash
# lib/schema.sh — Juggernaut v2 block schema: construction, validation, derivation of native settings.json keys.
# Pure functions. Requires: bash 4+, jq.

set -euo pipefail

JUGGERNAUT_SCHEMA_VERSION=1

schema_default_region() { echo "us-east-1"; }

schema_supported_regions() {
  jq -r '.regions[]' "${BEDROCK_CONFIG_PATH:-bedrock-config.json}"
}

schema_is_supported_region() {
  local region="$1"
  schema_supported_regions | grep -Fxq -- "$region"
}

schema_default_env_from_bedrock_config() {
  jq -c '.environment' "${BEDROCK_CONFIG_PATH:-bedrock-config.json}"
}

# schema_new_juggernaut_block
# Builds a fresh juggernaut block from flags/detected state.
# Inputs via env vars (caller sets): J_PROVIDER, J_USE_MANTLE, J_MANTLE_BASE_URL,
#   J_MODEL, J_OPUS_MODEL, J_SONNET_MODEL, J_HAIKU_MODEL, J_SUBAGENT_MODEL,
#   J_USE_1M, J_OPUSPLAN, J_EFFORT, J_AUTH_MODE, J_STORAGE, J_REGION,
#   J_SHELL_FALLBACK_MODE ("both"|"settings-only"|"shell-only"), J_SCOPE ("user"|"project")
# Outputs: JSON object (juggernaut block) to stdout.
schema_new_juggernaut_block() {
  local now
  now="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  local bedrock_env
  bedrock_env="$(schema_default_env_from_bedrock_config)"

  local provider="${J_PROVIDER:-bedrock}"
  local use_mantle="${J_USE_MANTLE:-true}"
  local mantle_base_url="${J_MANTLE_BASE_URL:-}"
  local model="${J_MODEL:-global.anthropic.claude-sonnet-4-6}"
  local opus_model="${J_OPUS_MODEL:-global.anthropic.claude-opus-4-7[1m]}"
  local sonnet_model="${J_SONNET_MODEL:-global.anthropic.claude-sonnet-4-6}"
  local haiku_model="${J_HAIKU_MODEL:-global.anthropic.claude-haiku-4-5-20251001-v1:0}"
  local subagent_model="${J_SUBAGENT_MODEL:-$haiku_model}"
  local use_1m="${J_USE_1M:-true}"
  local opusplan="${J_OPUSPLAN:-false}"
  local effort="${J_EFFORT:-xhigh}"
  local auth_mode="${J_AUTH_MODE:-iam}"
  local storage="${J_STORAGE:-keychain}"
  local region="${J_REGION:-$(schema_default_region)}"
  local shell_mode="${J_SHELL_FALLBACK_MODE:-both}"
  local scope="${J_SCOPE:-user}"
  local version="${J_VERSION:-2.0.0}"

  # Assemble env map: start from bedrock-config.json defaults, then overlay.
  local env_json
  env_json="$(jq -n \
    --argjson base "$bedrock_env" \
    --arg region "$region" \
    --arg model "$model" \
    --arg opus "$opus_model" \
    --arg sonnet "$sonnet_model" \
    --arg haiku "$haiku_model" \
    --arg subagent "$subagent_model" \
    --arg effort "$effort" \
    --argjson opusplan "$opusplan" \
    --argjson use_mantle "$use_mantle" \
    --arg mantle_base_url "$mantle_base_url" \
    '
    $base
    + { AWS_REGION: $region }
    + { ANTHROPIC_MODEL: (if $opusplan then "opusplan" else $model end) }
    + { ANTHROPIC_DEFAULT_OPUS_MODEL: $opus,
        ANTHROPIC_DEFAULT_SONNET_MODEL: $sonnet,
        ANTHROPIC_DEFAULT_HAIKU_MODEL: $haiku,
        CLAUDE_CODE_SUBAGENT_MODEL: $subagent,
        CLAUDE_CODE_EFFORT_LEVEL: $effort }
    + (if $use_mantle then { CLAUDE_CODE_USE_MANTLE: "1" } else {} end)
    + (if $use_mantle and ($mantle_base_url | length > 0) then
         { ANTHROPIC_BEDROCK_MANTLE_BASE_URL: $mantle_base_url }
       else {} end)
    ')"

  jq -n \
    --argjson schemaVersion "$JUGGERNAUT_SCHEMA_VERSION" \
    --arg provider "$provider" \
    --argjson useMantle "$use_mantle" \
    --arg mantleBaseUrl "$mantle_base_url" \
    --arg model "$model" \
    --arg opusModel "$opus_model" \
    --arg sonnetModel "$sonnet_model" \
    --arg haikuModel "$haiku_model" \
    --arg subagentModel "$subagent_model" \
    --argjson use1M "$use_1m" \
    --argjson opusplan "$opusplan" \
    --arg effort "$effort" \
    --arg authMode "$auth_mode" \
    --arg storage "$storage" \
    --arg region "$region" \
    --arg shellMode "$shell_mode" \
    --arg scope "$scope" \
    --arg version "$version" \
    --arg now "$now" \
    --argjson env "$env_json" \
    '
    {
      schemaVersion: $schemaVersion,
      provider: $provider,
      useMantle: $useMantle,
      mantle: { baseUrl: (if $mantleBaseUrl | length > 0 then $mantleBaseUrl else null end) },
      model: $model,
      context: { maxContextTokens: 1000000, use1MContext: $use1M },
      auth: { mode: $authMode, region: $region, storage: $storage },
      modelOverrides: {
        opus: $opusModel,
        sonnet: $sonnetModel,
        haiku: $haikuModel,
        subagent: $subagentModel
      },
      effortLevel: $effort,
      opusplan: $opusplan,
      shellFallback: { enabled: ($shellMode != "settings-only"), mode: $shellMode, lastWrittenProfiles: [] },
      env: $env,
      legacyEnv: null,
      meta: {
        managedBy: "juggernaut",
        version: $version,
        scope: $scope,
        lastUpdated: $now,
        detectedClients: []
      }
    }
    '
}

# schema_validate <block_json>
# Exits 0 if block is valid, 1 with error to stderr if not.
schema_validate() {
  local block="$1"
  local errors=""

  # Required top-level fields
  local required=(schemaVersion provider useMantle model context auth modelOverrides effortLevel env meta)
  for field in "${required[@]}"; do
    if ! echo "$block" | jq -e "has(\"$field\")" >/dev/null; then
      errors+="  - missing required field: $field"$'\n'
    fi
  done

  # Enum: auth.mode
  local auth_mode
  auth_mode="$(echo "$block" | jq -r '.auth.mode // ""')"
  case "$auth_mode" in
    iam|api-key) ;;
    *) errors+="  - auth.mode must be 'iam' or 'api-key' (got: '$auth_mode')"$'\n' ;;
  esac

  # Enum: auth.storage
  local storage
  storage="$(echo "$block" | jq -r '.auth.storage // ""')"
  case "$storage" in
    profile|keychain) ;;
    *) errors+="  - auth.storage must be 'profile' or 'keychain' (got: '$storage')"$'\n' ;;
  esac

  # Enum: effortLevel
  local effort
  effort="$(echo "$block" | jq -r '.effortLevel // ""')"
  case "$effort" in
    low|medium|high|xhigh|max) ;;
    *) errors+="  - effortLevel must be one of low|medium|high|xhigh|max (got: '$effort')"$'\n' ;;
  esac

  # Enum: shellFallback.mode
  local shell_mode
  shell_mode="$(echo "$block" | jq -r '.shellFallback.mode // "both"')"
  case "$shell_mode" in
    both|settings-only|shell-only) ;;
    *) errors+="  - shellFallback.mode must be one of both|settings-only|shell-only (got: '$shell_mode')"$'\n' ;;
  esac

  # meta.managedBy must be "juggernaut"
  local managed_by
  managed_by="$(echo "$block" | jq -r '.meta.managedBy // ""')"
  if [[ "$managed_by" != "juggernaut" ]]; then
    errors+="  - meta.managedBy must be 'juggernaut' (got: '$managed_by')"$'\n'
  fi

  # Region must be in supported list
  local region
  region="$(echo "$block" | jq -r '.auth.region // ""')"
  if [[ -n "$region" ]] && ! schema_is_supported_region "$region"; then
    errors+="  - auth.region '$region' is not in bedrock-config.json .regions"$'\n'
  fi

  if [[ -n "$errors" ]]; then
    printf 'Schema validation failed:\n%s' "$errors" >&2
    return 1
  fi
  return 0
}

# schema_derive_native_keys <block_json>
# Returns a JSON object containing the native settings.json keys Claude Code reads.
# Caller merges this into the top-level settings.json object alongside the juggernaut block.
schema_derive_native_keys() {
  local block="$1"
  jq '
    {
      env: .env,
      model: .model,
      modelOverrides: {
        opus: .modelOverrides.opus,
        sonnet: .modelOverrides.sonnet,
        haiku: .modelOverrides.haiku,
        subagent: .modelOverrides.subagent
      }
    }
  ' <<<"$block"
}

# schema_native_key_names
# Lists the top-level keys in settings.json that Juggernaut owns (derived from the block).
# Used during uninstall to scrub only what we wrote.
schema_native_key_names() {
  printf '%s\n' env model modelOverrides availableModels
}
