# Changelog

All notable changes to Juggernaut will be documented in this file.

## [5.0.2] - 2026-06-19

**Patch release.** Fixes Windows upgrades when existing Claude settings were saved with a UTF-8 byte-order mark.

### Fixed

- **BOM-prefixed Claude settings.** `juggernaut apply` and `juggernaut doctor` now tolerate `settings.json` files that start with a UTF-8 BOM, preserving existing settings and rewriting the file as plain UTF-8 on the next successful apply.

## [5.0.1] - 2026-06-18

**Patch release.** Fixes Bedrock model routing and effort defaults for the v5 activation flow.

### Fixed

- **Mantle is opt-in.** `juggernaut apply` no longer enables Mantle by default. Standard Bedrock now preserves `global.*` inference profile IDs so Claude Code invokes profiles such as `global.anthropic.claude-sonnet-4-6` instead of failing on raw `anthropic.*` base model IDs. Use `--mantle` to opt into Mantle routing.
- **Claude Code model picker routing.** Native `modelOverrides` now include Claude Code's version keys such as `claude-sonnet-4-6` and `claude-opus-4-8`, plus the Bedrock raw IDs Claude Code may save from `/model`, so picker changes continue to route through Bedrock inference profiles.
- **Effort display alignment.** Default effort is now `high`, matching Sonnet 4.6's supported levels. `max` remains supported through `CLAUDE_CODE_EFFORT_LEVEL` but is not written to the native `effortLevel` setting, which Claude Code treats as non-`max`.
- **Sonnet capabilities.** Sonnet 4.6 now declares `max_effort` capability and does not advertise unsupported `xhigh` behavior.

## [5.0.0] - 2026-06-18

**Major release.** Juggernaut is now npm-only and no longer installs, replaces, or owns any `claude` binary. Anthropic's installer owns Claude Code; Juggernaut owns Bedrock settings, credentials, and shell activation.

### Changed

- **npm-only install.** `scripts/install.sh` and `scripts/install.ps1` are now deprecation stubs that point users to `curl -fsSL https://claude.ai/install.sh | bash` and `npm install -g juggernaut-bedrock`.
- **Shell activation replaces file shims.** `juggernaut apply` writes marked shell profile blocks that define `claude` as a function delegating to `juggernaut launch -- "$@"`.
- **Hidden launch command.** `juggernaut launch` injects Bedrock runtime environment, reads the Bedrock API key from the keychain when settings require it, resolves the real Anthropic `claude` binary while avoiding Juggernaut recursion, and forwards the original arguments.
- **Doctor diagnostics.** `doctor` now separately reports settings, keychain, shell activation, real Claude Code binary resolution, and legacy v4.2.6 artifacts.

### Removed

- **Launcher shims.** Juggernaut no longer creates `~/.local/bin/claude`, `claude.cmd`, `juggernaut-claude`, or symlink-based launchers.
- **v3 migration.** The `migrate` command, v3 auto-migration on `apply`, and v3 migration internals were removed.

### Fixed

- **v4.2.6 recovery.** `apply` and `uninstall --full` restore `~/.local/bin/claude.juggernaut-original` only when `claude` is missing, remove only positively identified Juggernaut shims, and leave unknown `claude` files untouched.

## [4.2.6] - 2026-06-17

**Patch release.** Fixes stale credential cleanup and adds diagnostics for the launch path and poisoned settings that can keep Claude retrying after a fresh Bedrock API key is installed.

### Fixed

- **Stale credential cleanup.** `apply` now removes legacy v3 credential files after storing a new Bedrock API key, so old profile or DPAPI artifacts do not linger beside the refreshed keychain entry.
- **Launcher-path diagnostics.** `doctor` now warns when `claude` resolves to something other than Juggernaut’s shim and checks both user and project settings scopes.
- **Poisoned model state.** `doctor` now flags top-level `model = "opusplan"` in settings, which can drive repeated Bedrock retries until `apply` rewrites the config.

### Other

- **Launcher path helper.** Added a shared shim-path helper so launcher install, uninstall, and presence checks use the same platform-specific path logic.

## [4.2.5] - 2026-06-17

**Patch release.** Tightens the npm wrapper and Codacy config after the 4.2.4 release, resolving the command-injection finding and keeping PMD/ESLint behavior aligned with the repo.

### Fixed

- **npm wrapper command execution.** Hardened `npm/index.js` so the wrapper resolves the target binary safely and no longer leaves a command-injection path that Codacy can flag as critical.
- **Binary resolution path.** Resolved the npm launcher target to a real absolute path before spawning it, which keeps the published wrapper behavior stable and removes the brittle path construction.
- **Codacy/npm lint alignment.** Excluded `npm/` from the root ESLint engine and corrected `nosemgrep` rule IDs so the repo’s suppressions match the scanner rules Codacy actually applies.

### Other

- **PMD ruleset tracked.** Added `.codacy/tools-configs/ruleset.xml` to source control so the repo’s PMD scan uses the same config locally and in CI.

## [4.2.4] - 2026-06-15

**Patch release.** Fixes Claude Code failing to connect when Mantle is enabled — `global.` cross-region inference prefixes are now stripped from model IDs when Mantle routing is active.

### Fixed

- **Mantle + global model IDs incompatible.** Mantle routes using `anthropic.*` model IDs; `global.anthropic.*` cross-region inference profiles are not valid Mantle targets. When Mantle is enabled (the default), Juggernaut now strips the `global.` prefix so model IDs are in the correct format. Without Mantle (`--no-mantle`), the prefix is preserved for standard Bedrock cross-region inference.

---

## [4.2.3] - 2026-06-15

