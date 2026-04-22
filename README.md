<p align="center">
  <img src="docs/logo.png" alt="Juggernaut" width="200">
</p>

<h1 align="center">Juggernaut</h1>

<p align="center"><strong>Claude Code Bedrock Setup</strong></p>

**One-command setup for Claude Code with Amazon Bedrock using Global CRIS inference profiles.**

## What's New in v2.0

Juggernaut v2.0 introduces a settings.json-first CLI that stores configuration under `~/.claude/settings.json` instead of (or alongside) your shell profile — the same file Claude Code reads natively. The shell profile block is now an optional fallback.

Enable v2 with `--v2` or `export JUGGERNAUT_USE_V2=1`:

```bash
juggernaut apply --v2           # Configure Claude Code for Bedrock
juggernaut show --v2            # Print current configuration
juggernaut doctor --v2          # Diagnose credential and config issues
juggernaut migrate --v2         # Upgrade from a v1 profile block
juggernaut uninstall --v2       # Safely remove all Juggernaut configuration
```

Or activate permanently for your session:
```bash
export JUGGERNAUT_USE_V2=1
juggernaut apply
juggernaut doctor
```

**v1 is unchanged.** Existing `setup` / `setup-claude-bedrock.sh` / `setup-claude-bedrock.ps1` workflows continue to work exactly as before.

### v2.0 Commands

| Command | Description |
|---------|-------------|
| `apply` | Write Juggernaut config to `settings.json`. Supports `--scope=user\|project`, `--dry-run`, `--force`, `--auth=iam\|api-key`, `--1m-context`, `--opusplan`, `--effort`, `--mantle`, and more. |
| `show` | Print the current Juggernaut block from both user and project scopes. |
| `doctor` | Read-only diagnostics — checks credentials, region, models, Mantle status, and drift between settings.json and the shell fallback. |
| `migrate` | Migrate a v1 shell profile block to settings.json. Supports `--dry-run`, `--clean`, `--rollback`. |
| `uninstall` | Remove the Juggernaut block from settings.json (all scopes by default), shell profiles, and OS keychain. Supports `--dry-run`, `--force`, `--scope=user\|project`. |

```bash
# Uninstall with a preview first
juggernaut uninstall --v2 --dry-run

# Uninstall for real
juggernaut uninstall --v2

# Limit to one scope
juggernaut uninstall --v2 --scope=user
```

## What This Does

Configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API, with optimized settings for enterprise use:

- **Global CRIS**: Primary model uses cross-region inference for better availability
- **Optimized Tokens**: Bedrock-specific token limits (32768 output, 65536 thinking)
- **Cost Control**: Route through your AWS account for billing/governance
- **Enterprise Ready**: Works with AWS SSO, IAM roles, and corporate identity providers

## What's New in v1.7.4

Correctness fixes and new Claude Code feature support.

- **Opus 4.7 defaults to 1M context** — `[1m]` suffix is now the default for Opus 4.7 (eliminates duplicate picker entries)
- **`--opusplan` mode** — sets `ANTHROPIC_MODEL=opusplan` so Claude uses Opus during `/plan` and Sonnet during execution
- **`--effort=low|medium|high|xhigh|max`** — sets `CLAUDE_CODE_EFFORT_LEVEL` for all models; `xhigh` is now the default
- **`CLAUDE_CODE_SUBAGENT_MODEL`** — background/subagent work now explicitly routed to Haiku 4.5 for cost efficiency
- **Prompt caching fix** — replaced deprecated `ENABLE_PROMPT_CACHING_1H_BEDROCK` with `ENABLE_PROMPT_CACHING_1H`

## What's New in v1.7.3

Added support for Claude Opus 4.7 — released April 16, 2026 — as the default Opus on Amazon Bedrock.

- **Claude Opus 4.7** (`global.anthropic.claude-opus-4-7[1m]`) — new flagship model in the `/model` picker; 1M context is the default
- 1M context window, high-res vision (up to 2576px / ~3.75MP), new `xhigh` effort level, stronger agentic reasoning, self-verification, and improved long-running task performance
- If you see a model-not-found error, try `--opus-model=us.anthropic.claude-opus-4-7[1m]` while global rollout completes

