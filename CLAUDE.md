# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform CLI tool that configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API. It sets environment variables in shell profiles (bash/zsh/fish/PowerShell) using marker-based config blocks (`# BEGIN/END: Claude Code Bedrock Configuration`).

## Commands

```bash
# Run tests (requires Bash 4.0+)
bash ./test.sh

# Dry-run setup (preview changes without modifying files)
./setup --dry-run
./setup-claude-bedrock.sh bash --dry-run

# Lint shell scripts (all bash scripts including the unified entry point)
shellcheck setup-claude-bedrock.sh uninstall.sh validate-setup.sh apply-config.sh install.sh setup

# Validate an existing installation
./validate-setup.sh
```

## Architecture

**Entry flow:** `setup` (unified entry point) detects OS/shell, then delegates to `setup-claude-bedrock.sh` (Unix) or user runs `setup-claude-bedrock.ps1` (Windows PowerShell) directly.

**Single source of truth:** `bedrock-config.json` contains all environment variable defaults, valid regions, and default settings. Both bash and PowerShell scripts read from this file. Never hardcode values in scripts.

**Script pairs:** Each feature has a Unix (.sh) and Windows (.ps1) variant:
- `setup-claude-bedrock.{sh,ps1}` - Main setup
- `validate-setup.{sh,ps1}` - Configuration validator
- `apply-config.{sh,ps1}` - Apply config to current session
- `uninstall.{sh,ps1}` - Remove configuration

**Config block pattern:** Scripts inject a marker-delimited block into shell profiles. The block includes metadata comments (`# Auth mode:`, `# Model:`, `# Storage:`) that are parsed on re-runs to preserve user choices across updates.

**JSON loading:** `setup-claude-bedrock.sh` uses `jq` with a `python3` fallback for parsing `bedrock-config.json`. All Python calls use `sys.argv` (not f-strings or format) to avoid shell injection.

## Key Design Patterns

- **Auth mode persistence:** The `--auth=iam|api-key` choice is stored as a comment in the config block and auto-detected on re-run so users don't need to re-specify it.
- **Custom model persistence:** Same pattern for `--model` and `--fast-model` overrides.
- **Keychain storage:** API keys can be stored in OS keychain (macOS Keychain, Linux secret-tool, Windows Credential Manager) instead of plaintext in profiles. The config block contains a shell command to retrieve the key at startup.
- **Platform-aware defaults:** macOS/Windows default to keychain storage; Linux defaults to profile storage.
- **Credential conflict prevention:** Config blocks actively `unset` conflicting auth variables (e.g., API key mode unsets IAM vars and vice versa).
- **1M context flag:** `--1m-context` appends a `[1m]` suffix to Opus and Sonnet model IDs in the config block. Claude Code strips this suffix before sending to Bedrock — it's used purely as a signal to select the 1M-context variant of the inference profile.

## Cross-Platform Requirements

All changes must work on: macOS (zsh/bash/fish), Linux (bash/zsh/fish), Windows PowerShell, Windows Git Bash, and WSL. Fish shell uses `set -gx` syntax instead of `export`. The test suite validates shell-specific syntax generation.

## Testing Notes

- `test.sh` is a self-contained bash test framework with `run_test`, `skip_test`, and `section` helpers
- Tests heavily use `--dry-run` mode to verify output without modifying the system
- There is no way to run individual tests — `test.sh` always runs the full suite
- Security tests verify API key special characters don't cause command injection
- CI has 3 separate jobs: `test-unix` (ubuntu + macOS matrix), `test-windows-powershell`, and `test-windows-gitbash`
- CI also runs dry-run tests for each shell (bash/zsh/fish) and keychain storage beyond the test suite

## Version Management

Version is tracked in two places that must stay in sync: `VERSION` file and `bedrock-config.json` `.version` field.

## Gotchas

- **README drift:** The README contains hardcoded model names and token values that can fall out of sync with `bedrock-config.json`. Always treat `bedrock-config.json` as authoritative and update README when changing defaults.
- **`setup` is a bash script without `.sh` extension:** It's the unified entry point and must be included in shellcheck linting.
- **Fish syntax differs:** Fish uses `set -gx VAR value` instead of `export VAR=value`. Any config block generation must handle this.

## Shellcheck

`.shellcheckrc` disables SC1091 (sourcing non-existent paths) and SC2016 (expressions in single quotes). Default dialect is bash.