**Patch release.** Fixes the npm binary not found error on global installs — sub-packages are now resolved via `require.resolve` instead of a hardcoded path that only exists in the repo.

### Fixed

- **npm binary not found on global install.** `npm install -g juggernaut-bedrock` installed sub-packages as siblings in `node_modules/`, but the code looked for them inside the main package's `packages/` subdirectory. Now uses `require.resolve(pkgName + "/package.json")` to find the installed sub-package regardless of where npm places it, with a fallback to `packages/` for local development.

---

## [4.2.2] - 2026-06-15

**Patch release.** Fixes the missing shebang in the npm package binary and resolves all Codacy findings across the npm package and CI pipeline.

### Fixed

- **npm binary missing shebang.** `npm/index.js` was missing `#!/usr/bin/env node`, causing the installed `juggernaut` command to fail with a shell syntax error on Linux and macOS.
- **Path traversal validation.** `getBinaryPath` now validates the resolved package name against a fixed allowlist before constructing the binary path.
- **`migrate_test.go` file permissions.** Two `os.MkdirAll` calls in tests used bare `0o700`; replaced with `safepath.MkdirAll` for consistency.

### Other

- **Trivy upgraded to 0.71.0.** Bumps the pinned Trivy version in `.codacy/codacy.yaml` from 0.70.0 to 0.71.0 (safe; March 2026 supply chain compromise only affected 0.69.4–0.69.6).
- **codacy-cli pinned to 1.0.0-main.380.** Updates the CI workflow pin from the April 14 build to the June 11 build.
- **npm package code quality.** Rewrote `npm/index.js` and `npm/index.test.js` to conform to Codacy ESLint rules: replaced ES2015+ syntax with ES5-compatible equivalents, added JSDoc annotations, and updated `engines` to `>=20.0.0`.

---

## [4.2.1] - 2026-06-14

**Patch release.** Fixes the release workflow — GoReleaser v2.16.0 does not support the `after` top-level key; binary copy moved into the release workflow as an explicit shell step.

### Fixed

- **GoReleaser `after` hook removed.** `after.hooks` is not a valid key in GoReleaser v2.16.0, causing releases to fail at the YAML parse stage. The platform binary copy (`scripts/copy-npm-binaries.sh`) is now run as an explicit workflow step between GoReleaser and npm publish.

---

## [4.2.0] - 2026-06-14

**Feature release.** Eliminates the `--allow-scripts` friction for npm 9+ users by shipping platform-specific binaries directly inside optional sub-packages — no postinstall script, no download at install time.

### New Features

- **Platform sub-packages.** Five new optional packages (`juggernaut-bedrock-linux-x64`, `-linux-arm64`, `-darwin-x64`, `-darwin-arm64`, `-win32-x64`) each ship the pre-built binary directly. npm installs only the matching one based on OS/arch. No scripts, no downloads, no `--allow-scripts` flag needed.
- **`index.js` binary resolver.** Replaces `install.js`: resolves the binary from the installed sub-package and execs it directly. Prints a clear, actionable error on unsupported platforms or missing binaries.

### Fixed

- **npm 9+ silent install failure.** `npm install -g juggernaut-bedrock` previously succeeded silently but left no working binary on npm 9+ because postinstall scripts are blocked by default. Now works with a plain `npm install -g juggernaut-bedrock`.
- **Main package tarball bloat.** `packages/` subdirectories and `index.test.js` were accidentally included in the main package tarball. Added `.npmignore` to exclude them.
- **repository.url format.** Package `repository.url` fields now use the canonical `git+https://` format to suppress npm auto-correct warnings on publish.

### Other

- **`install.js` deleted.** The 200-line postinstall downloader is no longer needed.
- **GoReleaser `after` hooks.** Copies each compiled binary into its sub-package `bin/` directory after build so the release workflow can publish all packages in one job.

---

## [4.1.0] - 2026-06-14

**Feature release.** Brings Claude Code feature parity for Bedrock users — Opus 4.8, agentic permission modes, effort level parity, and a reliable v3→v4 migration path for all credential storage backends.

### New Features

