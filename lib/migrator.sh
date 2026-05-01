#!/usr/bin/env bash
# lib/migrator.sh — v1.7.x profile-block → v2 settings.json migration for Juggernaut.
# Requires: lib/schema.sh, lib/config_manager.sh, bash 4+, jq.

set -euo pipefail

# ---------------------------------------------------------------------------
# Detection
# ---------------------------------------------------------------------------

# migrator_has_v1_block <profile_file>
# Returns 0 if a Juggernaut v1 BEGIN/END marker block is present AND the
# user has not previously declined migration. Normalizes CRLF before
# matching so Windows Git Bash profiles are detected correctly.
migrator_has_v1_block() {
  local file="$1"
  [[ -f "$file" ]] || return 1
  tr -d '\r' < "$file" 2>/dev/null | grep -q "# BEGIN: Claude Code Bedrock Configuration" || return 1
  tr -d '\r' < "$file" 2>/dev/null | grep -q "^# Juggernaut v2 shell fallback$" && return 1
  # Suppress detection if a MigrationDeclined marker is present.
  if [[ "${JUGGERNAUT_FORCE_MIGRATION_PROMPT:-}" != "1" ]] \
     && tr -d '\r' < "$file" 2>/dev/null | grep -q "^# MigrationDeclined:"; then
    return 1
  fi
  return 0
}

# migrator_mark_migration_declined <profile_file>
# Inserts a "# MigrationDeclined: <ISO8601>" comment immediately after the
# BEGIN marker so subsequent apply invocations don't re-prompt. Users can
# re-enable the prompt with JUGGERNAUT_FORCE_MIGRATION_PROMPT=1 or by
# removing the marker line by hand.
migrator_mark_migration_declined() {
  local file="$1"
  [[ -f "$file" ]] || return 1
  # No-op if a decline marker already exists.
  if tr -d '\r' < "$file" | grep -q "^# MigrationDeclined:"; then
    return 0
  fi
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  local tmp
  tmp="$(mktemp)"
  awk -v marker="# MigrationDeclined: $ts" '
    { print }
    /^# BEGIN: Claude Code Bedrock Configuration/ && !done { print marker; done=1 }
  ' "$file" > "$tmp" && mv "$tmp" "$file"
}

# migrator_extract_block <profile_file>
# Prints the raw content between (and including) the v1 BEGIN/END markers.
migrator_extract_block() {
  local file="$1"
  sed -n '/# BEGIN: Claude Code Bedrock Configuration/,/# END: Claude Code Bedrock Configuration/p' "$file"
}

# ---------------------------------------------------------------------------
# Metadata comment parsers — mirror the read-back logic in setup-claude-bedrock.sh
# ---------------------------------------------------------------------------

_migrator_meta() {
  local block="$1" key="$2"
  printf '%s' "$block" | grep "^# ${key}:" | head -1 | sed "s/^# ${key}: //"
}

_migrator_flag() {
  local block="$1" key="$2"
  printf '%s' "$block" | grep -q "^# ${key}: true"
}

# _migrator_export_val <block> <var_name>
# Extracts the value of "export VAR=..." (quoted or unquoted) from block text.
_migrator_export_val() {
  local block="$1" var="$2"
  printf '%s' "$block" | grep "^export ${var}=" | head -1 \
    | sed "s/^export ${var}=[\"']\(.*\)[\"']$/\1/" \
    | sed "s/^export ${var}=\(.*\)$/\1/"
}

# _migrator_fish_val <block> <var_name>
# Extracts the value of "set -gx VAR value" from fish config block text.
_migrator_fish_val() {
  local block="$1" var="$2"
  printf '%s' "$block" | grep "^set -gx ${var} " | head -1 \
    | sed "s/^set -gx ${var} //"
}

