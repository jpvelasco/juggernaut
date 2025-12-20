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
    run_test "setup is executable" "test -x $SCRIPT_DIR/setup"
}

test_json_validity() {
    section "JSON Validation"

    if command -v python3 >/dev/null 2>&1; then
        run_test "iam-policy.json valid (python3)" "python3 -m json.tool $SCRIPT_DIR/iam-policy.json"
    elif command -v jq >/dev/null 2>&1; then
        run_test "iam-policy.json valid (jq)" "jq . $SCRIPT_DIR/iam-policy.json"
    else
        skip_test "iam-policy.json valid" "no JSON validator (install jq or python3)"
    fi
}

test_unified_entry_point() {
    section "Unified Entry Point"
    run_test "setup detects OS" "$SCRIPT_DIR/setup --help | grep -q 'macOS\\|Linux\\|Windows'"
    run_test "setup detects shell types" "$SCRIPT_DIR/setup --help | grep -q 'bash, zsh, fish'"
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
