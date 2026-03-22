# Quick Start Guide

## New Machine Setup (5 Minutes)

### 1. Prerequisites Check
```bash
# Check AWS CLI is installed
aws --version

# Check Claude Code is installed
claude --version

# If not installed:
npm install -g @anthropic-ai/claude-code

# Check Bash version (must be 4.0+)
bash --version

# macOS users with Bash 3.x:
brew install bash
```

### 2. AWS Setup
```bash
# Configure AWS credentials (choose one method)

# Method A: AWS Configure
aws configure

# Method B: SSO
aws sso login --profile=<your-profile>
export AWS_PROFILE=<your-profile>

# Verify credentials work
aws sts get-caller-identity
```

### 3. Run Setup Script
```bash
# Navigate to juggernaut directory
cd ~/path/to/juggernaut

# Run setup (auto-detects your shell)
./setup

# Or specify shell manually
./setup-claude-bedrock.sh zsh      # macOS default
./setup-claude-bedrock.sh bash     # Linux/WSL/Git Bash
./setup-claude-bedrock.sh fish     # Fish shell

# Windows PowerShell
.\setup-claude-bedrock.ps1
```

### 4. Apply & Launch
```bash
# Apply configuration
source ~/.zshrc  # or ~/.bashrc or ~/.config/fish/config.fish

# Windows PowerShell
. $PROFILE.CurrentUserAllHosts

# Launch Claude Code
claude
```

## Verify Setup

```bash
# Check environment variables are set correctly
echo $CLAUDE_CODE_USE_BEDROCK          # Should output: 1
echo $AWS_REGION                       # Should output: us-west-2
echo $CLAUDE_CODE_MAX_OUTPUT_TOKENS    # Should output: 16384
echo $ANTHROPIC_MODEL                  # Should output: global.anthropic.claude-opus-4-5-20251101-v1:0

# Test Bedrock access
aws bedrock list-foundation-models --region us-west-2 --by-provider anthropic
```

## Configuration Applied

Your setup includes:
- ✅ Bedrock integration enabled
- ✅ Claude Opus 4.5 as primary model (Global CRIS)
- ✅ Claude Sonnet 4.5 as fast model (Global CRIS)
- ✅ Optimized token limits for Bedrock (16384 output, 32768 thinking)
- ✅ Persistent configuration in shell profile

**Note:** Only Opus 4.5 appears in the `/model` selector. Sonnet 4.5 is used automatically by Claude Code for background tasks (agent operations, file exploration). This is expected behavior.

## Updating Existing Terminals

If you have terminals open before running setup:
```bash
source apply-config.sh
```

## Need Help?

See [README.md](README.md) for detailed documentation and troubleshooting.