## What's New in v1.7.2

- **Pre-flight dependency checks** — setup validates required tools (`jq`/`python3`, `aws` CLI) before running, with platform-specific install instructions when something is missing
- **`--skip-preflight`** (`-SkipPreflight` / `JUGGERNAUT_SKIP_PREFLIGHT=1`) to bypass checks in CI or advanced environments
- **Security fixes** — API key quoting hardened across bash, zsh, fish, and PowerShell to prevent shell expansion
- **Shellcheck CI** — all bash scripts linted on every PR with zero warnings
- **173 tests** covering pre-flight checks, credential conflicts, version sync, and more

## What's New in v1.7.1

- **Default Model**: Changed default from Opus 4.6 to Sonnet 4.6 (Recommended) to better match official Claude Code UX
- **Official-Style Model Labels**: `/model` picker names now match Claude Code — "Sonnet 4.6 (Recommended)", "Opus 4.6 (Most capable)", "Haiku 4.5 (Fast)"
- **1M Context Labels**: When `--1m-context` is enabled, names update to "Opus 4.6 (Most capable, 1M Context)" and "Sonnet 4.6 (Recommended, 1M Context)"

## What's New in v1.7.0

- **1M Context Windows**: `--1m-context` / `-OneM` enables 1 million token context for Opus and Sonnet (revert with `--no-1m-context`)
- **Model Capabilities**: Automatically declares effort levels, adaptive thinking, and extended thinking for Bedrock models

## What's New in v1.6.0

- **Full Model Picker**: All Claude models (Opus, Sonnet, Haiku) now appear in the `/model` selector with friendly names and descriptions, all routed through Bedrock global inference profiles
- **Claude Code v2.1.69+ Compatibility**: Automatically sets `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` and `ENABLE_PROMPT_CACHING_1H_BEDROCK` to prevent 400 errors and enable prompt caching
- **Per-Model Overrides**: New `--opus-model`, `--sonnet-model`, `--haiku-model` flags for granular model control
- **Model Prefix**: `--model-prefix=us|eu|ap` to use region-specific inference profiles instead of global
- **One-Line Install**: `curl | bash` (Unix) or `irm | iex` (PowerShell)
- **Improved Validation**: `validate-setup.sh` now tests Bedrock inference profile access directly

See [Model Switching on Bedrock](#model-switching-on-bedrock-v16) for details.

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

## Quick Setup (New Machine)

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
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.0.0

# Windows PowerShell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.0.0
```

**One-Command Setup (manual clone):**
```bash
# Clone and run setup
git clone --branch v2.0.0 --depth 1 https://github.com/jpvelasco/juggernaut.git && cd juggernaut
./setup  # Auto-detects your OS and shell

# Apply configuration
source ~/.zshrc  # or ~/.bashrc

# Launch Claude Code
claude
```

### Version Pinning

By default, the installer clones the `main` branch (always latest). Pass `--version` to install a specific release tag instead:

```bash
# Bash — accepts "2.0.0" or "v2.0.0"
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.sh | bash -s -- --version 2.0.0

# PowerShell — accepts "2.0.0" or "v2.0.0"
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/jpvelasco/juggernaut/main/install.ps1))) -Version 2.0.0

# After downloading
bash install.sh --version 2.0.0
.\install.ps1 -Version 2.0.0
```

Both scripts normalize the version automatically — `2.0.0` and `v2.0.0` both work. Extra arguments after `--version <tag>` are forwarded to the setup script.

**Verification:**
```bash
# Quick validation
./validate-setup.sh

# Manual checks
echo $CLAUDE_CODE_USE_BEDROCK     # Should show: 1
echo $ANTHROPIC_MODEL             # Should show: global.anthropic.claude-sonnet-4-6
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

### 3. Run Setup Script

Use the provided setup script for your operating system:

**For macOS/Linux (Bash):**
```bash
./setup-claude-bedrock.sh bash
```

