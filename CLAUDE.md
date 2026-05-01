# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform CLI tool that configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API. It writes settings into `~/.claude/settings.json` (primary) with an optional shell profile fallback block (`# BEGIN/END: Claude Code Bedrock Configuration`).

The project has two active code paths:
- **v1 (legacy):** `setup` → `setup-claude-bedrock.sh` / `setup-claude-bedrock.ps1` — profile-only approach
- **v2 (active):** `juggernaut` / `juggernaut.ps1` → `commands/` → `lib/` — settings.json-first approach

## Commands

```bash
# Run all v2 bash tests (order matches CI)
bash ./tests/v2/test_schema.sh
bash ./tests/v2/test_config_manager.sh
bash ./tests/v2/test_feature_flag.sh
bash ./tests/v2/test_migrator.sh
bash ./tests/v2/test_keychain.sh
bash ./tests/v2/test_apply.sh
bash ./tests/v2/test_show.sh
bash ./tests/v2/test_doctor.sh
bash ./tests/v2/test_uninstall.sh
bash ./tests/v2/test_install.sh
bash ./tests/v2/test_profile_writer.sh

# Run all v2 PowerShell tests (Pester 5 required)
pwsh -Command "Invoke-Pester ./tests/v2 -CI"

# Run v1 tests
bash ./test.sh

# Dry-run v2 apply (preview without writing files)
./juggernaut apply --v2 --dry-run

# Lint shell scripts
shellcheck setup-claude-bedrock.sh uninstall.sh validate-setup.sh apply-config.sh install.sh setup
shellcheck juggernaut commands/apply.sh lib/keychain.sh lib/profile_writer.sh lib/schema.sh lib/config_manager.sh lib/migrator.sh
shellcheck tests/v2/test_keychain.sh tests/v2/test_apply.sh tests/v2/test_install.sh
```

## v2 Architecture

**Entry:** `juggernaut` (bash) / `juggernaut.ps1` (PowerShell) — dispatches subcommands. v2 requires `JUGGERNAUT_USE_V2=1` or `--v2` flag to activate.

**Subcommands in `commands/`:**
- `apply.{sh,ps1}` — writes Juggernaut block to settings.json; optional shell profile fallback
- `show.{sh,ps1}` — prints current config from settings.json
- `doctor.{sh,ps1}` — reads `lib/doctor.{sh,ps1}` for diagnostics
- `migrate.{sh,ps1}` — migrates v1 profile block to settings.json
- `uninstall.{sh,ps1}` — removes the Juggernaut block and optional profile entries

**Library in `lib/`:**
- `schema.{sh,ps1}` — constructs/validates the Juggernaut JSON block; requires `jq`
- `config_manager.{sh,ps1}` — atomic read/merge/write of settings.json; backup rotation; best-effort file locking
- `profile_writer.{sh,ps1}` — injects/removes shell profile fallback blocks
- `keychain.{sh,ps1}` — OS keychain read/write (macOS Keychain, Linux secret-tool, Windows Credential Manager)
- `migrator.{sh,ps1}` — detects and migrates v1 config blocks
- `doctor.{sh,ps1}` — read-only diagnostics, scope checks, credential checks

**v2 tests in `tests/v2/`:** Each `lib/` and `commands/` module has a paired bash test file (`test_*.sh`) and PowerShell Pester file (`*.Tests.ps1`). Tests use `JUGGERNAUT_USE_V2=1` and `BEDROCK_CONFIG_PATH` env vars. There is no single runner for bash v2 tests — run each file individually or see CI for the full sequence.

## Key Design Patterns

- **Single source of truth:** `bedrock-config.json` holds all defaults, valid regions, and version. Never hardcode these in scripts.
- **settings.json block:** v2 stores config under a `juggernaut` key inside `~/.claude/settings.json` or `./.claude/settings.json`. The `config_manager` module handles atomic writes with file-mode preservation and rotated backups (`CONFIG_BACKUP_RETAIN=5`).
- **Shell fallback:** Optional profile block (`--shell-fallback-only` / `--no-shell-fallback` flags on `apply`). Drift between settings.json and the profile block is reported by `doctor`.
- **Scope:** `--scope=user` (default, `~/.claude/settings.json`) vs `--scope=project` (`./.claude/settings.json`). `doctor` auto-detects the active scope by walking up from CWD.
- **Auth mode persistence:** The `--auth=iam|api-key` choice is stored in the Juggernaut block and auto-detected on re-run.
- **Keychain storage:** API keys can be stored in OS keychain instead of plaintext. Platform defaults: macOS/Windows → keychain, Linux → profile.
- **1M context flag:** `--1m-context` appends `[1m]` suffix to model IDs. Claude Code strips it before sending to Bedrock.
- **JSON loading:** `setup-claude-bedrock.sh` uses `jq` with a `python3` fallback. `schema.sh` hard-fails if `jq` is absent. All Python calls use `sys.argv` (never f-strings) to avoid shell injection.

## Cross-Platform Requirements

All v2 changes need `.sh` and `.ps1` variants. Targets: macOS (zsh/bash/fish), Linux (bash/zsh/fish), Windows PowerShell, Windows Git Bash, WSL. Fish uses `set -gx VAR value` instead of `export VAR=value`.

## Testing Notes

- v2 bash tests have their own `PASS`/`FAIL` counters and `section`/`assert_eq`/`assert_true`/`assert_nonempty` helpers inline — no shared framework file.
- v2 PowerShell tests use Pester 5 (`Describe`/`It`/`Should`).
- `test.sh` (v1) is a separate self-contained runner — `run_test`, `skip_test`, `section` helpers. No way to run a single v1 test.
- CI: `lint` job → `test-unix`, `test-windows-powershell`, `test-windows-gitbash` jobs (all need lint to pass).

## Version Management

Version must stay in sync across two places: `VERSION` file and `bedrock-config.json` `.version` field.

## Gotchas

- **v2 gate (asymmetric):** `setup` defaults to v2 (`JUGGERNAUT_USE_V2:-1`); use `--legacy-v1` to force v1. `juggernaut` binary still requires explicit `JUGGERNAUT_USE_V2=1` or `--v2` — without it, it exits 0 with a message.
- **`setup` and `juggernaut` have no `.sh` extension** — both must be included in shellcheck linting explicitly.
- **README drift:** README hardcodes model names and token values. `bedrock-config.json` is authoritative; update README when defaults change.
- **Fish syntax differs:** Profile writer must emit `set -gx VAR value` for fish, not `export`.

## Shellcheck

`.shellcheckrc` disables SC1091 (sourcing non-existent paths) and SC2016 (expressions in single quotes). Default dialect is bash.
