#!/usr/bin/env bash
# lib/profile_writer.sh — Shell profile block write/update/detect for Juggernaut v2.
# Extracted from setup-claude-bedrock.sh. setup-claude-bedrock.sh is unchanged.
# Requires: bash 4+, jq (or python3 as fallback via setup-claude-bedrock.sh).

set -euo pipefail

PROFILE_WRITER_BEGIN_MARKER="# BEGIN: Claude Code Bedrock Configuration"
PROFILE_WRITER_END_MARKER="# END: Claude Code Bedrock Configuration"

# ---------------------------------------------------------------------------
# profile_writer_detect_shell_config_path <shell>
# Returns the default config file path for a given shell name.
# ---------------------------------------------------------------------------
profile_writer_detect_shell_config_path() {
  local shell="$1"
  case "$shell" in
    bash) echo "$HOME/.bashrc" ;;
    zsh)  echo "$HOME/.zshrc" ;;
    fish) echo "$HOME/.config/fish/config.fish" ;;
    *)    echo "" ;;
  esac
}

# ---------------------------------------------------------------------------
# profile_writer_export_syntax <shell>
# Returns the export keyword for the given shell.
# ---------------------------------------------------------------------------
profile_writer_export_syntax() {
  local shell="$1"
  [[ "$shell" == "fish" ]] && echo "set -gx" || echo "export"
}

# ---------------------------------------------------------------------------
# profile_writer_has_block <profile_file>
# Returns 0 if the file contains a Juggernaut marker block.
# ---------------------------------------------------------------------------
profile_writer_has_block() {
  local f="$1"
  [[ -f "$f" ]] && grep -Fq -- "$PROFILE_WRITER_BEGIN_MARKER" "$f" 2>/dev/null
}

