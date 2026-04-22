# Changelog

All notable changes to Juggernaut will be documented in this file.

## [2.0.0] - 2026-04-21

### Added

- **`juggernaut` v2 CLI** — new settings.json-first approach replaces shell profile-only v1. Activated with `--v2` flag or `JUGGERNAUT_USE_V2=1`.
- **`juggernaut apply`** — configures Claude Code via `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope), with optional shell profile fallback. Supports `--dry-run`, `--force`, `--scope=user|project`, `--auth=iam|api-key`, `--1m-context`, `--opusplan`, `--effort`, `--mantle`, and more.
- **`juggernaut show`** — prints current Juggernaut configuration from settings.json for both user and project scopes.
- **`juggernaut doctor`** — read-only diagnostics: checks both scopes, credentials, region/models, Mantle status, and drift between settings.json and the shell fallback.
- **`juggernaut migrate`** — migrates a v1 shell profile block to settings.json. Supports `--dry-run`, `--clean`, `--rollback`.
- **`juggernaut uninstall`** — safely removes the Juggernaut block from settings.json (all scopes with a block by default), shell profiles, and OS keychain. Supports `--dry-run`, `--force`, `--scope=user|project`.
- **`lib/config_manager`** — atomic read/merge/write of settings.json with backup rotation (5 most recent), best-effort file locking, and file-mode preservation.
- **`lib/schema`** — constructs and validates the Juggernaut JSON block; single source of truth via `bedrock-config.json`.
- **`lib/profile_writer`** — writes/removes the `# BEGIN/END: Claude Code Bedrock Configuration` block in shell profiles. Supports bash, zsh, and fish.
- **`lib/keychain`** — OS keychain abstraction for macOS (Keychain), Linux (secret-tool), and Windows (Credential Manager / cmdkey).
- **`lib/migrator`** — detects and migrates v1 config blocks; preserves profile block as compatibility fallback with annotated notice.
- **`lib/doctor`** — read-only diagnostics library used by the doctor command.
- **PowerShell parity** — all v2 commands have `.ps1` equivalents in `commands/` and `lib/`, tested with Pester 5.
- **Full test coverage** — bash integration tests (`tests/v2/test_*.sh`) and Pester 5 tests (`tests/v2/*.Tests.ps1`) for all commands and library modules.

### Changed

- **Settings.json-first** — v2 stores all configuration under a `.juggernaut` key in `settings.json`. The shell profile block is now an optional fallback.
- **Scope support** — `--scope=user` (default, `~/.claude/settings.json`) and `--scope=project` (`./.claude/settings.json`).
- **Auth mode persistence** — the `--auth=iam|api-key` choice is stored in the Juggernaut block and auto-detected on re-run.
- **Version bumped to 2.0.0** in both `VERSION` and `bedrock-config.json`.

### Notes

- **v1 is unchanged** — `setup`, `setup-claude-bedrock.sh`, and `setup-claude-bedrock.ps1` are untouched. All v2 paths require `JUGGERNAUT_USE_V2=1` or `--v2`.
- **Backwards compatible** — existing v1 profile blocks continue to work. Use `juggernaut migrate` to upgrade.

## [1.7.5] - 2026-04-17

### Fixed

- **PowerShell profile encoding on Windows** — fixes a `ParseException` that prevented the PowerShell profile from loading after running the installer. Non-ASCII en/em dashes in model display names were written to the profile without an explicit encoding, and Windows PowerShell 5.1 defaults to the system ANSI code page (Windows-1252), where UTF-8 byte `0x93` (part of the en dash sequence) decodes as a curly quote — closing the `$env:` string assignment early.
- Replaced en/em dashes with ASCII hyphens in `ANTHROPIC_DEFAULT_OPUS_MODEL_NAME` and `ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION`
- Added `-Encoding utf8` to every `Set-Content`, `Add-Content`, and `Get-Content` profile I/O in `setup-claude-bedrock.ps1` and `uninstall.ps1` so reads and writes stay symmetric on both PS 5.1 and PS 7+

## [1.7.4] - 2026-04-16

### Added

- **`--opusplan` / `-OpusPlan`** — sets `ANTHROPIC_MODEL=opusplan` to use Opus during plan mode and Sonnet during execution; persisted via `# OpusPlan: true` metadata comment
- **`--effort=LEVEL` / `-Effort LEVEL`** — sets `CLAUDE_CODE_EFFORT_LEVEL` (values: `low`, `medium`, `high`, `xhigh`, `max`); persisted via `# EffortLevel:` metadata comment
- **`CLAUDE_CODE_SUBAGENT_MODEL`** — explicitly routes background/subagent work to Haiku 4.5 for cost efficiency

### Changed