**For macOS (Zsh - default):**
```bash
./setup-claude-bedrock.sh zsh
```

**For Linux/macOS (Fish):**
```bash
./setup-claude-bedrock.sh fish
```

**For Windows (PowerShell):**

> **Note:** If you get an execution policy error, run this first:
> ```powershell
> Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
> ```

```powershell
.\setup-claude-bedrock.ps1
```

**For Windows (WSL/Git Bash):**
```bash
./setup-claude-bedrock.sh bash
```

**Preview changes (dry run):**
```bash
./setup --dry-run                          # Unix/macOS/Linux
.\setup-claude-bedrock.ps1 -DryRun         # Windows PowerShell
```

**Skip confirmation prompts:**
```bash
./setup --force                            # Unix/macOS/Linux
.\setup-claude-bedrock.ps1 -Force          # Windows PowerShell
```

**Custom region (default: us-west-2):**
```bash
./setup --region=us-east-1                 # Override default region
.\setup-claude-bedrock.ps1 -Region us-east-1  # Windows PowerShell
```

**Skip pre-flight dependency checks:**
```bash
./setup --skip-preflight                   # Unix/macOS/Linux
.\setup-claude-bedrock.ps1 -SkipPreflight  # Windows PowerShell
JUGGERNAUT_SKIP_PREFLIGHT=1 ./setup        # Environment variable (useful for CI)
```

The setup script checks for required tools (`jq`/`python3`, `aws` CLI) before running. Use `--skip-preflight` to bypass the `aws` CLI check if you know your environment is configured correctly or you're using API key authentication.

### API Key Authentication (Alternative)

Instead of IAM/SSO, you can use a Bedrock API key for simpler setup:

**Interactive mode (recommended - secure):**
```bash
./setup --auth=api-key                     # Prompts securely for key
.\setup-claude-bedrock.ps1 -Auth api-key   # Windows PowerShell
```

The script will prompt for your API key with hidden input (like a password):
```
Get your Bedrock API key from:
  AWS Console → Amazon Bedrock → API keys

Enter your Bedrock API key: ********
```

**Inline mode (for CI/CD and scripting):**
```bash
./setup --auth=api-key --bedrock-key=br-xxxxxxxxxxxx
.\setup-claude-bedrock.ps1 -Auth api-key -BedrockKey br-xxxxxxxxxxxx
```

> **Note:** In non-interactive environments (CI/CD, piped input, cron), you must use `--bedrock-key` or `--preserve-key` as the script cannot prompt for input.

**Preserve existing key (reuse from environment):**
```bash
./setup --auth=api-key --preserve-key                 # Reuses AWS_BEARER_TOKEN_BEDROCK from env
.\setup-claude-bedrock.ps1 -Auth api-key -PreserveKey # Windows PowerShell
```

Use `--preserve-key` when you already have `AWS_BEARER_TOKEN_BEDROCK` set in your environment and want to keep using it. This is useful for re-running setup without re-entering your key.

**Secure keychain storage (optional):**
For enhanced security, you can store your API key in the OS keychain instead of your shell profile using `--storage=keychain`.

**API Key Lifetime (AWS Bedrock):**
| Type | Duration | Use Case |
|------|----------|----------|
| Short-term | Up to 12 hours | Production (recommended) |
| Long-term | Up to 30 days | Exploration/testing only |

### 4. Apply Configuration

**Bash/Zsh:**
```bash
source ~/.bashrc  # or ~/.zshrc
```

**Fish:**
```fish
source ~/.config/fish/config.fish
```

**PowerShell (Windows):**
```powershell
. $PROFILE.CurrentUserAllHosts
```

### 5. Launch Claude Code

```bash
claude
```

## Files Included

