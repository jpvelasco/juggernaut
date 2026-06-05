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

# Lint
make lint

# Dry-run apply (preview settings.json write without committing)
./bin/juggernaut apply --auth=iam --dry-run
```

## Architecture

Single Go binary. Entry point: `main.go` → `cmd/` → `internal/`.

**Subcommands in `cmd/`:**
- `apply.go` — builds and writes the Juggernaut block to settings.json; installs the `claude` launcher shim
- `show.go` — prints current config from settings.json
- `doctor.go` — read-only diagnostics (block presence, credentials, launcher shim)
- `uninstall.go` — removes the Juggernaut block from settings.json and deletes bearer token
- `migrate.go` — migrates from v3 (shell) to v4 (Go); runs automatically on first apply
- `version.go` — prints version

**Internal packages in `internal/`:**
- `bedrock/` — loads `bedrock-config.json` into typed structs (`Config`, `ModelSet`, etc.)
- `schema/` — builds and validates the `JuggernautBlock`; derives native settings.json keys. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`.
- `config/` — atomic read/merge/write of settings.json; backup rotation (5 most recent); file locking via `gofrs/flock`
- `keychain/` — cross-platform credential storage via `go-keyring` (macOS Keychain, Linux Secret Service, Windows Credential Manager). Service name: `juggernaut-bedrock`.
- `launcher/` — installs the `claude` shim: symlink on Unix, `claude.cmd` batch shim on Windows. `RunAsLauncher()` injects `AWS_BEARER_TOKEN_BEDROCK` from keychain and execs the real claude binary.
- `migrate/` — detects v3 block (schemaVersion:1), transfers bearer token, strips legacy shell launcher blocks. Minimum supported migration source: v3.2.3.
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`

**Launcher:** when invoked as `claude` (detected via `os.Args[0]`) or with `--launcher`, the binary reads the keychain and execs the real claude. No shell profile modification required.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary via `//go:embed` in `main.go`. No external file needed after install.
- **Single output target:** settings.json only.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default, `~/.claude/settings.json`) vs `--scope=project` (`./.claude/settings.json`).
- **Mantle on by default:** opt out with `--no-mantle`.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.

## Version Management

Version must stay in sync across **three** locations: `VERSION`, `bedrock-config.json` (`.version`), and `var Version` in `cmd/root.go`. CI enforces this — a mismatch fails the lint job.

## Testing

- Standard Go `testing` package. Run with `go test ./... -v` or `make test`.
- Keychain tests skip gracefully when the backend is unavailable (headless CI).
- To isolate keychain: set `JUGGERNAUT_KEYCHAIN_SERVICE` env var.
- Tests that need a clean home use `t.TempDir()` with `t.Setenv("HOME", ...)`.
- Cobra global flag state is reset between `ExecuteArgs()` calls via `resetFlags()` in `cmd/root.go`.

## Gotchas

- **bedrock-config.json is embedded** — changing it requires rebuilding the binary.
- **Launcher mode detection** — `os.Args[0]` basename check (`claude`) must match exactly; `.exe` suffix is stripped on Windows.
- **Migration gate** — v3 installs older than v3.2.3 are blocked with a clear error directing users to upgrade the shell version first.
- **`--model` flag overrides all three model IDs** (opus, sonnet, haiku) when set.
- **Re-apply preserves existing auth mode** — if `--auth` is omitted and a block already exists, the existing auth mode is read from the block rather than defaulting to `iam`.
