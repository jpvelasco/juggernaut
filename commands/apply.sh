#!/usr/bin/env bash
# commands/apply.sh — Juggernaut v3 apply subcommand.
# Configures Claude Code to use Amazon Bedrock via settings.json (sole output).
#
# Requires: bash 4+, jq, lib/schema.sh, lib/config_manager.sh, lib/keychain.sh

set -uo pipefail
set +e   # Manual error handling — we want to warn and continue on non-fatal failures.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BEDROCK_CONFIG_PATH="${BEDROCK_CONFIG_PATH:-$SCRIPT_DIR/bedrock-config.json}"
export BEDROCK_CONFIG_PATH

. "$SCRIPT_DIR/lib/schema.sh"
. "$SCRIPT_DIR/lib/config_manager.sh"
. "$SCRIPT_DIR/lib/keychain.sh"
# Lib files call `set -euo pipefail`; restore manual error handling.
set +e

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
DRY_RUN=false
J_AUTH_MODE=""          # Populated from existing block or flag; default applied below.
J_AUTH_EXPLICIT=false
J_API_KEY=""
J_API_KEY_FROM_CLIPBOARD=false
J_PRESERVE_KEY=false
J_STORAGE=""            # Populated from existing block or platform default.
J_STORAGE_EXPLICIT=false
J_REGION=""
J_MODEL=""
J_MODEL_EXPLICIT=false
J_OPUS_MODEL=""
J_SONNET_MODEL=""
J_HAIKU_MODEL=""
J_EFFORT=""
J_OPUSPLAN=""           # Tri-state: ""=unset, "true", "false"
J_OPUSPLAN_EXPLICIT=false
J_1M_CONTEXT=""
J_USE_MANTLE=true       # v3: Mantle on by default.
J_MANTLE_URL=""
J_SCOPE="user"
J_SKIP_PREFLIGHT=false

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
_apply_help() {
  cat <<'EOF'
juggernaut apply — configure Claude Code for Amazon Bedrock

Usage: juggernaut apply [options]

Options:
  --auth=iam|bedrock-api-key  Authentication mode (required on first run)
  --bedrock-key=KEY        Bedrock API key (prompts if omitted)
  --bedrock-key-from-clipboard
                           Read Bedrock API key from the system clipboard
                           Pipe it in: echo $KEY | juggernaut apply ...
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
  --no-mantle              Disable Mantle routing (Mantle is on by default)
  --mantle-url=URL         Mantle base URL
  --scope=user|project     Write target (default: user)
  --dry-run                Preview without writing
  --skip-preflight         Skip dependency checks
  --help, -h               Show this help

EOF
}

_apply_acquire_key() {
  local value

  if [[ ! -t 0 ]]; then
    value="$(cat)" || return 1
    # Strip all trailing CR/LF — handles printf, echo, and here-string sources.
    while [[ "${value: -1}" == $'\n' || "${value: -1}" == $'\r' ]]; do
      value="${value%?}"
    done
    printf '%s' "$value"
    return 0
  fi

  if [[ "${JUGGERNAUT_NO_TTY_PROMPTS:-0}" == "1" ]]; then
    return 1
  fi

  {
    printf '\n'
    printf 'Get your Bedrock API key from: AWS Console → Amazon Bedrock → API keys\n'
    printf 'Paste your key, then press Enter.\n'
    printf '(Tip: you can also pipe it in: echo $KEY | juggernaut apply ...)\n'
    printf '> '
  } > /dev/tty 2>/dev/null || return 1

  if [[ -n "${BASH_VERSION:-}" && -t 0 ]]; then
    IFS= read -e -r value || return 1
  else
    IFS= read -r value < /dev/tty || return 1
  fi
  printf '\n' > /dev/tty 2>/dev/null

  value="${value//$'\e[200~'/}"
  value="${value//$'\e[201~'/}"
  value="${value//$'\r'/}"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"

  printf '%s' "$value"
}