- `setup` - **Unified entry point** (auto-detects OS and shell)
- `setup-claude-bedrock.sh` - Unix/macOS/Linux setup script
- `setup-claude-bedrock.ps1` - Windows PowerShell setup script
- `install.sh` - One-liner installer for Unix/macOS/Linux
- `install.ps1` - One-liner installer for Windows PowerShell
- `bedrock-config.json` - **Single source of truth** for environment variables and settings
- `uninstall.sh` - Remove Bedrock configuration from shell profiles (Unix/macOS/Linux)
- `uninstall.ps1` - Remove Bedrock configuration from PowerShell profile (Windows)
- `apply-config.sh` - Apply configuration to current terminal session (Unix/macOS/Linux)
- `apply-config.ps1` - Apply configuration to current PowerShell session (Windows)
- `validate-setup.sh` - Comprehensive configuration validator (Unix/macOS/Linux)
- `validate-setup.ps1` - Comprehensive configuration validator (Windows)
- `test.sh` - Test suite for verifying scripts work correctly
- `iam-policy.json` - Required IAM permissions template
- `README.md` - Complete documentation
- `QUICKSTART.md` - 5-minute setup guide

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         User runs ./setup                           │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     setup (unified entry point)                     │
│                   Detects OS: macOS/Linux/Windows                   │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                 ┌─────────────────┴─────────────────┐
                 ▼                                   ▼
┌───────────────────────────────┐   ┌───────────────────────────────┐
│   setup-claude-bedrock.sh     │   │   setup-claude-bedrock.ps1    │
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
│                      Shell Profile Modified                         │
│  ~/.bashrc | ~/.zshrc | ~/.config/fish/config.fish | $PROFILE       │
│                                                                     │
│  # BEGIN: Claude Code Bedrock Configuration                         │
│  export CLAUDE_CODE_USE_BEDROCK=1                                   │
│  export AWS_REGION=us-west-2                                        │
│  export ANTHROPIC_MODEL=global.anthropic.claude-sonnet-4-6          │
│  ...                                                                │
│  # END: Claude Code Bedrock Configuration                           │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Claude Code → Amazon Bedrock                     │
│            Uses configured env vars for authentication              │
└─────────────────────────────────────────────────────────────────────┘
```

**Key Design Decisions:**
- **Single config file**: `bedrock-config.json` is the source of truth for both Bash and PowerShell scripts
- **Marker-based config**: Scripts use `# BEGIN/END` markers to safely update/remove configuration
- **Automatic backups**: Profile files are backed up before modification (`.backup.YYYYMMDD_HHMMSS`)
- **File locking**: Prevents concurrent modifications to profile files

## Configuration Details

The setup adds these environment variables:

