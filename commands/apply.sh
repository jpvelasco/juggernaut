#!/usr/bin/env bash
# commands/apply.sh — Juggernaut v2 apply subcommand.
# Configures Claude Code to use Amazon Bedrock via settings.json (+ optional shell fallback).
#
# Requires: bash 4+, jq, lib/schema.sh, lib/config_manager.sh,
#           lib/migrator.sh, lib/keychain.sh, lib/profile_writer.sh

set -uo pipefail
set +e   # Manual error handling — we want to warn and continue on non-fatal failures.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BEDROCK_CONFIG_PATH="${BEDROCK_CONFIG_PATH:-$SCRIPT_DIR/bedrock-config.json}"
export BEDROCK_CONFIG_PATH

. "$SCRIPT_DIR/lib/schema.sh"
. "$SCRIPT_DIR/lib/config_manager.sh"
. "$SCRIPT_DIR/lib/migrator.sh"
. "$SCRIPT_DIR/lib/keychain.sh"
. "$SCRIPT_DIR/lib/profile_writer.sh"
. "$SCRIPT_DIR/lib/profile_paths.sh"
# Lib files call `set -euo pipefail`; restore manual error handling.
set +e

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
DRY_RUN=false
J_YES=false
J_AUTH_MODE=""          # Populated from existing block or flag; default applied below.
J_AUTH_EXPLICIT=false
J_API_KEY=""
J_PRESERVE_KEY=false
J_STORAGE=""            # Populated from existing block or platform default.
J_STORAGE_EXPLICIT=false
J_REGION=""
J_MODEL=""
J_OPUS_MODEL=""
J_SONNET_MODEL=""
J_HAIKU_MODEL=""
J_EFFORT=""
J_OPUSPLAN=""           # Tri-state: ""=unset, "true", "false"
J_1M_CONTEXT=""
J_USE_MANTLE=false
J_MANTLE_EXPLICIT=false
J_MANTLE_URL=""
J_SCOPE="user"
J_NO_SHELL_FALLBACK=false
J_SHELL_FALLBACK_ONLY=false
J_SKIP_PREFLIGHT=false
J_FORCE_MIGRATION_PROMPT=false
SHELL_TYPE=""

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
_apply_help() {
  cat <<'EOF'
juggernaut apply — configure Claude Code for Amazon Bedrock

Usage: juggernaut apply [options] [bash|zsh|fish]

Positional argument (optional): override shell detection for the profile
fallback block. Defaults to $SHELL. Only bash, zsh, and fish are supported.

Options:
  --auth=iam|bedrock-api-key  Authentication mode (legacy: api-key)
  --bedrock-key=KEY        Bedrock API key (prompts if omitted)
  --preserve-key           Reuse existing key from env/keychain
  --storage=profile|keychain  API key storage (default: keychain on macOS/Win)
  --region=REGION          AWS region (default: us-west-2)
  --model=ID               Primary model ID
  --opus-model=ID          Opus model override
  --sonnet-model=ID        Sonnet model override
  --haiku-model=ID         Haiku model override
  --effort=LEVEL           Effort: low|medium|high|xhigh|max (default: xhigh)
  --opusplan               Use Opus for planning, Sonnet for execution
  --no-opusplan            Disable opusplan
  --1m-context             Enable 1M token context
  --no-1m-context          Disable 1M token context
  --mantle                 Enable Mantle routing
  --mantle-url=URL         Mantle base URL
  --scope=user|project     Write target (default: user)
  --no-shell-fallback      Write settings.json only
  --shell-fallback-only    Write profile block only
  --dry-run                Preview without writing
  --yes, --force, -f       Confirm migration prompts
  --force-migration-prompt Ignore previous migration decline marker
  --skip-preflight         Skip dependency checks
  --help, -h               Show this help

EOF
}

_apply_prompt_confirm() {
  local prompt="$1" answer
  if [[ "${JUGGERNAUT_NO_TTY_PROMPTS:-0}" != "1" ]] && printf '%s' "$prompt" > /dev/tty 2>/dev/null; then
    IFS= read -r answer < /dev/tty || return 1
  elif [[ -t 0 ]]; then
    IFS= read -r -p "$prompt" answer || return 1
  else
    return 1
  fi
  case "$answer" in
    y|Y|yes|YES) return 0 ;;
    *) return 1 ;;
  esac
}

