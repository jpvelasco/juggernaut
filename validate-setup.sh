#!/usr/bin/env bash

# Juggernaut - Claude Code Bedrock Configuration Validator
# Validates that Claude Code is properly configured for Bedrock with Global CRIS
# Usage: ./validate-setup.sh [--help]

#───────────────────────────────────────────────────────────────────────────────
# Bash Check
#───────────────────────────────────────────────────────────────────────────────

if [[ -z "$BASH_VERSION" ]]; then
    echo "This script requires bash"
    exit 1
fi

#───────────────────────────────────────────────────────────────────────────────
# Help
#───────────────────────────────────────────────────────────────────────────────

for arg in "$@"; do
    case "$arg" in
        --help|-h)
            cat << 'EOF'
Juggernaut - Configuration Validator

Usage: ./validate-setup.sh

Validates that Claude Code is properly configured for Amazon Bedrock.
Checks environment variables, AWS credentials, Bedrock access, and Claude Code installation.

Options:
  --help, -h    Show this help message
EOF
            exit 0
            ;;
    esac
done

#───────────────────────────────────────────────────────────────────────────────
# Configuration
#───────────────────────────────────────────────────────────────────────────────

# Expected environment variable values
declare -A EXPECTED_ENV_VARS=(
    [CLAUDE_CODE_USE_BEDROCK]="1"
    [CLAUDE_CODE_MAX_OUTPUT_TOKENS]="16384"
    [MAX_THINKING_TOKENS]="1024"
    [ANTHROPIC_MODEL]="global.anthropic.claude-opus-4-5-20251101-v1:0"
    [ANTHROPIC_SMALL_FAST_MODEL]="global.anthropic.claude-sonnet-4-5-20250929-v1:0"
    [DISABLE_ERROR_REPORTING]="1"
    [DISABLE_TELEMETRY]="1"
    [DISABLE_AUTOUPDATE]="1"
    [DISABLE_BUG_COMMAND]="1"
)

# Variables that just need to be set (any value)
declare -a REQUIRED_ENV_VARS=(AWS_REGION)

# Optional API key variable (for api-key auth mode)
API_KEY_VAR="AWS_BEARER_TOKEN_BEDROCK"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

# Counters
ERRORS=0
WARNINGS=0

#───────────────────────────────────────────────────────────────────────────────
# Detection Functions
#───────────────────────────────────────────────────────────────────────────────

detect_os() {
    case "$OSTYPE" in
        darwin*)      echo "macOS" ;;
        linux*)
            [[ -f /proc/version ]] && grep -qi microsoft /proc/version 2>/dev/null \
                && echo "WSL" || echo "Linux" ;;
        msys*|mingw*) echo "Git Bash" ;;
        cygwin*)      echo "Cygwin" ;;
        *)            echo "Unknown" ;;
    esac
}

