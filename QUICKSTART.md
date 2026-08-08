# Quick Start Guide

## Get Started in 60 Seconds

```bash
npm install -g juggernaut-bedrock
juggernaut apply --auth=iam   # or --auth=bedrock-api-key
# restart shell, then:
claude
```

Claude Code from Anthropic is a prerequisite:

```bash
curl -fsSL https://claude.ai/install.sh | bash
```

Run `juggernaut apply` without flags for an interactive first-run prompt. Apply will not enable Bedrock routing unless a valid credential source is confirmed; if the target config already has foreign values on keys Juggernaut would write, apply refuses unless you pass `--force`.

Upgrading from an older Juggernaut? Install v5 directly with npm; there is no required v3 → v4 → v5 chain. Windows v3 API-key users who need to keep an old DPAPI-stored key should use the bridge script in [README.md](README.md#windows-v3-api-key-installs).

## Prerequisites (if using IAM/SSO)

```bash
# Check AWS CLI is installed
aws --version

# Method A: AWS Configure
aws configure

# Method B: SSO
aws sso login --profile=<your-profile>
export AWS_PROFILE=<your-profile>

# Verify
aws sts get-caller-identity
```

Using `--auth=bedrock-api-key` instead? Skip AWS setup entirely — the key is stored securely in your OS keychain.

## Other Coding CLIs

Codex / OpenCode / Grok route through Mantle and require a Bedrock API key (not IAM):

```bash
juggernaut apply --cli=opencode --auth=bedrock-api-key
juggernaut apply --cli=codex --auth=bedrock-api-key
juggernaut apply --cli=grok --auth=bedrock-api-key
```

Activation blocks for different CLIs coexist in one shell profile. Juggernaut installs a marked shell function and never overwrites the real binary. Restart your shell, or source the updated profile, then run the CLI normally.

## Verify Setup

```bash
juggernaut doctor
```

## What Gets Configured

- Bedrock routing enabled (auth-gated)
- Claude Sonnet 5 as the primary model (Global CRIS inference profile)
- Claude Opus 5 as the default Opus override, with adaptive thinking and 1M context accounting (the default output cap is 32768 tokens; the model supports up to 128K)
- Claude Haiku 4.5 as fast/background model
- Claude Code 1M context accounting for Opus and Sonnet by default
- All three model tiers visible in `/model` selector
- Optimized token limits (32768 output, 65536 thinking)
- Standard Bedrock inference profiles by default; Mantle is opt-in with `--mantle`
- Configuration in `~/.claude/settings.json`
- A non-secret runtime fallback in `~/.juggernaut/runtime/claude.json` so Claude updates cannot silently disable Bedrock routing
- A marked shell activation block that delegates `claude` to `juggernaut launch`

## Common Options

```bash
# OpusPlan — Opus in /plan mode, Sonnet during execution
juggernaut apply --auth=iam --opusplan

# Auto mode — lets Claude Code auto-approve safe tool calls with background checks
juggernaut apply --auth=iam --mode=auto

# Curate the /model picker (user/project settings — not OS-managed enforcement)
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-5 --enforce-available-models

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

**Claude stopped using Bedrock after an update** — Run `juggernaut doctor`; if it reports missing managed config, re-run `juggernaut apply --cli=claude`. The runtime fallback keeps launches routed while you repair.

See [README.md](README.md) for full documentation.