# ---------------------------------------------------------------------------
# profile_writer_read_api_key <profile_file>
# Reads a plaintext AWS_BEARER_TOKEN_BEDROCK assignment from a Juggernaut
# profile block. Keychain-backed blocks intentionally return empty.
# ---------------------------------------------------------------------------
profile_writer_read_api_key() {
  local f="$1"
  local line value
  [[ -f "$f" ]] || return 1

  line="$(
    awk -v begin_marker="$PROFILE_WRITER_BEGIN_MARKER" \
        -v end_marker="$PROFILE_WRITER_END_MARKER" '
      $0 == begin_marker { in_block=1; next }
      $0 == end_marker { in_block=0; next }
      in_block { print }
    ' "$f" \
      | grep -E '^(export AWS_BEARER_TOKEN_BEDROCK=|set -gx AWS_BEARER_TOKEN_BEDROCK )' \
      | tail -1
  )" || true
  [[ -n "$line" ]] || return 1

  case "$line" in
    export\ AWS_BEARER_TOKEN_BEDROCK=*)
      value="${line#export AWS_BEARER_TOKEN_BEDROCK=}" ;;
    set\ -gx\ AWS_BEARER_TOKEN_BEDROCK\ *)
      value="${line#set -gx AWS_BEARER_TOKEN_BEDROCK }" ;;
    *) return 1 ;;
  esac

  case "$value" in
    *keychain*|*\$\(*|*\`*) return 1 ;;
  esac

  if [[ "$value" == \'*\' && ${#value} -ge 2 ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "$value" == \"*\" && ${#value} -ge 2 ]]; then
    value="${value:1:${#value}-2}"
    value="${value//\\\"/\"}"
    value="${value//\\\\/\\}"
  fi

  [[ -n "$value" ]] || return 1
  printf '%s' "$value"
}

# ---------------------------------------------------------------------------
# profile_writer_remove_block <profile_file> [dry_run]
# Removes the marker-delimited block from the profile file.
# Pass "true" as second arg to skip actual removal (dry-run).
# ---------------------------------------------------------------------------
profile_writer_remove_block() {
  local f="$1"
  local dry_run="${2:-false}"
  if [[ "$dry_run" == "true" ]]; then
    echo "[dry-run] would remove block from $f"
    return 0
  fi
  if [[ "$OSTYPE" == darwin* ]]; then
    sed -i '' "/$PROFILE_WRITER_BEGIN_MARKER/,/$PROFILE_WRITER_END_MARKER/d" "$f"
  else
    sed -i "/$PROFILE_WRITER_BEGIN_MARKER/,/$PROFILE_WRITER_END_MARKER/d" "$f"
  fi
}

# _pw_export_line <syntax> <shell> <key> <value>
# Emits a single shell export/set-gx line with value safely double-quoted.
_pw_export_line() {
  local syntax="$1" shell="$2" k="$3" v="$4"
  local ev="${v//\"/\\\"}"
  if [[ "$shell" == "fish" ]]; then
    printf '%s %s "%s"\n' "$syntax" "$k" "$ev"
  else
    printf '%s %s="%s"\n' "$syntax" "$k" "$ev"
  fi
}

# ---------------------------------------------------------------------------
# profile_writer_build_block <shell> <region> <auth_mode> <api_key_expr>
#                             <storage_mode> <bedrock_config_path>
#                             [model] [opus_model] [sonnet_model] [haiku_model]
#                             [effort_level] [opusplan] [use_mantle] [mantle_url]
#
# Prints the full marker-delimited config block to stdout.
# api_key_expr: the literal shell expression to assign to AWS_BEARER_TOKEN_BEDROCK
#               (empty for IAM mode; plaintext key or $(keychain_get_command) for api-key).
# ---------------------------------------------------------------------------
profile_writer_build_block() {
  local shell="$1"
  local region="$2"
  local auth_mode="$3"
  local api_key_expr="$4"
  local storage_mode="$5"
  local bedrock_cfg="$6"
  local model="${7:-}"
  local opus_model="${8:-}"
  local sonnet_model="${9:-}"
  local haiku_model="${10:-}"
  local effort_level="${11:-}"
  local opusplan="${12:-false}"
  local use_mantle="${13:-false}"
  local mantle_url="${14:-}"

  local syntax
  syntax="$(profile_writer_export_syntax "$shell")"

  # Load defaults from bedrock-config.json
  local default_model="" default_opus="" default_sonnet="" default_haiku="" default_effort=""
  local max_output="" thinking_tokens="" prompt_cache=""
  if [[ -f "$bedrock_cfg" ]]; then
    default_model=$(jq -r '.environment.ANTHROPIC_MODEL // empty' "$bedrock_cfg" 2>/dev/null || true)
    default_opus=$(jq -r '.environment.ANTHROPIC_DEFAULT_OPUS_MODEL // empty' "$bedrock_cfg" 2>/dev/null || true)
    default_sonnet=$(jq -r '.environment.ANTHROPIC_DEFAULT_SONNET_MODEL // empty' "$bedrock_cfg" 2>/dev/null || true)
    default_haiku=$(jq -r '.environment.ANTHROPIC_DEFAULT_HAIKU_MODEL // empty' "$bedrock_cfg" 2>/dev/null || true)
    default_effort=$(jq -r '.environment.CLAUDE_CODE_EFFORT_LEVEL // empty' "$bedrock_cfg" 2>/dev/null || true)
    max_output=$(jq -r '.environment.CLAUDE_CODE_MAX_OUTPUT_TOKENS // empty' "$bedrock_cfg" 2>/dev/null || true)
    thinking_tokens=$(jq -r '.environment.MAX_THINKING_TOKENS // empty' "$bedrock_cfg" 2>/dev/null || true)
    prompt_cache=$(jq -r '.environment.ENABLE_PROMPT_CACHING_1H // empty' "$bedrock_cfg" 2>/dev/null || true)
  fi

  local eff_model="${model:-$default_model}"
  local eff_opus="${opus_model:-$default_opus}"
  local eff_sonnet="${sonnet_model:-$default_sonnet}"
  local eff_haiku="${haiku_model:-$default_haiku}"
  local eff_effort="${effort_level:-${default_effort:-xhigh}}"
  local eff_max="${max_output:-32768}"
  local eff_thinking="${thinking_tokens:-65536}"
  local eff_cache="${prompt_cache:-1}"

  # When opusplan=true, ANTHROPIC_MODEL is set to "opusplan" signal value.
  [[ "$opusplan" == "true" ]] && eff_model="opusplan"

  local block=""
  block+=$'\n'"$PROFILE_WRITER_BEGIN_MARKER"$'\n'

  # Metadata comments (parsed by migrator on v1→v2 upgrade)
  block+="# Auth mode: $auth_mode"$'\n'
  [[ "$storage_mode" == "keychain" ]] && block+="# Storage: keychain (encrypted)"$'\n'
  [[ -n "$model" ]]       && block+="# Model: $model"$'\n'
  [[ -n "$opus_model" ]]  && block+="# OpusModel: $opus_model"$'\n'
  [[ -n "$sonnet_model" ]] && block+="# SonnetModel: $sonnet_model"$'\n'
  [[ -n "$haiku_model" ]] && block+="# HaikuModel: $haiku_model"$'\n'
  [[ "$opusplan" == "true" ]] && block+="# OpusPlan: true"$'\n'
  [[ -n "$effort_level" ]] && block+="# EffortLevel: $effort_level"$'\n'

  # Unset conflicting auth vars
  if [[ "$auth_mode" == "api-key" || "$auth_mode" == "bedrock-api-key" ]]; then
    if [[ "$shell" == "fish" ]]; then
      block+="set -e AWS_ACCESS_KEY_ID 2>/dev/null"$'\n'
      block+="set -e AWS_SECRET_ACCESS_KEY 2>/dev/null"$'\n'
      block+="set -e AWS_SESSION_TOKEN 2>/dev/null"$'\n'
      block+="set -e AWS_PROFILE 2>/dev/null"$'\n'
    else
      block+="unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_PROFILE 2>/dev/null || true"$'\n'
    fi
  fi

  # Core env vars — use module-level helper _pw_export_line <syntax> <shell> <key> <value>
  block+="$(_pw_export_line "$syntax" "$shell" AWS_REGION "$region")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" CLAUDE_CODE_USE_BEDROCK "1")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" CLAUDE_CODE_MAX_OUTPUT_TOKENS "$eff_max")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" MAX_THINKING_TOKENS "$eff_thinking")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" ANTHROPIC_MODEL "$eff_model")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" ANTHROPIC_DEFAULT_OPUS_MODEL "$eff_opus")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" ANTHROPIC_DEFAULT_SONNET_MODEL "$eff_sonnet")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" ANTHROPIC_DEFAULT_HAIKU_MODEL "$eff_haiku")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" CLAUDE_CODE_SUBAGENT_MODEL "$eff_haiku")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" CLAUDE_CODE_EFFORT_LEVEL "$eff_effort")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" ENABLE_PROMPT_CACHING_1H "$eff_cache")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" DISABLE_ERROR_REPORTING "1")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" DISABLE_TELEMETRY "1")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" DISABLE_AUTOUPDATE "1")"$'\n'
  block+="$(_pw_export_line "$syntax" "$shell" DISABLE_BUG_COMMAND "1")"$'\n'

  # Mantle
  if [[ "$use_mantle" == "true" ]]; then
    block+="$(_pw_export_line "$syntax" "$shell" CLAUDE_CODE_USE_MANTLE "1")"$'\n'
    [[ -n "$mantle_url" ]] && block+="$(_pw_export_line "$syntax" "$shell" ANTHROPIC_BEDROCK_MANTLE_BASE_URL "$mantle_url")"$'\n'
  fi

  # API key expression (api-key mode only)
  if [[ ( "$auth_mode" == "api-key" || "$auth_mode" == "bedrock-api-key" ) && -n "$api_key_expr" ]]; then
    if [[ "$shell" == "fish" ]]; then
      block+="$syntax AWS_BEARER_TOKEN_BEDROCK $api_key_expr"$'\n'
    else
      block+="$syntax AWS_BEARER_TOKEN_BEDROCK=$api_key_expr"$'\n'
    fi
  fi

  block+="$PROFILE_WRITER_END_MARKER"$'\n'

  printf '%s' "$block"
}

# ---------------------------------------------------------------------------
# profile_writer_write <profile_file> <block_content> [dry_run]
# Appends block_content to profile_file (after removing any existing block).
# Creates parent directories and profile file if missing.
# ---------------------------------------------------------------------------
profile_writer_write() {
  local f="$1"
  local block="$2"
  local dry_run="${3:-false}"

  if [[ "$dry_run" == "true" ]]; then
    echo "[dry-run] would write block to $f:"
    printf '%s\n' "$block"
    return 0
  fi

  mkdir -p -- "$(dirname -- "$f")"
  [[ ! -f "$f" ]] && touch "$f"

  # Remove existing block first (idempotent).
  profile_writer_remove_block "$f"

  if ! printf '%s\n' "$block" >> "$f" 2>/dev/null; then
    echo "profile_writer_write: cannot write to $f" >&2
    return 1
  fi
}

# ---------------------------------------------------------------------------
# profile_writer_annotate <profile_file> [dry_run]
# Replaces the metadata comment lines inside an existing block with a single
# v2-migration notice, leaving export/set-gx lines intact.
# Called by migrator after writing settings.json, so the v1 block stays as
# a compatibility fallback with a clear notice.
# ---------------------------------------------------------------------------
profile_writer_annotate() {
  local f="$1"
  local dry_run="${2:-false}"

  if ! profile_writer_has_block "$f"; then
    return 0
  fi

  if [[ "$dry_run" == "true" ]]; then
    echo "[dry-run] would annotate v1 block in $f"
    return 0
  fi

  # Extract the block, strip metadata comment lines, re-insert with notice.
  local tmp
  tmp="$(mktemp)"

  awk -v begin_marker="$PROFILE_WRITER_BEGIN_MARKER" \
      -v end_marker="$PROFILE_WRITER_END_MARKER" '
    $0 == begin_marker {
      in_block=1
      print begin_marker
      print "# Juggernaut v2: PRIMARY config is now in ~/.claude/settings.json."
      print "# This block remains as a compatibility fallback."
      print "# Run `juggernaut migrate --clean` to remove it once Claude Code works."
      next
    }
    $0 == end_marker {
      in_block=0
      print end_marker
      next
    }
    in_block && /^# (Auth mode|Storage|Model|FastModel|OpusModel|SonnetModel|HaikuModel|1MContext|OpusPlan|EffortLevel):/ {
      next
    }
    { print }
  ' "$f" > "$tmp" || { rm -f "$tmp"; return 1; }
  mv "$tmp" "$f" || { rm -f "$tmp"; return 1; }
}