- **Claude Opus 4.8.** Default Opus model updated to `global.anthropic.claude-opus-4-8` (1M context, 128K output, adaptive thinking only). Opus 4.7 remains usable via `--opus-model`.
- **Permission modes (`--mode`).** Sets `permissions.defaultMode` in settings.json. Supported values: `default`, `acceptEdits`, `plan`, `auto`, `dontAsk`, `bypassPermissions`. When `auto` is set, Juggernaut also writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` — required on Bedrock for auto mode to activate.
- **Always-thinking (`--always-thinking`).** Writes `alwaysThinkingEnabled: true` as a native settings.json key, enabling extended thinking by default for all sessions.
- **Service tier (`--service-tier`).** Sets `ANTHROPIC_BEDROCK_SERVICE_TIER` (values: `default`, `flex`, `priority`) for provisioned throughput users.
- **Native settings.json keys.** `effortLevel`, `skipWebFetchPreflight`, `permissions`, and `modelOverrides` are now written as top-level settings.json keys (in addition to env vars), matching the Claude Code settings schema. All are removed cleanly on uninstall.
- **Effort level `max`.** All five effort levels (`low`, `medium`, `high`, `xhigh`, `max`) are now fully supported and documented.

### Fixed

- **Keychain re-prompt after migration.** `juggernaut apply` no longer re-prompts for the API key if the token was already transferred to the keychain during v3→v4 migration.
- **Keychain error handling.** A keychain backend failure no longer causes a hard error when `--preserve-key` is not set; the command falls through to an interactive prompt instead.
- **v3 profile token migration.** v3 users on Linux who used `--storage=profile` (token stored at `~/.config/juggernaut/bearer-token`) had their credentials silently lost on upgrade. The migration now reads and transfers profile-stored tokens correctly.
- **v3 DPAPI migration.** Windows users with DPAPI-stored tokens now receive a clear re-entry message instead of silent failure.
- **permissions deep-merge.** Juggernaut previously replaced the entire `permissions` object, silently wiping user-defined `allow`/`deny`/`ask` rules on every apply. Now only `permissions.defaultMode` is managed; all other user rules are preserved.
- **Permission mode preservation on re-apply.** Running `juggernaut apply` without `--mode` on an existing block now preserves the previously-set permission mode.
- **modelOverrides top-level key.** `modelOverrides` was stored inside the `juggernaut` block but not written as a top-level native key — Claude Code reads the top-level key. Fixed.
- **npm installer path hardening.** Resolves Codacy path-traversal findings in `npm/install.js` with explicit path containment checks.

### Verification

- 30+ new tests covering permission modes, effort levels, native key lifecycle, v3 migration storage backends, and uninstall/re-apply cycles.

---

## [4.0.4] - 2026-06-13

**Patch release.** Drops npm archive dependencies and ships tar.gz release artifacts on all platforms.

### Changed

- **npm installer.** Removes `tar` and `adm-zip` dependencies; extracts release archives with the system `tar` command and a PowerShell fallback for legacy Windows `.zip` assets.
- **Release artifacts.** GoReleaser now publishes `.tar.gz` on all platforms, including Windows.

### Fixed

- **PowerShell installer.** Picks archive format from `checksums.txt` so pinned installs of pre-v4.0.4 releases still download the Windows `.zip` asset.
- **Codacy local analysis.** Restores ESLint coverage and skips opengrep path-traversal false positives on the npm installer via `.semgrepignore`.

### Other

- **Refactor.** Drop npm archive deps ([#148](https://github.com/jpvelasco/juggernaut/pull/148)).

---

## [4.0.3] - 2026-06-13

**Patch release.** Brings Go Report Card to 100% on the v4 module path and updates CI tooling.

### Fixed

- **Go Report Card quality.** Normalize Go sources to LF via `.gitattributes`, fix missing final newlines, and reduce cyclomatic complexity in `apply`/`uninstall` so all GRC checks pass.
- **Go Report Card badge.** Point the README badge at `github.com/jpvelasco/juggernaut/v4` so the report resolves the correct module.

### Other

- **CI.** Bump `golangci/golangci-lint-action` to latest v9 SHA ([#146](https://github.com/jpvelasco/juggernaut/pull/146)).

---

## [4.0.2] - 2026-06-13

**Patch release.** Fixes Go module path for v4 semver tags so the module is indexable on `proxy.golang.org` and Go Report Card works.

### Fixed

- **Go module path.** Declares `github.com/jpvelasco/juggernaut/v4` in `go.mod` and updates all internal imports — required for v4.x tags per Go module versioning rules.

---

## [4.0.1] - 2026-06-13

**Patch release.** Security hardening for the Go binary and npm installer, Codacy cleanup, and dependency/CI updates since v4.0.0.

### Fixed

- **npm installer hardening.** Downloads release artifacts to memory with GitHub host allowlisting; extracts archives via buffer APIs to avoid path traversal; ships `npm/bin` in the package; tightens installed binary permissions to `0o700`.
- **Go path handling and process execution.** Adds `internal/safepath` for path containment checks; migrate and config code use safepath instead of raw `os` calls; launcher resolves `claude` via `exec.LookPath` after scrubbing the shim from PATH; renames keychain account to `bedrock-credential` with legacy `api-key` fallback.
- **Codacy false positives and local parity.** Centralizes auth mode strings in `internal/authmode`; hardens settings/profile paths via safepath; adds local Codacy parity workflow; satisfies PSScriptAnalyzer in the Codacy CI job.
- **Security scan hygiene.** Pins npm deps to exact versions (`tar` 7.5.16, `adm-zip` 0.5.17); bumps Go to 1.26.4; tightens file permissions (`0o700` dirs, `0o600` files); suppresses intentional false-positive findings with targeted `nolint` and ESLint exclusions.
- **CI alignment.** Aligns CI `go-version` with `go.mod`; suppresses `os.Setenv` errcheck false positive.

### Documentation

- **README rewrite for v4.** Documents npm/curl/PowerShell install paths and the Go binary workflow.
- **CLAUDE.md.** Adds single-test commands, helpers pattern, Codacy, and CI/release notes.

### Other

- Upgrades to Go 1.26 stable and applies modern Go idioms (`slices.Contains`, `maps.Copy`, etc.).
- Bumps GitHub Actions: `checkout` 4→6, `setup-go` 5→6, `setup-node` 4→6, `golangci-lint-action` 6→9, `goreleaser-action` 6→7.
- Repo cleanup: removes planning artifacts, updates docs for v4, fixes release workflow.

---

## [4.0.0] - 2026-06-05

**Major release.** Full rewrite from bash/PowerShell to a single cross-platform Go binary with npm distribution.

### Changed
- Full rewrite from bash/PowerShell to Go — single cross-platform binary, no shell dependencies
- Launcher is now a symlink (Unix) or `.cmd` shim (Windows) — no shell profile modification required
- Keychain handled via go-keyring — no platform-specific shell commands or jq required
- `jq` is no longer a dependency

### Added
- `juggernaut migrate` — explicit v3→v4 migration command with `--dry-run` support
- Automatic migration detection on first run of any command when v3 config is found
- Interactive first-run prompts via `charmbracelet/huh` when `apply` is run without flags
- `--json` flag on `show`, `doctor`, and `version` for machine-readable output
- npm distribution: `npm install -g juggernaut-bedrock`
- GoReleaser: pre-built binaries for linux/darwin/windows × amd64/arm64

### Removed
- Bash and PowerShell source scripts (preserved on `legacy/v3` branch, pinned at v3.2.3)

### Migration
Upgrade from v3.2.3: install the v4 binary, then run `juggernaut migrate` or simply `juggernaut apply`.
Pre-v3.2.3 installations must upgrade to v3.2.3 first before migrating to v4.

---

## [3.2.3] - 2026-05-15

**Patch release.** Launcher resilience fix.

### Fixed

- **Launcher now exports `CLAUDE_CODE_USE_BEDROCK=1`** (bash/zsh/fish/PowerShell). Bedrock routing survives `claude update` wiping `settings.json`.
- **Doctor drift detection.** Reports when the juggernaut block is missing from `settings.json` but credentials still exist, with a re-apply hint.

## [3.2.2] - 2026-05-11

**Enhancement release.** Installer now performs a non-destructive light update when upgrading from v3.2.0 or later, preserving credentials and `~/.claude/settings.json`.

### New Features

- **Version Gate Policy.** `install.sh` / `install.ps1` detect the currently installed version (from `$INSTALL_DIR/VERSION`) before acting. Upgrades from `>= MIN_SUPPORTED_VERSION` (`3.2.0`) skip the profile-block strip, `settings.json` `juggernaut` key removal, keychain entry deletion, and profile-token deletion. Fresh installs and upgrades from older versions keep the full destructive wipe.
- **`--force-wipe` / `-ForceWipe`.** Escape hatch to force the legacy full-wipe behavior on any eligible version — useful for recovery or debugging.
- **Pre-wipe summary expanded.** Installer now prints the detected installed version, the minimum version for a light update, the chosen mode, and the reason.
- **Post-install message respects mode.** Light-update runs print "Credentials and settings preserved; no re-apply needed." Full-wipe runs keep the existing "Configure Juggernaut explicitly with..." prompt.

### Documentation

- **README**: new `### Upgrade Behavior` subsection under `## Install` with a mode-selection matrix and `--force-wipe` examples. Key Design Decisions bullet updated from "Destructive installer" to "Version Gate installer".