- `CLAUDE_CODE_USE_BEDROCK=1` - Enables Bedrock integration
- `AWS_REGION=us-west-2` - Default region (change as needed)
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS=32768` - **Required for Bedrock** (allows longer responses)
- `MAX_THINKING_TOKENS=65536` - Extended reasoning for complex tasks
- `ANTHROPIC_MODEL=global.anthropic.claude-sonnet-4-6` - Global CRIS primary model (Sonnet 4.6, recommended)
- `DISABLE_ERROR_REPORTING=1` - Disable error reporting to Anthropic
- `DISABLE_TELEMETRY=1` - Disable telemetry collection
- `DISABLE_AUTOUPDATE=1` - Disable automatic updates
- `DISABLE_BUG_COMMAND=1` - Disable the /bug command
- `ANTHROPIC_DEFAULT_OPUS_MODEL` - Opus model for /model picker (Global CRIS)
- `ANTHROPIC_DEFAULT_SONNET_MODEL` - Sonnet model for /model picker (Global CRIS)
- `ANTHROPIC_DEFAULT_HAIKU_MODEL` - Haiku model for /model picker (Global CRIS)
- `ANTHROPIC_DEFAULT_*_MODEL_NAME` - Friendly names shown in /model picker
- `ANTHROPIC_DEFAULT_*_MODEL_DESCRIPTION` - Descriptions shown in /model picker
- `ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES` - Declares Opus capabilities (effort, thinking, etc.)
- `ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES` - Declares Sonnet capabilities
- `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1` - Prevents 400 errors on Claude Code v2.1.69+
- `ENABLE_PROMPT_CACHING_1H_BEDROCK=1` - Enables 1-hour prompt caching on Bedrock

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|-------------------|
| **Primary** | Claude Sonnet 4.6 | `global.anthropic.claude-sonnet-4-6` |
| **Fast** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |
| **Haiku** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

## How Models Are Used

Claude Code uses multiple models for different purposes:

| Variable | Model | Usage | Visible in `/model`? |
|----------|-------|-------|---------------------|
| `ANTHROPIC_MODEL` | Sonnet 4.6 | Primary conversation model - all direct interactions | Yes |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | Haiku 4.5 | Background tasks, subagents, and quick tasks via /model | Yes |

**What this means in practice:**
- When you chat with Claude Code, you're talking to Sonnet 4.6 (recommended default)
- When Claude Code spawns background agents, it uses Haiku 4.5 (fastest, lowest cost)
- The `/model` command shows all three tiers (Opus, Sonnet, Haiku) — you can switch between them
- For higher-quality background work: `./setup --fast-model=global.anthropic.claude-sonnet-4-6`

### Fast / Background Model (v1.6+)

Juggernaut now defaults background tasks, subagents, exploration, and lightweight operations to **Haiku 4.5** (`ANTHROPIC_DEFAULT_HAIKU_MODEL`). This follows Anthropic's current recommendation for better cost efficiency and speed on internal operations.

`ANTHROPIC_SMALL_FAST_MODEL` has been **removed** as it is officially deprecated.

To use Sonnet 4.6 for higher-quality background tasks:
```bash
./setup --fast-model=global.anthropic.claude-sonnet-4-6
```

Official Anthropic docs: https://code.claude.com/docs/en/model-config

### 1M Context Windows (v1.7+)

Opus 4.7 and Sonnet 4.6 support a 1 million token context window. Enable it with:

```bash
./setup --1m-context
.\setup-claude-bedrock.ps1 -OneM
```

This appends `[1m]` to the Opus and Sonnet model IDs. Claude Code strips the suffix before sending to Bedrock — no changes needed on the AWS side. Standard context (~200K) remains the default.

This setting persists across re-runs. To revert to standard context:

```bash
./setup --no-1m-context
.\setup-claude-bedrock.ps1 -NoOneM
```

**What it sets:**
- `ANTHROPIC_DEFAULT_OPUS_MODEL` -> `global.anthropic.claude-opus-4-7[1m]`
- `ANTHROPIC_DEFAULT_SONNET_MODEL` -> `global.anthropic.claude-sonnet-4-6[1m]`
- Model names updated to include "1M Context"
- Haiku is not affected (does not support 1M context)

> **Tip:** For large codebases, `--1m-context` lets Opus and Sonnet work with up to 1M tokens while Haiku stays fast at ~200K.

### Model Capabilities (v1.7+)

Juggernaut sets `ANTHROPIC_DEFAULT_*_MODEL_SUPPORTED_CAPABILITIES` for Opus and Sonnet. This enables features that Claude Code can't auto-detect from Bedrock inference profile IDs:

| Feature | Opus 4.7 | Sonnet 4.6 | Haiku 4.5 |
|---------|----------|------------|-----------|
| Effort levels | Yes | Yes | No |
| Max effort | Yes | No | No |
| Extended thinking | Yes | Yes | No |
| Adaptive thinking | Yes | Yes | No |
| Interleaved thinking | Yes | Yes | No |

These are set automatically — no flags needed.

Official docs: https://code.claude.com/docs/en/model-config

## Custom Models

Override the default model IDs from `bedrock-config.json`:

```bash
# Unix/macOS/Linux
./setup --model=anthropic.claude-3-opus-20240229-v1:0
./setup --fast-model=anthropic.claude-3-haiku-20240307-v1:0
./setup --model=anthropic.claude-3-opus-20240229-v1:0 --fast-model=anthropic.claude-3-haiku-20240307-v1:0

