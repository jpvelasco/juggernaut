# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform setup utility that configures Claude Code to use Amazon Bedrock instead of the direct Anthropic API. Supports macOS, Linux, WSL, and Windows with auto-detection of OS and shell type.

## Commands

### Testing
```bash
bash ./test.sh                    # Run full test suite (requires Bash 4.0+)
```
Tests run as a full suite only (no individual test execution). Categories: syntax validation, help flags, dry-run mode, region handling, API key auth, credential conflicts, keychain storage, shell-specific syntax, config block integrity, error handling.

### Dry-Run Mode
```bash
./setup --dry-run                           # Unix (auto-detect shell)
./setup-claude-bedrock.sh bash --dry-run    # Unix (specific shell)
.\setup-claude-bedrock.ps1 -DryRun          # PowerShell
```

### Force Mode (skip prompts)
```bash
./setup --force                             # Unix
.\setup-claude-bedrock.ps1 -Force           # PowerShell
```

### Preserve Existing API Key
```bash
./setup --auth=api-key --preserve-key       # Reuse AWS_BEARER_TOKEN_BEDROCK from env
```

### Upgrade Models (Preserve Auth Mode)
```bash
./setup --force --preserve-key              # Re-running preserves existing auth mode automatically
```
When re-running setup without `--auth`, the script detects the existing auth mode from the config block (`# Auth mode: api-key` or `# Auth mode: iam`) and preserves it. Use explicit `--auth=` to override.

### Keychain Storage (optional)
```bash
./setup --auth=api-key --storage=keychain   # Store key in OS keychain instead of profile
.\setup-claude-bedrock.ps1 -Auth api-key -Storage keychain  # PowerShell
```
Requires: `libsecret-tools` (Linux), Keychain (macOS), or Credential Manager (Windows).

### Validation
```bash
./validate-setup.sh               # Unix - checks env vars, AWS creds, Bedrock access
.\validate-setup.ps1              # Windows PowerShell
```

### Uninstall
```bash
./uninstall.sh zsh                # Remove from specific shell (bash/zsh/fish)
./uninstall.sh all                # Remove from all shells
.\uninstall.ps1                   # PowerShell
```

### Apply Config (current session only)
```bash
source ./apply-config.sh              # Unix - apply to current terminal without modifying profile
. .\apply-config.ps1                  # PowerShell
```

### Linting
```bash
shellcheck setup-claude-bedrock.sh uninstall.sh validate-setup.sh apply-config.sh
```
ShellCheck configured via `.shellcheckrc`. Disabled: SC1091 (source paths), SC2016 (single-quote expressions).

## Architecture

```
./setup → detects OS → setup-claude-bedrock.sh (Unix) or .ps1 (Windows)
                              ↓
                    bedrock-config.json (single source of truth)
                              ↓
                    Shell profile modified with markers
```

**Single Source of Truth:** `bedrock-config.json` contains all environment variables, valid regions, and defaults. Both Bash and PowerShell scripts read from this file.

**JSON Parsing Fallback Chain (Bash):** jq → python3 → python. Scripts work without jq installed.

**Configuration Markers:** Scripts use `# BEGIN: Claude Code Bedrock Configuration` and `# END: Claude Code Bedrock Configuration` to identify managed blocks for updates/uninstallation.

**Safety Features:**
- Automatic backup before modification (`.backup.YYYYMMDD_HHMMSS`)
- File locking prevents concurrent modifications (flock on Linux, mkdir fallback on macOS)
- Non-interactive mode detection (requires `--bedrock-key` or `--preserve-key` in CI/CD)
- Credential conflict detection warns when multiple auth methods are present
- Setup script unsets conflicting env vars (`AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, etc.) when switching auth modes
- Auth mode preservation: re-running setup detects existing auth mode from `# Auth mode:` comment in config block

## Authentication Modes

| Mode | Flag | Notes |
|------|------|-------|
| IAM/SSO | `--auth=iam` (default) | Uses AWS credentials |
| API Key | `--auth=api-key` | Prompts securely; use `--bedrock-key` for CI/CD |
| Preserve Key | `--auth=api-key --preserve-key` | Reuses existing `AWS_BEARER_TOKEN_BEDROCK` from env |

## Storage Modes

| Mode | Flag | Notes |
|------|------|-------|
| Profile | `--storage=profile` (default) | API key stored in shell profile (plaintext) |
| Keychain | `--storage=keychain` | API key stored in OS keychain (encrypted) |

Keychain storage requires OS-specific tools:
- **Linux**: `libsecret-tools` (GNOME Keyring / KWallet)
- **macOS**: Keychain Access (built-in)
- **Windows**: Credential Manager (built-in)

## Key Files

- `bedrock-config.json` - Environment variables, regions, defaults (edit this to change models)
- `setup-claude-bedrock.sh` - Main Unix implementation
- `setup-claude-bedrock.ps1` - PowerShell implementation
- `test.sh` - Full test suite with ~75 tests across 15 categories
- `iam-policy.json` - Required AWS IAM permissions

## CI/CD

GitHub Actions (`.github/workflows/test.yml`):
- Ubuntu + macOS: Bash, Zsh, Fish
- Windows: PowerShell, Git Bash
