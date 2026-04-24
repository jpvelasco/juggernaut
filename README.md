<p align="center">
  <img src="docs/logo.png" alt="Juggernaut" width="200">
</p>

<h1 align="center">Juggernaut</h1>

<p align="center"><strong>Claude Code Bedrock Setup</strong></p>

**One-command setup for Claude Code with Amazon Bedrock using Global CRIS inference profiles.**

## What This Does

Configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API, with optimized settings for enterprise use:

- **Global CRIS**: Primary model uses cross-region inference for better availability
- **Optimized Tokens**: Bedrock-specific token limits (32768 output, 65536 thinking)
- **Cost Control**: Route through your AWS account for billing/governance
- **Enterprise Ready**: Works with AWS SSO, IAM roles, and corporate identity providers

## Why Bedrock?

| Feature | Direct Anthropic API | Amazon Bedrock |
|---------|---------------------|----------------|
| **Billing** | Separate Anthropic account | Consolidated AWS billing |
| **Authentication** | API keys only | IAM, SSO, roles, federation |
| **Data Residency** | Anthropic infrastructure | Your chosen AWS region |
| **Compliance** | Anthropic's certifications | AWS compliance (SOC, HIPAA, FedRAMP, etc.) |
| **Network** | Public internet | VPC endpoints, PrivateLink |
| **Governance** | Limited controls | IAM policies, CloudTrail, quotas |
| **Cost Tracking** | Anthropic console | AWS Cost Explorer, tags, budgets |

**Choose Bedrock if you:**
- Need consolidated billing through AWS
- Require enterprise SSO/identity federation
- Have data residency or compliance requirements
- Want to use existing AWS governance (IAM, CloudTrail)
- Need private network connectivity (VPC)

**Stick with direct API if you:**
- Want the simplest setup (just an API key)
- Don't have AWS infrastructure
- Are exploring/prototyping individually

## Prerequisites

1. AWS account with Bedrock access enabled
2. Access to Claude models (Opus 4.7, Sonnet 4.6, Haiku 4.5) in Bedrock
3. Claude Code installed
4. Valid AWS credentials
5. Bash 4.0+ (macOS users: `brew install bash`)
6. `jq` or `python3` (for JSON parsing — the setup script checks this automatically)

## Quick Setup

**Prerequisites:**
- AWS account with Bedrock access
- Claude Code installed (`npm install -g @anthropic-ai/claude-code`)
- AWS CLI configured (`aws configure` or SSO)

**Install latest (always tracks main):**
```bash
# Unix/macOS/Linux
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash

# Windows PowerShell
irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1 | iex
```

**Install a pinned version (recommended for stability):**
```bash
# Unix/macOS/Linux
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.1.0

# Windows PowerShell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.1.0
```

**Manual clone:**
```bash
git clone --branch v2.1.0 --depth 1 https://github.com/jpvelasco/juggernaut.git && cd juggernaut
export JUGGERNAUT_USE_V2=1
./juggernaut apply
```

### Version Pinning

By default, the installer clones the `main` branch (always latest). Pass `--version` to install a specific release tag instead:

```bash
# Bash — accepts "2.1.0" or "v2.1.0"
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.1.0

# PowerShell — accepts "2.1.0" or "v2.1.0"
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.1.0

# After downloading
bash install.sh --version 2.1.0
.\install.ps1 -Version 2.1.0
```

Both scripts normalize the version automatically — `2.1.0` and `v2.1.0` both work.

The v2.1 installers also repair executable bits, create a user-local launcher (`~/.local/bin/juggernaut` on Unix-like systems or a PowerShell shim under `$HOME\.local\bin` on Windows), and print the exact verification command. On Windows, first-run script policy friction can usually be resolved with:

```powershell
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
```

## Commands

Activate v2 permanently for your session:
```bash
export JUGGERNAUT_USE_V2=1
```

Or pass `--v2` per command. Then:

```bash
juggernaut apply       # Configure Claude Code for Bedrock
juggernaut show        # Print current configuration
juggernaut doctor      # Diagnose credential and config issues
juggernaut migrate     # Upgrade from a v1 profile block
juggernaut uninstall   # Safely remove all Juggernaut configuration
```