# Windows PowerShell
.\setup-claude-bedrock.ps1 -Model anthropic.claude-3-opus-20240229-v1:0
.\setup-claude-bedrock.ps1 -FastModel anthropic.claude-3-haiku-20240307-v1:0
```

**Reset to defaults:**
```bash
./setup --model=default --fast-model=default
.\setup-claude-bedrock.ps1 -Model default -FastModel default
```

| Flag | PowerShell | Notes |
|------|------------|-------|
| `--model=ID` | `-Model ID` | Custom primary model |
| `--fast-model=ID` | `-FastModel ID` | Custom fast model |
| `--model=default` | `-Model default` | Reset to bedrock-config.json default |
| `--opusplan` | `-OpusPlan` | Use Opus during plan mode, Sonnet during execution — keeps costs down while getting Opus-quality plans |
| `--no-opusplan` | `-NoOpusPlan` | Disable opusplan mode |
| `--effort=LEVEL` | `-Effort LEVEL` | Set effort level: `low`, `medium`, `high`, `xhigh` (default), `max` |

Custom models are persisted via comments in the config block and preserved on re-run, just like auth mode. Use `--model=default` to revert.

## Model Switching on Bedrock (v1.6+)

Juggernaut maps all Claude models to Bedrock global inference profiles. The `/model` picker shows:

| Picker Entry | Bedrock Model ID | Description |
|-------------|-----------------|-------------|
| Opus 4.7 (New flagship – 1M context) | `global.anthropic.claude-opus-4-7[1m]` | Most capable model yet — 1M context, high-res vision, xhigh effort by default, stronger agentic reasoning and self-verification |
| Sonnet 4.6 (Recommended) | `global.anthropic.claude-sonnet-4-6` | Best balance of speed and intelligence |
| Haiku 4.5 (Fast) | `global.anthropic.claude-haiku-4-5-20251001-v1:0` | Fastest model for everyday tasks and subagents |

Override individual models:
```bash
./setup --opus-model=us.anthropic.claude-opus-4-7[1m]
./setup --sonnet-model=eu.anthropic.claude-sonnet-4-6
./setup --haiku-model=ap.anthropic.claude-haiku-4-5-20251001-v1:0
```

Use region-specific inference profiles:
```bash
./setup --model-prefix=us    # All models use us.anthropic.* prefix
./setup --model-prefix=eu    # All models use eu.anthropic.* prefix
./setup --global             # Explicit global prefix (default)
```

## Troubleshooting

### Check if environment variables are set:
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

When using Bedrock, Claude Code follows this precedence:

| Priority | Credential Source | Notes |
|----------|-------------------|-------|
| 1 (highest) | `AWS_BEARER_TOKEN_BEDROCK` | API key always takes precedence if set |
| 2 | Environment variables | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` |
| 3 | AWS credentials file | `~/.aws/credentials` |
| 4 | AWS config/SSO | `~/.aws/config` with profiles |

**Important behavior:**
- If `AWS_BEARER_TOKEN_BEDROCK` is set but **invalid or expired**, Claude Code will **hang** - it does NOT fall back to AWS credentials
- There is no automatic fallback between authentication methods
- If your API key expires, you must either get a new key or `unset AWS_BEARER_TOKEN_BEDROCK`

**Recommendation:** Use one authentication method at a time. The setup script handles this by unsetting conflicting environment variables, but cannot modify `~/.aws/credentials`.

### Common Issues

1. **"API Error: exceeded token maximum"**
   - Restart terminal to load new environment variables
   - Run: `source ~/.zshrc` (or your shell config)

2. **Claude Code hangs on startup**
   - API key may be expired/invalid: `unset AWS_BEARER_TOKEN_BEDROCK` then retry
   - Or get a new API key and re-run setup with `--auth=api-key`
   - Run `./validate-setup.sh` to test API key validity

3. **Authentication errors**
   - Re-authenticate: `aws sso login --profile=<profile>`
   - Check credentials haven't expired

4. **Region errors**
   - Verify model availability in your region
   - Try `us-east-1` or `us-west-2`

5. **PowerShell execution policy error** (Windows)
   - Run: `Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser`
   - Then retry the setup script