_apply_prompt_secret() {
  local prompt="$1" value
  if [[ "${JUGGERNAUT_NO_TTY_PROMPTS:-0}" != "1" ]] && printf '%s' "$prompt" > /dev/tty 2>/dev/null; then
    IFS= read -r -s value < /dev/tty || return 1
    printf '\n' > /dev/tty
  elif [[ -t 0 ]]; then
    IFS= read -r -s -p "$prompt" value || return 1
    echo
  else
    return 1
  fi
  printf '%s' "$value"
}

# ---------------------------------------------------------------------------
# Flag parsing
# ---------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  arg="$1"
  case "$arg" in
    --dry-run)              DRY_RUN=true ;;
    --yes|--force|-f)       J_YES=true ;;
    --skip-preflight)       J_SKIP_PREFLIGHT=true ;;
    --force-migration-prompt) J_FORCE_MIGRATION_PROMPT=true ;;
    --auth=*)               J_AUTH_MODE="${arg#--auth=}"; J_AUTH_EXPLICIT=true ;;
    --auth)                 shift; [[ $# -gt 0 ]] || { echo "apply: --auth requires a value" >&2; exit 1; }; J_AUTH_MODE="$1"; J_AUTH_EXPLICIT=true ;;
    --bedrock-key=*)        J_API_KEY="${arg#--bedrock-key=}" ;;
    --bedrock-key)          shift; [[ $# -gt 0 ]] || { echo "apply: --bedrock-key requires a value" >&2; exit 1; }; J_API_KEY="$1" ;;
    --preserve-key)         J_PRESERVE_KEY=true ;;
    --storage=*)            J_STORAGE="${arg#--storage=}"; J_STORAGE_EXPLICIT=true ;;
    --storage)              shift; [[ $# -gt 0 ]] || { echo "apply: --storage requires a value" >&2; exit 1; }; J_STORAGE="$1"; J_STORAGE_EXPLICIT=true ;;
    --region=*)             J_REGION="${arg#--region=}" ;;
    --region)               shift; [[ $# -gt 0 ]] || { echo "apply: --region requires a value" >&2; exit 1; }; J_REGION="$1" ;;
    --model=*)              J_MODEL="${arg#--model=}" ;;
    --model)                shift; [[ $# -gt 0 ]] || { echo "apply: --model requires a value" >&2; exit 1; }; J_MODEL="$1" ;;
    --opus-model=*)         J_OPUS_MODEL="${arg#--opus-model=}" ;;
    --opus-model)           shift; [[ $# -gt 0 ]] || { echo "apply: --opus-model requires a value" >&2; exit 1; }; J_OPUS_MODEL="$1" ;;
    --sonnet-model=*)       J_SONNET_MODEL="${arg#--sonnet-model=}" ;;
    --sonnet-model)         shift; [[ $# -gt 0 ]] || { echo "apply: --sonnet-model requires a value" >&2; exit 1; }; J_SONNET_MODEL="$1" ;;
    --haiku-model=*)        J_HAIKU_MODEL="${arg#--haiku-model=}" ;;
    --haiku-model)          shift; [[ $# -gt 0 ]] || { echo "apply: --haiku-model requires a value" >&2; exit 1; }; J_HAIKU_MODEL="$1" ;;
    --effort=*)             J_EFFORT="${arg#--effort=}" ;;
    --effort)               shift; [[ $# -gt 0 ]] || { echo "apply: --effort requires a value" >&2; exit 1; }; J_EFFORT="$1" ;;
    --opusplan)             J_OPUSPLAN=true ;;
    --no-opusplan)          J_OPUSPLAN=false ;;
    --1m-context)           J_1M_CONTEXT=true ;;
    --no-1m-context)        J_1M_CONTEXT=false ;;
    --mantle)               J_USE_MANTLE=true; J_MANTLE_EXPLICIT=true ;;
    --mantle-url=*)         J_MANTLE_URL="${arg#--mantle-url=}"; J_USE_MANTLE=true; J_MANTLE_EXPLICIT=true ;;
    --mantle-url)           shift; [[ $# -gt 0 ]] || { echo "apply: --mantle-url requires a value" >&2; exit 1; }; J_MANTLE_URL="$1"; J_USE_MANTLE=true; J_MANTLE_EXPLICIT=true ;;
    --scope=*)              J_SCOPE="${arg#--scope=}" ;;
    --scope)                shift; [[ $# -gt 0 ]] || { echo "apply: --scope requires a value" >&2; exit 1; }; J_SCOPE="$1" ;;
    --no-shell-fallback)    J_NO_SHELL_FALLBACK=true ;;
    --shell-fallback-only)  J_SHELL_FALLBACK_ONLY=true ;;
    bash|zsh|fish)          SHELL_TYPE="$arg" ;;
    --version|-v)
      cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown"; exit 0 ;;
    --help|-h)
      _apply_help; exit 0 ;;
    *)
      echo "apply: unknown option '$arg' (ignored)" >&2 ;;
  esac
  shift
