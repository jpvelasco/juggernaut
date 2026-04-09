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
echo $CLAUDE_CODE_MAX_OUTPUT_TOKENS    # Should output: 32768
echo $ANTHROPIC_MODEL                  # Should output: global.anthropic.claude-sonnet-4-6

# Test Bedrock access
aws bedrock list-foundation-models --region us-west-2 --by-provider anthropic
```

## Configuration Applied

Your setup includes:
- ✅ Bedrock integration enabled
- ✅ Claude Sonnet 4.6 as primary model (Global CRIS)
- ✅ Claude Haiku 4.5 as fast/background model (Global CRIS)
- ✅ Claude Haiku 4.5 available via /model picker (Global CRIS)
- ✅ All three model tiers visible in `/model` selector
- ✅ Optimized token limits for Bedrock (32768 output, 65536 thinking)
- ✅ Persistent configuration in shell profile

## Updating Existing Terminals

If you have terminals open before running setup:
```bash
source apply-config.sh
```

## Fast / Background Model Note

Juggernaut defaults background tasks and subagents to **Haiku 4.5** (`ANTHROPIC_DEFAULT_HAIKU_MODEL`). `ANTHROPIC_SMALL_FAST_MODEL` has been removed as it is officially deprecated. For higher-quality background work: `./setup --fast-model=global.anthropic.claude-sonnet-4-6`. Official Anthropic docs: https://code.claude.com/docs/en/model-config

## 1M Context Windows

Enable 1M token context for Opus and Sonnet: `./setup --1m-context`. Standard context is the default.

## Troubleshooting

### Common 400/403 Errors
- **400 on latest Claude Code**: Already handled — Juggernaut sets `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1`
- **403 Access Denied**: Complete the Anthropic model access form in the Bedrock console
- **Model not found**: Ensure you're using global inference profiles (Juggernaut does this by default)

### Only One Model in /model Picker?
Update to Juggernaut **v1.7.0+** — it fully maps all model tiers (Opus, Sonnet, Haiku) to Bedrock with friendly names, clear descriptions, and **1M context support** for Opus and Sonnet.

## Need Help?

See [README.md](README.md) for detailed documentation and troubleshooting.
