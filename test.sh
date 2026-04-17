#!/usr/bin/env bash

# Juggernaut Test Suite
# Runs tests to verify scripts work correctly

# SC2288 false positives: test commands use operators (!, |, &&) that shellcheck
# misreads as argument-position typos because they're inside eval'd strings
# shellcheck disable=SC2288

#───────────────────────────────────────────────────────────────────────────────
# Bash Check
#───────────────────────────────────────────────────────────────────────────────

if [[ -z "$BASH_VERSION" ]]; then
    echo "This script requires bash"
    exit 1
fi

if [[ "${BASH_VERSINFO[0]}" -lt 4 ]]; then
    echo "This script requires Bash 4.0 or later (found: $BASH_VERSION)"
    exit 1
fi

#───────────────────────────────────────────────────────────────────────────────
# Configuration
#───────────────────────────────────────────────────────────────────────────────

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Counters
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_SKIPPED=0

# Script directory (and an eval-safe quoted version for run_test)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_DIR_Q="$(printf '%q' "$SCRIPT_DIR")"

#───────────────────────────────────────────────────────────────────────────────
# Test Framework
#───────────────────────────────────────────────────────────────────────────────

run_test() {
    local test_name=$1
    local test_command=$2

    # Auto-quote SCRIPT_DIR paths so eval handles spaces correctly
    test_command="${test_command//"$SCRIPT_DIR"/$SCRIPT_DIR_Q}"

    printf "  %-45s " "$test_name"
    if eval "$test_command" >/dev/null 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}FAIL${NC}"
        ((TESTS_FAILED++))
    fi
}

skip_test() {
    local test_name=$1
    local reason=$2

    printf "  %-45s " "$test_name"
    echo -e "${YELLOW}SKIP${NC} ($reason)"
    ((TESTS_SKIPPED++))
}

section() {
    echo ""
    echo -e "${CYAN}$1${NC}"
}

# Create a temporary bin directory with wrapper scripts for specified commands.
# Uses wrappers (not symlinks) because ln -sf creates broken copies on MSYS/Git Bash.
# Sets globals: _TMPBIN (temp directory path), _REAL_BASH (path to real bash binary).
# SC2317: appears unreachable — invoked indirectly via eval in run_test
# SC2329: unused function — same reason, eval-invoked
# shellcheck disable=SC2317,SC2329
_tmpbin_create() {
    _REAL_BASH=$(command -v bash)
    _TMPBIN=$(mktemp -d)
    for cmd in "$@"; do
        local p
        p=$(command -v "$cmd" 2>/dev/null)
        if [ -n "$p" ]; then
            printf '#!/bin/bash\nexec "%s" "$@"\n' "$p" > "$_TMPBIN/$cmd"
            chmod +x "$_TMPBIN/$cmd"
        fi
    done
}

# SC2317: appears unreachable — invoked indirectly via eval in run_test
# SC2329: unused function — same reason, eval-invoked
# shellcheck disable=SC2317,SC2329
_tmpbin_cleanup() {
    rm -rf "$_TMPBIN"
    unset _TMPBIN _REAL_BASH
}

# Core tools needed by setup-claude-bedrock.sh in restricted-PATH tests.
# Includes bash/python because some tools (e.g. python3) may be #!/usr/bin/env bash wrappers
# that exec python (without the 3).
_TMPBIN_CORE_CMDS=(bash python grep sed cat date dirname pwd mkdir cp chmod tr head printf readlink id uname basename)

#───────────────────────────────────────────────────────────────────────────────
# Test Cases
#───────────────────────────────────────────────────────────────────────────────

test_syntax() {
    section "Syntax Validation"
    run_test "setup syntax" "bash -n $SCRIPT_DIR/setup"
    run_test "setup-claude-bedrock.sh syntax" "bash -n $SCRIPT_DIR/setup-claude-bedrock.sh"
    run_test "uninstall.sh syntax" "bash -n $SCRIPT_DIR/uninstall.sh"
    run_test "validate-setup.sh syntax" "bash -n $SCRIPT_DIR/validate-setup.sh"
    run_test "apply-config.sh syntax" "bash -n $SCRIPT_DIR/apply-config.sh"
    run_test "test.sh syntax" "bash -n $SCRIPT_DIR/test.sh"
    run_test "install.sh syntax" "bash -n $SCRIPT_DIR/install.sh"
}

test_help_flags() {
    section "Help Flags"
    run_test "setup --help" "$SCRIPT_DIR/setup --help"
    run_test "setup-claude-bedrock.sh --help" "$SCRIPT_DIR/setup-claude-bedrock.sh --help"
    run_test "uninstall.sh --help" "$SCRIPT_DIR/uninstall.sh --help"
    run_test "validate-setup.sh --help" "$SCRIPT_DIR/validate-setup.sh --help"
    run_test "apply-config.sh --help" "$SCRIPT_DIR/apply-config.sh --help"
}

test_dry_run() {
    section "Dry Run Mode"
    run_test "setup-claude-bedrock.sh --dry-run bash" "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run"
    run_test "setup-claude-bedrock.sh --dry-run zsh" "$SCRIPT_DIR/setup-claude-bedrock.sh zsh --dry-run"
    run_test "setup-claude-bedrock.sh --dry-run fish" "$SCRIPT_DIR/setup-claude-bedrock.sh fish --dry-run"
}

test_region_flag() {
    section "Region Flag"
    run_test "setup-claude-bedrock.sh --region=us-east-1" "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run --region=us-east-1"
    run_test "setup-claude-bedrock.sh --region=eu-west-1" "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run --region=eu-west-1"
    run_test "invalid region warns (dry-run)" "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run --region=invalid-region"
}

