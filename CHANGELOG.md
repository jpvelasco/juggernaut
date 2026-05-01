# Changelog

All notable changes to Juggernaut will be documented in this file.

## [2.3.0] - 2026-04-30

### Migration Notes

**v2 is now the default.** Running `juggernaut <subcommand>` activates v2 without any extra flag. Pass `--legacy-v1` (or set `JUGGERNAUT_USE_V2=0`) to keep the v1 shell-profile-only path. The `--v2` flag is accepted as a no-op alias for backwards compatibility.

The installer now shows an upgrade banner when it detects a v1 profile block or version change. The banner explains the v2 settings.json migration target, asks before migrating, and requires `--yes` or `--legacy-v1` for non-interactive installs.

### Changed (breaking default)

- **v2 is ON by default.** `juggernaut` and `juggernaut.ps1` now default `JUGGERNAUT_USE_V2` to `1` instead of `0`. CI scripts or wrappers that pinned `JUGGERNAUT_USE_V2=0` can continue to do so — explicit `=0` is a supported first-class opt-out.
- **Subcommand gate exits 2 (not 0).** `commands/show`, `doctor`, `migrate`, and `uninstall` now exit 2 with an error message when invoked standalone with `JUGGERNAUT_USE_V2=0`. Previously they silently exited 0 with a "not active" note.

### Added

- **Upgrade banner** (`lib/upgrade_banner.sh`, `lib/upgrade_banner.ps1`) — detects v1 profile blocks and version upgrades on installer entry. Shows version diff, prompts for migration consent. Non-TTY installs require `--yes` or `--legacy-v1`; aborts with exit 3 otherwise.
- **Unified profile-path source of truth** (`lib/profile_paths.sh`, `lib/profile_paths.ps1`) — single canonical list of v1 candidate profiles consumed by `apply`, `migrate`, `uninstall`, `doctor`, and `upgrade_banner`.
- **Shared PowerShell arg-parser** (`lib/arg_parsing.ps1`) — `Convert-GnuStyleArgs` extracted from `juggernaut.ps1` so `install.ps1` can dot-source it without duplication.
- **Doctor v1 artifact awareness** — `doctor` now reports a WARN when a v1 profile block coexists with v2 settings (hints `juggernaut migrate --clean`), and an INFO when only a v1 block is present (hints `juggernaut apply`). Previously a v1-only machine reported FAIL as if unconfigured.
- **Backup rotation** — installers keep the 5 most-recent `.backup.*` directories and delete older ones. Pass `--keep-all-backups` to opt out.
- **Windows shim install-dir resolver** — `juggernaut.ps1` and `juggernaut.cmd` shims now read `juggernaut-install-dir.txt` at runtime so moving the install directory works after updating the `.txt`.
- **PowerShell v1 parser** (`ConvertFrom-MigratorV1Block`) — extended to parse `$env:KEY = 'VALUE'` lines from PowerShell-style v1 profile blocks. Ships behind `JUGGERNAUT_PS_V1_SCAN=1` opt-in; default-on planned for 2.4.0.
- **v1 deprecation notice** — running any subcommand via `--legacy-v1` now prints a one-line deprecation notice to stderr. Suppress with `JUGGERNAUT_SUPPRESS_DEPRECATION=1`.
- **`--yes` / `--legacy-v1` / `--keep-all-backups` installer flags** (`install.sh`, `install.ps1`).

### Deprecated

- **v1 shell-profile-only mode** — reachable via `--legacy-v1` or `JUGGERNAUT_USE_V2=0`. Removal planned for v3.0.

### Fixed

- `apply.ps1` no longer sets `$env:JUGGERNAUT_USE_V2 = '1'` unconditionally on load (was asymmetric with `apply.sh`).
- `Convert-InstallerApplyArgs` duplication in `install.ps1` removed; replaced with dot-sourced `lib/arg_parsing.ps1`.
- Profile scan candidates are now identical across `apply`, `migrate`, `uninstall`, and `doctor` (no more detection drift).

## [2.2.5] - 2026-04-26

### Fixed

