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
- **Optimized Tokens**: Bedrock-specific token limits (16384 output, 1024 thinking)
- **Cost Control**: Route through your AWS account for billing/governance
- **Enterprise Ready**: Works with AWS SSO, IAM roles, and corporate identity providers

## Prerequisites

1. AWS account with Bedrock access enabled
2. Access to Claude models (Opus 4.5, Sonnet 4.5) in Bedrock
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
echo $ANTHROPIC_MODEL             # Should show: global.anthropic.claude-opus-4-5-20251101-v1:0
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
- `uninstall.sh` - Remove Bedrock configuration from shell profiles (Unix/macOS/Linux)
- `uninstall.ps1` - Remove Bedrock configuration from PowerShell profile (Windows)
- `apply-config.sh` - Apply configuration to current terminal session
- `validate-setup.sh` - Comprehensive configuration validator
- `test.sh` - Test suite for verifying scripts work correctly
- `iam-policy.json` - Required IAM permissions template
- `README.md` - Complete documentation
- `QUICKSTART.md` - 5-minute setup guide

## Configuration Details

The setup adds these environment variables:

- `CLAUDE_CODE_USE_BEDROCK=1` - Enables Bedrock integration
- `AWS_REGION=us-west-2` - Default region (change as needed)
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS=16384` - **Required for Bedrock** (allows longer responses)
- `MAX_THINKING_TOKENS=1024` - Balanced reasoning without cutting off tool responses
- `ANTHROPIC_MODEL=global.anthropic.claude-opus-4-5-20251101-v1:0` - Global CRIS primary model
- `ANTHROPIC_SMALL_FAST_MODEL=global.anthropic.claude-sonnet-4-5-20250929-v1:0` - Global CRIS fast model

## Default Models

- **Primary**: Claude Opus 4.5 (Global CRIS: `global.anthropic.claude-opus-4-5-20251101-v1:0`)
- **Fast**: Claude Sonnet 4.5 (Global CRIS: `global.anthropic.claude-sonnet-4-5-20250929-v1:0`)

**Note**: This configuration uses Global Cross-Region Inference Service (CRIS) profiles for optimal availability and performance across AWS regions. Opus 4.5 provides the most powerful intelligence for complex tasks, while Sonnet 4.5 offers excellent performance for faster operations.

## How Models Are Used

Claude Code uses these two models differently:

| Variable | Model | Usage | Visible in `/model`? |
|----------|-------|-------|---------------------|
| `ANTHROPIC_MODEL` | Opus 4.5 | Primary conversation model - all direct interactions | Yes (as custom model) |
| `ANTHROPIC_SMALL_FAST_MODEL` | Sonnet 4.5 | Background agent tasks - file exploration, quick searches, codebase analysis | No (automatic) |

**What this means in practice:**
- When you chat with Claude Code, you're talking to Opus 4.5
- When Claude Code spawns background agents for tasks like exploring code or quick searches, it automatically uses Sonnet 4.5
- The `/model` command only shows the primary model because the fast model is used internally, not for direct conversation
- This setup optimizes both capability (Opus for complex work) and cost/speed (Sonnet for background operations)

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

### Common Issues

1. **"API Error: exceeded token maximum"**
   - Restart terminal to load new environment variables
   - Run: `source ~/.zshrc` (or your shell config)

2. **Authentication errors**
   - Re-authenticate: `aws sso login --profile=<profile>`
   - Check credentials haven't expired

3. **Region errors**
   - Verify model availability in your region
   - Try `us-east-1` or `us-west-2`

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
