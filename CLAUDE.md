# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform CLI tool that configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API. It writes settings **only** to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). v3 does not write to shell profiles — `settings.json` is the sole output.

Entry points: `juggernaut` (bash) / `juggernaut.ps1` (PowerShell) → `commands/` → `lib/`.

## Commands

```bash
# Run all bash tests (order matches CI)
bash ./tests/v2/test_schema.sh
bash ./tests/v2/test_config_manager.sh
bash ./tests/v2/test_keychain.sh
bash ./tests/v2/test_apply.sh
bash ./tests/v2/test_show.sh
bash ./tests/v2/test_doctor.sh
bash ./tests/v2/test_uninstall.sh
bash ./tests/v2/test_install.sh

# Run all PowerShell tests (Pester 5 required)
pwsh -Command "Invoke-Pester ./tests/v2 -CI"

# Dry-run apply (preview without writing files)
./juggernaut apply --auth=iam --dry-run

# Lint shell scripts
shellcheck juggernaut install.sh commands/*.sh lib/keychain.sh lib/schema.sh lib/config_manager.sh lib/profile_paths.sh lib/doctor.sh tests/v2/test_*.sh
```

## Architecture

**Subcommands in `commands/`:**
- `apply.{sh,ps1}` — validates auth (`aws sts get-caller-identity` or `$AWS_BEARER_TOKEN_BEDROCK` or keychain entry) and writes the Juggernaut block to settings.json
- `show.{sh,ps1}` — prints current config from settings.json
- `doctor.{sh,ps1}` — read-only diagnostics (delegates to `lib/doctor.{sh,ps1}`); includes opusplan drift check
- `uninstall.{sh,ps1}` — removes the Juggernaut block from settings.json and deletes the OS keychain entry

**Library in `lib/`:**
- `schema.{sh,ps1}` — constructs/validates the Juggernaut JSON block; requires `jq`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `J_AUTH_VALIDATED=true`.
- `config_manager.{sh,ps1}` — atomic read/merge/write of settings.json; backup rotation (`CONFIG_BACKUP_RETAIN=5`); best-effort file locking
- `keychain.{sh,ps1}` — OS keychain read/write (macOS Keychain, Linux secret-tool, Windows Credential Manager). Service name: `juggernaut-bedrock`.
- `doctor.{sh,ps1}` — scope checks, credential checks, opusplan drift check
- `profile_paths.{sh,ps1}` — list of shell-profile paths scanned by the installer's wipe phase (v3 code itself does not write to these files)

**Installer:** `install.sh` / `install.ps1` run a destructive wipe phase on every invocation — strip `# BEGIN: Juggernaut` and `# BEGIN: Claude Code Bedrock Configuration` blocks from all profile paths, remove the `juggernaut` key from settings.json, delete the `juggernaut-bedrock` keychain entry — then install fresh files. Does **not** auto-apply. Supports `--dry-run`/`-DryRun` to preview without writing.

**Tests in `tests/v2/`:** Each `lib/` and `commands/` module has a paired bash test file (`test_*.sh`) and PowerShell Pester file (`*.Tests.ps1`). Run each file individually or see CI for the full sequence.

## Key Design Patterns

- **Single source of truth:** `bedrock-config.json` holds all defaults, valid regions, and version. Never hardcode these in scripts.
- **Single output target:** settings.json only. No shell-profile fallback. No triple-write drift.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in `settings.json` when apply validates a credential source. Prevents installer-silently-enabled-Bedrock hangs.
- **Scope:** `--scope=user` (default, `~/.claude/settings.json`) vs `--scope=project` (`./.claude/settings.json`). `doctor` auto-detects the active scope by walking up from CWD.
- **Auth mode persistence:** The `--auth=iam|bedrock-api-key` choice is stored in the Juggernaut block and auto-detected on re-run.
- **Keychain storage:** API keys can be stored in OS keychain instead of plaintext. Platform defaults: macOS/Windows → keychain, Linux → profile-env-var-only (no plaintext-to-disk fallback).
- **Mantle default:** `J_USE_MANTLE=true` by default in v3. Opt out with `--no-mantle`.
- **1M context flag:** `--1m-context` appends `[1m]` suffix to model IDs. Claude Code strips it before sending to Bedrock.
- **JSON loading:** `schema.sh` hard-fails if `jq` is absent. `config_manager.sh` uses jq for all merges.

## Cross-Platform Requirements

All changes need `.sh` and `.ps1` variants. Targets: macOS (zsh/bash/fish), Linux (bash/zsh/fish), Windows PowerShell 5.1, Windows PowerShell 7, Windows Git Bash, WSL. On Windows the installer scans both `Documents\WindowsPowerShell\profile.ps1` (5.1) and `Documents\PowerShell\profile.ps1` (7), plus `$PROFILE.AllUsersAllHosts` and `$PROFILE.CurrentUserAllHosts` — OneDrive-redirected `Documents\` is followed automatically.

## Testing Notes

- Bash tests have their own `PASS`/`FAIL` counters and `section`/`assert_eq`/`assert_true`/`assert_nonempty` helpers inline — no shared framework file.
- PowerShell tests use Pester 5 (`Describe`/`It`/`Should`).
- CI: `lint` job → `test-unix`, `test-windows-powershell`, `test-windows-gitbash` jobs (all need lint to pass).

## Version Management

Version must stay in sync across `VERSION`, `bedrock-config.json` (`.version`), and the `${J_VERSION:-3.0.0}` fallback in `lib/schema.sh` / `lib/schema.ps1`.

## Gotchas

- **`juggernaut` has no `.sh` extension** — must be included in shellcheck linting explicitly.
- **README drift:** README hardcodes model names and token values. `bedrock-config.json` is authoritative; update README when defaults change.
- **Installer is destructive:** Every run of `install.sh`/`install.ps1` wipes profile blocks, settings.json's `juggernaut` key, and the keychain entry before installing. Use `--dry-run`/`-DryRun` to preview.

## Shellcheck

`.shellcheckrc` disables SC1091 (sourcing non-existent paths) and SC2016 (expressions in single quotes). Default dialect is bash.
