# Quick Start Guide

## New Machine Setup (5 Minutes)

### 1. Prerequisites Check
```bash
# Check Claude Code is installed
claude --version

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

**npm (recommended — works on every machine that has Claude Code):**
```bash
npm install -g juggernaut-bedrock
```

**curl one-liner (Unix / macOS / Linux / Git Bash / WSL):**
```bash
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/latest/scripts/install.sh | bash
```

**PowerShell (Windows):**
```powershell
irm https://raw.githubusercontent.com/jpvelasco/juggernaut/latest/scripts/install.ps1 | iex
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

The `claude` shim installed by Juggernaut reads your bearer token from the OS keychain and injects it before launching Claude Code. No manual environment setup required.

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
- Configuration in `~/.claude/settings.json` only — no shell profile writes

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

# Migrate from v3 (shell-based) to v4 (Go)
juggernaut migrate
```

## Troubleshooting

**403 Access Denied** — Complete the Anthropic model access request in the AWS Bedrock console.

**Model not found** — Use global inference profile IDs (Juggernaut does this by default).

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. Bedrock API key auth requires a Secret Service daemon or use `--storage=profile`.

See [README.md](README.md) for full documentation.