done

case "$J_AUTH_MODE" in
  api-key) J_AUTH_MODE="bedrock-api-key" ;;
esac

# Validate scope
case "$J_SCOPE" in
  user|project) ;;
  *) echo "apply: --scope must be 'user' or 'project' (got: '$J_SCOPE')" >&2; exit 1 ;;
esac

# Validate --shell-fallback-only and --no-shell-fallback are mutually exclusive.
if [[ "$J_NO_SHELL_FALLBACK" == "true" && "$J_SHELL_FALLBACK_ONLY" == "true" ]]; then
  echo "apply: --no-shell-fallback and --shell-fallback-only are mutually exclusive" >&2
  exit 1
fi

# Auto-detect the user's login shell if not provided. This script itself runs
# under Bash, so BASH_VERSION is only a fallback.
if [[ -z "$SHELL_TYPE" ]]; then
  _shell_name="$(basename "${SHELL:-}")"
  if [[ "$_shell_name" == "bash" || "$_shell_name" == "zsh" || "$_shell_name" == "fish" ]]; then
    SHELL_TYPE="$_shell_name"
  elif [[ -n "${ZSH_VERSION:-}" ]];  then SHELL_TYPE="zsh"
  elif [[ -n "${FISH_VERSION:-}" ]]; then SHELL_TYPE="fish"
  elif [[ -n "${BASH_VERSION:-}" ]]; then SHELL_TYPE="bash"
  else SHELL_TYPE="$(basename "${SHELL:-bash}")"
  fi
fi

# Determine shell fallback mode for schema.
if [[ "$J_NO_SHELL_FALLBACK" == "true" ]]; then
  J_SHELL_FALLBACK_MODE="settings-only"
elif [[ "$J_SHELL_FALLBACK_ONLY" == "true" ]]; then
  J_SHELL_FALLBACK_MODE="shell-only"
else
  J_SHELL_FALLBACK_MODE="both"
fi
export J_SHELL_FALLBACK_MODE

# ---------------------------------------------------------------------------
# Resolve settings.json target path
# ---------------------------------------------------------------------------
SETTINGS_PATH="$(config_resolve_target "$J_SCOPE")"

# ---------------------------------------------------------------------------
# Step 1: Read existing settings and detect state
# ---------------------------------------------------------------------------
EXISTING_JSON="{}"
if config_exists "$SETTINGS_PATH"; then
  EXISTING_JSON="$(config_read "$SETTINGS_PATH")" || {
    echo "apply: cannot read $SETTINGS_PATH — file may be corrupted" >&2
    exit 1
  }
fi

HAS_V2_BLOCK=false
if config_has_juggernaut_block "$EXISTING_JSON"; then
  HAS_V2_BLOCK=true
fi