# migrator_parse_v1_block <raw_block_text>
# Emits a JSON object with all v1 metadata fields.
# Handles bash/zsh (export VAR=value) and fish (set -gx VAR value) syntax.
migrator_parse_v1_block() {
  local block="$1"

  local auth_mode region model opus_model sonnet_model haiku_model effort_level
  local storage use_1m opusplan

  auth_mode="$(_migrator_meta "$block" "Auth mode")"
  [[ -z "$auth_mode" ]] && auth_mode="iam"
  # If the block *assigns* AWS_BEARER_TOKEN_BEDROCK (export/set -gx, not set -e/unset)
  # it was running in API-key mode regardless of the metadata comment.
  if [[ "$auth_mode" == "iam" ]] && printf '%s' "$block" | grep -qE '(^export AWS_BEARER_TOKEN_BEDROCK=|^set -gx AWS_BEARER_TOKEN_BEDROCK )'; then
    auth_mode="bedrock-api-key"
  fi

  storage="profile"
  if printf '%s' "$block" | grep -q "^# Storage: keychain"; then
    storage="keychain"
  fi

  use_1m="false"
  _migrator_flag "$block" "1MContext" && use_1m="true"

  opusplan="false"
  _migrator_flag "$block" "OpusPlan" && opusplan="true"

  effort_level="$(_migrator_meta "$block" "EffortLevel")"
  [[ -z "$effort_level" ]] && effort_level="xhigh"

  model="$(_migrator_meta "$block" "Model")"
  opus_model="$(_migrator_meta "$block" "OpusModel")"
  sonnet_model="$(_migrator_meta "$block" "SonnetModel")"
  haiku_model="$(_migrator_meta "$block" "HaikuModel")"

  # Fall back to export lines, then fish set -gx lines, for model IDs when
  # metadata comments are absent (older v1 blocks / fish profiles).
  if [[ -z "$model" ]]; then
    model="$(_migrator_export_val "$block" "ANTHROPIC_MODEL")"
    [[ -z "$model" ]] && model="$(_migrator_fish_val "$block" "ANTHROPIC_MODEL")"
    [[ "$model" == "opusplan" ]] && model=""
  fi
  if [[ -z "$opus_model" ]]; then
    opus_model="$(_migrator_export_val "$block" "ANTHROPIC_DEFAULT_OPUS_MODEL")"
    [[ -z "$opus_model" ]] && opus_model="$(_migrator_fish_val "$block" "ANTHROPIC_DEFAULT_OPUS_MODEL")"
  fi
  if [[ -z "$sonnet_model" ]]; then
    sonnet_model="$(_migrator_export_val "$block" "ANTHROPIC_DEFAULT_SONNET_MODEL")"
    [[ -z "$sonnet_model" ]] && sonnet_model="$(_migrator_fish_val "$block" "ANTHROPIC_DEFAULT_SONNET_MODEL")"
  fi
  if [[ -z "$haiku_model" ]]; then
    haiku_model="$(_migrator_export_val "$block" "ANTHROPIC_DEFAULT_HAIKU_MODEL")"
    [[ -z "$haiku_model" ]] && haiku_model="$(_migrator_fish_val "$block" "ANTHROPIC_DEFAULT_HAIKU_MODEL")"
  fi

  # Region: export line first, fish set -gx fallback, then hardcoded default.
  # auth.region is the single source of truth in v2; parse it from AWS_REGION.
  region="$(_migrator_export_val "$block" "AWS_REGION")"
  [[ -z "$region" ]] && region="$(_migrator_fish_val "$block" "AWS_REGION")"
  [[ -z "$region" ]] && region="us-east-1"

  # Snapshot all variable-setting lines for legacyEnv (export and fish set -gx).
  local legacy_env
  legacy_env="$(
    {
      # bash/zsh: strip "export " prefix, keep KEY=VALUE
      printf '%s' "$block" | grep "^export [A-Z_][A-Z0-9_]*=" | sed 's/^export //'
      # fish: convert "set -gx KEY VALUE" → "KEY=VALUE"
      printf '%s' "$block" | grep "^set -gx [A-Z_][A-Z0-9_]* " \
        | sed 's/^set -gx \([A-Z_][A-Z0-9_]*\) \(.*\)$/\1=\2/'
    } | jq -Rn '[inputs | capture("^(?<k>[^=]+)=(?<v>.*)$")] | map({(.k): .v}) | add // {}'
  )"

  jq -n \
    --arg auth_mode   "$auth_mode" \
    --arg region      "$region" \
    --arg model       "$model" \
    --arg opus_model  "$opus_model" \
    --arg sonnet_model "$sonnet_model" \
    --arg haiku_model "$haiku_model" \
    --arg effort_level "$effort_level" \
    --arg storage     "$storage" \
    --argjson use_1m  "$use_1m" \
    --argjson opusplan "$opusplan" \
    --argjson legacy_env "$legacy_env" \
    '{
      authMode:    $auth_mode,
      region:      $region,
      model:       $model,
      opusModel:   $opus_model,
      sonnetModel: $sonnet_model,
      haikuModel:  $haiku_model,
      effortLevel: $effort_level,
      storage:     $storage,
      use1MContext: $use_1m,
      opusPlan:    $opusplan,
      legacyEnv:   $legacy_env
    }'
}

# ---------------------------------------------------------------------------
# Build v2 block from parsed v1 data
# ---------------------------------------------------------------------------

