#!/usr/bin/env bash
# tests/v2/test_apply.sh — v3 tests for commands/apply.sh
# Focus: auth validation gate, settings.json write, dry-run, scope, Mantle default.

set -uo pipefail
set +e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BEDROCK_CONFIG_PATH="$REPO_ROOT/bedrock-config.json"

PASS=0; FAIL=0
fail() { echo "  FAIL: $1" >&2; FAIL=$((FAIL + 1)); }
pass() { PASS=$((PASS + 1)); }
section() { echo; echo "== $1 =="; }

_clean_env() {
  unset AWS_PROFILE AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN
  unset AWS_BEARER_TOKEN_BEDROCK
  export JUGGERNAUT_NO_TTY_PROMPTS=1
  # Point keychain at a guaranteed-absent service name so we never find cached creds.
  local _stamp
  _stamp="$(date +%s%N 2>/dev/null || date +%s)"
  export JUGGERNAUT_KEYCHAIN_SERVICE="juggernaut-absent-apply-$$-${_stamp}"
}

# ---------------------------------------------------------------------------
# Auth validation gate — no auth, no creds → exit 2
# ---------------------------------------------------------------------------
section "auth validation gate: exits 2 with no --auth and no detectable creds"
_clean_env
FAKE_HOME="$(mktemp -d)"
PATH_BACKUP="$PATH"
# Block `aws sts get-caller-identity` by shadowing aws with a failing stub.
STUB_DIR="$(mktemp -d)"
cat > "$STUB_DIR/aws" <<'EOF'
#!/usr/bin/env bash
exit 255
EOF
chmod +x "$STUB_DIR/aws"
PATH="$STUB_DIR:$PATH" BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --dry-run >/dev/null 2>&1
RC=$?
if [[ "$RC" -eq 2 ]]; then pass; else fail "no-auth, no-creds apply should exit 2 (got $RC)"; fi
rm -rf "$STUB_DIR" "$FAKE_HOME"
PATH="$PATH_BACKUP"

# ---------------------------------------------------------------------------
# Auth via --auth=iam (explicit) — dry-run should succeed
# ---------------------------------------------------------------------------
section "explicit --auth=iam dry-run proceeds"
_clean_env
FAKE_HOME="$(mktemp -d)"
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --dry-run --skip-preflight >/dev/null 2>&1
RC=$?
if [[ "$RC" -eq 0 ]]; then pass; else fail "explicit --auth=iam dry-run should exit 0 (got $RC)"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# AWS_BEARER_TOKEN_BEDROCK present → auth auto-detected as bedrock-api-key
# ---------------------------------------------------------------------------
section "AWS_BEARER_TOKEN_BEDROCK present → auto-detected as bedrock-api-key"
_clean_env
FAKE_HOME="$(mktemp -d)"
AWS_BEARER_TOKEN_BEDROCK="br-test-token" \
  BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --dry-run --skip-preflight >/dev/null 2>&1
RC=$?
if [[ "$RC" -eq 0 ]]; then pass; else fail "apply with AWS_BEARER_TOKEN_BEDROCK should exit 0 (got $RC)"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# Actual write: --auth=iam produces settings.json with juggernaut block + env
# ---------------------------------------------------------------------------
section "--auth=iam writes settings.json with CLAUDE_CODE_USE_BEDROCK=1"
_clean_env
FAKE_HOME="$(mktemp -d)"
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --region=us-west-2 --skip-preflight \
    >/dev/null 2>&1