# ---------------------------------------------------------------------------
# Step 2: Explicit migration — if no v2 block, look for v1 profile blocks.
# ---------------------------------------------------------------------------
if [[ "$HAS_V2_BLOCK" == "false" ]]; then
  mapfile -t V1_CANDIDATES < <(profile_paths_v1_candidates)
  [[ "$J_FORCE_MIGRATION_PROMPT" == "true" ]] && export JUGGERNAUT_FORCE_MIGRATION_PROMPT=1
  for candidate in "${V1_CANDIDATES[@]}"; do
    if migrator_has_v1_block "$candidate" 2>/dev/null; then
      echo "Juggernaut found an existing v1 profile block:" >&2
      echo "  $candidate" >&2
      echo "Migration writes the equivalent v2 settings to:" >&2
      echo "  $SETTINGS_PATH" >&2
      echo "The old profile block remains as a fallback unless you later run migrate --clean." >&2
      if [[ "$DRY_RUN" == "true" ]]; then
        echo "[dry-run] Would migrate $candidate to $SETTINGS_PATH" >&2
        break
      fi
      if [[ "$J_YES" != "true" ]]; then
        if ! _apply_prompt_confirm "Migrate this v1 block now? [y/N] "; then
          if [[ "${JUGGERNAUT_NO_TTY_PROMPTS:-0}" == "1" || ! -t 0 ]]; then
            echo "apply: migration requires confirmation. Re-run with --yes, or run juggernaut migrate --dry-run first." >&2
          else
            migrator_mark_migration_declined "$candidate" 2>/dev/null || true
            echo "apply: migration skipped. Re-run with --force-migration-prompt to re-prompt, or --yes to confirm non-interactively." >&2
          fi
          exit 1
        fi
      fi
      if migrator_run "$candidate" "$SETTINGS_PATH" "$BEDROCK_CONFIG_PATH"; then
        # Re-read after migration.
        EXISTING_JSON="$(config_read "$SETTINGS_PATH")"
        HAS_V2_BLOCK=true
        echo "Migration complete. Settings written to: $SETTINGS_PATH" >&2
      else
        echo "  Migration encountered an error — continuing with defaults. Your profile block is unchanged." >&2
      fi
      break
    fi
  done
fi

# ---------------------------------------------------------------------------
# Step 3: Load existing block (if any) and overlay CLI flags.
# Fields not specified on the CLI carry over from the stored block.
# ---------------------------------------------------------------------------
if [[ "$HAS_V2_BLOCK" == "true" ]]; then
  EXISTING_BLOCK="$(config_get_juggernaut_block "$EXISTING_JSON")"

  # Carry over stored values for fields not provided on CLI.
  [[ -z "$J_AUTH_MODE" ]]  && J_AUTH_MODE="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.auth.mode // ""')"
  [[ "$J_AUTH_MODE" == "api-key" ]] && J_AUTH_MODE="bedrock-api-key"
  [[ -z "$J_STORAGE" ]]    && J_STORAGE="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.auth.storage // ""')"
  [[ -z "$J_REGION" ]]     && J_REGION="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.auth.region // ""')"
  [[ -z "$J_MODEL" ]]      && J_MODEL="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.model // ""')"
  [[ -z "$J_OPUS_MODEL" ]] && J_OPUS_MODEL="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.modelOverrides.opus // ""')"
  [[ -z "$J_SONNET_MODEL" ]] && J_SONNET_MODEL="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.modelOverrides.sonnet // ""')"
  [[ -z "$J_HAIKU_MODEL" ]] && J_HAIKU_MODEL="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.modelOverrides.haiku // ""')"
  [[ -z "$J_EFFORT" ]]     && J_EFFORT="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.effortLevel // ""')"
  [[ -z "$J_OPUSPLAN" ]]   && J_OPUSPLAN="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.opusplan | tostring')"
  [[ -z "$J_1M_CONTEXT" ]] && J_1M_CONTEXT="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.context.use1MContext | tostring')"
  if [[ "$J_MANTLE_EXPLICIT" == "false" && "$J_USE_MANTLE" == "false" ]]; then
    J_USE_MANTLE="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.useMantle | tostring')"
  fi
  [[ -z "$J_MANTLE_URL" ]] && J_MANTLE_URL="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.mantle.baseUrl // ""')"
fi

# ---------------------------------------------------------------------------
# Step 4: Apply hard defaults for anything still unset.
# ---------------------------------------------------------------------------