### Verification

- `tests/v2/test_install.sh` — new runtime scenarios: light-update from v3.2.0 fixture (credentials + settings preserved, launcher refreshed), `--force-wipe` on eligible version (full wipe happens), fresh install (full wipe, "no previous installation" reason). Updated existing full-wipe test to simulate a pre-gate v3.1.0 install.
- `tests/v2/Installer.Tests.ps1` — new static assertions for `MIN_SUPPORTED_VERSION`, `-ForceWipe`, `Compare-SemVer`, `Get-InstalledVersion` in both installers.

## [3.2.1] - 2026-05-11

**Patch release.** Documentation update for Claude Opus 4.7 tokenizer behavior.

### Documentation

- **Opus 4.7 tokenizer note.** README now calls out that Opus 4.7 uses an updated tokenizer that may consume approximately 1–1.35× more tokens than Opus 4.6 for equivalent tasks — relevant when estimating Bedrock costs when switching from an older Opus model.

## [3.2.0] - 2026-05-10

**Feature release.** Completes the v3 toolchain with a Makefile, Fish shell launcher, stricter CLI handling, version subcommand, improved CI, and deeper test coverage.

### New Features

- **Makefile** for local development (`make test`, `make lint`, `make install-dry-run`, `make doctor`, per-suite targets, and `make help`).
- **Fish shell launcher support** — `install.sh` now writes a proper Fish `function claude` block to `~/.config/fish/config.fish` when Fish is installed. Fully idempotent; `juggernaut uninstall` cleans it up.
- **`juggernaut version` subcommand** (Bash + PowerShell). `--version` / `-v` flags remain supported for backward compatibility.

### Fixed

- Unknown options now exit with code 1 (in `apply`, `doctor`, and `show` for both Bash and PowerShell), showing the bad flag and a usage hint.
- `doctor` now correctly reports bearer-token storage source (no longer incorrectly says "system keychain" for profile storage).
- `ANTHROPIC_MODEL` is only written when `--opusplan` is enabled. Re-applying with `--no-opusplan` removes it.

### Changed (CI & Tooling)

- Git Bash CI job now runs full end-to-end `install.sh` scenario.
- Version sync check now validates `lib/schema.sh` and `lib/schema.ps1` fallbacks.
- All test jobs upload artifacts (retained 14 days).

### Verification

- Expanded Pester coverage for unknown options, version subcommand, etc.
- Full Bash test suite + `make ci`.

## [3.1.5] - 2026-05-10

**Patch release.** Adds a `Makefile` for local development convenience and expands CI with a Git Bash full-install scenario and test artifact upload.

### Added

- **`Makefile`.** Common development targets: `make test` (all bash suites), `make lint` (shellcheck), `make install-dry-run` (preview install without writing), `make doctor` (check active installation), plus per-suite targets (`make test-apply`, `make test-launcher`, etc.) and a `make help` summary.
- **CI: Git Bash full-install scenario.** The `test-windows-gitbash` job now runs a full `install.sh` end-to-end install (in addition to the existing dry-run), cloning from a local fixture repo, verifying the installed VERSION, and asserting no auto-apply occurred.
- **CI: test artifact upload.** All three test jobs upload their output for easier post-mortem on failures:
  - `test-unix` (ubuntu + macos): uploads `test-results/*.txt` (one file per bash test suite) as `test-results-<os>`.
  - `test-windows-gitbash`: uploads `test-results/*.txt` as `test-results-windows-gitbash`.
  - `test-windows-powershell`: uploads `pester-results.xml` as `pester-results`.
  Artifacts are retained for 14 days.