# migrator_build_v2_block <parsed_json> [bedrock_config_path]
# Returns a full juggernaut v2 block JSON string.
# schema_new_juggernaut_block reads its inputs from J_* env vars.
migrator_build_v2_block() {
  local parsed="$1"
  local bedrock_config_path="${2:-}"

  local legacy_env
  legacy_env="$(printf '%s' "$parsed" | jq '.legacyEnv')"

  local block
  block="$(
    J_AUTH_MODE="$(printf '%s' "$parsed" | jq -r '.authMode')" \
    J_REGION="$(printf '%s' "$parsed" | jq -r '.region')" \
    J_MODEL="$(printf '%s' "$parsed" | jq -r '.model // ""')" \
    J_OPUS_MODEL="$(printf '%s' "$parsed" | jq -r '.opusModel // ""')" \
    J_SONNET_MODEL="$(printf '%s' "$parsed" | jq -r '.sonnetModel // ""')" \
    J_HAIKU_MODEL="$(printf '%s' "$parsed" | jq -r '.haikuModel // ""')" \
    J_EFFORT="$(printf '%s' "$parsed" | jq -r '.effortLevel')" \
    J_STORAGE="$(printf '%s' "$parsed" | jq -r '.storage')" \
    J_USE_1M="$(printf '%s' "$parsed" | jq -r '.use1MContext')" \
    J_OPUSPLAN="$(printf '%s' "$parsed" | jq -r '.opusPlan')" \
    J_USE_MANTLE="false" \
    BEDROCK_CONFIG_PATH="${bedrock_config_path:-${BEDROCK_CONFIG_PATH:-}}" \
    schema_new_juggernaut_block
  )"

  # Inject legacyEnv snapshot and migration provenance.
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf '%s' "$block" | jq \
    --argjson legacy "$legacy_env" \
    --arg now "$now" \
    '.legacyEnv = {
       source:     "v1.7.x-profile-block",
       migratedAt: $now,
       snapshot:   $legacy
     }
     | .meta.migratedFrom = "v1.7.x"'
}

# ---------------------------------------------------------------------------
# Profile block notice replacement
# ---------------------------------------------------------------------------

# migrator_annotate_profile <profile_file>
# Replaces the v1 metadata comment lines inside the block with a single
# migration notice. The export lines are left untouched for compatibility.
migrator_annotate_profile() {
  local file="$1"
  [[ -f "$file" ]] || return 0

  # Build the replacement header using a temp file to avoid in-place sed issues on all platforms.
  local tmp
  tmp="$(mktemp)"
  local in_block=0
  local header_written=0

  while IFS= read -r line; do
    if [[ "$line" == "# BEGIN: Claude Code Bedrock Configuration" ]]; then
      in_block=1
      header_written=0
      printf '%s\n' "$line" >> "$tmp"
      continue
    fi
    if [[ "$line" == "# END: Claude Code Bedrock Configuration" ]]; then
      in_block=0
      printf '%s\n' "$line" >> "$tmp"
      continue
    fi
    if (( in_block )); then
      # Write the notice once, then skip subsequent metadata comment lines.
      if (( ! header_written )); then
        printf '# Juggernaut v2: PRIMARY config is now in ~/.claude/settings.json.\n' >> "$tmp"
        printf '# This block is a compatibility fallback. Run `juggernaut migrate --clean` to remove it.\n' >> "$tmp"
        header_written=1
      fi
      # Skip old metadata comments; preserve export/unset lines.
      if [[ "$line" =~ ^#\ (Auth\ mode|Model|FastModel|OpusModel|SonnetModel|HaikuModel|Storage|1MContext|OpusPlan|EffortLevel): ]]; then
        continue
      fi
    fi
    printf '%s\n' "$line" >> "$tmp"
  done < "$file"

  mv -f -- "$tmp" "$file"
}

# ---------------------------------------------------------------------------
# Top-level entry points
# ---------------------------------------------------------------------------

# migrator_run <profile_file> <settings_json_path> [bedrock_config_path]
# Full migration: parse v1 block → build v2 block → write settings.json → annotate profile.
# Returns 0 on success, 1 if no v1 block found, 2 on error.
migrator_run() {
  local profile_file="$1"
  local settings_path="$2"
  local bedrock_config_path="${3:-}"

  if ! migrator_has_v1_block "$profile_file"; then
    echo "migrator_run: no v1 block found in $profile_file" >&2
    return 1
  fi

  local raw_block
  raw_block="$(migrator_extract_block "$profile_file")"

  local parsed
  parsed="$(migrator_parse_v1_block "$raw_block")"

  local new_block
  new_block="$(migrator_build_v2_block "$parsed" "$bedrock_config_path")"

  local native_keys
  native_keys="$(schema_derive_native_keys "$new_block")"

  local existing
  existing="$(config_read "$settings_path")"

  local merged
  merged="$(config_merge_juggernaut_block "$existing" "$new_block" "$native_keys")"

  config_write_atomic "$settings_path" "$merged"
  migrator_annotate_profile "$profile_file"

  echo "Migration complete: $profile_file → $settings_path"
}

# migrator_rollback <settings_path>
# Restores the most recent backup of settings_path.
migrator_rollback() {
  local settings_path="$1"
  local dir base latest_backup

  dir="$(dirname -- "$settings_path")"
  base="$(basename -- "$settings_path")"

  # shellcheck disable=SC2012  # ls -t is portable; backup names contain no spaces
  latest_backup="$(ls -1t "$dir"/"${base}".backup.* 2>/dev/null | head -1)"

  if [[ -z "$latest_backup" ]]; then
    echo "migrator_rollback: no backup found for $settings_path" >&2
    return 1
  fi

  cp -p -- "$latest_backup" "$settings_path"
  echo "Rolled back $settings_path from $latest_backup"
}
