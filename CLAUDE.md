# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform setup utility that configures Claude Code to use Amazon Bedrock instead of the direct Anthropic API. Supports macOS, Linux, WSL, and Windows with auto-detection of OS and shell type.

## Commands

### Testing
```bash
bash ./test.sh                    # Run full test suite (33 tests)
```

### Dry-Run Mode
```bash
./setup --dry-run                           # Unix (auto-detect shell)
./setup-claude-bedrock.sh bash --dry-run    # Unix (specific shell)
.\setup-claude-bedrock.ps1 -DryRun          # PowerShell
```

### Validation
```bash
./validate-setup.sh               # Unix - checks env vars, AWS creds, Bedrock access
.\validate-setup.ps1              # Windows PowerShell
```

### Linting
ShellCheck via `.shellcheckrc`. Disabled: SC1091, SC2016.

## Architecture

```
./setup → detects OS → setup-claude-bedrock.sh (Unix) or .ps1 (Windows)
                              ↓
                    bedrock-config.json (single source of truth)
                              ↓
                    Shell profile modified with markers
```

**Single Source of Truth:** `bedrock-config.json` contains all environment variables, valid regions, and defaults. Both Bash and PowerShell scripts read from this file (Bash uses jq/python fallback, PowerShell uses ConvertFrom-Json).

**Configuration Markers:** Scripts use `# BEGIN: Claude Code Bedrock Configuration` and `# END: Claude Code Bedrock Configuration` to identify managed blocks for updates/uninstallation.

**Safety Features:**
- Automatic backup before modification (`.backup.YYYYMMDD_HHMMSS`)
- File locking prevents concurrent modifications (flock on Linux, mkdir fallback on macOS)
- Non-interactive mode detection (requires `--bedrock-key` in CI/CD)

## Authentication Modes

| Mode | Flag | Notes |
|------|------|-------|
| IAM/SSO | `--auth=iam` (default) | Uses AWS credentials |
| API Key | `--auth=api-key` | Prompts securely; use `--bedrock-key` for CI/CD |

## Key Files

- `bedrock-config.json` - Environment variables, regions, defaults (edit this to change models)
- `setup-claude-bedrock.sh` - Main Unix implementation (~640 lines)
- `setup-claude-bedrock.ps1` - PowerShell implementation (~280 lines)
- `iam-policy.json` - Required AWS IAM permissions

## CI/CD

GitHub Actions (`.github/workflows/test.yml`):
- Ubuntu + macOS: Bash, Zsh, Fish
- Windows: PowerShell, Git Bash
