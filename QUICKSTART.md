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

Upgrading from an older Juggernaut? Install v5 directly with npm; there is no required v3 → v4 → v5 chain. Windows v3 API-key users who need to keep an old DPAPI-stored key should use the bridge script in [README.md](README.md#windows-v3-api-key-installs).

### 4. Configure
```bash
# IAM / SSO (recommended for organizations) — Claude Code default
juggernaut apply --auth=iam

# Bedrock API key (key stored securely in OS keychain)
juggernaut apply --auth=bedrock-api-key

# Other coding CLIs
juggernaut apply --cli=opencode --auth=iam
juggernaut apply --cli=codex --auth=iam
juggernaut apply --cli=grok --auth=bedrock-api-key
```

Run without flags for an interactive first-run prompt.

`juggernaut apply` will not enable Bedrock routing unless a valid credential source is confirmed. If the target config already has foreign values on keys Juggernaut would write, apply refuses unless you pass `--force`.

### 5. Launch
```bash
claude
# or codex / opencode / grok after apply --cli=<name>
```

Restart your shell, or source the updated profile, then run the CLI normally. Juggernaut installs a marked shell function and never overwrites the real binary.

## Verify Setup

```bash
juggernaut doctor
```

## What Gets Configured

- Bedrock routing enabled (auth-gated)
- Claude Sonnet 4.6 as primary model (Global CRIS inference profile)
- Claude Haiku 4.5 as fast/background model
- Claude Code 1M context accounting for Opus and Sonnet by default
- All three model tiers visible in `/model` selector
- Optimized token limits (32768 output, 65536 thinking)
- Standard Bedrock inference profiles by default; Mantle is opt-in with `--mantle`
- Configuration in `~/.claude/settings.json`
- A marked shell activation block that delegates `claude` to `juggernaut launch`

## Common Options

```bash
# OpusPlan — Opus in /plan mode, Sonnet during execution
juggernaut apply --auth=iam --opusplan

# Auto mode — lets Claude Code auto-approve safe tool calls with background checks
juggernaut apply --auth=iam --mode=auto

# Curate the /model picker (user/project settings — not OS-managed enforcement)
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-4-8 --enforce-available-models

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

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. Bedrock API key auth requires a Secret Service daemon on Linux.

See [README.md](README.md) for full documentation.