- **`.gitignore` covers `test-results/`.** The CI `tee` output directory is now excluded from version control.

## [3.1.4] - 2026-05-10

**Patch release.** Adds fish shell support to the Claude launcher installer.

### Added

- **Fish shell launcher support.** `install.sh` now writes a `function claude` block (using proper fish syntax) to `~/.config/fish/config.fish` when that file exists or fish is installed. The fish block shells out to bash to call `lib/keychain.sh` (fish cannot source bash functions), injecting `AWS_BEARER_TOKEN_BEDROCK` before exec'ing the real `claude` binary via `command claude` with `$argv`.
- **Idempotent fish install.** Re-running `install.sh` strips any existing `# BEGIN: Juggernaut Launcher` / `# END: Juggernaut Launcher` block from `config.fish` before writing, same as bash/zsh profiles.
- **`uninstall.sh` removes fish launcher block.** `~/.config/fish/config.fish` is now included in `_launcher_profile_candidates` so `juggernaut uninstall` cleans up fish profile blocks too.

### Testing

- `test_launcher.sh`: new sections verifying the fish block uses fish syntax (`function claude` / `command claude $argv` rather than `claude()` / `"$@"`), that idempotent re-install produces exactly one block, and that `uninstall.sh` correctly strips it.

## [3.1.3] - 2026-05-09

**Patch release.** Adds `juggernaut version` subcommand and adds CI enforcement that schema fallback version strings stay in sync with `VERSION`.

### Added

- **`juggernaut version` subcommand.** `juggernaut version` (and `juggernaut.ps1 version`) now print the installed version and exit 0. The existing `--version` / `-v` flags continue to work; `version` is the new canonical subcommand form, consistent with other CLI tools.
- **CI version-sync check covers `lib/schema.sh` and `lib/schema.ps1`.** The `Verify version sync` step in `.github/workflows/test.yml` now also asserts that the `${J_VERSION:-X.Y.Z}` fallback in `lib/schema.sh` and the `[string]$Version = 'X.Y.Z'` default in `lib/schema.ps1` both match `VERSION`. This catches the class of bug where a version bump updates `VERSION` and `bedrock-config.json` but forgets the schema fallbacks, causing Pester's `meta.version` assertions to fail.
- **Schema fallback strings updated to 3.1.3.**

### Testing

- `test_apply.sh`: new "juggernaut version subcommand prints semver and exits 0" and "--version flag still works (backward compat)" sections.

## [3.1.2] - 2026-05-09

**Patch release.** Hardens unknown-option handling so typos and unrecognised flags fail fast.

### Fixed

- **`juggernaut apply` now exits non-zero on unknown options.** Previously, unrecognised flags were silently ignored with an `(ignored)` warning, allowing typos to go unnoticed. The command now exits 1 and prints the unknown flag name and a usage hint.
- **`juggernaut doctor` and `juggernaut show` now exit non-zero on unknown options.** Both commands had a silent `*)` fallthrough in their option-parsing loops. They now exit 1 with a message consistent with the rest of the codebase.
- **`juggernaut.ps1 doctor` and `juggernaut.ps1 show` now exit non-zero on unknown options.** The PowerShell `RemainingArgs` loops had no default case (or a silent `default {}`). Both now write an error and exit 1.
- **`.gitignore` covers `pester-results.xml`.** The CI Pester job writes `pester-results.xml`; only `testResults.xml` was previously excluded. Both are now ignored.

### Testing

- `test_apply.sh`, `test_show.sh`, `test_doctor.sh`: new section each verifying that an unrecognised flag exits non-zero and names the offending option.

## [3.1.1] - 2026-05-09

**Patch release.** Fixes stale `doctor` remediation text after the v3 auth gate.

### Fixed

- **`juggernaut doctor` now prints explicit auth commands on failure.** Fresh installs no longer tell users to run bare `juggernaut apply`, which now intentionally rejects missing credentials on first run. The PowerShell and Bash doctor summaries now point at `juggernaut apply -Auth iam` and `juggernaut apply -Auth bedrock-api-key` / `--auth=...` instead.

### Verification

- **Doctor regression coverage added.** PowerShell and Bash tests now cover the fresh-install no-config case that surfaced the stale remediation.

## [3.1.0] - 2026-05-09

**Feature release.** Adds first-class full uninstall support.

### New Features

- **Full uninstall support.** Added `juggernaut uninstall --full` (and `-Full` on PowerShell) to completely remove Juggernaut, including the command launcher/shims (`~/.local/bin/juggernaut`, `juggernaut.cmd`, `juggernaut.ps1`) and install directory (`~/.juggernaut` or custom `JUGGERNAUT_DIR`).
- **Non-interactive full removal.** Added `--yes` / `-Yes` for confirmed, non-interactive full removal.
- **Full-removal previews.** Added `--full --dry-run` to preview exactly what will be deleted before removing files.

### Documentation

- **Uninstall docs refreshed.** Reworked the README uninstall section with clearer guidance and platform-specific full removal instructions for macOS, Linux/WSL/Git Bash, and Windows PowerShell.
- **Shell cleanup covered.** Added fish shell cleanup steps and guidance for already-loaded `claude`/`juggernaut` shell state.
- **Risk and behavior clarified.** Added a permanent deletion warning and clarified the difference between configuration cleanup and full removal.

