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

Upgrading from an older Juggernaut? Install v6 directly with npm (`npm install -g juggernaut-bedrock@latest`); there is no required v3 → v4 → v5 chain. **v6 is breaking:** Mantle is removed — every CLI now uses `bedrock-runtime`. After upgrading, re-run `juggernaut models refresh --source native --region <region>` and re-apply each CLI (`juggernaut apply --cli=<cli> --region <region>`); a re-apply over a Mantle-era config migrates it in place (backup saved first). Codex now routes through its built-in `amazon-bedrock-runtime` provider and needs Codex CLI ≥ 0.153.4 (`npm i -g @openai/codex@latest` if `apply`/`doctor` warn). Windows v3 API-key users who need to keep an old DPAPI-stored key should still use the bridge script in [README.md](README.md#windows-v3-api-key-installs).

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

Every CLI routes natively via `bedrock-runtime` (Mantle removed in v6). IAM and Bedrock API key both work for every CLI:

```bash
juggernaut apply --cli=opencode --auth=iam              # or --auth=bedrock-api-key
juggernaut apply --cli=codex --auth=iam
juggernaut apply --cli=grok --auth=iam
# After upgrading from a Mantle release, refresh the catalog first:
juggernaut models refresh --source native --region us-west-2
```

Activation blocks for different CLIs coexist in one shell profile. Juggernaut installs a marked shell function and never overwrites the real binary. Restart your shell, or source the updated profile, then run the CLI normally.

## Verify Setup

```bash
juggernaut doctor
juggernaut logs export                 # redacted zip for support
# juggernaut logs export --raw         # local/self only; includes secrets
```

## What Gets Configured

- Bedrock routing enabled (auth-gated)
- Claude Sonnet 5 as the primary model (Global CRIS inference profile)
- Claude Opus 5 as the default Opus override, with adaptive thinking and 1M context accounting (the default output cap is 32768 tokens; the model supports up to 128K)
- Claude Haiku 4.5 as fast/background model
- Claude Code 1M context accounting for Opus and Sonnet by default
- All three model tiers visible in `/model` selector
- Optimized token limits (32768 output, 65536 thinking)
- Native `bedrock-runtime` routing for every CLI (no proxy/Mantle); Codex uses the built-in `amazon-bedrock-runtime` provider (Codex CLI ≥ 0.153.4 — apply/doctor warn on older binaries), OpenCode uses `provider.amazon-bedrock` (region + live models + `whitelist`, omitted when none are discovered — OpenCode's strict schema rejects `null`), Grok uses `https://bedrock-runtime.{region}.amazonaws.com/openai/v1`
- Configuration in `~/.claude/settings.json` (Claude), `~/.codex/config.toml` (Codex), `~/.config/opencode/opencode.json` (OpenCode), `~/.grok/config.toml` (Grok)
- A non-secret runtime fallback in `~/.juggernaut/runtime/claude.json` so Claude updates cannot silently disable Bedrock routing (wrapper-only: `juggernaut launch` reads it; a directly-launched `claude` relies on the `env` block in `settings.json`)
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

# Show current config (default --cli=claude)
juggernaut show
juggernaut show --cli=codex --json
```

## Troubleshooting

**403 Access Denied** — Complete the Anthropic model access request in the AWS Bedrock console.

**Model not found** — Use global inference profile IDs (Juggernaut does this by default).

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. Bedrock API key auth requires a Secret Service daemon on Linux.

**Claude stopped using Bedrock after an update** — Run `juggernaut doctor`; if it reports missing managed config, re-run `juggernaut apply --cli=claude`. The runtime fallback keeps `juggernaut launch` routed while you repair — a directly-launched `claude` only sees Bedrock env while the managed `env` block is in `settings.json`. If doctor flags a symlinked config path, prefer a real file: writes pass through the link and the durable target may strip the managed block.

See [README.md](README.md) for full documentation.
