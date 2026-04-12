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

if [[ "${BASH_VERSINFO[0]}" -lt 4 ]]; then
    echo "This script requires Bash 4.0 or later (found: $BASH_VERSION)"
    echo ""
    echo "Upgrade instructions:"
    echo "  macOS:  brew install bash"
    echo "  Ubuntu: sudo apt install bash"
    echo "  RHEL:   sudo yum install bash"
    exit 1
fi

#───────────────────────────────────────────────────────────────────────────────
# Help
#───────────────────────────────────────────────────────────────────────────────

for arg in "$@"; do
    case "$arg" in
        --version|-v)
            cat "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/VERSION" 2>/dev/null || echo "unknown"
            exit 0
            ;;
        --help|-h)
            cat << 'EOF'
Juggernaut - Configuration Validator

Usage: ./validate-setup.sh

Validates that Claude Code is properly configured for Amazon Bedrock.
Checks environment variables, AWS credentials, Bedrock access, and Claude Code installation.

Options:
  --version, -v Show version
  --help, -h    Show this help message
EOF
            exit 0
            ;;
    esac
done

#───────────────────────────────────────────────────────────────────────────────
# Configuration
#───────────────────────────────────────────────────────────────────────────────

# Find the config file (same directory as this script)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/bedrock-config.json"

# Load expected values from bedrock-config.json (single source of truth)
declare -A EXPECTED_ENV_VARS

load_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        echo "Error: Config file not found: $CONFIG_FILE" >&2
        exit 1
    fi

    local json_content
    json_content=$(cat "$CONFIG_FILE")

    # Try jq first, fall back to python3
    if command -v jq >/dev/null 2>&1; then
        while IFS='=' read -r key value; do
            [[ -n "$key" ]] && EXPECTED_ENV_VARS["$key"]="$value"
        done < <(echo "$json_content" | jq -r '.environment | to_entries[] | "\(.key)=\(.value)"' | tr -d '\r')
    elif command -v python3 >/dev/null 2>&1; then
        while IFS='=' read -r key value; do
            [[ -n "$key" ]] && EXPECTED_ENV_VARS["$key"]="$value"
        done < <(echo "$json_content" | python3 -c "
import sys, json
data = json.load(sys.stdin)
for k, v in data.get('environment', {}).items():
    print(f'{k}={v}')
" | tr -d '\r')
    else
        echo "Error: jq or python3 required to parse config" >&2
        exit 1
    fi
}

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