_apply_clipboard_key() {
  local value=""

  if command -v pbpaste >/dev/null 2>&1; then
    value="$(pbpaste 2>/dev/null)" || return 1
  elif command -v wl-paste >/dev/null 2>&1; then
    value="$(wl-paste --no-newline 2>/dev/null)" || return 1
  elif command -v xclip >/dev/null 2>&1; then
    value="$(xclip -selection clipboard -o 2>/dev/null)" || return 1
  elif command -v xsel >/dev/null 2>&1; then
    value="$(xsel --clipboard --output 2>/dev/null)" || return 1
  elif command -v powershell.exe >/dev/null 2>&1; then
    value="$(powershell.exe -NoProfile -Command 'Get-Clipboard -Raw' 2>/dev/null)" || return 1
  elif command -v pwsh >/dev/null 2>&1; then
    value="$(pwsh -NoProfile -Command 'Get-Clipboard -Raw' 2>/dev/null)" || return 1
  elif command -v powershell >/dev/null 2>&1; then
    value="$(powershell -NoProfile -Command 'Get-Clipboard -Raw' 2>/dev/null)" || return 1
  else
    echo "apply: no supported clipboard reader found (pbpaste, wl-paste, xclip, xsel, or PowerShell)" >&2
    return 1
  fi

  value="${value//$'\e[200~'/}"
  value="${value//$'\e[201~'/}"
  value="${value//$'\r'/}"
  while [[ "${value: -1}" == $'\n' || "${value: -1}" == $'\r' ]]; do
    value="${value%?}"
  done
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

# ---------------------------------------------------------------------------
# Flag parsing
# ---------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  arg="$1"
  case "$arg" in
    --dry-run)              DRY_RUN=true ;;
    --skip-preflight)       J_SKIP_PREFLIGHT=true ;;
    --auth=*)               J_AUTH_MODE="${arg#--auth=}"; J_AUTH_EXPLICIT=true ;;
    --auth)                 shift; [[ $# -gt 0 ]] || { echo "apply: --auth requires a value" >&2; exit 1; }; J_AUTH_MODE="$1"; J_AUTH_EXPLICIT=true ;;
    --bedrock-key=*)        J_API_KEY="${arg#--bedrock-key=}" ;;
    --bedrock-key)          shift; [[ $# -gt 0 ]] || { echo "apply: --bedrock-key requires a value" >&2; exit 1; }; J_API_KEY="$1" ;;
    --bedrock-key-from-clipboard) J_API_KEY_FROM_CLIPBOARD=true ;;
    --preserve-key)         J_PRESERVE_KEY=true ;;
    --storage=*)            J_STORAGE="${arg#--storage=}"; J_STORAGE_EXPLICIT=true ;;
    --storage)              shift; [[ $# -gt 0 ]] || { echo "apply: --storage requires a value" >&2; exit 1; }; J_STORAGE="$1"; J_STORAGE_EXPLICIT=true ;;
    --region=*)             J_REGION="${arg#--region=}" ;;
    --region)               shift; [[ $# -gt 0 ]] || { echo "apply: --region requires a value" >&2; exit 1; }; J_REGION="$1" ;;
    --model=*)              J_MODEL="${arg#--model=}"; J_MODEL_EXPLICIT=true ;;
    --model)                shift; [[ $# -gt 0 ]] || { echo "apply: --model requires a value" >&2; exit 1; }; J_MODEL="$1"; J_MODEL_EXPLICIT=true ;;
    --opus-model=*)         J_OPUS_MODEL="${arg#--opus-model=}" ;;
    --opus-model)           shift; [[ $# -gt 0 ]] || { echo "apply: --opus-model requires a value" >&2; exit 1; }; J_OPUS_MODEL="$1" ;;
    --sonnet-model=*)       J_SONNET_MODEL="${arg#--sonnet-model=}" ;;
    --sonnet-model)         shift; [[ $# -gt 0 ]] || { echo "apply: --sonnet-model requires a value" >&2; exit 1; }; J_SONNET_MODEL="$1" ;;
    --haiku-model=*)        J_HAIKU_MODEL="${arg#--haiku-model=}" ;;
    --haiku-model)          shift; [[ $# -gt 0 ]] || { echo "apply: --haiku-model requires a value" >&2; exit 1; }; J_HAIKU_MODEL="$1" ;;
    --effort=*)             J_EFFORT="${arg#--effort=}" ;;
    --effort)               shift; [[ $# -gt 0 ]] || { echo "apply: --effort requires a value" >&2; exit 1; }; J_EFFORT="$1" ;;
    --opusplan)             J_OPUSPLAN=true; J_OPUSPLAN_EXPLICIT=true ;;
    --no-opusplan)          J_OPUSPLAN=false; J_OPUSPLAN_EXPLICIT=true ;;
    --1m-context)           J_1M_CONTEXT=true ;;
    --no-1m-context)        J_1M_CONTEXT=false ;;
    --no-mantle)            J_USE_MANTLE=false ;;
    --mantle-url=*)         J_MANTLE_URL="${arg#--mantle-url=}" ;;
    --mantle-url)           shift; [[ $# -gt 0 ]] || { echo "apply: --mantle-url requires a value" >&2; exit 1; }; J_MANTLE_URL="$1" ;;
    --scope=*)              J_SCOPE="${arg#--scope=}" ;;
    --scope)                shift; [[ $# -gt 0 ]] || { echo "apply: --scope requires a value" >&2; exit 1; }; J_SCOPE="$1" ;;
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

# Schema still accepts a shellFallback block for historical shape; v3 always pins it to settings-only.
J_SHELL_FALLBACK_MODE="settings-only"
export J_SHELL_FALLBACK_MODE

# ---------------------------------------------------------------------------
# Resolve settings.json target path
# ---------------------------------------------------------------------------
SETTINGS_PATH="$(config_resolve_target "$J_SCOPE")"

# ---------------------------------------------------------------------------
# Step 1: Read existing settings (if any).
# ---------------------------------------------------------------------------
EXISTING_JSON="{}"
if config_exists "$SETTINGS_PATH"; then
  EXISTING_JSON="$(config_read "$SETTINGS_PATH")" || {
    echo "apply: cannot read $SETTINGS_PATH — file may be corrupted" >&2
    exit 1
  }
fi

HAS_BLOCK=false
if config_has_juggernaut_block "$EXISTING_JSON"; then
  HAS_BLOCK=true
fi

# ---------------------------------------------------------------------------
# Step 2: Load existing block (if any) and overlay CLI flags.
# Fields not specified on the CLI carry over from the stored block.
# ---------------------------------------------------------------------------
if [[ "$HAS_BLOCK" == "true" ]]; then
  EXISTING_BLOCK="$(config_get_juggernaut_block "$EXISTING_JSON")"

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
  [[ -z "$J_MANTLE_URL" ]] && J_MANTLE_URL="$(printf '%s' "$EXISTING_BLOCK" | jq -r '.mantle.baseUrl // ""')"
fi

if [[ "$J_OPUSPLAN_EXPLICIT" != "true" ]]; then
  _settings_model="$(printf '%s' "$EXISTING_JSON" | jq -r '.model // ""')"
  _settings_env_model="$(printf '%s' "$EXISTING_JSON" | jq -r '.env.ANTHROPIC_MODEL // ""')"
  if [[ "$J_MODEL" == "opusplan" || "$_settings_model" == "opusplan" || "$_settings_env_model" == "opusplan" ]]; then
    J_OPUSPLAN=true
  fi
fi

if [[ "$J_MODEL" == "opusplan" ]]; then
  if [[ "$J_MODEL_EXPLICIT" == "true" ]]; then
    echo "apply: --model cannot be 'opusplan'; use --opusplan for that routing mode" >&2
    exit 1
  fi
  J_MODEL=""
fi

# ---------------------------------------------------------------------------
# Step 3: Auth validation gate.
# On first run (no stored auth), require an explicit --auth flag UNLESS we can
# detect live credentials. Prevents the installer-poisons-auth bug class where
# CLAUDE_CODE_USE_BEDROCK=1 is written without a working credential path.
# ---------------------------------------------------------------------------
if [[ "$J_AUTH_EXPLICIT" != "true" && -z "${EXISTING_BLOCK:-}" ]]; then
  _auth_detected=""
  if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
    _auth_detected="bedrock-api-key"
  elif keychain_available 2>/dev/null && keychain_get >/dev/null 2>&1; then
    _auth_detected="bedrock-api-key"
  elif command -v aws >/dev/null 2>&1 && aws sts get-caller-identity >/dev/null 2>&1; then
    _auth_detected="iam"
  fi
  if [[ -z "$_auth_detected" ]]; then
    cat >&2 <<'EOF'
apply: no authentication mode specified and no credentials detected.

Pass --auth=iam to use AWS IAM credentials (requires `aws configure` or `aws sso login`),
or pass --auth=bedrock-api-key to use a Bedrock API key (supplies `--bedrock-key=KEY` or
sets AWS_BEARER_TOKEN_BEDROCK).

Juggernaut will not enable CLAUDE_CODE_USE_BEDROCK without a validated auth path —
this prevents Claude Code from hanging on launch.
EOF
    exit 2
  fi
  J_AUTH_MODE="$_auth_detected"
fi

# ---------------------------------------------------------------------------
# Step 4: Apply hard defaults for anything still unset.
# ---------------------------------------------------------------------------
: "${J_AUTH_MODE:=iam}"
: "${J_REGION:=$(jq -r '.defaults.region // "us-west-2"' "$BEDROCK_CONFIG_PATH" 2>/dev/null || echo "us-west-2")}"
: "${J_EFFORT:=xhigh}"
: "${J_OPUSPLAN:=false}"
: "${J_1M_CONTEXT:=true}"

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
if [[ "$J_AUTH_MODE" == "bedrock-api-key" ]]; then
  if [[ "$J_PRESERVE_KEY" == "true" ]]; then
    if [[ -n "${AWS_BEARER_TOKEN_BEDROCK:-}" ]]; then
      J_API_KEY="$AWS_BEARER_TOKEN_BEDROCK"
    fi
    if [[ -z "$J_API_KEY" ]]; then
      _kc_val="$(bearer_token_get 2>&1)"
      _kc_rc=$?
      case "$_kc_rc" in
        0) J_API_KEY="$_kc_val" ;;
        1) : ;;
        *) echo "apply: warning — secure storage read failed: $_kc_val" >&2 ;;
      esac
    fi
    if [[ -z "$J_API_KEY" ]]; then
      echo "apply: --preserve-key specified but no existing key found in env, keychain, DPAPI, or profile storage" >&2
      exit 1
    fi
  fi

  if [[ -z "$J_API_KEY" ]]; then
    if [[ "$J_API_KEY_FROM_CLIPBOARD" == "true" ]]; then
      if ! J_API_KEY="$(_apply_clipboard_key)"; then
        echo "apply: failed to read Bedrock API key from clipboard" >&2
        exit 1
      fi
    fi
  fi

  if [[ -z "$J_API_KEY" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
      J_API_KEY="dry-run-placeholder"
    else
      if ! J_API_KEY="$(_apply_acquire_key)"; then
        echo "apply: --bedrock-key or --preserve-key is required in non-interactive mode" >&2
        echo "       Or pipe the key: echo \$KEY | juggernaut apply --auth=bedrock-api-key" >&2
        exit 1
      fi
      J_API_KEY="${J_API_KEY//$'\r'/}"
      J_API_KEY="${J_API_KEY#"${J_API_KEY%%[![:space:]]*}"}"
      J_API_KEY="${J_API_KEY%"${J_API_KEY##*[![:space:]]}"}"
      if [[ -z "$J_API_KEY" ]]; then
        echo "apply: API key cannot be empty" >&2
        echo "       Pass --bedrock-key=KEY, or pipe: echo \$KEY | juggernaut apply ..." >&2
        exit 1
      fi
      if [[ "${#J_API_KEY}" -lt 40 ]]; then
        echo "apply: API key looks truncated (${#J_API_KEY} chars)." >&2
        echo "       Bedrock API keys are typically 100+ chars. Pipe the key instead:" >&2
        echo "       echo \$KEY | juggernaut apply --auth=bedrock-api-key" >&2
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
  if [[ "$J_STORAGE" == "profile" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
      echo "[dry-run] would store API key in profile token file"
    else
      profile_token_store "$J_API_KEY"
    fi
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
export J_AUTH_VALIDATED=true
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
  echo ""
  echo "[dry-run] Done."
  exit 0
fi

# ---------------------------------------------------------------------------
# Step 10: Write settings.json.
# ---------------------------------------------------------------------------
if ! config_write_atomic "$SETTINGS_PATH" "$MERGED_JSON"; then
  echo "apply: failed to write $SETTINGS_PATH" >&2
  exit 1
fi
echo "Settings written to: $SETTINGS_PATH"

# ---------------------------------------------------------------------------
# Step 11: Summary
# ---------------------------------------------------------------------------
echo ""
echo "Juggernaut v3 apply complete."
if [[ "$J_AUTH_MODE" == "bedrock-api-key" ]]; then
  echo "  Auth:     Bedrock API key"
else
  echo "  Auth:     IAM"
fi
echo "  Region:   $J_REGION"
echo "  Effort:   $J_EFFORT"
echo "  Opusplan: $J_OPUSPLAN"
echo "  Mantle:   $J_USE_MANTLE"
echo ""
if [[ "$J_AUTH_MODE" == "iam" ]]; then
  echo "Verify AWS credentials, then launch Claude Code:"
  echo "  aws sts get-caller-identity && claude"
else
  echo "Launch Claude Code:"
  echo "  claude"
fi