| Command | Description |
|---------|-------------|
| `apply` | Write Juggernaut config to `settings.json`. Supports `--scope=user\|project`, `--dry-run`, `--yes`, `--auth=iam\|bedrock-api-key`, `--1m-context`, `--opusplan`, `--effort`, `--mantle`, and more. |
| `show` | Print the current Juggernaut block from both user and project scopes. |
| `doctor` | Read-only diagnostics — checks credentials, region, models, Mantle status, and drift between settings.json and the shell fallback. |
| `migrate` | Migrate a v1 shell profile block to settings.json. Supports `--dry-run`, `--yes`, `--clean`, `--rollback`. |
| `uninstall` | Remove the Juggernaut block from settings.json (all scopes by default), shell profiles, and OS keychain. Supports `--dry-run`, `--force`, `--scope=user\|project`. |

### v1 Migration

`juggernaut apply --v2` no longer migrates a v1 shell profile block silently. In an interactive terminal it asks before writing; in non-interactive use it exits with a clear message unless you pass `--yes`. Use `--dry-run` to preview the proposed migration without writing anything.

```bash
juggernaut apply --v2 --dry-run
juggernaut apply --v2 --yes
juggernaut migrate --dry-run
juggernaut migrate --yes
```

## Detailed Setup Steps

### 1. Submit Use Case Details (One-time)

First-time Anthropic model users must submit use case details:

1. Go to [Amazon Bedrock Console](https://console.aws.amazon.com/bedrock/)
2. Select **Chat/Text playground**
3. Choose any Anthropic model
4. Fill out the use case form when prompted

### 2. Configure AWS Credentials

Ensure your AWS credentials are configured. Choose one method:

**Option A: AWS CLI**
```bash
aws configure
```

**Option B: SSO Profile (Recommended)**
```bash
aws sso login --profile=<your-profile-name>
export AWS_PROFILE=your-profile-name
```

**Option C: Access Keys**
```bash
export AWS_ACCESS_KEY_ID=your-access-key-id
export AWS_SECRET_ACCESS_KEY=your-secret-access-key
```

Verify credentials:
```bash
aws sts get-caller-identity
```

### 3. Run Setup

```bash
export JUGGERNAUT_USE_V2=1

# Preview changes first
juggernaut apply --dry-run

# Apply
juggernaut apply

# Check everything looks right
juggernaut doctor
```

**Custom region (default: us-west-2):**
```bash
juggernaut apply --region=us-east-1
```

**Skip pre-flight dependency checks:**
```bash
juggernaut apply --skip-preflight
JUGGERNAUT_SKIP_PREFLIGHT=1 juggernaut apply   # via environment variable
```

### API Key Authentication (Alternative)

Instead of IAM/SSO, you can use a Bedrock API key:

**Interactive mode (recommended — secure):**
```bash
juggernaut apply --auth=bedrock-api-key   # Prompts securely for key
```

**Inline mode (for CI/CD and scripting):**
```bash
juggernaut apply --auth=bedrock-api-key --bedrock-key=br-xxxxxxxxxxxx
```

> **Note:** In non-interactive environments (CI/CD, piped input, cron), you must use `--bedrock-key` as the script cannot prompt for input.

`--auth=api-key` is accepted as a legacy compatibility alias. New v2 writes persist `auth.mode` as `bedrock-api-key`.

**Secure keychain storage (optional):**
Use `--storage=keychain` to store your API key in the OS keychain instead of your shell profile.

**API Key Lifetime (AWS Bedrock):**
| Type | Duration | Use Case |
|------|----------|----------|
| Short-term | Up to 12 hours | Production (recommended) |
| Long-term | Up to 30 days | Exploration/testing only |

### 4. Launch Claude Code

```bash
claude
```

## Configuration Details

Juggernaut stores all configuration under a `juggernaut` key in `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). An optional shell profile fallback block can be written alongside it.

Key environment variables set:

- `CLAUDE_CODE_USE_BEDROCK=1` — enables Bedrock integration
- `AWS_REGION=us-west-2` — default region (change as needed)
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS=32768` — required for Bedrock (allows longer responses)
- `MAX_THINKING_TOKENS=65536` — extended reasoning for complex tasks
- `ANTHROPIC_MODEL=global.anthropic.claude-sonnet-4-6` — primary model (Global CRIS)
- `DISABLE_ERROR_REPORTING=1`, `DISABLE_TELEMETRY=1`, `DISABLE_AUTOUPDATE=1`, `DISABLE_BUG_COMMAND=1`
- `ANTHROPIC_DEFAULT_OPUS_MODEL` / `SONNET` / `HAIKU` — `/model` picker entries
- `ENABLE_PROMPT_CACHING_1H=1` — enables 1-hour prompt caching on Bedrock

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|-------------------|
| **Primary** | Claude Sonnet 4.6 | `global.anthropic.claude-sonnet-4-6` |
| **Opus** | Claude Opus 4.7 | `global.anthropic.claude-opus-4-7` |
| **Fast** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

### Model Picker (`/model`)

| Picker Entry | Bedrock Model ID | Description |
|-------------|-----------------|-------------|
| Opus 4.7 (New flagship, native 1M context) | `global.anthropic.claude-opus-4-7` | Most capable — 1M context, high-res vision, stronger agentic reasoning |
| Sonnet 4.6 (Recommended) | `global.anthropic.claude-sonnet-4-6` | Best balance of speed and intelligence |
| Haiku 4.5 (Fast) | `global.anthropic.claude-haiku-4-5-20251001-v1:0` | Fastest model for everyday tasks and subagents |

### 1M Context Windows

Opus 4.7 uses its native 1M context window without a suffix. Enable 1M context for Sonnet with:

```bash
juggernaut apply --1m-context
```

This records the 1M-context preference in the Juggernaut settings block while keeping the official Bedrock model ID intact. To revert:

```bash
juggernaut apply --no-1m-context
```

### Model Capabilities

Juggernaut sets `ANTHROPIC_DEFAULT_*_MODEL_SUPPORTED_CAPABILITIES` for Opus and Sonnet, enabling features Claude Code can't auto-detect from Bedrock inference profile IDs:

| Feature | Opus 4.7 | Sonnet 4.6 | Haiku 4.5 |
|---------|----------|------------|-----------|
| Effort levels | Yes | Yes | No |
| Max effort | Yes | No | No |
| Extended thinking | Yes | Yes | No |
| Adaptive thinking | Yes | Yes | No |
| Interleaved thinking | Yes | Yes | No |

### Custom Models

Override individual model IDs:

```bash
juggernaut apply --opus-model=us.anthropic.claude-opus-4-7
juggernaut apply --sonnet-model=eu.anthropic.claude-sonnet-4-6
juggernaut apply --haiku-model=ap.anthropic.claude-haiku-4-5-20251001-v1:0
juggernaut apply --model-prefix=us    # All models use us.anthropic.* prefix
```

### OpusPlan and Effort

```bash
juggernaut apply --opusplan            # Opus during /plan, Sonnet during execution
juggernaut apply --effort=xhigh        # low | medium | high | xhigh (default) | max
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                      juggernaut apply                               │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                 ┌─────────────────┴─────────────────┐
                 ▼                                   ▼
┌───────────────────────────────┐   ┌───────────────────────────────┐
│   commands/apply.sh           │   │   commands/apply.ps1          │
│   (Unix/macOS/Linux/WSL)      │   │   (Windows PowerShell)        │
└───────────────────────────────┘   └───────────────────────────────┘
                 │                                   │
                 └─────────────────┬─────────────────┘
                                   ▼
                 ┌─────────────────────────────────────┐
                 │       bedrock-config.json           │
                 │   (single source of truth for       │
                 │    env vars, regions, defaults)     │
                 └─────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   ~/.claude/settings.json                           │
│                                                                     │
│  {                                                                  │
│    "juggernaut": {                                                  │
│      "region": "us-west-2",                                         │
│      "model": "global.anthropic.claude-sonnet-4-6",                 │
│      ...                                                            │
│    }                                                                │
│  }                                                                  │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Claude Code → Amazon Bedrock                     │
│            Uses configured env vars for authentication              │
└─────────────────────────────────────────────────────────────────────┘
```

**Key Design Decisions:**
- **Single config file**: `bedrock-config.json` is the source of truth for both Bash and PowerShell
- **Settings.json-first**: configuration lives in `~/.claude/settings.json`, the same file Claude Code reads natively
- **Atomic writes**: config_manager handles backup rotation, file locking, and file-mode preservation
- **Optional shell fallback**: `--shell-fallback-only` writes a profile block in addition to settings.json

## Files

- `juggernaut` / `juggernaut.ps1` — v2 CLI entry point
- `commands/` — subcommand implementations (apply, show, doctor, migrate, uninstall)
- `lib/` — shared libraries (schema, config_manager, profile_writer, keychain, migrator, doctor)
- `install.sh` / `install.ps1` — one-liner installers
- `bedrock-config.json` — single source of truth for env vars, regions, and defaults
- `tests/v2/` — bash and Pester test suites

## Troubleshooting

Run diagnostics first:
```bash
export JUGGERNAUT_USE_V2=1
juggernaut doctor
```

For a bearer-token setup, the credential section should look like:

```text
Credentials
  Auth: Bedrock API key
  Source: AWS_BEARER_TOKEN_BEDROCK
  Status: OK

Mantle
  Status: enabled
  Reason: Bedrock API key detected
```

### Check environment variables:
```bash
echo $CLAUDE_CODE_USE_BEDROCK
echo $AWS_REGION
```

### Verify AWS credentials:
```bash
aws sts get-caller-identity
```

### List available Bedrock models:
```bash
aws bedrock list-foundation-models --region us-west-2 --by-provider anthropic
```

### Authentication Precedence

| Priority | Credential Source | Notes |
|----------|-------------------|-------|
| 1 (highest) | `AWS_BEARER_TOKEN_BEDROCK` | API key always takes precedence if set |
| 2 | Environment variables | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` |
| 3 | AWS credentials file | `~/.aws/credentials` |
| 4 | AWS config/SSO | `~/.aws/config` with profiles |

**Important:** If `AWS_BEARER_TOKEN_BEDROCK` is set but expired, Claude Code will hang — it does NOT fall back to AWS credentials. Unset it and re-run if this happens.

### Common Issues

1. **"API Error: exceeded token maximum"** — restart terminal or `source ~/.zshrc`
2. **Claude Code hangs on startup** — API key may be expired: `unset AWS_BEARER_TOKEN_BEDROCK` then retry
3. **Authentication errors** — re-authenticate: `aws sso login --profile=<profile>`
4. **Region errors** — verify model availability; try `us-east-1` or `us-west-2`
5. **PowerShell execution policy error** — `Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser`
6. **"jq or python3 is required"** — `brew install jq` / `sudo apt install jq` / `winget install jqlang.jq`

## Uninstalling

```bash
export JUGGERNAUT_USE_V2=1

# Preview what will be removed
juggernaut uninstall --dry-run

# Remove all Juggernaut configuration (settings.json, profile block, keychain)
juggernaut uninstall

# Limit to one scope
juggernaut uninstall --scope=user
juggernaut uninstall --scope=project
```

After uninstalling, Claude Code will prompt you to log in with your Anthropic account.

## IAM Permissions Required

Your AWS user/role needs:
- `bedrock:InvokeModel`
- `bedrock:InvokeModelWithResponseStream`
- `bedrock:ListInferenceProfiles`

See `iam-policy.json` for the complete policy.

**Security Note:** The provided IAM policy uses wildcard regions (`arn:aws:bedrock:*:...`) for flexibility. For tighter security, restrict to specific regions (e.g., `arn:aws:bedrock:us-west-2:...`).

## Security Considerations

### Authentication Methods (Most to Least Secure)

| Method | Security | Use Case |
|--------|----------|----------|
| IAM/SSO | Most secure | Production — no secrets in commands |
| API key + keychain | Secure | Key encrypted at rest in OS keychain |
| API key (interactive) | Secure | When IAM not available — hidden prompt |
| API key (inline) | Least secure | CI/CD only — visible in process list |

### API Key Authentication

**Interactive mode (`juggernaut apply --auth=bedrock-api-key`)** is secure:
- Key entered with hidden input (not displayed while typing)
- Not visible in `ps aux` or process listings
- Not saved to shell history

**Inline mode (`--bedrock-key=xxx`)** — use only for CI/CD:
- Command-line arguments are visible to other users via `ps aux`
- Commands are saved to shell history

**For CI/CD**, use secrets management:
```bash
# GitHub Actions
juggernaut apply --auth=bedrock-api-key --bedrock-key=${{ secrets.BEDROCK_KEY }}
```

### Shell Profile Security

- API keys stored in shell profiles are readable by your user account
- Ensure proper file permissions: `chmod 600 ~/.bashrc`
- Backups are created before modifications (`.backup.YYYYMMDD_HHMMSS`)
- Use `--storage=keychain` to store API keys in your OS keychain instead of plaintext profiles

## Notes

- `/login` and `/logout` commands are disabled when using Bedrock
- Authentication is handled through AWS credentials
- `AWS_REGION` is required (Claude Code doesn't read from `.aws/config`)
- Credentials need periodic refresh if using SSO/temporary credentials