# Bearer-token / keychain auto-detection: if no explicit --auth flag, infer
# bedrock-api-key from live evidence. When a stored block says "iam" but a key
# exists, warn (corrupted install); for fresh installs just silently switch.
if [[ "$J_AUTH_EXPLICIT" == "false" ]]; then
  if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
    if [[ "$J_AUTH_MODE" == "iam" && -n "${EXISTING_BLOCK:-}" ]]; then
      echo "apply: WARNING — stored auth mode is 'iam' but AWS_BEARER_TOKEN_BEDROCK is set." >&2
      echo "apply: Auto-correcting to bedrock-api-key. Pass --auth=iam to suppress." >&2
    fi
    J_AUTH_MODE="bedrock-api-key"
    J_PRESERVE_KEY=true
    if [[ "$J_MANTLE_EXPLICIT" == "false" ]]; then J_USE_MANTLE=true; fi
  elif [[ "$J_AUTH_MODE" == "iam" && -n "${EXISTING_BLOCK:-}" ]]; then
    _existing_storage="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.auth.storage // ""')"
    if [[ "$_existing_storage" == "keychain" ]] && keychain_available 2>/dev/null; then
      _kc_val="$(keychain_get 2>/dev/null || true)"
      if [[ -n "$_kc_val" ]]; then
        echo "apply: WARNING — stored auth mode is 'iam' but a key exists in the system keychain." >&2
        echo "apply: Auto-correcting to bedrock-api-key. Pass --auth=iam to suppress." >&2
        J_AUTH_MODE="bedrock-api-key"
        J_PRESERVE_KEY=true
      fi
    fi
  fi
fi
: "${J_AUTH_MODE:=iam}"
: "${J_REGION:=$(jq -r '.defaults.region // "us-west-2"' "$BEDROCK_CONFIG_PATH" 2>/dev/null || echo "us-west-2")}"
: "${J_EFFORT:=xhigh}"
: "${J_OPUSPLAN:=false}"
: "${J_1M_CONTEXT:=true}"
: "${J_USE_MANTLE:=false}"

# Platform-aware storage default: keychain on macOS/Windows, profile on Linux.
if [[ -z "$J_STORAGE" && "$J_STORAGE_EXPLICIT" == "false" ]]; then
  _os="$(keychain_detect_os)"
  case "$_os" in
    macos|gitbash|cygwin)
      if keychain_available 2>/dev/null; then J_STORAGE="keychain"; else J_STORAGE="profile"; fi ;;
    *) J_STORAGE="profile" ;;
  esac
fi
: "${J_STORAGE:=profile}"

# Validate auth mode.
case "$J_AUTH_MODE" in
  iam|bedrock-api-key) ;;
  *) echo "apply: --auth must be 'iam' or 'bedrock-api-key' (got: '$J_AUTH_MODE')" >&2; exit 1 ;;
esac

# Validate effort.
case "$J_EFFORT" in
  low|medium|high|xhigh|max) ;;
  *) echo "apply: --effort must be one of low|medium|high|xhigh|max (got: '$J_EFFORT')" >&2; exit 1 ;;
esac

# Preflight: warn if aws CLI missing in IAM mode.
if [[ "$J_SKIP_PREFLIGHT" != "true" && "$J_AUTH_MODE" == "iam" ]]; then
  if ! command -v aws >/dev/null 2>&1; then
    echo "apply: warning — aws CLI not found; IAM auth requires it at runtime." >&2
  fi
fi

# ---------------------------------------------------------------------------
# Step 5: Resolve API key for Bedrock API-key mode.
# ---------------------------------------------------------------------------
API_KEY_EXPR=""   # Shell expression to embed in profile block; empty for IAM.