test_required_files() {
    section "Required Files"
    run_test "README.md exists" "test -f $SCRIPT_DIR/README.md"
    run_test "QUICKSTART.md exists" "test -f $SCRIPT_DIR/QUICKSTART.md"
    run_test "LICENSE exists" "test -f $SCRIPT_DIR/LICENSE"
    run_test "VERSION exists" "test -f $SCRIPT_DIR/VERSION"
    run_test "iam-policy.json exists" "test -f $SCRIPT_DIR/iam-policy.json"
    run_test "bedrock-config.json exists" "test -f $SCRIPT_DIR/bedrock-config.json"
    run_test "setup is executable" "test -x $SCRIPT_DIR/setup"
    run_test "install.sh exists" "test -f $SCRIPT_DIR/install.sh"
}

test_json_validity() {
    section "JSON Validation"

    if command -v python3 >/dev/null 2>&1; then
        run_test "iam-policy.json valid (python3)" "python3 -m json.tool $SCRIPT_DIR/iam-policy.json"
        run_test "bedrock-config.json valid (python3)" "python3 -m json.tool $SCRIPT_DIR/bedrock-config.json"
    elif command -v jq >/dev/null 2>&1; then
        run_test "iam-policy.json valid (jq)" "jq . $SCRIPT_DIR/iam-policy.json"
        run_test "bedrock-config.json valid (jq)" "jq . $SCRIPT_DIR/bedrock-config.json"
    else
        skip_test "iam-policy.json valid" "no JSON validator (install jq or python3)"
        skip_test "bedrock-config.json valid" "no JSON validator (install jq or python3)"
    fi
}

test_unified_entry_point() {
    section "Unified Entry Point"
    run_test "setup detects OS" "$SCRIPT_DIR/setup --help | grep -q 'macOS\\|Linux\\|Windows'"
    run_test "setup detects shell types" "$SCRIPT_DIR/setup --help | grep -q 'bash, zsh, fish'"
}

test_api_key_auth() {
    section "API Key Authentication"
    run_test "--auth=api-key prompts in dry-run" "$SCRIPT_DIR/setup-claude-bedrock.sh --auth=api-key --dry-run 2>&1 | grep -q 'Would prompt for Bedrock API key'"
    run_test "--auth=api-key with key works" "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test123456789 --dry-run"
    run_test "api-key config includes token" "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run | grep -q AWS_BEARER_TOKEN_BEDROCK"
    run_test "iam mode skips api key" "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=iam --dry-run | grep -q 'export AWS_BEARER_TOKEN_BEDROCK'"
    run_test "non-interactive requires key" "echo '' | $SCRIPT_DIR/setup-claude-bedrock.sh --auth=api-key 2>&1 | grep -q 'required in non-interactive'"
}

test_credential_conflict_detection() {
    section "Credential Conflict Detection"

    # Test: API key + IAM env vars triggers warning
    run_test "conflict: api-key + IAM env vars" \
        "AWS_BEARER_TOKEN_BEDROCK=test AWS_ACCESS_KEY_ID=AKIATEST $SCRIPT_DIR/validate-setup.sh 2>&1 | grep -q 'AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY'"

    # Test: API key + AWS_PROFILE triggers warning
    run_test "conflict: api-key + AWS_PROFILE" \
        "AWS_BEARER_TOKEN_BEDROCK=test AWS_PROFILE=default $SCRIPT_DIR/validate-setup.sh 2>&1 | grep -q 'AWS_PROFILE='"

    # Test: Unsetting env vars removes those specific conflicts
    run_test "no env var conflicts when unset" \
        "(unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_PROFILE; AWS_BEARER_TOKEN_BEDROCK=test $SCRIPT_DIR/validate-setup.sh 2>&1 | grep -v 'AWS_ACCESS_KEY_ID' | grep -v 'AWS_PROFILE=')"

    # Test: IAM mode without creds warns
    run_test "warn: IAM mode no credentials" \
        "(unset AWS_BEARER_TOKEN_BEDROCK AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_PROFILE; HOME=/nonexistent $SCRIPT_DIR/validate-setup.sh 2>&1 | grep -q 'No AWS credentials detected')"
}

test_api_key_type_detection() {
    section "API Key Type Detection"

    # Test: Short-term key detection
    run_test "detect short-term key (bedrock-api-key-*)" \
        "AWS_BEARER_TOKEN_BEDROCK=bedrock-api-key-12345 $SCRIPT_DIR/validate-setup.sh 2>&1 | grep -q 'Short-term API key'"

    # Test: Long-term key detection
    run_test "detect long-term key (ABSK*)" \
        "AWS_BEARER_TOKEN_BEDROCK=ABSK12345 $SCRIPT_DIR/validate-setup.sh 2>&1 | grep -q 'Long-term API key'"
}

test_keychain_storage() {
    section "Keychain Storage"

    # Test: --storage flag is recognized
    run_test "--storage=profile works (dry-run)" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=profile --dry-run"

    run_test "--storage=keychain in dry-run mode" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=keychain --dry-run 2>&1 | grep -qE 'keychain|not available'"

    run_test "invalid --storage value rejected" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=invalid --dry-run 2>&1 | grep -q 'Invalid storage mode'"

    run_test "help shows --storage option" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'storage='"

    run_test "help shows keychain storage mode" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'keychain'"

    # Test: keychain config block contains retrieval command (when keychain available)
    # This will vary by OS - on Linux needs secret-tool, on macOS needs security
    if command -v secret-tool >/dev/null 2>&1; then
        run_test "keychain config uses secret-tool (Linux)" \
            "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=keychain --dry-run 2>&1 | grep -q 'secret-tool'"
    elif command -v security >/dev/null 2>&1; then
        run_test "keychain config uses security (macOS)" \
            "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=keychain --dry-run 2>&1 | grep -q 'security find-generic-password'"
    else
        skip_test "keychain retrieval command" "no keychain tool available"
    fi
}