detect_shell() {
    if [[ -n "$ZSH_VERSION" ]]; then
        echo "zsh"
    elif [[ -n "$BASH_VERSION" ]]; then
        echo "bash"
    elif [[ -n "$FISH_VERSION" ]]; then
        echo "fish"
    else
        basename "$SHELL"
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Validation Functions
#───────────────────────────────────────────────────────────────────────────────

check_env_var_exact() {
    local var_name=$1
    local expected=$2
    local current="${!var_name}"

    if [[ -z "$current" ]]; then
        echo -e "${RED}FAIL${NC} $var_name is not set"
        ((ERRORS++))
    elif [[ "$current" != "$expected" ]]; then
        echo -e "${YELLOW}WARN${NC} $var_name=$current (expected: $expected)"
        ((WARNINGS++))
    else
        echo -e "${GREEN}PASS${NC} $var_name=$current"
    fi
}

check_env_var_exists() {
    local var_name=$1
    local current="${!var_name}"

    if [[ -z "$current" ]]; then
        echo -e "${RED}FAIL${NC} $var_name is not set"
        ((ERRORS++))
    else
        echo -e "${GREEN}PASS${NC} $var_name=$current"
    fi
}

detect_auth_mode() {
    if [[ -n "${!API_KEY_VAR}" ]]; then
        echo "api-key"
    else
        echo "iam"
    fi
}

check_api_key() {
    local key="${!API_KEY_VAR}"
    if [[ -n "$key" ]]; then
        # Mask the key for display
        local masked="${key:0:8}...${key: -4}"
        echo -e "${GREEN}PASS${NC} $API_KEY_VAR is set ($masked)"
    else
        echo -e "${YELLOW}INFO${NC} $API_KEY_VAR not set (using IAM/SSO auth)"
    fi
}

check_aws_credentials() {
    if aws sts get-caller-identity >/dev/null 2>&1; then
        local account_id
        local user_arn
        account_id=$(aws sts get-caller-identity --query Account --output text 2>/dev/null)
        user_arn=$(aws sts get-caller-identity --query Arn --output text 2>/dev/null)
        echo -e "${GREEN}PASS${NC} AWS credentials valid"
        echo "     Account: $account_id"
        echo "     Identity: $user_arn"
    else
        echo -e "${RED}FAIL${NC} AWS credentials not configured or expired"
        echo "     Run: aws configure or aws sso login"
        ((ERRORS++))
    fi
}

check_bedrock_access() {
    local region="${AWS_REGION:-us-west-2}"

    if aws bedrock list-foundation-models --region "$region" --by-provider anthropic >/dev/null 2>&1; then
        local model_count
        model_count=$(aws bedrock list-foundation-models --region "$region" --by-provider anthropic --query 'length(modelSummaries)' --output text 2>/dev/null)
        echo -e "${GREEN}PASS${NC} Bedrock access confirmed"
        echo "     Available Anthropic models: $model_count"
    else
        echo -e "${RED}FAIL${NC} Cannot access Bedrock models"
        echo "     Check IAM permissions and region availability"
        ((ERRORS++))
    fi
}

check_claude_code() {
    if command -v claude >/dev/null 2>&1; then
        local version
        version=$(claude --version 2>/dev/null || echo "unknown")
        echo -e "${GREEN}PASS${NC} Claude Code installed"
        echo "     Version: $version"
    else
        echo -e "${RED}FAIL${NC} Claude Code not found"
        echo "     Install: npm install -g @anthropic-ai/claude-code"
        ((ERRORS++))
    fi
}

#───────────────────────────────────────────────────────────────────────────────
# Main
#───────────────────────────────────────────────────────────────────────────────

main() {
    echo "Validating Claude Code Bedrock Configuration..."
    echo ""

    # Detect auth mode
    local auth_mode
    auth_mode=$(detect_auth_mode)

    # System Info
    echo -e "${CYAN}System${NC}"
    echo "  OS:    $(detect_os)"
    echo "  Shell: $(detect_shell)"
    echo "  Auth:  $auth_mode"
    echo ""

    # Environment Variables
    echo -e "${CYAN}Environment Variables${NC}"
    for var in "${!EXPECTED_ENV_VARS[@]}"; do
        check_env_var_exact "$var" "${EXPECTED_ENV_VARS[$var]}"
    done
    for var in "${REQUIRED_ENV_VARS[@]}"; do
        check_env_var_exists "$var"
    done
    check_api_key
    echo ""

    # Authentication check (depends on auth mode)
    if [[ "$auth_mode" == "api-key" ]]; then
        echo -e "${CYAN}Authentication (API Key)${NC}"
        echo -e "${GREEN}PASS${NC} Using Bedrock API key authentication"
        echo "     (Skipping IAM credential check - not needed with API key)"
        echo ""
    else
        echo -e "${CYAN}AWS Credentials (IAM/SSO)${NC}"
        check_aws_credentials
        echo ""

        # Bedrock Access (only test with IAM - API key can't be tested without making a call)
        echo -e "${CYAN}Bedrock Access${NC}"
        check_bedrock_access
        echo ""
    fi

    # Claude Code
    echo -e "${CYAN}Claude Code${NC}"
    check_claude_code
    echo ""

    # Summary
    echo -e "${CYAN}Summary${NC}"
    if [[ $ERRORS -eq 0 && $WARNINGS -eq 0 ]]; then
        echo -e "${GREEN}All checks passed! Claude Code is ready for Bedrock.${NC}"
        echo ""
        echo "Next steps:"
        echo "  1. Launch Claude Code: claude"
        echo "  2. Verify it connects to Bedrock (should not prompt for login)"
    elif [[ $ERRORS -eq 0 ]]; then
        echo -e "${YELLOW}Configuration mostly correct with $WARNINGS warning(s)${NC}"
        echo "Claude Code should work, but consider addressing warnings above."
    else
        echo -e "${RED}Found $ERRORS error(s) and $WARNINGS warning(s)${NC}"
        echo "Please fix the errors above before using Claude Code with Bedrock."
        exit 1
    fi
}

main "$@"