- **Auth-conflict guard** — `apply` now detects when the stored Juggernaut block says `auth.mode = "iam"` but `AWS_BEARER_TOKEN_BEDROCK` is live in the environment (or a key exists in the system keychain). It warns and auto-corrects to `bedrock-api-key` with `--preserve-key`. Passing `--auth=iam` explicitly suppresses the guard. Fixes silent misconfiguration caused by earlier botched releases.
- **Migrator auth inference** — `migrator_parse_v1_block` / `ConvertFrom-MigratorV1Block` now infer `bedrock-api-key` from v1 blocks that export `AWS_BEARER_TOKEN_BEDROCK` even when the `# Auth mode:` metadata comment is absent or says `iam`.
- **Preserve-key probe widened** — `--preserve-key` / `-PreserveKey` now probes env → keychain → shell profile unconditionally, regardless of the stored `auth.storage` preference. Previously a corrupted `storage=keychain` + empty keychain would immediately fail.
- **Doctor auth-mode contradiction** — `doctor` now emits a WARN (not a silent note) when `auth.mode = "iam"` is stored but `AWS_BEARER_TOKEN_BEDROCK` is present, with a remediation hint to run `juggernaut apply --v2`.
- **Version bumped to 2.2.5** in `VERSION`, `bedrock-config.json`, and v2 schema defaults.

## [2.2.4] - 2026-04-25

### Fixed

- **Version drift** — `commands/apply.sh` and `commands/apply.ps1` now read the repo `VERSION` file at runtime instead of using baked-in literals. Safety fallbacks in `lib/schema.sh` and `lib/schema.ps1` bumped to `2.2.4`.
- **Fish API-key escaping** — profile writer now uses POSIX `'abc'\''def'` single-quote escaping for bedrock API keys across bash/zsh/fish. Keys containing `'`, `$`, backslashes, or backticks are preserved verbatim.
- **CRLF robustness on Windows Git Bash** — `profile_writer_has_block`, `profile_writer_remove_block`, and `migrator_has_v1_block` now normalize CRLF line endings on read so profile detection and migration work correctly against profiles with Windows line endings.
- **Persistent migration decline** — declining the v1→v2 migration prompt now writes a `# MigrationDeclined: <timestamp>` marker into the v1 block so future `apply` runs do not re-prompt. Pass `--force-migration-prompt` (bash) or `-ForceMigrationPrompt` (PowerShell) to bypass the marker for one run.
- **Destructive installer upgrade** — `install.sh` and `install.ps1` now clone into `${INSTALL_DIR}.new` and atomically swap into place after backing up the existing install. If the clone fails, the original install is preserved untouched.
- **Lock-timeout constants** — `lib/config_manager.sh` now defines `CONFIG_LOCK_TIMEOUT_SECS`/`CONFIG_STALE_LOCK_SECS`; `lib/config_manager.ps1` defines `$Script:ConfigLockTimeoutMs`. Prior hard-coded values consolidated.
- **Keychain error signalling** — `keychain_get` (bash) now returns `0` for found, `1` for not-found, and `2` for tool errors. `Get-KeychainEntry` (PowerShell) returns `$null` for not-found and throws on tool errors so callers can distinguish absence from failure.
- **CLI help polish** — `apply.sh --help` now documents the `[bash|zsh|fish]` positional argument.
- **Version bumped to 2.2.4** in `VERSION`, `bedrock-config.json`, docs, and v2 defaults.

## [2.2.3] - 2026-04-25

### Fixed

- **Dirty installer upgrades** — Unix and Windows installers now detect local changes in an existing install directory, back it up as `.backup.YYYYMMDD_HHMMSS`, and clone a fresh release instead of failing during `git checkout`.
- **Installer testability** — installers accept `JUGGERNAUT_REPO_URL` so tests can exercise upgrade behavior against a local tagged repository.
- **Version bumped to 2.2.3** in `VERSION`, `bedrock-config.json`, docs, and v2 defaults.

## [2.2.2] - 2026-04-25

### Fixed