- **Opus 4.7 defaults to `[1m]`** — `ANTHROPIC_DEFAULT_OPUS_MODEL` now uses `global.anthropic.claude-opus-4-7[1m]` by default, eliminating duplicate picker entries (regular + 1M)
- **Default effort is `xhigh`** — `CLAUDE_CODE_EFFORT_LEVEL=xhigh` is now the default per Anthropic's recommendation for Opus 4.7
- **Opus 4.7 picker label** — updated to "Opus 4.7 (New flagship – 1M context)" to reflect the 1M default

### Fixed

- Replaced deprecated `ENABLE_PROMPT_CACHING_1H_BEDROCK` with `ENABLE_PROMPT_CACHING_1H`
- Removed invalid `vision_highres` from model supported capabilities (not a valid Claude Code capability value)

## [1.7.3] - 2026-04-16

### Added

- **Claude Opus 4.7 support** — new flagship model now available as `global.anthropic.claude-opus-4-7` on Amazon Bedrock
- Updated `/model` picker: "Opus 4.7 (New flagship – most capable)" with description highlighting improved vision, instruction following, and self-verification
- Updated `ANTHROPIC_DEFAULT_OPUS_MODEL` default to `global.anthropic.claude-opus-4-7`

## [1.7.2] - 2026-04-12

Reliability, security, and developer experience improvements across the entire setup pipeline.

### Added

- **Pre-flight dependency checks** — setup validates `jq` (or `python3`) and `aws` CLI before proceeding, with platform-specific install instructions (brew/apt/winget) when dependencies are missing
- **`--skip-preflight` flag** (`-SkipPreflight` on PowerShell) and `JUGGERNAUT_SKIP_PREFLIGHT=1` environment variable to bypass AWS CLI checks for CI or advanced users
- **Shellcheck CI job** — all bash scripts are linted on every PR with zero warnings
- **Version sync enforcement** — CI verifies `VERSION` file matches `bedrock-config.json` version
- **`.gitattributes`** for cross-platform line ending consistency (LF for shell scripts, CRLF for PowerShell)
- 40+ new tests covering pre-flight checks, credential conflicts, version sync, shellcheck compliance, and uninstall behavior (173 total)

### Fixed

- **API key quoting (security)** — bash/zsh config blocks now use single quotes to prevent `$`, backtick, and `$()` expansion when profiles are sourced
- **PowerShell API key quoting (security)** — single quotes prevent backtick expansion in generated PowerShell config blocks
- **PowerShell BSTR memory leak** — plaintext API keys are now freed from unmanaged memory after SecureString conversion
- **Fish single-quote escaping** — fixed bash 5.2 `patsub_replacement` bug that silently broke fish config generation
- **AWS_PROFILE conflict** — PowerShell api-key mode now unsets `AWS_PROFILE`, matching bash behavior
- **PowerShell validator** — corrected `--inference-config` flag (was using deprecated `--max-tokens`)
- **Variable leak in `apply-config.sh`** — refactored to function scope to prevent `_juggernaut_rc` from leaking into the user's shell environment
- File existence guard added to `detect_existing_1m_context`
- Removed emojis from script output for consistent cross-platform display

### Changed

- CI now runs on PRs to any branch, not just `main`
- Shellcheck warnings resolved with proper code fixes (not suppressions): SC2155, SC2162, SC2034, SC2002
- `apply-config.sh` refactored from return-or-exit pattern to clean function wrapper

## [1.7.1] - 2026-04-09

- Default model changed from Opus 4.6 to Sonnet 4.6 (Recommended) to match Claude Code's default UX
- Updated model display names in `/model` picker: "Opus 4.6 (Most capable)", "Sonnet 4.6 (Recommended)", "Haiku 4.5 (Fast)"
- 1M context labels now show cleanly: e.g., "Sonnet 4.6 (Recommended, 1M Context)"

## [1.7.0] - 2026-04-07

- `--1m-context` / `-OneM` enables 1M token context for Opus and Sonnet models
- Model capabilities declared via `SUPPORTED_CAPABILITIES` (effort levels, adaptive/extended/interleaved thinking)

## [1.6.0] - 2026-04-06

- Full model picker: Opus, Sonnet, and Haiku in `/model` selector with friendly names
- Per-model overrides: `--opus-model`, `--sonnet-model`, `--haiku-model`
- `--model-prefix=us|eu|ap` for region-specific inference profiles
- One-line install: `curl | bash` (Unix) and `irm | iex` (PowerShell)

[1.7.3]: https://github.com/jpvelasco/juggernaut/compare/v1.7.2...v1.7.3
[1.7.2]: https://github.com/jpvelasco/juggernaut/compare/v1.7.1...v1.7.2
[1.7.1]: https://github.com/jpvelasco/juggernaut/compare/v1.7.0...v1.7.1
[1.7.0]: https://github.com/jpvelasco/juggernaut/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v1.6.0
