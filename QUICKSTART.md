# Quick Start Guide

## New Machine Setup (5 Minutes)

### 1. Prerequisites Check
```bash
# Install Claude Code if needed
curl -fsSL https://claude.ai/install.sh | bash

# Check AWS CLI is installed (for IAM/SSO auth)
aws --version
```

### 2. AWS Setup (IAM/SSO only — skip for Bedrock API key)
```bash
# Method A: AWS Configure
aws configure

# Method B: SSO
aws sso login --profile=<your-profile>
export AWS_PROFILE=<your-profile>

# Verify
aws sts get-caller-identity
```

### 3. Install Juggernaut

```bash
npm install -g juggernaut-bedrock
```

### 4. Configure
```bash
# IAM / SSO (recommended for organizations)
juggernaut apply --auth=iam

# Bedrock API key (key stored securely in OS keychain)
juggernaut apply --auth=bedrock-api-key
```

Run without flags for an interactive first-run prompt.

`juggernaut apply` will not write `CLAUDE_CODE_USE_BEDROCK=1` unless a valid credential source is confirmed.

### 5. Launch
```bash
claude
```

Restart your shell, or source the updated profile, then run `claude` normally. Juggernaut installs a shell function and never overwrites the real Claude Code binary.

## Verify Setup

```bash
juggernaut doctor
```

## What Gets Configured

- Bedrock routing enabled (auth-gated)
- Claude Sonnet 4.6 as primary model (Global CRIS inference profile)
- Claude Haiku 4.5 as fast/background model
- All three model tiers visible in `/model` selector
- Optimized token limits (32768 output, 65536 thinking)
- Mantle routing enabled by default
- Configuration in `~/.claude/settings.json`
- A marked shell activation block that delegates `claude` to `juggernaut launch`

## Common Options

```bash
# OpusPlan — Opus in /plan mode, Sonnet during execution
juggernaut apply --auth=iam --opusplan

# Override the subagent/background model
juggernaut apply --auth=iam --haiku-model=global.anthropic.claude-sonnet-4-6

# Preview without writing
juggernaut apply --auth=iam --dry-run

# Show current config
juggernaut show

```

## Troubleshooting

**403 Access Denied** — Complete the Anthropic model access request in the AWS Bedrock console.

**Model not found** — Use global inference profile IDs (Juggernaut does this by default).

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. Bedrock API key auth requires a Secret Service daemon or use `--storage=profile`.

See [README.md](README.md) for full documentation.