6. **Permission denied writing to profile**
   - Check file permissions on your shell profile
   - On Windows, try running PowerShell as Administrator
   - A backup is automatically created before modifications

7. **"jq or python3 is required" error**
   - Install jq: `brew install jq` (macOS), `sudo apt install jq` (Linux), `winget install jqlang.jq` (Windows)
   - Or install python3 as a fallback

## Uninstalling

**v2 (recommended):**
```bash
# Preview what will be removed
juggernaut uninstall --v2 --dry-run

# Remove all Juggernaut configuration (settings.json, profile block, keychain)
juggernaut uninstall --v2

# Limit to one scope
juggernaut uninstall --v2 --scope=user
juggernaut uninstall --v2 --scope=project
```

**v1 (legacy — profile-only):**

Unix/macOS/Linux:
```bash
./uninstall.sh zsh   # or bash/fish
./uninstall.sh all   # all shells
source ~/.zshrc
```

Windows (PowerShell):
```powershell
.\uninstall.ps1
. $PROFILE.CurrentUserAllHosts
```

After uninstalling, Claude Code will prompt you to log in with your Anthropic account.

## Notes

- `/login` and `/logout` commands are disabled when using Bedrock
- Authentication is handled through AWS credentials
- `AWS_REGION` is required (Claude Code doesn't read from `.aws/config`)
- Credentials need periodic refresh if using SSO/temporary credentials

## IAM Permissions Required

Your AWS user/role needs:
- `bedrock:InvokeModel`
- `bedrock:InvokeModelWithResponseStream`
- `bedrock:ListInferenceProfiles`

See `iam-policy.json` for the complete policy.

**Security Note:** The provided IAM policy uses wildcard regions (`arn:aws:bedrock:*:...`) for flexibility. For tighter security, you can restrict to specific regions by replacing `*` with your region (e.g., `arn:aws:bedrock:us-west-2:...`).

## Security Considerations

### Authentication Methods (Most to Least Secure)

| Method | Security | Use Case |
|--------|----------|----------|
| IAM/SSO | Most secure | Production - no secrets in commands |
| API key + keychain | Secure | Key encrypted at rest in OS keychain |
| API key (interactive) | Secure | When IAM not available - hidden prompt |
| API key (inline) | Least secure | CI/CD only - visible in process list |

### API Key Authentication

**Interactive mode (`./setup --auth=api-key`)** is secure:
- Key is entered with hidden input (not displayed while typing)
- Not visible in `ps aux` or process listings
- Not saved to shell history

**Inline mode (`--bedrock-key=xxx`)** has risks - use only for CI/CD:

1. **Process visibility**: Command-line arguments are visible to other users via `ps aux`:
   ```
   $ ps aux | grep setup
   user  12345  ./setup --auth=api-key --bedrock-key=br-YOUR_KEY_IS_VISIBLE
   ```

2. **Shell history**: Commands are saved to `~/.bash_history` or `~/.zsh_history`

3. **System logs**: Some systems log process execution with arguments

**Recommendations:**

- **Prefer IAM/SSO authentication** when possible (most secure, no secrets anywhere)
- **Use interactive mode** for API key auth: `./setup --auth=api-key`
- **Clear shell history** if you used inline mode:
  ```bash
  history -d $(history 1 | awk '{print $1}')  # Delete last command (bash)
  ```
- **For CI/CD**, use secrets management:
  ```bash
  # GitHub Actions
  ./setup --auth=api-key --bedrock-key=${{ secrets.BEDROCK_KEY }}

  # Generic CI/CD (key from environment)
  ./setup --auth=api-key --bedrock-key="$BEDROCK_KEY"
  ```
- **On shared systems**: Use IAM roles or run setup in a private session

### Shell Profile Security

- API keys stored in shell profiles (`~/.bashrc`, `~/.zshrc`) are readable by your user account
- Ensure proper file permissions: `chmod 600 ~/.bashrc`
- Backups are created before modifications (`.backup.YYYYMMDD_HHMMSS`)
- **For enhanced security**, use `--storage=keychain` to store API keys in your OS keychain instead of plaintext in shell profiles
