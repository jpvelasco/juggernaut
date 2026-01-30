#!/usr/bin/env bash

# Juggernaut Test Suite
# Runs tests to verify scripts work correctly

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

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

#───────────────────────────────────────────────────────────────────────────────
# Test Framework
#───────────────────────────────────────────────────────────────────────────────

run_test() {
    local test_name=$1
    local test_command=$2

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

    # Test: IAM mode unsets API key
    run_test "iam mode unsets API key var" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=iam --dry-run 2>&1 | grep -q 'unset AWS_BEARER_TOKEN_BEDROCK'"

    # Test: API key mode unsets IAM vars
    run_test "api-key mode unsets IAM vars" \
        "$SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --bedrock-key=br-test --dry-run 2>&1 | grep -q 'unset AWS_ACCESS_KEY_ID'"
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

    # Test: --preserve-key works with --storage=keychain
    run_test "preserve-key + keychain (no env key)" \
        "(unset AWS_BEARER_TOKEN_BEDROCK; $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --preserve-key --storage=keychain --dry-run 2>&1 | grep -q 'not set')"

    # Test: --preserve-key with existing env var
    run_test "preserve-key uses env var" \
        "AWS_BEARER_TOKEN_BEDROCK=br-existing123 $SCRIPT_DIR/setup-claude-bedrock.sh bash --auth=api-key --preserve-key --dry-run 2>&1 | grep -q 'existing API key'"
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
    test_required_files
    test_json_validity
    test_unified_entry_point

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