### Other

- **Installer full-wipe path documented.** Re-running the installer remains the easiest full-wipe path for most users because it performs a destructive wipe before reinstalling.

## [3.0.8] - 2026-05-04

**Patch release.** Fixes Linux Bedrock API-key profile storage.

### Fixed

- **Linux `apply --auth=bedrock-api-key` now persists profile-storage keys.** The Bash apply path writes profile-storage keys to a per-user token file and the launcher/doctor paths read that same file.
- **`juggernaut doctor` now recognizes Linux profile token storage.** Bedrock API-key configs no longer report a missing key immediately after a successful Linux apply.
- **Installer and uninstall cleanup include the profile token file.** Wipe/reinstall and uninstall remove stale Linux profile-storage tokens.

## [3.0.7] - 2026-05-04

**Patch release.** Suppresses Git's detached-HEAD advisory during pinned release installs.

### Fixed

- **Pinned Linux/macOS installs are quiet again.** `install.sh --version vX.Y.Z` now disables Git's detached-HEAD advice when cloning or checking out a tag, including the dirty-install backup path.
- **PowerShell pinned installs get the same cleanup.** `install.ps1 -Version vX.Y.Z` now suppresses the same Git advisory on tagged installs.

## [3.0.6] - 2026-05-04

**Patch release.** Fixes the `opusplan` repair path and hardens Bedrock API key entry across Windows, macOS, and Linux.

### Fixed

- **`opusplan` is preserved as a routing mode, not persisted as a model ID.** If Claude Code writes `"opusplan"` into a model field, `juggernaut apply` now translates that state into `juggernaut.opusplan = true` and `env.ANTHROPIC_MODEL = "opusplan"` while restoring top-level `.model` and `juggernaut.model` to a real Bedrock model ID. Explicit `--model=opusplan` / `-Model opusplan` is rejected with a clear error.
- **Long Bedrock API key paste path is hardened.** Bash now uses Readline for the normal interactive paste prompt, and PowerShell uses `[Console]::ReadLine()` instead of `Read-Host`. Optional clipboard input is available when terminal paste is unreliable.
- **Settings writes are more robust.** Backup creation failures now warn and continue, while PowerShell atomic write failures are treated as real failures instead of reporting false success.

## [3.0.5] - 2026-05-03

**Patch release.** Adds a `juggernaut doctor` check that detects when Claude Code's `/model` UI has poisoned the top-level `.model` field with `"opusplan"`, causing infinite Bedrock retries.

### Added

- **`juggernaut doctor` detects `opusplan` poisoning of top-level `.model`.** Claude Code's `/model` UI can write the literal string `"opusplan"` into the top-level `.model` field of `settings.json`. Claude Code then sends that string to Bedrock as a model ID; Bedrock rejects it and Claude Code retries on a fixed schedule, hanging every session. Doctor now surfaces a `WARN` in the "Region & Models" section when `top-level .model == "opusplan"` and points at `juggernaut apply` as the repair. `opusplan` is a routing mode for `env.ANTHROPIC_MODEL` and belongs only there — never in top-level `.model`.

### Notes

- `juggernaut apply` already repairs the poisoned state idempotently (`config_merge_juggernaut_block` rewrites top-level `.model` from the stored block on every run). No apply-side change was needed; the gap was purely that doctor did not surface the broken state.

## [3.0.4] - 2026-05-04

**Patch release.** Fixes Windows installer one-liner triggering Microsoft Defender's AMSI fileless-malware heuristics.

### Fixed

- **Windows installer one-liner no longer trips Microsoft Defender.** The `& ([scriptblock]::Create((irm ...)))` form matched Defender's AMSI fileless-malware heuristics (`Trojan:PowerShell/Powdow`, `Trojan:Script/Wacatac.*!ml`) independent of script content — AMSI double-scans the command string and the constructed scriptblock body. Replaced with a download-then-run one-liner: `irm -OutFile $p`, `Unblock-File $p`, execute the file, then `Remove-Item $p`. Same one-line paste UX; Defender scans the file on disk via its static-analysis path. Follows Microsoft's own pattern for `dotnet-install.ps1`.

### Changed

- `install.ps1` `| iex` guard error message now steers users to the download-then-run form instead of the old `[scriptblock]::Create` form.
- README.md and QUICKSTART.md Windows one-liners updated to v3.0.4 form. Old tagged releases still have the old `[scriptblock]::Create` text in their checked-in `install.ps1` headers — those tags continue to work but Defender may flag the advertised command for pre-v3.0.4 tags.

## [3.0.3] - 2026-05-04

**Patch release.** Fixes API key paste truncation on Linux, macOS, and PowerShell when using `juggernaut apply --auth=bedrock-api-key`.

### Fixed

- **API key paste on Linux/macOS/PowerShell.** `juggernaut apply --auth=bedrock-api-key` no longer drops pasted keys under tmux, screen, SSH sessions, or long-key Windows Terminal clipboards. Root cause was `read -s < /dev/tty` (bash) and `Read-Host -AsSecureString` (PowerShell) — both suppress bracketed-paste sequences and have terminal line-buffer caps that silently truncate long keys. Both are replaced by paste-safe implementations.

### Added

- **Piped stdin key input.** The API key can now be passed via stdin pipe: `echo $KEY | juggernaut apply --auth=bedrock-api-key`. This is the recommended path for scripts, CI/CD, and anywhere paste truncation is a concern.
- **Truncation sanity check.** Keys under 40 characters are rejected with an error that steers toward the pipe form. Bedrock API keys are 100–2400+ chars in practice; anything shorter indicates a truncation failure.

### Changed

