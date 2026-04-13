# Changelog

All notable changes to Juggernaut will be documented in this file.

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

[1.7.2]: https://github.com/jpvelasco/juggernaut/compare/v1.7.1...v1.7.2
[1.7.1]: https://github.com/jpvelasco/juggernaut/compare/v1.7.0...v1.7.1
[1.7.0]: https://github.com/jpvelasco/juggernaut/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v1.6.0
