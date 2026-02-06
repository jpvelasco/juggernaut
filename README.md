```
                                    ▄█████▄
                                   ▐▓░▀▀▀░▓▌
                                    ▀█████▀
                                 ▄██▀█████▀██▄
                                ▐██▌▐█████▌▐██▌
                                 ▀██ ▀███▀ ██▀
                                     ▄█ █▄
                                    ▐█▌ ▐█▌
                                    ▀▀   ▀▀

     ██╗██╗   ██╗ ██████╗  ██████╗ ███████╗██████╗ ███╗   ██╗ █████╗ ██╗   ██╗████████╗
     ██║██║   ██║██╔════╝ ██╔════╝ ██╔════╝██╔══██╗████╗  ██║██╔══██╗██║   ██║╚══██╔══╝
     ██║██║   ██║██║  ███╗██║  ███╗█████╗  ██████╔╝██╔██╗ ██║███████║██║   ██║   ██║
██   ██║██║   ██║██║   ██║██║   ██║██╔══╝  ██╔══██╗██║╚██╗██║██╔══██║██║   ██║   ██║
╚█████╔╝╚██████╔╝╚██████╔╝╚██████╔╝███████╗██║  ██║██║ ╚████║██║  ██║╚██████╔╝   ██║
 ╚════╝  ╚═════╝  ╚═════╝  ╚═════╝ ╚══════╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝  ╚═╝ ╚═════╝    ╚═╝

                                 Claude Code → Bedrock
```

# Claude Code Bedrock Setup

**One-command setup for Claude Code with Amazon Bedrock using Global CRIS inference profiles.**

## What This Does

Configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API, with optimized settings for enterprise use:

- **Global CRIS**: Primary model uses cross-region inference for better availability
- **Optimized Tokens**: Bedrock-specific token limits (16384 output, 32768 thinking)
- **Cost Control**: Route through your AWS account for billing/governance
- **Enterprise Ready**: Works with AWS SSO, IAM roles, and corporate identity providers

## What's New in v1.3.0

- **Custom Model IDs**: Override default models with `--model` and `--fast-model` flags
  ```bash
  ./setup --model=anthropic.claude-3-opus-20240229-v1:0
  ./setup --fast-model=anthropic.claude-3-haiku-20240307-v1:0
  ```
- **Model Persistence**: Custom models are preserved across re-runs (like auth mode)
- **Reset to Defaults**: Use `--model=default` or `--fast-model=default` to revert to `bedrock-config.json`
- **Format Validation**: Warns on non-standard Bedrock model ID patterns

See [Custom Models](#custom-models) for full details.

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
2. Access to Claude models (Opus 4.6, Sonnet 4.5) in Bedrock
3. Claude Code installed
4. Valid AWS credentials
5. Bash 4.0+ (macOS users: `brew install bash`)

## Quick Setup (New Machine)

**Prerequisites:**
- AWS account with Bedrock access
- Claude Code installed (`npm install -g @anthropic-ai/claude-code`)
- AWS CLI configured (`aws configure` or SSO)

**One-Command Setup:**
```bash
# Clone and run setup
git clone https://github.com/jpvelasco/juggernaut.git && cd juggernaut
./setup  # Auto-detects your OS and shell

# Apply configuration
source ~/.zshrc  # or ~/.bashrc

# Launch Claude Code
claude
```

**Verification:**
```bash
# Quick validation
./validate-setup.sh

# Manual checks
echo $CLAUDE_CODE_USE_BEDROCK     # Should show: 1
echo $ANTHROPIC_MODEL             # Should show: global.anthropic.claude-opus-4-6-v1
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
. $PROFILE
```

### 5. Launch Claude Code

```bash
claude
```

## Files Included

- `setup` - **Unified entry point** (auto-detects OS and shell)
- `setup-claude-bedrock.sh` - Unix/macOS/Linux setup script
- `setup-claude-bedrock.ps1` - Windows PowerShell setup script
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
│  export ANTHROPIC_MODEL=global.anthropic.claude-opus-4-5-...        │
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
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS=16384` - **Required for Bedrock** (allows longer responses)
- `MAX_THINKING_TOKENS=32768` - Extended reasoning for complex tasks
- `ANTHROPIC_MODEL=global.anthropic.claude-opus-4-6-v1` - Global CRIS primary model
- `ANTHROPIC_SMALL_FAST_MODEL=global.anthropic.claude-sonnet-4-5-20250929-v1:0` - Global CRIS fast model
- `DISABLE_ERROR_REPORTING=1` - Disable error reporting to Anthropic
- `DISABLE_TELEMETRY=1` - Disable telemetry collection
- `DISABLE_AUTOUPDATE=1` - Disable automatic updates
- `DISABLE_BUG_COMMAND=1` - Disable the /bug command

## Default Models

- **Primary**: Claude Opus 4.6 (Global CRIS: `global.anthropic.claude-opus-4-6-v1`)
- **Fast**: Claude Sonnet 4.5 (Global CRIS: `global.anthropic.claude-sonnet-4-5-20250929-v1:0`)

**Note**: This configuration uses Global Cross-Region Inference Service (CRIS) profiles for optimal availability and performance across AWS regions. Opus 4.6 provides the most powerful intelligence for complex tasks, while Sonnet 4.5 offers excellent performance for faster operations.

## How Models Are Used

Claude Code uses these two models differently:

| Variable | Model | Usage | Visible in `/model`? |
|----------|-------|-------|---------------------|
| `ANTHROPIC_MODEL` | Opus 4.6 | Primary conversation model - all direct interactions | Yes (as custom model) |
| `ANTHROPIC_SMALL_FAST_MODEL` | Sonnet 4.5 | Background agent tasks - file exploration, quick searches, codebase analysis | No (automatic) |

**What this means in practice:**
- When you chat with Claude Code, you're talking to Opus 4.6
- When Claude Code spawns background agents for tasks like exploring code or quick searches, it automatically uses Sonnet 4.5
- The `/model` command only shows the primary model because the fast model is used internally, not for direct conversation
- This setup optimizes both capability (Opus for complex work) and cost/speed (Sonnet for background operations)

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

Custom models are persisted via comments in the config block and preserved on re-run, just like auth mode. Use `--model=default` to revert.

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

## Uninstalling

To remove the Bedrock configuration and revert to Anthropic's direct API:

**Unix/macOS/Linux:**
```bash
# Remove from specific shell
./uninstall.sh zsh      # or bash/fish

# Remove from all shells
./uninstall.sh all

# Then restart terminal or source your shell config
source ~/.zshrc
```

**Windows (PowerShell):**
```powershell
.\uninstall.ps1

# Then restart PowerShell or reload profile
. $PROFILE
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