if [[ "$J_AUTH_MODE" == "bedrock-api-key" ]]; then
  if [[ "$J_PRESERVE_KEY" == "true" ]]; then
    if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
      J_API_KEY="$AWS_BEARER_TOKEN_BEDROCK"
    fi
    # Try keychain regardless of stored storage preference — the storage setting
    # may have been corrupted alongside the auth mode.
    if [[ -z "$J_API_KEY" ]] && keychain_available 2>/dev/null; then
      # keychain_get exit codes: 0 found, 1 not found, 2 tool error.
      _kc_val="$(keychain_get 2>&1)"
      _kc_rc=$?
      case "$_kc_rc" in
        0) J_API_KEY="$_kc_val" ;;
        1) : ;;
        *) echo "apply: warning — keychain read failed: $_kc_val" >&2 ;;
      esac
    fi
    # Fall back to shell profile plaintext.
    if [[ -z "$J_API_KEY" ]]; then
      _profile_path="$(profile_writer_detect_shell_config_path "$SHELL_TYPE")"
      J_API_KEY="$(profile_writer_read_api_key "$_profile_path" 2>/dev/null || true)"
    fi
    if [[ -z "$J_API_KEY" ]]; then
      echo "apply: --preserve-key specified but no existing key found in env, keychain, or shell profile" >&2
      exit 1
    fi
  fi

  if [[ -z "$J_API_KEY" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
      J_API_KEY="dry-run-placeholder"
    else
      echo "Get your Bedrock API key from: AWS Console → Amazon Bedrock → API keys"
      if ! J_API_KEY="$(_apply_prompt_secret "Enter your Bedrock API key: ")"; then
        echo "apply: --bedrock-key or --preserve-key is required in non-interactive mode" >&2
        exit 1
      fi
      J_API_KEY="${J_API_KEY%"${J_API_KEY##*[![:space:]]}"}"
      J_API_KEY="${J_API_KEY#"${J_API_KEY%%[![:space:]]*}"}"
      if [[ -z "$J_API_KEY" ]]; then
        echo "apply: API key cannot be empty" >&2
        exit 1
      fi
    fi
  fi

  if [[ "$J_STORAGE" == "keychain" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
      echo "[dry-run] would store API key in system keychain"
    elif ! keychain_store "$J_API_KEY" 2>/dev/null; then
      if [[ "$J_STORAGE_EXPLICIT" == "true" ]]; then
        echo "apply: keychain store failed. Re-run with --storage=profile if you want plaintext profile storage." >&2
        exit 1
      fi
      echo "apply: warning — failed to store API key in keychain; falling back to profile storage" >&2
      J_STORAGE="profile"
    fi
  fi

  if [[ "$J_STORAGE" == "keychain" ]]; then
    API_KEY_EXPR="$(keychain_get_command "$SHELL_TYPE")"
  else
    # Profile storage: single-quote the key. The POSIX `'abc'\''def'` idiom
    # works identically in bash, zsh, and fish — never emit fish-specific
    # backslash escapes inside single quotes (fish treats them literally).
    _J_ESCAPED="${J_API_KEY//\'/\'\\\'\'}"
    API_KEY_EXPR="'$_J_ESCAPED'"
  fi
fi

# ---------------------------------------------------------------------------
# Step 6: Export J_* vars for schema_new_juggernaut_block.
# ---------------------------------------------------------------------------
export J_PROVIDER="bedrock"
export J_AUTH_MODE J_STORAGE J_REGION J_EFFORT
export J_USE_MANTLE
export J_MANTLE_BASE_URL="$J_MANTLE_URL"
export J_OPUSPLAN
export J_USE_1M="$J_1M_CONTEXT"
export J_SCOPE
J_VERSION="$(cat "$SCRIPT_DIR/VERSION" 2>/dev/null || echo "unknown")"
export J_VERSION

# Only export model overrides if explicitly set (schema uses bedrock-config.json defaults otherwise).
[[ -n "$J_MODEL" ]]        && export J_MODEL
[[ -n "$J_OPUS_MODEL" ]]   && export J_OPUS_MODEL
[[ -n "$J_SONNET_MODEL" ]] && export J_SONNET_MODEL
[[ -n "$J_HAIKU_MODEL" ]]  && export J_HAIKU_MODEL
[[ -n "$J_HAIKU_MODEL" ]]  && export J_SUBAGENT_MODEL="$J_HAIKU_MODEL"

# ---------------------------------------------------------------------------
# Step 7: Build and validate block.
# ---------------------------------------------------------------------------
NEW_BLOCK="$(schema_new_juggernaut_block)" || {
  echo "apply: failed to build juggernaut block" >&2
  exit 1
}

if ! schema_validate "$NEW_BLOCK" 2>/dev/null; then
  echo "apply: block validation failed — check your options" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Step 8: Merge into full settings.json.
# ---------------------------------------------------------------------------
NATIVE_KEYS="$(schema_derive_native_keys "$NEW_BLOCK")"
MERGED_JSON="$(config_merge_juggernaut_block "$EXISTING_JSON" "$NEW_BLOCK" "$NATIVE_KEYS")"

# ---------------------------------------------------------------------------
# Step 9: Dry-run exit.
# ---------------------------------------------------------------------------
if [[ "$DRY_RUN" == "true" ]]; then
  echo "[dry-run] No files will be written."
  echo ""
  echo "Would write to: $SETTINGS_PATH"
  echo "─────────────────────────────────────────"
  printf '%s\n' "$MERGED_JSON" | jq '.'
  echo "─────────────────────────────────────────"
  if [[ "$J_SHELL_FALLBACK_MODE" != "settings-only" ]]; then
    _profile="$(profile_writer_detect_shell_config_path "$SHELL_TYPE")"
    echo ""
    echo "Would also update shell profile: $_profile"
  fi
  echo ""
  echo "[dry-run] Done."
  exit 0
fi

# ---------------------------------------------------------------------------
# Step 10: Write settings.json (unless shell-fallback-only mode).
# ---------------------------------------------------------------------------
if [[ "$J_SHELL_FALLBACK_ONLY" != "true" ]]; then
  if ! config_write_atomic "$SETTINGS_PATH" "$MERGED_JSON"; then
    echo "apply: failed to write $SETTINGS_PATH" >&2
    exit 1
  fi
  echo "Settings written to: $SETTINGS_PATH"
fi

# ---------------------------------------------------------------------------
# Step 11: Write shell profile block (unless no-shell-fallback mode).
# ---------------------------------------------------------------------------
if [[ "$J_NO_SHELL_FALLBACK" != "true" ]]; then
  PROFILE_PATH="$(profile_writer_detect_shell_config_path "$SHELL_TYPE")"
  if [[ -z "$PROFILE_PATH" ]]; then
    echo "apply: warning — unknown shell '$SHELL_TYPE'; skipping profile block" >&2
  else
    BLOCK_CONTENT="$(profile_writer_build_block \
      "$SHELL_TYPE" "$J_REGION" "$J_AUTH_MODE" "$API_KEY_EXPR" "$J_STORAGE" \
      "$BEDROCK_CONFIG_PATH" \
      "$J_MODEL" "$J_OPUS_MODEL" "$J_SONNET_MODEL" "$J_HAIKU_MODEL" \
      "$J_EFFORT" "$J_OPUSPLAN" "$J_USE_MANTLE" "$J_MANTLE_URL")"

    if ! profile_writer_write "$PROFILE_PATH" "$BLOCK_CONTENT"; then
      echo "apply: warning — could not write profile block to $PROFILE_PATH" >&2
    else
      echo "Profile block written to: $PROFILE_PATH"
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Step 12: Summary
# ---------------------------------------------------------------------------
echo ""
echo "Juggernaut v2 apply complete."
if [[ "$J_AUTH_MODE" == "bedrock-api-key" ]]; then
  echo "  Auth:     Bedrock API key"
else
  echo "  Auth:     IAM"
fi
echo "  Region:   $J_REGION"
echo "  Effort:   $J_EFFORT"
echo "  Opusplan: $J_OPUSPLAN"
echo ""
echo "To apply changes in this shell, restart your terminal or run:"
case "$SHELL_TYPE" in
  fish) echo "  source ~/.config/fish/config.fish" ;;
  *)
    _profile_path="$(profile_writer_detect_shell_config_path "$SHELL_TYPE")"
    printf '  source %s\n' "$_profile_path"
    ;;
esac
echo ""
if [[ "$J_AUTH_MODE" == "iam" ]]; then
  echo "Verify AWS credentials, then launch Claude Code:"
  echo "  aws sts get-caller-identity && claude"
else
  echo "Launch Claude Code:"
  echo "  claude"
fi
