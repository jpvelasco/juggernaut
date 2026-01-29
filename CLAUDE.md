# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a one-command setup utility that configures Claude Code to use Amazon Bedrock instead of the direct Anthropic API. It supports macOS, Linux, WSL, and Windows with auto-detection of OS and shell type.

## Commands

### Testing
```bash
bash ./test.sh                    # Run full test suite
```

Tests include: syntax validation, help flags, dry-run mode, region validation, API key auth, JSON validity.

### Validation
```bash
./validate-setup.sh               # Unix/macOS/Linux
.\validate-setup.ps1              # Windows PowerShell
```

### Dry-Run Mode (preview changes without applying)
```bash
./setup --dry-run                           # Auto-detect
./setup-claude-bedrock.sh --dry-run         # Unix explicit
.\setup-claude-bedrock.ps1 -DryRun          # PowerShell
```

### Linting
ShellCheck is configured via `.shellcheckrc`. Disabled rules: SC1091 (non-existent paths), SC2016 (single-quoted expressions).

## Architecture

**Entry Point Flow:**
1. `./setup` - Unified entry point that auto-detects OS/shell
2. Delegates to `setup-claude-bedrock.sh` (Unix) or `setup-claude-bedrock.ps1` (Windows)
3. Scripts modify shell profile files with configuration blocks

**Shell Profile Targets:**
- Bash: `~/.bashrc`
- Zsh: `~/.zshrc`
- Fish: `~/.config/fish/config.fish`
- PowerShell: `$PROFILE`

**Authentication Modes:**
- IAM/SSO (default): Uses AWS credentials from `~/.aws/config`
- API Key: Uses `AWS_BEARER_TOKEN_BEDROCK` environment variable

**Configuration Markers:**
Scripts use `# >>> claude-bedrock-config >>>` and `# <<< claude-bedrock-config <<<` markers to identify managed configuration blocks for updates and uninstallation.

## Key Environment Variables Set

The setup configures Claude Code for Bedrock with:
- `CLAUDE_CODE_USE_BEDROCK=1`
- `ANTHROPIC_MODEL=global.anthropic.claude-opus-4-5-20251101-v1:0` (Global CRIS)
- `ANTHROPIC_SMALL_FAST_MODEL=global.anthropic.claude-sonnet-4-5-20250929-v1:0`
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS=16384` (Bedrock limit)
- Telemetry/autoupdate disabled for enterprise environments

## CI/CD

GitHub Actions workflow (`.github/workflows/test.yml`) runs matrix tests across:
- Ubuntu + macOS: Bash, Zsh, Fish shells
- Windows: PowerShell, Git Bash
