# Changelog

All notable changes to Juggernaut will be documented in this file.

## [2.1.2] - 2026-04-24

### Fixed

- **Installers are install-only by default** — `install.sh` and `install.ps1` no longer auto-run setup or mutate Claude/AWS profile configuration after creating the launcher.
- **Explicit configure path** — installers now print `juggernaut apply --v2 --auth=bedrock-api-key` and `juggernaut apply --v2 --auth=iam` as separate next steps; advanced users can opt into immediate configuration with `--configure` / `-Configure`.
- **Bearer-token preservation** — IAM profile fallback no longer clears `AWS_BEARER_TOKEN_BEDROCK`, so API-key credentials are not wiped from new shells.
- **Windows installer hotfix** — legacy PowerShell setup accepts its empty default auth value, accepts `bedrock-api-key`, and maps it into the existing Bedrock API-key setup flow.
- **Pinned install commands** — README install examples pass the release version explicitly so tagged raw installer URLs stay pinned instead of updating an existing install to `main`.
- **Version bumped to 2.1.2** in `VERSION` and `bedrock-config.json`.

## [2.1.0] - 2026-04-23

### Added

- **Installer launchers** — Unix installers repair executable bits and create/update `~/.local/bin/juggernaut`; Windows installers create user-local PowerShell and `.cmd` shims.
- **Bedrock API-key auth mode** — v2 now persists `auth.mode = "bedrock-api-key"` and treats legacy `"api-key"` as a read-only compatibility alias that is rewritten on the next apply or migrate.
- **Bearer-token detection** — `AWS_BEARER_TOKEN_BEDROCK` is detected as a first-class auth source, with Mantle enabled by default when no explicit Mantle preference is supplied.
- **Installer acceptance tests** — Bash and Pester coverage now checks installer permissions, launcher creation, and post-install messaging.

### Changed

- **Migration confirmation** — `apply --v2` no longer silently migrates v1 shell profile blocks. Interactive runs prompt, non-interactive runs require `--yes`, and `--dry-run` writes nothing.
- **Model defaults** — Opus now defaults to `global.anthropic.claude-opus-4-7` without a `[1m]` suffix because Opus 4.7 has native 1M context on Bedrock.
- **Doctor/show output** — credentials, Mantle, auth labels, and source details are calmer and consistent across Bash and PowerShell.
- **Version bumped to 2.1.0** in `VERSION` and `bedrock-config.json`.

### Fixed

- **Codacy static-analysis finding** — removed the unused `active_path` assignment in `commands/show.sh`.
- **IAM false warnings** — bearer-token users no longer receive IAM credential warnings under Bedrock API-key auth; mixed credentials are reported as informational notes.

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

- **Settings.json-first** — v2 stores all configuration under a `juggernaut` key in `settings.json`. The shell profile block is now an optional fallback.
- **Scope support** — `--scope=user` (default, `~/.claude/settings.json`) and `--scope=project` (`./.claude/settings.json`).
- **Auth mode persistence** — the `--auth=iam|api-key` choice is stored in the Juggernaut block and auto-detected on re-run.
- **Version bumped to 2.0.0** in both `VERSION` and `bedrock-config.json`.

### Notes

- **Backwards compatible** — existing v1 profile blocks continue to work. Use `juggernaut migrate` to upgrade.

[2.1.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.2
[2.1.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.0
[2.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.0.0