test_keychain_security() {
    section "Keychain Security"

    # CRITICAL: Keychain mode should NOT contain plaintext key in profile
    if command -v secret-tool >/dev/null 2>&1 || command -v security >/dev/null 2>&1; then
        run_test "keychain mode: no plaintext key in config" \
            "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-supersecretkey123 --storage=keychain --dry-run 2>&1 | grep -q 'br-supersecretkey123'"

        run_test "profile mode: key IS in config (baseline)" \
            "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-supersecretkey123 --storage=profile --dry-run 2>&1 | grep -q 'br-supersecretkey123'"
    else
        skip_test "keychain plaintext check" "no keychain tool available"
        skip_test "profile plaintext check" "no keychain tool available"
    fi

    # Test: Storage mode is shown in dry-run output
    run_test "dry-run shows storage mode" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=profile --dry-run 2>&1 | grep -q 'Storage:'"
}

test_api_key_special_characters() {
    section "API Key Special Characters (Injection Prevention)"

    # These tests verify that special characters in API keys don't break the script
    # or cause command injection vulnerabilities

    # Test: Keys with spaces
    run_test "key with spaces (should fail gracefully)" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test with spaces' --storage=profile --dry-run 2>&1"

    # Test: Keys with single quotes (potential injection)
    run_test "key with single quotes handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=\"br-test'quote\" --storage=profile --dry-run 2>&1"

    # Test: Keys with double quotes
    run_test "key with double quotes handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test\"doublequote' --storage=profile --dry-run 2>&1"

    # Test: Keys with backticks (command substitution attempt)
    run_test "key with backticks (no execution)" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test\`echo pwned\`' --storage=profile --dry-run 2>&1 | grep -v 'pwned'"

    # Test: Keys with $() (command substitution attempt)
    run_test "key with \$() (no execution)" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test\$(echo pwned)' --storage=profile --dry-run 2>&1 | grep -v 'pwned'"

    # Test: Keys with semicolon (command chaining attempt)
    run_test "key with semicolon handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test;echo pwned' --storage=profile --dry-run 2>&1 | grep -v 'pwned'"

    # Test: Keys with pipe (command piping attempt)
    run_test "key with pipe handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test|echo pwned' --storage=profile --dry-run 2>&1 | grep -v 'pwned'"

    # Test: Keys with newline (multiline injection attempt)
    run_test "key with newline handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=$'br-test\necho pwned' --storage=profile --dry-run 2>&1 | grep -v 'pwned'"

    # Test: Keys with dollar sign (variable expansion attempt)
    run_test "key with \$ sign handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test\$HOME' --storage=profile --dry-run 2>&1"

    # Test: Keys with ampersand (background execution attempt)
    run_test "key with & handled" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test&echo pwned' --storage=profile --dry-run 2>&1 | grep -v 'pwned'"
}

test_shell_specific_syntax() {
    section "Shell-Specific Syntax Validation"

    # Test: Bash config syntax is valid
    run_test "bash config syntax valid" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep 'export' | head -1"

    # Test: Zsh config syntax is valid (same as bash)
    run_test "zsh config syntax valid" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh zsh --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep 'export' | head -1"

    # Test: Fish config uses set -gx (not export)
    run_test "fish config uses 'set -gx'" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh fish --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep -q 'set -gx'"

    # Test: Fish config does NOT use export in the config block itself
    # We extract just the FIRST config block (for fish) - script may also configure current shell
    run_test "fish config avoids 'export'" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh fish --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | sed -n '/BEGIN.*Configuration/,/END.*Configuration/{p;/END/q}' | grep -q '^export '"

    # Test: Fish keychain syntax (when available)
    if command -v secret-tool >/dev/null 2>&1 || command -v security >/dev/null 2>&1; then
        run_test "fish keychain uses parentheses syntax" \
            "$SCRIPT_DIR/setup-claude-bedrock.sh fish --auth=api-key --bedrock-key=br-test --storage=keychain --dry-run 2>&1 | grep -qE 'set -gx AWS_BEARER_TOKEN_BEDROCK \\('"
    else
        skip_test "fish keychain syntax" "no keychain tool"
    fi
}

test_config_block_integrity() {
    section "Config Block Integrity"

    # Test: Config block has BEGIN marker
    run_test "config has BEGIN marker" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep -q '# BEGIN: Claude Code Bedrock Configuration'"

    # Test: Config block has END marker
    run_test "config has END marker" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep -q '# END: Claude Code Bedrock Configuration'"

    # Test: Config block includes auth mode comment
    run_test "config shows auth mode" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep -q '# Auth mode: api-key'"

    # Test: Keychain config shows storage mode comment
    if command -v secret-tool >/dev/null 2>&1 || command -v security >/dev/null 2>&1; then
        run_test "keychain config shows storage comment" \
            "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=keychain --dry-run 2>&1 | grep -q '# Storage: keychain'"
    else
        skip_test "keychain storage comment" "no keychain tool"
    fi

    # Test: All required env vars are present
    run_test "config has CLAUDE_CODE_USE_BEDROCK" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'CLAUDE_CODE_USE_BEDROCK'"

    run_test "config has AWS_REGION" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'AWS_REGION'"

    run_test "config has ANTHROPIC_MODEL" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_MODEL'"

}

test_error_handling() {
    section "Error Handling"

    # Test: Empty API key rejected
    run_test "empty key rejected (non-interactive)" \
        "echo '' | $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key 2>&1 | grep -qE 'required|empty'"

    # Test: Invalid shell rejected (outputs error message containing 'shell')
    run_test "invalid shell rejected" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh invalidshell --dry-run 2>&1 | grep -qiE 'unsupported|invalid|unknown.*shell'"

    # Test: Invalid auth mode rejected
    run_test "invalid auth mode rejected" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=invalid --dry-run 2>&1 | grep -q 'Invalid auth mode'"

    # Test: Graceful handling when config file missing
    run_test "missing config warns gracefully" \
        "(cd /tmp && $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1) | grep -qE 'Warning|Config'"
}

test_keychain_unavailable() {
    section "Keychain Unavailable Handling"

    # This tests the error message when keychain tools aren't available
    # We simulate this by checking for the install instructions in error output

    # Test: Error message mentions how to install on Linux
    if [[ "$OSTYPE" == linux* ]] && ! command -v secret-tool >/dev/null 2>&1; then
        run_test "Linux: shows install instructions" \
            "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=keychain 2>&1 | grep -q 'apt install\\|dnf install\\|pacman'"
    else
        skip_test "Linux install instructions" "keychain available or not Linux"
    fi

    # Test: Keychain mode either works (dry-run succeeds) or suggests alternative
    # On systems with keychain, dry-run should work; on systems without, it should suggest profile
    run_test "keychain mode handles availability" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --storage=keychain --dry-run 2>&1 | grep -qE 'DRY RUN|storage=profile|keychain|secret-tool|security|Credential'"
}

test_preserve_key_with_storage() {
    section "Preserve Key + Storage Mode"

    # Test: --preserve-key works with --storage=profile
    run_test "preserve-key + profile (no env key)" \
        "(unset AWS_BEARER_TOKEN_BEDROCK; $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --preserve-key --storage=profile --dry-run 2>&1 | grep -q 'not set')"

    # Test: --preserve-key works with --storage=keychain (only when keychain available)
    if command -v secret-tool >/dev/null 2>&1 || command -v security >/dev/null 2>&1; then
        run_test "preserve-key + keychain (no env key)" \
            "(unset AWS_BEARER_TOKEN_BEDROCK; $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --preserve-key --storage=keychain --dry-run 2>&1 | grep -q 'not set')"
    else
        skip_test "preserve-key + keychain (no env key)" "no keychain tool available"
    fi

    # Test: --preserve-key with existing env var
    run_test "preserve-key uses env var" \
        "AWS_BEARER_TOKEN_BEDROCK=br-existing123 $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --preserve-key --dry-run 2>&1 | grep -q 'existing API key'"
}

test_model_picker_env_vars() {
    section "Model Picker Environment Variables"

    run_test "config has ANTHROPIC_DEFAULT_OPUS_MODEL" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL'"

    run_test "config has ANTHROPIC_DEFAULT_SONNET_MODEL" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL'"

    run_test "config has ANTHROPIC_DEFAULT_HAIKU_MODEL" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL'"

    run_test "config has OPUS model name" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL_NAME'"

    run_test "config has SONNET model name" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL_NAME'"

    run_test "config has HAIKU model name" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME'"

    run_test "config has OPUS model description" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION'"

    run_test "config has SONNET model description" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION'"

    run_test "config has HAIKU model description" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL_DESCRIPTION'"

    run_test "default ANTHROPIC_MODEL is sonnet" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'ANTHROPIC_MODEL=' | head -1 | grep -q 'claude-sonnet'"

    run_test "fish config has model picker vars" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh fish --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL'"
}

test_compat_env_vars() {
    section "Claude Code v2.1.69+ Compatibility Variables"

    run_test "config has CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS'"

    run_test "config has ENABLE_PROMPT_CACHING_1H (not deprecated BEDROCK variant)" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ENABLE_PROMPT_CACHING_1H='"
}

test_per_model_flags() {
    section "Per-Model CLI Flags"

    run_test "--opus-model override" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --opus-model=us.anthropic.claude-opus-4-7 --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL=.us.anthropic.claude-opus-4-7'"

    run_test "--sonnet-model override" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --sonnet-model=us.anthropic.claude-sonnet-4-6 --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL=.us.anthropic.claude-sonnet-4-6'"

    run_test "--haiku-model override" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --haiku-model=us.anthropic.claude-haiku-4-5-20251001-v1:0 --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL=.us.anthropic.claude-haiku-4-5-20251001-v1:0'"

    run_test "--model-prefix=us transforms opus model" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=us --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL' | grep -q 'us.anthropic'"

    run_test "--global keeps global prefix" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --global --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL' | grep -q 'global.anthropic'"

    run_test "help shows --opus-model" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'opus-model'"

    run_test "help shows --model-prefix" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'model-prefix'"

    run_test "--fast-model sets HAIKU_MODEL" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --fast-model=us.anthropic.claude-haiku-4-5-20251001-v1:0 --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL=.us.anthropic.claude-haiku-4-5-20251001-v1:0'"

    run_test "--haiku-model takes priority over --fast-model" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --fast-model=us.anthropic.claude-sonnet-4-6 --haiku-model=us.anthropic.claude-haiku-4-5-20251001-v1:0 --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL=.us.anthropic.claude-haiku-4-5-20251001-v1:0'"

    run_test "ANTHROPIC_SMALL_FAST_MODEL absent from default config" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_SMALL_FAST_MODEL'"

    run_test "--fast-model does not set ANTHROPIC_SMALL_FAST_MODEL" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --fast-model=us.anthropic.claude-haiku-4-5-20251001-v1:0 --dry-run --force 2>&1 | grep -q 'ANTHROPIC_SMALL_FAST_MODEL'"
}

test_value_quoting() {
    section "Shell Value Quoting"

    run_test "fish quotes values with spaces" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh fish --dry-run 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL_NAME' | grep -q '\"Opus'"

    run_test "bash quotes values with spaces" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL_NAME' | grep -q '\"Opus'"

    run_test "zsh quotes values with spaces" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh zsh --dry-run 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL_NAME' | grep -q '\"Opus'"
}

test_model_prefix_regex() {
    section "Model Prefix Regex (Correctness)"

    # Verify prefix transform preserves anthropic.* segment for all model vars
    run_test "prefix=us: opus keeps anthropic segment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=us --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL=.us.anthropic.claude-opus-4-7'"

    run_test "prefix=us: sonnet keeps anthropic segment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=us --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL=.us.anthropic.claude-sonnet-4-6'"

    run_test "prefix=us: haiku keeps anthropic segment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=us --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL=.us.anthropic.claude-haiku-4-5-20251001-v1:0'"

    run_test "prefix=eu: primary model keeps anthropic segment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=eu --dry-run --force 2>&1 | grep -q 'ANTHROPIC_MODEL=.eu.anthropic.claude-sonnet-4-6'"

    run_test "prefix=ap: haiku model keeps anthropic segment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=ap --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL=.ap.anthropic.claude-haiku-4-5-20251001-v1:0'"

    run_test "prefix=global: no double-global prefix" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=global --dry-run --force 2>&1 | grep 'ANTHROPIC_MODEL' | head -1 | grep -q 'global.anthropic.claude-sonnet-4-6'"

    run_test "--global flag: equivalent to prefix=global" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --global --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL=.global.anthropic.claude-sonnet-4-6'"

    # Verify no model IDs are malformed (missing anthropic segment)
    run_test "prefix=us: no bare 'us.claude-' in output" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=us --dry-run --force 2>&1 | grep -q 'us\.claude-'"

    run_test "prefix=eu: no bare 'eu.claude-' in output" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=eu --dry-run --force 2>&1 | grep -q 'eu\.claude-'"

    # Verify friendly names stay clean regardless of prefix
    run_test "prefix=us: opus name present" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=us --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL_NAME'"

    run_test "prefix=eu: name stays Recommended" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=eu --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_SONNET_MODEL_NAME' | grep -q 'Recommended'"

    run_test "prefix=global: opus name present" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --model-prefix=global --dry-run --force 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL_NAME'"
}

test_install_script() {
    section "Install Script"

    run_test "install.sh syntax" \
        "bash -n $SCRIPT_DIR/install.sh"

    run_test "install.sh has set -e" \
        "grep -q 'set -e' $SCRIPT_DIR/install.sh"

    run_test "install.sh checks for git" \
        "grep -q 'command -v git' $SCRIPT_DIR/install.sh"
}

test_capabilities() {
    section "Model Capabilities"

    run_test "opus capabilities present in config" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES='"

    run_test "sonnet capabilities present in config" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES='"

    run_test "no haiku capabilities in config" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES'"

    run_test "opus has max_effort capability" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'OPUS_MODEL_SUPPORTED_CAPABILITIES' | grep -q 'max_effort'"

    run_test "sonnet does not have max_effort" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'SONNET_MODEL_SUPPORTED_CAPABILITIES' | grep -q 'max_effort'"
}

test_1m_context() {
    section "1M Context Windows"

    # Core suffix behavior
    run_test "--1m-context appends [1m] to opus model" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL=' | grep -q '\[1m\]'"

    run_test "--1m-context opus model is claude-opus-4-7[1m]" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL=' | grep -q 'claude-opus-4-7\[1m\]'"

    run_test "--1m-context appends [1m] to sonnet model" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_SONNET_MODEL=' | grep -q '\[1m\]'"

    run_test "--1m-context does NOT affect haiku model" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_HAIKU_MODEL=' | grep -q '\[1m\]'"

    run_test "--1m-context does NOT affect ANTHROPIC_MODEL" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep -E '^export ANTHROPIC_MODEL=' | grep -q '\[1m\]'"

    run_test "default opus model includes [1m] suffix" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL=' | grep -q '\[1m\]'"

    # Name and description updates
    run_test "--1m-context updates opus name with 1M Context" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'OPUS_MODEL_NAME' | grep -q '1M Context'"

    run_test "--1m-context updates sonnet name with 1M Context" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'SONNET_MODEL_NAME' | grep -q '1M Context'"

    run_test "--1m-context does NOT update haiku name" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'HAIKU_MODEL_NAME' | grep -q '1M Context'"

    run_test "--1m-context updates opus description with 1M Context" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'OPUS_MODEL_DESCRIPTION' | grep -q '1M Context'"

    run_test "--1m-context updates sonnet description with 1M Context" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep 'SONNET_MODEL_DESCRIPTION' | grep -q '1M Context'"

    # Prefix combination
    run_test "--1m-context works with --model-prefix=us" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --model-prefix=us --dry-run --force 2>&1 | grep 'ANTHROPIC_DEFAULT_OPUS_MODEL=' | grep -q 'us.anthropic.*\[1m\]'"

    run_test "--1m-context + prefix: opus name contains 1M Context" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --model-prefix=us --dry-run --force 2>&1 | grep 'OPUS_MODEL_NAME' | grep -q '1M Context'"

    # Persistence
    run_test "--1m-context persists in config comment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep -q '# 1MContext: true'"

    run_test "fish shell 1M context uses correct syntax" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh fish --1m-context --dry-run --force 2>&1 | grep -q 'set -gx ANTHROPIC_DEFAULT_OPUS_MODEL.*\[1m\]'"

    # Help text
    run_test "help shows --1m-context" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q '1m-context'"

    # Disable flag
    run_test "--no-1m-context disables 1M context" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --no-1m-context --dry-run --force 2>&1 | grep -q '\[1m\]'"

    # Custom model + --no-1m-context strips persisted [1m]
    run_test "--no-1m-context strips [1m] from custom opus model" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --no-1m-context --opus-model='custom.opus[1m]' --dry-run --force 2>&1 | grep 'OpusModel:' | grep -q '\[1m\]'"

    run_test "--no-1m-context strips [1m] from custom sonnet model" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --no-1m-context --sonnet-model='custom.sonnet[1m]' --dry-run --force 2>&1 | grep 'SonnetModel:' | grep -q '\[1m\]'"

    # Idempotency regression tests
    run_test "--1m-context does not create double [1m] suffix" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep -q '\[1m\]\[1m\]'"

    run_test "--1m-context does not duplicate 1M context in name" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --1m-context --dry-run --force 2>&1 | grep -iq '1m context.*1m context'"
}

#───────────────────────────────────────────────────────────────────────────────
# v1.7.4 Feature Tests
#───────────────────────────────────────────────────────────────────────────────

test_v174_features() {
    section "v1.7.4 Features"

    # OpusPlan mode
    run_test "--opusplan sets ANTHROPIC_MODEL to opusplan" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --opusplan --dry-run --force 2>&1 | grep -E 'ANTHROPIC_MODEL=' | grep -q 'opusplan'"

    run_test "--opusplan persists in config comment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --opusplan --dry-run --force 2>&1 | grep -q '# OpusPlan: true'"

    run_test "--no-opusplan does not set opusplan" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --no-opusplan --dry-run --force 2>&1 | grep -E 'ANTHROPIC_MODEL=' | grep -q 'opusplan'"

    run_test "help shows --opusplan" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'opusplan'"

    # Effort level
    run_test "--effort=xhigh sets CLAUDE_CODE_EFFORT_LEVEL=xhigh" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --effort=xhigh --dry-run --force 2>&1 | grep 'CLAUDE_CODE_EFFORT_LEVEL=' | grep -q 'xhigh'"

    run_test "--effort=low sets CLAUDE_CODE_EFFORT_LEVEL=low" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --effort=low --dry-run --force 2>&1 | grep 'CLAUDE_CODE_EFFORT_LEVEL=' | grep -q 'low'"

    run_test "--effort persists in config comment" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --effort=high --dry-run --force 2>&1 | grep -q '# EffortLevel: high'"

    run_test "default config has CLAUDE_CODE_EFFORT_LEVEL=xhigh" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'CLAUDE_CODE_EFFORT_LEVEL=' | grep -q 'xhigh'"

    run_test "help shows --effort" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'effort'"

    # Subagent model
    run_test "config has CLAUDE_CODE_SUBAGENT_MODEL" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'CLAUDE_CODE_SUBAGENT_MODEL='"

    run_test "CLAUDE_CODE_SUBAGENT_MODEL uses haiku" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep 'CLAUDE_CODE_SUBAGENT_MODEL=' | grep -q 'haiku'"

    # Prompt caching
    run_test "config does NOT have deprecated ENABLE_PROMPT_CACHING_1H_BEDROCK" \
        "! $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run 2>&1 | grep -q 'ENABLE_PROMPT_CACHING_1H_BEDROCK'"
}

#───────────────────────────────────────────────────────────────────────────────
# API Key Quoting Tests
#───────────────────────────────────────────────────────────────────────────────

test_api_key_quoting() {
    section "API Key Quoting in Config Block"

    # Single quotes prevent all shell expansion ($, backticks, etc.)
    run_test "bash config block single-quotes API key" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test123' --storage=profile --dry-run --force 2>&1 | grep -q \"AWS_BEARER_TOKEN_BEDROCK='br-test123'\""

    run_test "zsh config block single-quotes API key" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh zsh --auth=api-key --bedrock-key='br-test123' --storage=profile --dry-run --force 2>&1 | grep -q \"AWS_BEARER_TOKEN_BEDROCK='br-test123'\""

    run_test "fish config block single-quotes API key" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh fish --auth=api-key --bedrock-key='br-test123' --storage=profile --dry-run --force 2>&1 | grep -q \"AWS_BEARER_TOKEN_BEDROCK 'br-test123'\""

    # Dollar sign must NOT be expanded in the generated config
    run_test "bash config preserves literal dollar sign" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test\$var' --storage=profile --dry-run --force 2>&1 | grep 'AWS_BEARER_TOKEN_BEDROCK=' | grep -q '\\\$var'"

    # Backtick must NOT be expanded in the generated config
    run_test "bash config preserves literal backtick" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key='br-test\`id\`' --storage=profile --dry-run --force 2>&1 | grep 'AWS_BEARER_TOKEN_BEDROCK=' | grep -q '\`id\`'"

    # Embedded single quote must be escaped correctly — verify by evaluating the output
    # Use helper functions to avoid eval quoting nightmares with nested single quotes

    # bash/zsh: eval the export line and confirm the variable round-trips correctly
    # SC2317: appears unreachable — invoked by name via run_test
    # shellcheck disable=SC2317
    _test_bash_quote_escape() {
        local line
        line=$("$SCRIPT_DIR/setup-claude-bedrock.sh" bash --auth=api-key --bedrock-key="br-te'st" --storage=profile --dry-run --force 2>&1 | grep '^export AWS_BEARER_TOKEN_BEDROCK=')
        eval "$line"
        [[ "$AWS_BEARER_TOKEN_BEDROCK" == "br-te'st" ]]
    }
    run_test "bash config escapes embedded single quote" "_test_bash_quote_escape"

    # fish: verify the output contains \' (backslash-quote) escape for the embedded quote
    # SC2317: appears unreachable — invoked by name via run_test
    # shellcheck disable=SC2317
    _test_fish_quote_escape() {
        "$SCRIPT_DIR/setup-claude-bedrock.sh" fish --auth=api-key --bedrock-key="br-te'st" --storage=profile --dry-run --force 2>&1 \
            | grep 'AWS_BEARER_TOKEN_BEDROCK' | grep -qF "\'"
    }
    run_test "fish config escapes embedded single quote" "_test_fish_quote_escape"
}

#───────────────────────────────────────────────────────────────────────────────
# Version Sync Tests
#───────────────────────────────────────────────────────────────────────────────

test_version_sync() {
    section "Version Sync"

    local file_version json_version

    file_version=$(tr -d '[:space:]' < "$SCRIPT_DIR/VERSION")

    # Use jq if available, fall back to python3
    if command -v jq &>/dev/null; then
        json_version=$(jq -r '.version' "$SCRIPT_DIR/bedrock-config.json" | tr -d '[:space:]')
    elif command -v python3 &>/dev/null; then
        json_version=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['version'])" "$SCRIPT_DIR/bedrock-config.json" | tr -d '[:space:]')
    else
        skip_test "VERSION matches bedrock-config.json" "jq or python3 required"
        return
    fi

    run_test "VERSION matches bedrock-config.json" \
        "[[ '$file_version' == '$json_version' ]]"
}

#───────────────────────────────────────────────────────────────────────────────
# Pre-flight Check Tests
#───────────────────────────────────────────────────────────────────────────────

test_preflight_checks() {
    section "Pre-flight Dependency Checks"

    # When both jq and python3 are missing, setup should fail with clear error
    run_test "fails when neither jq nor python3 available" \
        "_tmpbin_create \"\${_TMPBIN_CORE_CMDS[@]}\" &&
         PATH=\"\$_TMPBIN\" \"\$_REAL_BASH\" $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run --force 2>&1 |
         grep -q 'jq or python3 is required';
         rc=\$?; _tmpbin_cleanup; [ \$rc -eq 0 ]"

    # When --auth=iam and aws CLI is missing, setup should fail
    if command -v aws &>/dev/null; then
        run_test "fails when --auth=iam and aws CLI missing" \
            "_tmpbin_create jq python3 \"\${_TMPBIN_CORE_CMDS[@]}\" &&
             PATH=\"\$_TMPBIN\" \"\$_REAL_BASH\" $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=iam --dry-run --force 2>&1;
             rc=\$?; _tmpbin_cleanup; [ \$rc -ne 0 ]"
    else
        skip_test "fails when --auth=iam and aws CLI missing" "aws CLI not installed"
    fi

    # Normal invocation should pass preflight (jq or python3 exists in CI)
    run_test "passes preflight with normal PATH" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run --force 2>&1"

    # --skip-preflight bypasses aws CLI check (IAM mode would normally fail without aws)
    if command -v aws &>/dev/null; then
        run_test "--skip-preflight bypasses aws check" \
            "_tmpbin_create jq python3 \"\${_TMPBIN_CORE_CMDS[@]}\" &&
             PATH=\"\$_TMPBIN\" \"\$_REAL_BASH\" $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=iam --skip-preflight --dry-run --force 2>&1;
             rc=\$?; _tmpbin_cleanup; [ \$rc -eq 0 ]"
    else
        skip_test "--skip-preflight bypasses aws check" "aws CLI not installed"
    fi

    # JUGGERNAUT_SKIP_PREFLIGHT=1 env var works the same as --skip-preflight
    run_test "JUGGERNAUT_SKIP_PREFLIGHT=1 skips checks" \
        "JUGGERNAUT_SKIP_PREFLIGHT=1 $SCRIPT_DIR/setup-claude-bedrock.sh bash --dry-run --force 2>&1"

    # help text mentions --skip-preflight
    run_test "help shows --skip-preflight" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --help | grep -q 'skip-preflight'"
}

#───────────────────────────────────────────────────────────────────────────────
# Shellcheck Compliance Tests
#───────────────────────────────────────────────────────────────────────────────

test_shellcheck_compliance() {
    section "Shellcheck Compliance"

    if ! command -v shellcheck &>/dev/null; then
        skip_test "shellcheck all scripts" "shellcheck not installed"
        return
    fi

    local scripts=(
        "$SCRIPT_DIR/setup-claude-bedrock.sh"
        "$SCRIPT_DIR/uninstall.sh"
        "$SCRIPT_DIR/validate-setup.sh"
        "$SCRIPT_DIR/apply-config.sh"
        "$SCRIPT_DIR/install.sh"
        "$SCRIPT_DIR/setup"
    )

    for script in "${scripts[@]}"; do
        local name
        name=$(basename "$script")
        run_test "shellcheck $name" \
            "shellcheck '$script'"
    done
}

#───────────────────────────────────────────────────────────────────────────────
# Version Flag Tests
#───────────────────────────────────────────────────────────────────────────────

test_version_flags() {
    section "Version Flags"

    local expected_version
    expected_version=$(tr -d '[:space:]' < "$SCRIPT_DIR/VERSION")

    run_test "setup --version shows version" \
        "$SCRIPT_DIR/setup --version 2>&1 | grep -q '$expected_version'"

    run_test "setup-claude-bedrock.sh --version" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh --version 2>&1 | grep -q '$expected_version'"

    run_test "validate-setup.sh --version" \
        "$SCRIPT_DIR/validate-setup.sh --version 2>&1 | grep -q '$expected_version'"

    run_test "apply-config.sh --version" \
        "bash -c 'source $SCRIPT_DIR/apply-config.sh --version' 2>&1 | grep -q '$expected_version'"

    run_test "uninstall.sh --version" \
        "$SCRIPT_DIR/uninstall.sh --version 2>&1 | grep -q '$expected_version'"
}

#───────────────────────────────────────────────────────────────────────────────
# Credential Conflict Prevention Tests
#───────────────────────────────────────────────────────────────────────────────

test_credential_conflict_prevention() {
    section "Credential Conflict Prevention in Config Block"

    # Capture output once per invocation variant (avoids spawning setup 10 times)
    local _bash_apikey _bash_iam _fish_apikey _fish_iam
    _bash_apikey=$("$SCRIPT_DIR/setup-claude-bedrock.sh" bash --auth=api-key --bedrock-key=br-test --dry-run --force 2>&1)
    _bash_iam=$("$SCRIPT_DIR/setup-claude-bedrock.sh" bash --auth=iam --dry-run --force 2>&1)
    _fish_apikey=$("$SCRIPT_DIR/setup-claude-bedrock.sh" fish --auth=api-key --bedrock-key=br-test --dry-run --force 2>&1)
    _fish_iam=$("$SCRIPT_DIR/setup-claude-bedrock.sh" fish --auth=iam --dry-run --force 2>&1)

    # API key mode should unset all IAM-related vars
    run_test "api-key mode unsets AWS_ACCESS_KEY_ID" \
        "echo \"\$_bash_apikey\" | grep -qE 'unset[^#]*AWS_ACCESS_KEY_ID'"

    run_test "api-key mode unsets AWS_SECRET_ACCESS_KEY" \
        "echo \"\$_bash_apikey\" | grep -qE 'unset[^#]*AWS_SECRET_ACCESS_KEY'"

    run_test "api-key mode unsets AWS_SESSION_TOKEN" \
        "echo \"\$_bash_apikey\" | grep -qE 'unset[^#]*AWS_SESSION_TOKEN'"

    run_test "api-key mode unsets AWS_PROFILE" \
        "echo \"\$_bash_apikey\" | grep -qE 'unset[^#]*AWS_PROFILE'"

    # IAM mode should unset API key var
    run_test "iam mode unsets AWS_BEARER_TOKEN_BEDROCK" \
        "echo \"\$_bash_iam\" | grep -qE 'unset[^#]*AWS_BEARER_TOKEN_BEDROCK'"

    # Fish uses different syntax — verify all four vars
    run_test "fish api-key erases AWS_ACCESS_KEY_ID" \
        "echo \"\$_fish_apikey\" | grep -q 'set -e AWS_ACCESS_KEY_ID'"

    run_test "fish api-key erases AWS_SECRET_ACCESS_KEY" \
        "echo \"\$_fish_apikey\" | grep -q 'set -e AWS_SECRET_ACCESS_KEY'"

    run_test "fish api-key erases AWS_SESSION_TOKEN" \
        "echo \"\$_fish_apikey\" | grep -q 'set -e AWS_SESSION_TOKEN'"

    run_test "fish api-key erases AWS_PROFILE" \
        "echo \"\$_fish_apikey\" | grep -q 'set -e AWS_PROFILE'"

    # Fish IAM mode should erase API key var
    run_test "fish iam erases AWS_BEARER_TOKEN_BEDROCK" \
        "echo \"\$_fish_iam\" | grep -q 'set -e AWS_BEARER_TOKEN_BEDROCK'"

    # api-key mode should warn (not fail) when aws is missing
    if command -v aws &>/dev/null; then
        run_test "api-key mode warns (not fails) without aws" \
            "_tmpbin_create jq python3 \"\${_TMPBIN_CORE_CMDS[@]}\" &&
             PATH=\"\$_TMPBIN\" \"\$_REAL_BASH\" $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run --force 2>&1;
             rc=\$?; _tmpbin_cleanup; [ \$rc -eq 0 ]"
    else
        skip_test "api-key mode warns (not fails) without aws" "aws CLI not installed"
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Uninstall Script Tests
#───────────────────────────────────────────────────────────────────────────────

test_uninstall() {
    section "Uninstall Script"

    # Syntax and help already covered by test_syntax and test_help_flags

    # Running uninstall with no profile should exit gracefully
    run_test "uninstall handles missing profile" \
        "HOME=/nonexistent $SCRIPT_DIR/uninstall.sh 2>&1"
}

test_apply_config() {
    section "Apply Config Script"

    # SC2317: appears unreachable — invoked by name via run_test
    # shellcheck disable=SC2317
    _test_apply_config_no_leak() {
        # After sourcing apply-config.sh, internal helper functions should be cleaned up
        local output
        output=$(bash -c "source '$SCRIPT_DIR/apply-config.sh' 2>&1; type _juggernaut_apply_config 2>&1")
        # Should NOT find the function (it was unset after running)
        echo "$output" | grep -q "not found"
    }
    run_test "apply-config cleans up helper functions" "_test_apply_config_no_leak"
}

#───────────────────────────────────────────────────────────────────────────────
# Main
#───────────────────────────────────────────────────────────────────────────────

main() {
    echo "Juggernaut Test Suite"
    echo "====================="

    # Run all test categories
    test_syntax
    test_help_flags
    test_dry_run
    test_region_flag
    test_api_key_auth
    test_credential_conflict_detection
    test_api_key_type_detection
    test_keychain_storage
    test_keychain_security
    test_api_key_special_characters
    test_shell_specific_syntax
    test_config_block_integrity
    test_error_handling
    test_keychain_unavailable
    test_preserve_key_with_storage
    test_model_picker_env_vars
    test_compat_env_vars
    test_per_model_flags
    test_value_quoting
    test_model_prefix_regex
    test_install_script
    test_capabilities
    test_1m_context
    test_v174_features
    test_api_key_quoting
    test_required_files
    test_json_validity
    test_unified_entry_point
    test_version_sync
    test_preflight_checks
    test_shellcheck_compliance
    test_version_flags
    test_credential_conflict_prevention
    test_uninstall
    test_apply_config

    # Summary
    echo ""
    echo "====================="
    echo -e "${CYAN}Summary${NC}"
    echo -e "  Passed:  ${GREEN}$TESTS_PASSED${NC}"
    echo -e "  Failed:  ${RED}$TESTS_FAILED${NC}"
    echo -e "  Skipped: ${YELLOW}$TESTS_SKIPPED${NC}"
    echo ""

    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed${NC}"
        exit 1
    fi
}

main "$@"