- **Interactive key prompt is now visible.** The prompt shows `>` and echoes typed/pasted characters instead of masking them with `*`. The key is not a shoulder-surf secret — it is immediately written to `settings.json` and the system keychain/DPAPI store. Masking provided no meaningful security and was the direct cause of the paste-truncation bug.

## [3.0.2] - 2026-05-03

**Patch release.** Fixes Windows long-form Bedrock API key storage via DPAPI and adds bearer token utilities.

### Added

- **DPAPI-backed bearer token storage.** Long-form Bedrock API keys (>1280 chars) now persist to `~/.juggernaut/bearer-token.dpapi.bin` on Windows instead of silently failing in Credential Manager.
- **PowerShell bearer token functions.** `Save-BearerToken`, `Read-BearerToken`, and `Remove-BearerToken` in `lib/keychain.ps1` for manual key management.
- **`dpapi_get` shell function.** Bash/zsh reads bearer token from DPAPI file on Git Bash/Cygwin.
- **`bearer_token_get` shell function.** Bash/zsh tries DPAPI first then keychain.
- **auth.storage option.** The Juggernaut block now supports `auth.storage` field to specify storage backend (`profile`, `keychain`, or `dpapi`).

### Changed

- **Doctor reporting.** `juggernaut doctor` now reports the bearer token storage backend (DPAPI file vs Credential Manager/system keychain).

### Fixed

- **Windows long-form Bedrock API keys.** On Windows, `apply -Auth bedrock-api-key` now correctly uses the DPAPI file for long-form keys (>~1280 chars) instead of silently failing in Credential Manager. Short keys (~≤1280 chars) still use Credential Manager.

### Notes

- DPAPI storage is per-user only; it does not sync across machines.

## [3.0.1] - 2026-05-02

**Patch release.** Closes the keychain → Claude Code hand-off gap introduced by v3.0.0's removal of the shell-profile fallback.

### Added

- **Claude launcher wrappers.** A bracketed `claude()` shell function appended to `~/.bashrc`/`~/.zshrc`/`~/.profile` (Unix) and a matching `function claude` block in `$PROFILE.CurrentUserCurrentHost` (PowerShell) now read `AWS_BEARER_TOKEN_BEDROCK` from the OS keychain and inject it into the child process's environment before invoking the real `claude` binary. Fixes the class of "fresh shell with `CLAUDE_CODE_USE_BEDROCK=1` in settings.json hangs on `claude`" failures. Installers place both automatically; uninstall strips them. The shell-function approach (not a file-on-disk symlink) survives Anthropic's `claude update` self-rewrites that would clobber `~/.local/bin/claude`.
- **Doctor launcher check.** `juggernaut doctor` now includes a **Launcher** section that warns when Bedrock is active, no bearer token is in env, and no launcher block is present in any shell profile — naming the fix ("re-run the installer or set `AWS_BEARER_TOKEN_BEDROCK`"). Not-applicable for IAM auth.
- **Launcher test matrices.** `tests/v2/test_launcher.sh` (bash cases: env preservation, keychain hit, keychain miss, keychain error fall-through, keychain lib absent, argv passthrough, install idempotency, uninstall strip) and `tests/v2/Launcher.Tests.ps1` (Pester cases: static source checks, idempotent install/uninstall of the profile block, function precedence, runtime behavior via a stub `claude.cmd`).

### Changed

- **Louder keychain-store failure on Windows.** `Set-KeychainEntry` now surfaces the Win32 error code when `CredWrite` fails (instead of returning `$false` silently), and `apply` prints a yellow warning when it falls back from keychain to profile storage. This makes the Windows long-key limitation (below) obvious at the moment it bites, rather than only at `claude` launch time.

### Notes

- No `settings.json` schema change. The launcher is purely a runtime hand-off — it never writes to settings or to the keychain, only reads.
- The launcher falls through silently on any keychain error so `claude` still launches (users can still pass `AWS_BEARER_TOKEN_BEDROCK` directly or use IAM auth).
- Unix uninstall only removes a `~/.local/bin/claude` entry if it is a symlink (legacy v3.0.x-dev artifact). A regular file at that path — such as Anthropic's own `claude` binary — is never touched.
- Editor / IDE integrations that invoke the `claude` binary directly, bypassing interactive shells and the PowerShell profile, will not get the env injection — they must set the token themselves.

## [3.0.0] - 2026-05-01

**Breaking release.** Clean break from v1 and v2's dual-code-path setup. Re-run the installer to upgrade — there is no migration script.

### Removed

- **v1 shell-profile-only mode** — every root-level v1 script (`setup`, `setup-claude-bedrock.{sh,ps1}`, `apply-config.{sh,ps1}`, `validate-setup.{sh,ps1}`, root `uninstall.{sh,ps1}`, `test.sh`), every v1 library (`lib/migrator.{sh,ps1}`, `lib/upgrade_banner.{sh,ps1}`), the `migrate` subcommand, and every v1 test fixture deleted. The `--legacy-v1`, `-LegacyV1`, `JUGGERNAUT_USE_V2`, `JUGGERNAUT_USE_V1`, `JUGGERNAUT_SUPPRESS_DEPRECATION`, `JUGGERNAUT_PS_V1_SCAN`, `JUGGERNAUT_FORCE_MIGRATION_PROMPT`, and `--force-migration-prompt` gates are gone.
- **Shell-profile fallback** — `lib/profile_writer.{sh,ps1}`, `--no-shell-fallback`/`--shell-fallback-only` flags, the orphaned `keychain_get_command`/`Get-KeychainRetrievalExpression` helpers, and every profile-related doctor/show/uninstall code path deleted. `settings.json` is the sole output.
- **Installer auto-apply** — `--configure`/`-Configure`, `--yes`/`-Yes`, `--keep-all-backups`/`-KeepAllBackups`, and `SETUP_ARGS` are gone. Installers never invoke `juggernaut apply`.

