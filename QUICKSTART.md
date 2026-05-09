# Quick Start Guide

## New Machine Setup (5 Minutes)

### 1. Prerequisites Check
```bash
# Check AWS CLI is installed
aws --version

# Check Claude Code is installed (install with: npm install -g @anthropic-ai/claude-code)
claude --version

# Check Bash version (must be 4.0+) — macOS users: brew install bash
bash --version
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

### 3. Install Juggernaut

The installer is a **destructive wipe-and-reinstall**: it strips any legacy Juggernaut shell-profile blocks, removes the `juggernaut` key from `~/.claude/settings.json`, and deletes Juggernaut bearer token storage before placing fresh files. The installer does **not** auto-apply.

```bash
# Unix / macOS / Linux / Git Bash / WSL
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.sh | bash -s -- --version v3.1.0
```

```powershell
# Windows PowerShell (5.1 or 7) — Defender-friendly (see README for AMSI note)
$u='https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.ps1'; $p="$env:TEMP\juggernaut-install.ps1"; irm $u -OutFile $p; Unblock-File $p; & $p -Version v3.1.0; Remove-Item $p
```

Preview the wipe without writing anything:

```bash
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.sh | bash -s -- --version v3.1.0 --dry-run
```

```powershell
$u='https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.ps1'; $p="$env:TEMP\juggernaut-install.ps1"; irm $u -OutFile $p; Unblock-File $p; & $p -Version v3.1.0 -DryRun; Remove-Item $p
```

### 4. Configure

```bash
# IAM / SSO (recommended)
juggernaut apply --auth=iam

# Bedrock API key (interactive — key stored in keychain/Credential Manager/DPAPI/profile storage)
juggernaut apply --auth=bedrock-api-key
```

```powershell
# Windows
.\juggernaut.ps1 apply -Auth iam
.\juggernaut.ps1 apply -Auth bedrock-api-key
```

`juggernaut apply` refuses to write `CLAUDE_CODE_USE_BEDROCK=1` to `settings.json` unless a valid auth source is present (`aws sts get-caller-identity` succeeds, `AWS_BEARER_TOKEN_BEDROCK` is set, or Juggernaut bearer token storage exists).

### 5. Launch
```bash
claude
```

Fresh shells automatically pick up the **launcher** installed in step 3 — a `claude()` shell function appended to `~/.bashrc`/`~/.zshrc`/`~/.profile` on Unix, or a `function claude` block in your PowerShell profile on Windows. The launcher reads your bearer token from Juggernaut bearer token storage (macOS keychain, Windows Credential Manager/DPAPI, or Linux profile token file) and injects it into the child process's environment before running the real `claude`. No manual env setup required, and the function approach survives Anthropic's `claude update` self-rewrites.

## Verify Setup

```bash
juggernaut doctor

aws bedrock list-foundation-models --region us-west-2 --by-provider anthropic
```

## Configuration Applied

Your setup includes:
- Bedrock integration enabled (gated behind explicit auth validation)
- Claude Sonnet 4.6 as primary model (Global CRIS)
- Claude Haiku 4.5 as fast/background model (Global CRIS)
- All three model tiers visible in `/model` selector
- Optimized token limits for Bedrock (32768 output, 65536 thinking)
- Persistent configuration in `~/.claude/settings.json` (no shell profile writes in v3)
- Mantle routing enabled by default

## Fast / Background Model Note

Juggernaut defaults background tasks and subagents to **Haiku 4.5** (`ANTHROPIC_DEFAULT_HAIKU_MODEL`). For higher-quality background work, override the Haiku/subagent model with:

```bash
juggernaut apply --auth=iam --haiku-model=global.anthropic.claude-sonnet-4-6
```

Official Anthropic docs: https://code.claude.com/docs/en/model-config

## 1M Context Windows

Enable 1M token context for Sonnet (Opus uses native 1M by default):

```bash
juggernaut apply --auth=iam --1m-context
```

Standard context is the default.

## OpusPlan

Opus in `/plan` mode, Sonnet during execution:

```bash
juggernaut apply --auth=iam --opusplan
```

`juggernaut doctor` includes an opusplan drift check that catches external overrides of `ANTHROPIC_MODEL`.

## Troubleshooting

### Common 400/403 Errors
- **400 on latest Claude Code**: Already handled — Juggernaut sets `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1`
- **403 Access Denied**: Complete the Anthropic model access form in the Bedrock console
- **Model not found**: Ensure you're using global inference profiles (Juggernaut does this by default)

### `juggernaut apply` exits with "auth validation required"
Pass `--auth=iam` (or `-Auth iam` on PowerShell) or `--auth=bedrock-api-key` explicitly. v3 will not silently enable Bedrock without a validated credential source.

## Need Help?

See [README.md](README.md) for detailed documentation and troubleshooting.