RC=$?
if [[ "$RC" -eq 0 ]]; then pass; else fail "apply --auth=iam should exit 0 (got $RC)"; fi
SETTINGS="$FAKE_HOME/.claude/settings.json"
if [[ -f "$SETTINGS" ]]; then pass; else fail "settings.json should be written at $SETTINGS"; fi
if jq -e '.juggernaut.auth.mode == "iam"' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "juggernaut.auth.mode should be 'iam'"; fi
if jq -e '.env.CLAUDE_CODE_USE_BEDROCK == "1"' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "env.CLAUDE_CODE_USE_BEDROCK should be '1' (auth validated)"; fi
if jq -e '.env.AWS_REGION == "us-west-2"' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "env.AWS_REGION should be 'us-west-2'"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# Mantle on by default
# ---------------------------------------------------------------------------
section "Mantle is enabled by default"
_clean_env
FAKE_HOME="$(mktemp -d)"
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --skip-preflight >/dev/null 2>&1
SETTINGS="$FAKE_HOME/.claude/settings.json"
if jq -e '.juggernaut.useMantle == true' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "juggernaut.useMantle should be true by default"; fi
if jq -e '.env.CLAUDE_CODE_USE_MANTLE == "1"' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "env.CLAUDE_CODE_USE_MANTLE should be '1' by default"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# --no-mantle disables it
# ---------------------------------------------------------------------------
section "--no-mantle disables Mantle"
_clean_env
FAKE_HOME="$(mktemp -d)"
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --no-mantle --skip-preflight >/dev/null 2>&1
SETTINGS="$FAKE_HOME/.claude/settings.json"
if jq -e '.juggernaut.useMantle == false' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "--no-mantle should set juggernaut.useMantle = false"; fi
if jq -e '.env | has("CLAUDE_CODE_USE_MANTLE") | not' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "--no-mantle should omit env.CLAUDE_CODE_USE_MANTLE"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# --scope=project writes to .claude/settings.json in CWD
# ---------------------------------------------------------------------------
section "--scope=project writes under CWD/.claude/settings.json"
_clean_env
PROJ_DIR="$(mktemp -d)"
cd "$PROJ_DIR" || exit 1
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$(mktemp -d)" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --scope=project --skip-preflight >/dev/null 2>&1
if [[ -f "$PROJ_DIR/.claude/settings.json" ]]; then pass; else fail "--scope=project should write to CWD/.claude/settings.json"; fi
cd "$REPO_ROOT" || exit 1
rm -rf "$PROJ_DIR"

# ---------------------------------------------------------------------------
# Opusplan: --opusplan sets ANTHROPIC_MODEL to 'opusplan' in env
# ---------------------------------------------------------------------------
section "--opusplan sets ANTHROPIC_MODEL to 'opusplan'"
_clean_env
FAKE_HOME="$(mktemp -d)"
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --opusplan --skip-preflight >/dev/null 2>&1
SETTINGS="$FAKE_HOME/.claude/settings.json"
if jq -e '.env.ANTHROPIC_MODEL == "opusplan"' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "--opusplan should set env.ANTHROPIC_MODEL = 'opusplan'"; fi
if jq -e '.juggernaut.opusplan == true' "$SETTINGS" >/dev/null 2>&1; then pass; else fail "--opusplan should set juggernaut.opusplan = true"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# Dry-run: writes nothing
# ---------------------------------------------------------------------------
section "--dry-run writes nothing"
_clean_env
FAKE_HOME="$(mktemp -d)"
BEDROCK_CONFIG_PATH="$BEDROCK_CONFIG_PATH" HOME="$FAKE_HOME" \
  bash "$REPO_ROOT/commands/apply.sh" --auth=iam --dry-run --skip-preflight >/dev/null 2>&1
if [[ ! -f "$FAKE_HOME/.claude/settings.json" ]]; then pass; else fail "--dry-run must not write settings.json"; fi
rm -rf "$FAKE_HOME"

# ---------------------------------------------------------------------------
# Help text
# ---------------------------------------------------------------------------
section "apply.sh --help exits 0 and mentions --auth"
HELP_OUT="$(bash "$REPO_ROOT/commands/apply.sh" --help 2>&1)"
RC=$?
if [[ "$RC" -eq 0 ]]; then pass; else fail "apply --help should exit 0 (got $RC)"; fi
if [[ "$HELP_OUT" == *"--auth"* ]]; then pass; else fail "apply --help should mention --auth"; fi
if [[ "$HELP_OUT" == *"--no-mantle"* ]]; then pass; else fail "apply --help should mention --no-mantle"; fi
if [[ "$HELP_OUT" != *"--legacy-v1"* ]]; then pass; else fail "apply --help should NOT mention --legacy-v1"; fi
if [[ "$HELP_OUT" != *"shell-fallback"* ]]; then pass; else fail "apply --help should NOT mention shell-fallback"; fi

# ---------------------------------------------------------------------------
# Unknown subcommand → non-zero
# ---------------------------------------------------------------------------
section "juggernaut dispatcher rejects unknown subcommand"
if ! bash "$REPO_ROOT/juggernaut" not-a-subcommand >/dev/null 2>&1; then pass; else fail "juggernaut should reject unknown subcommand"; fi

echo
echo "apply.sh tests: $PASS passed, $FAIL failed"
exit "$FAIL"