- **Installed launcher dispatch** — Unix launchers now resolve symlinks before dispatching, so `~/.local/bin/juggernaut` finds the installed `commands/` directory.
- **PowerShell launcher dispatch** — Windows PowerShell dispatch now resolves the launcher root defensively before loading subcommands.
- **Profile-backed API-key upgrades** — `apply --v2 --auth=bedrock-api-key --preserve-key --storage=profile` can reuse an existing Juggernaut-managed profile key.
- **Shell profile targeting** — Bash apply now prefers the user's login shell from `$SHELL`, avoiding accidental `.bashrc` writes for zsh/fish users.
- **Explicit keychain safety** — Bash apply now fails closed when explicit keychain storage fails instead of silently falling back to plaintext profile storage.
- **PowerShell 1M context default** — PowerShell fresh apply now matches Bash by defaulting 1M context on.
- **Version bumped to 2.2.2** in `VERSION`, `bedrock-config.json`, docs, and v2 defaults.

## [2.2.1] - 2026-04-24

### Fixed

- **Windows doctor scope reporting** — `doctor --v2 --scope=user` no longer treats `~/.claude/settings.json` as both user and project config when run from the Windows home directory.
- **Windows shell fallback diagnostics** — doctor now reads PowerShell profile fallback blocks and uses recorded profile paths before falling back to Bash-style profile guesses.
- **Windows show fallback display** — show now displays the PowerShell profiles written by v2 instead of reporting `~/.bashrc` when PowerShell profile paths are recorded.
- **Version bumped to 2.2.1** in `VERSION` and `bedrock-config.json`.

## [2.2.0] - 2026-04-24

### Upgrade Notes

- `./setup` now defaults to the v2 settings.json-first flow. This is the recommended path for new installs and upgrades.
- The older shell-profile-only v1 flow remains available for compatibility with `./setup --legacy-v1` or `./setup --v1`.
- Windows users should run `juggernaut apply --v2 --auth=bedrock-api-key` or `juggernaut apply --v2 --auth=iam` after install; the installer itself remains install-only unless `-Configure` is passed.

### Changed

- **v2 is the default setup path** — fresh `./setup` runs now route to v2 apply; v1 remains available with `--legacy-v1` / `--v1` for compatibility.
- **Windows profile fallback** — v2 PowerShell apply writes PowerShell profile blocks instead of Bash `.bashrc` blocks on Windows, covering both Windows PowerShell 5.1 and PowerShell 7 profile locations.

### Fixed

- **Windows keychain storage** — PowerShell v2 now writes Bedrock API keys directly to Windows Credential Manager instead of shelling through `cmdkey`, avoiding failures with generated key characters.
- **No silent plaintext fallback for explicit keychain use** — if users explicitly request keychain storage and it fails, apply now stops with a clear message instead of downgrading to profile storage.
- **Uninstall cleanup** — v2 uninstall removes Juggernaut blocks from Windows PowerShell profile targets as well as Unix-style shell profiles.
- **Version bumped to 2.2.0** in `VERSION` and `bedrock-config.json`.

## [2.1.3] - 2026-04-24

### Fixed

- **Windows configure command compatibility** — `juggernaut apply --v2 --auth=bedrock-api-key` now works from the PowerShell launcher exactly as shown in the README and release notes.
- **PowerShell GNU-style flags** — the PowerShell dispatcher now translates documented `--flag` and `--flag=value` options into native PowerShell parameter binding for all v2 subcommands.
- **Windows HOME fallback** — `apply.ps1` no longer fails when `$env:HOME` is missing; it falls back to `$env:USERPROFILE` or the Windows user-profile folder.
- **Regression coverage** — added a Pester test for the release-note style Bedrock API-key apply command with `$env:HOME` absent.
- **Version bumped to 2.1.3** in `VERSION` and `bedrock-config.json`.

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

[2.2.5]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.5
[2.2.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.4
[2.2.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.3
[2.2.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.2
[2.2.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.1
[2.2.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.0
[2.1.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.3
[2.1.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.2
[2.1.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.0
[2.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.0.0
