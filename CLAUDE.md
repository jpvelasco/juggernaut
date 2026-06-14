# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform Go CLI tool that configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API. It writes settings only to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). The binary is self-contained — `bedrock-config.json` is embedded at build time via `//go:embed`.

## Commands

```bash
# Build
make build

# Run all tests
make test

# Run a single package
go test ./internal/schema/... -v

# Run a single test
go test ./cmd/... -run TestApply_WritesSettings_IAM -v

# Lint
make lint

# Dry-run apply (preview settings.json write without committing)
./bin/juggernaut apply --auth=iam --dry-run

# Local Codacy analysis (requires WSL2 with codacy-cli installed)
make codacy

# Sync Codacy rules from server first (requires CODACY_API_TOKEN)
make codacy-sync
```

## Architecture

Single Go binary. Entry point: `main.go` → `cmd/` → `internal/`.

**`main.go`** embeds `bedrock-config.json` via `//go:embed` and passes the bytes to `cmd.SetEmbeddedConfig()` before calling `cmd.Execute()`.

**Shared helpers in `cmd/helpers.go`:** `homeDir()`, `settingsPath()`, `loadBedrockConfig()`, `toMap()`, `fileExists()`. All command files in `cmd/` use these — don't duplicate them.

**Subcommands in `cmd/`:**
- `apply.go` — builds and writes the Juggernaut block to settings.json; installs the `claude` launcher shim; runs migration automatically if v3 block detected
- `show.go` — prints current config from settings.json
- `doctor.go` — read-only diagnostics (block presence, credentials, launcher shim)
- `uninstall.go` — removes the Juggernaut block from settings.json and deletes bearer token
- `migrate.go` — explicit v3→v4 migration; also triggered automatically by `apply`
- `version.go` — prints version

**Internal packages in `internal/`:**
- `bedrock/` — loads `bedrock-config.json` into typed structs. `LoadBytes()` is used by cmd (embedded); `Load(path)` is the filesystem fallback for tests.
- `schema/` — builds and validates the `Block`; derives native settings.json keys. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`.
- `config/` — atomic read/merge/write of settings.json; backup rotation (5 most recent); file locking via `gofrs/flock`
- `keychain/` — cross-platform credential storage via `go-keyring`. Service name: `juggernaut-bedrock`.
- `launcher/` — installs the `claude` shim (symlink on Unix, `claude.cmd` on Windows). `RunAsLauncher()` injects `AWS_BEARER_TOKEN_BEDROCK` from keychain and execs the real claude binary.
- `migrate/` — detects v3 block (schemaVersion:1), transfers bearer token, strips legacy shell launcher blocks. Minimum supported migration source: v3.2.3.
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`
- `authmode/` — constants/helpers for auth mode strings (`IAM`, `BedrockAPIKey`). Use `authmode.BedrockAPIKey` (split var) instead of bare string literals to avoid static secret scanner hits.
- `safepath/` — path containment checks (`JoinUnder`, `withinBase`) and owner-only filesystem helpers (`MkdirAll`, `ReadFile`, `WriteFile` at `0o700`/`0o600`). Use these whenever writing files under a user-controlled base path.

**Launcher mode:** when invoked as `claude` (detected via `slices.Contains(os.Args[1:], "--launcher")` or `os.Args[0]` basename) the binary injects credentials and execs the real claude. No shell profile modification required.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary. Changing it requires rebuilding. Tests fall back to filesystem resolution via `findBedrockConfigFile()`.
- **Single output target:** settings.json only.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default) vs `--scope=project`.
- **Re-apply preserves existing auth mode** — if `--auth` is omitted and a block already exists, the existing auth mode is read from the block.
- **`--model` overrides all three model IDs** (opus, sonnet, haiku) when set; `--opus-model`, `--sonnet-model`, `--haiku-model` override individually.
- **`--storage`:** credential storage backend — `keychain` (default), `dpapi` (Windows), or `profile` (env/shell).
- **`--no-1m-context`:** disables 1M token context window (on by default). `--1m-context` is a hidden no-op kept for script compatibility.
- **Mantle on by default:** opt out with `--no-mantle`; `--mantle-url` sets a custom base URL.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.

## Version Management

Version must stay in sync across **three** locations: `VERSION`, `bedrock-config.json` (`.version`), and `var Version` in `cmd/root.go`. CI enforces this — a mismatch fails the lint job.

## Testing

- Standard Go `testing` package. Run with `go test ./... -v` or `make test`.
- Keychain tests skip gracefully when the backend is unavailable — detected via a `skipIfUnavailable()` probe in `keychain_test.go`.
- To isolate keychain: set `JUGGERNAUT_KEYCHAIN_SERVICE` env var.
- Tests needing a clean home use `t.TempDir()` with `t.Setenv("HOME", ...)`.
- Cobra global flag state leaks between `ExecuteArgs()` calls — `resetFlags()` in `cmd/root.go` resets all subcommand flags to defaults before each invocation.

## CI / Release

- CI (`ci.yml`): lint + version sync check → `go test ./... -v` on ubuntu/macos/windows. Actions pinned to full commit SHAs.
- Release (`release.yml`): triggered on `v*` tags → GoReleaser builds binaries → npm OIDC publish to `juggernaut-bedrock`.
- GoReleaser requires a clean git tree — never commit build artifacts (`bin/` is gitignored).
- npm package is in `npm/` — `postinstall` downloads the platform binary from GitHub Releases and verifies SHA256.

## Codacy

`.codacy/tools-configs/` holds generated configs for revive, eslint, trivy, etc. created by `codacy-cli init`. Run `codacy-cli analyze` in WSL2 to check locally. The `.codacy.yml` at root excludes `.remember/` and `.codacy/` from analysis.