### Changed (breaking)

- **Installer is destructive wipe-and-reinstall.** Every run of `install.sh` / `install.ps1` strips legacy `# BEGIN: Juggernaut` and `# BEGIN: Claude Code Bedrock Configuration` blocks from all known shell-profile paths, removes the `juggernaut` key from `~/.claude/settings.json`, and deletes the `juggernaut-bedrock` OS-keychain entry before placing fresh files. Pre-wipe summary printed in every run. `--dry-run` / `-DryRun` previews without writing.
- **`CLAUDE_CODE_USE_BEDROCK=1` now gated behind validated auth.** `juggernaut apply` refuses to write it unless `aws sts get-caller-identity` succeeds, `AWS_BEARER_TOKEN_BEDROCK` is set, or a `juggernaut-bedrock` keychain entry exists. Pass `--auth=iam` or `--auth=bedrock-api-key` (or `-Auth iam` / `-Auth bedrock-api-key` on PowerShell) to confirm. Internally threaded as `J_AUTH_VALIDATED=true` into `lib/schema.{sh,ps1}`.
- **Mantle routing enabled by default.** `--mantle`/`-Mantle` replaced with `--no-mantle`/`-NoMantle`. Auto-enable on bearer-token detection removed (Mantle is always on unless opted out).
- **Windows profile coverage.** Installer scans `Documents\WindowsPowerShell\profile.ps1` (5.1) and `Documents\PowerShell\profile.ps1` (7), plus `$PROFILE.AllUsersAllHosts` and `$PROFILE.CurrentUserAllHosts`. OneDrive-redirected `Documents\` followed automatically. Non-admin runs warn-and-skip AllUsers paths.

### Added

- **Opusplan drift diagnostic.** `juggernaut doctor` now compares `.env.ANTHROPIC_MODEL` in settings vs. the Juggernaut block and WARNs on mismatch when `opusplan` is enabled. Catches external `ANTHROPIC_MODEL` overrides.

### Migration

Re-run the installer. There is no other upgrade path.

## [2.3.4] - 2026-05-01

### Fixed

- **Linux doctor scope detection** — running `doctor --v2` from `$HOME` no longer reports the user settings file as both user and project scope.
- **Linux apply option parsing** — `--bedrock-key VALUE` now works in addition to `--bedrock-key=VALUE`.
- **Linux doctor keychain validation** — keychain shell fallback stubs no longer count as a valid API key when the OS keychain is empty.

## [2.3.3] - 2026-05-01

### Fixed

- **PowerShell doctor credentials** — `doctor --v2` now fails when config says `auth.storage=keychain` but the OS keychain has no Bedrock API key, even if a shell fallback block still contains the keychain lookup stub.
- **Pinned install examples** — README and QUICKSTART now pin the installer version to v2.3.3 so release-tagged install commands cannot drift to a later branch checkout.

## [2.3.2] - 2026-05-01

### Fixed

- **PowerShell migration cleanup** — `migrate -Clean -Yes` now scans the canonical PowerShell profile paths and removes any Juggernaut-managed profile block, including stale blocks left by failed v2.3.0/v2.3.1 upgrade attempts.
- **PowerShell upgrade detection** — marked v2 shell fallback blocks are no longer reported as v1 blocks by the upgrade banner.
- **PowerShell banner output** — installer upgrade banners now use ASCII borders/arrows to avoid mojibake in legacy Windows consoles.

## [2.3.1] - 2026-05-01

### Fixed

- **Windows installer one-liner guard** — `install.ps1` now detects the fragile `irm ... install.ps1 | iex` invocation path and exits with a clear message recommending the safer scriptblock form.
- **Windows install docs** — README and QUICKSTART pin Windows examples to the scriptblock installer command for v2.3.1.

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

[5.0.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v5.0.2
[5.0.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v5.0.1
[5.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v5.0.0
[4.0.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.4
[4.0.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.3
[4.0.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.2
[4.0.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.1
[4.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.0
[3.2.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.3
[3.2.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.2
[3.2.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.1
[3.2.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.0
[3.1.5]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.5
[4.2.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.4
[4.2.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.3
[4.2.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.2
[4.2.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.1
[4.2.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.0
[4.2.6]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.6
[4.2.5]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.2.5
[4.1.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.1.0
[4.0.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.4
[4.0.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.3
[4.0.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.2
[4.0.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.1
[4.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v4.0.0
[3.2.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.3
[3.2.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.2
[3.2.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.1
[3.2.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.0
[3.1.5]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.5
[3.1.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.4
[3.1.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.3
[3.1.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.2
[3.1.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.1
[3.1.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.1.0
[3.0.8]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.8
[3.0.7]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.7
[3.0.6]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.6
[3.0.5]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.5
[3.0.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.4
[3.0.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.3
[3.0.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.2
[3.0.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.1
[3.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v3.0.0
[2.3.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.3.4
[2.3.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.3.3
[2.3.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.3.2
[2.3.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.3.1
[2.3.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.3.0
[2.2.5]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.5
[2.2.4]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.4
[2.2.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.3
[2.2.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.2
[2.2.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.1
[2.2.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.2.0
[2.1.3]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.3
[2.1.2]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.2
[2.1.1]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.1
[2.1.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.1.0
[2.0.0]: https://github.com/jpvelasco/juggernaut/releases/tag/v2.0.0