check_credential_conflicts() {
    local has_api_key=false
    local has_iam_env=false
    local has_aws_profile=false
    local has_aws_creds_file=false
    local conflicts=()

    # Check what credentials are present
    [[ -n "${!API_KEY_VAR}" ]] && has_api_key=true
    [[ -n "$AWS_ACCESS_KEY_ID" || -n "$AWS_SECRET_ACCESS_KEY" ]] && has_iam_env=true
    [[ -n "$AWS_PROFILE" ]] && has_aws_profile=true
    [[ -f "$HOME/.aws/credentials" ]] && has_aws_creds_file=true

    echo -e "${CYAN}Credential Conflict Check${NC}"

    # Check for conflicts when using API key
    if [[ "$has_api_key" == true ]]; then
        if [[ "$has_iam_env" == true ]]; then
            conflicts+=("AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY")
        fi
        if [[ "$has_aws_profile" == true ]]; then
            conflicts+=("AWS_PROFILE=$AWS_PROFILE")
        fi
        if [[ "$has_aws_creds_file" == true ]]; then
            # SC2088: tilde is intentional in user-facing display string, not a path to expand
            # shellcheck disable=SC2088
            conflicts+=("~/.aws/credentials file exists")
        fi

        if [[ ${#conflicts[@]} -gt 0 ]]; then
            echo -e "${YELLOW}WARN${NC} API key mode active, but other credentials also present:"
            for conflict in "${conflicts[@]}"; do
                echo "     - $conflict"
            done
            echo "     API key takes precedence; other credentials are ignored."
            echo "     Consider removing unused credentials to avoid confusion."
            ((WARNINGS++))
        else
            echo -e "${GREEN}PASS${NC} No conflicting credentials detected"
        fi
    else
        # IAM mode - check for credentials file
        if [[ "$has_aws_creds_file" == true ]]; then
            # SC2088: tilde is intentional in user-facing display string, not a path to expand
            # shellcheck disable=SC2088
            echo -e "${GREEN}INFO${NC} ~/.aws/credentials file found (may be used for auth)"
        fi
        if [[ "$has_aws_profile" == true ]]; then
            echo -e "${GREEN}INFO${NC} AWS_PROFILE=$AWS_PROFILE is set"
        fi
        if [[ "$has_iam_env" == false && "$has_aws_profile" == false && "$has_aws_creds_file" == false ]]; then
            echo -e "${YELLOW}WARN${NC} No AWS credentials detected in environment or files"
            ((WARNINGS++))
        else
            echo -e "${GREEN}PASS${NC} IAM credentials configuration looks reasonable"
        fi
    fi
    echo ""
}

check_api_key() {
    local key="${!API_KEY_VAR}"
    if [[ -n "$key" ]]; then
        # Mask the key for display
        local masked="${key:0:8}...${key: -4}"
        echo -e "${GREEN}PASS${NC} $API_KEY_VAR is set ($masked)"

        # Detect key type by prefix
        if [[ "$key" == bedrock-api-key-* ]]; then
            echo -e "${YELLOW}WARN${NC} Short-term API key detected (expires ≤12 hours)"
            echo "     Consider using long-term key for persistent setups"
            ((WARNINGS++))
        elif [[ "$key" == ABSK* ]]; then
            echo -e "${GREEN}INFO${NC} Long-term API key detected"
            echo "     Check expiration in AWS console if issues occur"
        fi
    else
        echo -e "${YELLOW}INFO${NC} $API_KEY_VAR not set (using IAM/SSO auth)"
    fi
}

check_api_key_validity() {
    local key="${!API_KEY_VAR}"
    local region="${AWS_REGION:-us-west-2}"

    if [[ -z "$key" ]]; then
        return 0  # No API key to check
    fi

    echo -e "${CYAN}API Key Validity${NC}"
    echo "  Testing API key with Bedrock..."

    # Make a minimal Bedrock API call to test the key
    # Using converse API with minimal input to test authentication
    local test_result

    # Try to invoke the model with a minimal request
    # This will fail fast if the key is invalid
    # Use the configured fast model (cheapest available) for the probe
    local test_model="${EXPECTED_ENV_VARS[ANTHROPIC_DEFAULT_HAIKU_MODEL]:-anthropic.claude-haiku-4-5-20251001-v1:0}"
    test_result=$(aws bedrock-runtime converse \
        --region "$region" \
        --model-id "$test_model" \
        --messages '[{"role":"user","content":[{"text":"hi"}]}]' \
        --inference-config '{"maxTokens":1}' \
        2>&1)
    local exit_code=$?

    if [[ $exit_code -eq 0 ]]; then
        echo -e "${GREEN}PASS${NC} API key is valid and working"
    elif echo "$test_result" | grep -qi "expired\|invalid.*token\|unauthorized\|forbidden\|access denied"; then
        echo -e "${RED}FAIL${NC} API key appears to be invalid or expired"
        echo "     Claude Code will hang if you try to use it!"
        echo "     Fix: unset AWS_BEARER_TOKEN_BEDROCK"
        echo "     Or:  Get a new API key and run setup with --auth=api-key"
        ((ERRORS++))
    elif echo "$test_result" | grep -qi "could not connect\|timeout\|network"; then
        echo -e "${YELLOW}WARN${NC} Could not reach Bedrock (network issue?)"
        echo "     Unable to verify API key validity"
        ((WARNINGS++))
    else
        # Other error - might be permissions, model access, etc.
        echo -e "${YELLOW}WARN${NC} API key test returned unexpected result"
        echo "     $test_result"
        ((WARNINGS++))
    fi
    echo ""
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

check_bedrock_inference_profile() {
    local region="${AWS_REGION:-us-west-2}"
    local test_model="${EXPECTED_ENV_VARS[ANTHROPIC_DEFAULT_SONNET_MODEL]:-global.anthropic.claude-sonnet-4-6}"

    echo -e "${CYAN}Bedrock Inference Profile Access${NC}"
    echo "  Testing inference profile: $test_model"

    local test_result
    test_result=$(aws bedrock-runtime invoke-model \
        --region "$region" \
        --model-id "$test_model" \
        --body '{"anthropic_version":"bedrock-2023-05-31","max_tokens":10,"messages":[{"role":"user","content":"test"}]}' \
        --cli-binary-format raw-in-base64-out \
        /dev/null 2>&1)
    local exit_code=$?

    if [[ $exit_code -eq 0 ]]; then
        echo -e "${GREEN}PASS${NC} Inference profile accessible"
    elif echo "$test_result" | grep -qi "access denied\|not authorized\|forbidden"; then
        echo -e "${RED}FAIL${NC} Bedrock model access denied"
        echo "     Did you complete the Anthropic FTU form?"
        echo "     -> https://${region}.console.aws.amazon.com/bedrock/home?region=${region}#/anthropic-model-access"
        ((ERRORS++))
    elif echo "$test_result" | grep -qi "could not connect\|timeout\|network"; then
        echo -e "${YELLOW}WARN${NC} Could not reach Bedrock (network issue?)"
        ((WARNINGS++))
    else
        echo -e "${YELLOW}WARN${NC} Inference profile test returned unexpected result"
        echo "     $test_result"
        ((WARNINGS++))
    fi
    echo ""
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
    # Load expected values from config file
    load_config

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

    # Credential conflict check
    check_credential_conflicts

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

        # Test if the API key actually works
        check_api_key_validity
    else
        echo -e "${CYAN}AWS Credentials (IAM/SSO)${NC}"
        check_aws_credentials
        echo ""

        # Bedrock Access (only test with IAM - API key can't be tested without making a call)
        echo -e "${CYAN}Bedrock Access${NC}"
        check_bedrock_access
        echo ""

        # Bedrock Inference Profile Access
        check_bedrock_inference_profile
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
