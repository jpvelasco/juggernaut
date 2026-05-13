# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform CLI tool that configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API. It writes settings **only** to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). v3 does not write to shell profiles — `settings.json` is the sole output.

Entry points: `juggernaut` (bash) / `juggernaut.ps1` (PowerShell) → `commands/` → `lib/`.

## Commands

```bash
# Run all bash tests (preferred)
make test

# Run a single test suite
make test-apply
make test-launcher
# make help lists all targets

# Lint all shell scripts
make lint

# Preview install without writing files
make install-dry-run

# Check active installation
make doctor

# Run all PowerShell tests (Pester 5 required)
pwsh -Command "Invoke-Pester ./tests/v2 -CI"

# Dry-run apply (preview settings.json write without committing)
./juggernaut apply --auth=iam --dry-run
```

## Architecture

**Subcommands in `commands/`:**
- `apply.{sh,ps1}` — validates auth (`aws sts get-caller-identity` or `$AWS_BEARER_TOKEN_BEDROCK` or bearer token storage) and writes the Juggernaut block to settings.json
- `show.{sh,ps1}` — prints current config from settings.json
- `doctor.{sh,ps1}` — read-only diagnostics (delegates to `lib/doctor.{sh,ps1}`); includes opusplan drift check
- `uninstall.{sh,ps1}` — removes the Juggernaut block from settings.json and deletes bearer token storage
- `version.{sh,ps1}` — prints the installed Juggernaut version (delegates to the `J_VERSION` value from `lib/schema.{sh,ps1}`)

**Library in `lib/`:**
- `schema.{sh,ps1}` — constructs/validates the Juggernaut JSON block; requires `jq`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `J_AUTH_VALIDATED=true`.
- `config_manager.{sh,ps1}` — atomic read/merge/write of settings.json; backup rotation (`CONFIG_BACKUP_RETAIN=5`); best-effort file locking
- `keychain.{sh,ps1}` — OS keychain read/write (macOS Keychain, Linux secret-tool, Windows Credential Manager for short keys ≤1280 chars, Windows per-user DPAPI file for long keys). Service name: `juggernaut-bedrock`.
- `doctor.{sh,ps1}` — scope checks, credential checks, opusplan drift check
- `profile_paths.{sh,ps1}` — shell-profile paths scanned by the installer wipe phase
- `arg_parsing.ps1` — shared argument-parsing helper for PowerShell subcommands (bash uses inline getopts)

**Installer:** `install.sh` / `install.ps1` run a destructive wipe phase on every invocation — strip `# BEGIN: Juggernaut` and `# BEGIN: Claude Code Bedrock Configuration` blocks from all profile paths, remove the `juggernaut` key from settings.json, delete bearer token storage — then install fresh files. Does **not** auto-apply. Supports `--dry-run`/`-DryRun`.

**Launcher:** a bracketed shell function written into the user's shell profile(s) by `install.sh`'s `install_launcher_profile_block`:
- bash/zsh/sh: `claude()` function using `command claude "$@"`, sourcing `lib/keychain.sh` to inject `AWS_BEARER_TOKEN_BEDROCK`
- fish: `function claude` block using `command claude $argv`, shelling out to bash for keychain access (fish cannot source bash functions)
- PowerShell: `function claude` block in `$PROFILE.CurrentUserCurrentHost` reading from DPAPI file then Credential Manager

All variants use `# BEGIN: Juggernaut Launcher` / `# END: Juggernaut Launcher` markers for idempotent install/uninstall. The launcher is needed because Claude Code reads `AWS_BEARER_TOKEN_BEDROCK` from process env only, not from the keychain — without it, fresh shells with `CLAUDE_CODE_USE_BEDROCK=1` in settings.json would hang.

**Tests in `tests/v2/`:** Each `lib/` and `commands/` module has a paired bash test file (`test_*.sh`) and PowerShell Pester file (`*.Tests.ps1`). Run each file individually or via `make test` / `Invoke-Pester`.

## Key Design Patterns

- **Single source of truth:** `bedrock-config.json` holds all defaults, valid regions, and version. Never hardcode these in scripts.
- **Single output target:** settings.json only. No shell-profile fallback in v3.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when apply validates a credential source. Prevents installer-silently-enabled-Bedrock hangs on launch.
- **Scope:** `--scope=user` (default, `~/.claude/settings.json`) vs `--scope=project` (`./.claude/settings.json`). `doctor` auto-detects by walking up from CWD.
- **Keychain storage:** API keys stored in OS keychain for short keys (≤1280 chars) or DPAPI-encrypted file at `~/.juggernaut/bearer-token.dpapi.bin` for long keys. Platform defaults: macOS/Windows → keychain, Linux → profile file, Windows long keys → DPAPI.
- **Mantle default:** `J_USE_MANTLE=true` by default. Opt out with `--no-mantle`.
- **ANTHROPIC_MODEL is opusplan-gated:** `schema.sh/ps1` only writes `ANTHROPIC_MODEL` when `--opusplan` is active. Re-applying with `--no-opusplan` removes it from settings.json.
- **JSON loading:** `schema.sh` hard-fails if `jq` is absent. `config_manager.sh` uses jq for all merges.

## Cross-Platform Requirements

All changes need `.sh` and `.ps1` variants. Targets: macOS (zsh/bash/fish), Linux (bash/zsh/fish), Windows PowerShell 5.1, Windows PowerShell 7, Windows Git Bash, WSL. On Windows the installer scans both `Documents\WindowsPowerShell\profile.ps1` (5.1) and `Documents\PowerShell\profile.ps1` (7), plus `$PROFILE.AllUsersAllHosts` and `$PROFILE.CurrentUserAllHosts` — OneDrive-redirected `Documents\` is followed automatically.

## Testing Notes

- Bash tests have their own `PASS`/`FAIL` counters and `section` helper inline — no shared framework file.
- PowerShell tests use Pester 5 (`Describe`/`It`/`Should`).
- CI: `lint` job → `test-unix` (ubuntu + macos), `test-windows-powershell`, `test-windows-gitbash` — all parallel, all need lint to pass first.
- To isolate keychain in tests: set `JUGGERNAUT_KEYCHAIN_SERVICE` to a guaranteed-absent service name. Tests that need a clean home use `mktemp -d` and `HOME=` override.

## Version Management

Version must stay in sync across **four** locations: `VERSION`, `bedrock-config.json` (`.version`), `${J_VERSION:-...}` fallback in `lib/schema.sh`, and `[string]$Version = '...'` default in `lib/schema.ps1`. CI enforces this in the `Verify version sync` step — a mismatch fails the lint job.

## Gotchas

- **`juggernaut` has no `.sh` extension** — must be listed explicitly in shellcheck commands.
- **Installer is destructive:** Every run of `install.sh`/`install.ps1` wipes profile blocks, settings.json's `juggernaut` key, and bearer token storage before reinstalling. Use `--dry-run`/`-DryRun` to preview.
- **README drift:** README hardcodes model names and token values. `bedrock-config.json` is authoritative; update README when defaults change.
- **Fish launcher uses bash subprocess:** The fish `function claude` block calls `bash -c '. lib/keychain.sh; bearer_token_get'` — it does not use fish builtins for keychain access.

## Shellcheck

`.shellcheckrc` disables SC1091 (sourcing non-existent paths) and SC2016 (expressions in single quotes). Default dialect is bash.
