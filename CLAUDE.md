# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform Go CLI tool that configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API. It writes Bedrock settings to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope), and installs marked shell activation blocks that define a `claude` function. The binary is self-contained — `bedrock-config.json` is embedded at build time via `//go:embed`.

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
- `apply.go` — builds and writes the Juggernaut block to settings.json; installs shell activation; safely recovers known broken v4.2.6 launcher artifacts
- `launch.go` — hidden command used by shell activation; injects Bedrock runtime env and runs the real Claude Code binary
- `show.go` — prints current config from settings.json
- `doctor.go` — read-only diagnostics (settings, credentials via the configured backend, activation, Claude Code binary, v4.2.6 artifacts, stale v3 PowerShell/Bash installs)
- `uninstall.go` — removes the Juggernaut block from settings.json and deletes bearer token; `--full` removes activation blocks
- `version.go` — prints version

**Internal packages in `internal/`:**
- `bedrock/` — loads `bedrock-config.json` into typed structs. `LoadBytes()` is used by cmd (embedded); `Load(path)` is the filesystem fallback for tests.
- `schema/` — builds and validates the `Block`; derives native settings.json keys via `NativeKeys()`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`. `CLAUDE_CODE_ENABLE_AUTO_MODE=1` is auto-set when `PermissionMode=="auto"`.
- `config/` — atomic read/merge/write of settings.json; backup rotation (5 most recent); file locking via `gofrs/flock`. `MergeJuggernautBlock(block, nativeEnv, nativeKeys)` owns all Juggernaut-managed top-level keys.
- `keychain/` — credential storage behind the `Backend` interface (`Set`/`Get`/`Delete`). `Resolve(mode, home)` selects: `Store` (OS keyring via `go-keyring`, service `juggernaut-bedrock`, default), `ProfileBackend` (plaintext file), or `DPAPIBackend` (Windows DPAPI file; build-tagged `dpapi_windows.go`/`dpapi_other.go`, errors off Windows). `ClearOthers(selected, home)` wipes the non-selected backends (best-effort: probes each via `Get` and skips unreachable ones so a missing keyring daemon isn't fatal). `MigrateInto(target, home)` imports a v3-era key (Windows Credential Manager bare-target UTF-16 entry, DPAPI file, or profile file) when the target is empty, then removes the source. `Set` maps go-keyring's oversize error to `ErrCredentialTooBig`.
- `activation/` — manages shell profile marker blocks, implements `juggernaut launch` (reads the configured storage backend at runtime to inject `AWS_BEARER_TOKEN_BEDROCK`), resolves the real Anthropic `claude` binary while avoiding recursion, recovers only positively identified v4.2.6 launcher artifacts, and detects stale v3 installs via `DetectV3Install` (`v3detect.go`).
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`
- `authmode/` — constants/helpers for auth mode strings (`IAM`, `BedrockAPIKey`). Use `authmode.BedrockAPIKey` (split var) instead of bare string literals to avoid static secret scanner hits.
- `safepath/` — path containment checks (`JoinUnder`, `withinBase`) and owner-only filesystem helpers (`MkdirAll`, `ReadFile`, `WriteFile` at `0o700`/`0o600`). Use these whenever writing files under a user-controlled base path.

**Activation mode:** shell profiles contain `# BEGIN: Juggernaut Claude Activation` / `# END: Juggernaut Claude Activation` blocks defining `claude` as a function that delegates to `juggernaut launch -- "$@"` (or shell equivalent). Juggernaut must never install, overwrite, move, symlink over, or delete an unknown file named `claude`.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary. Changing it requires rebuilding. Tests fall back to filesystem resolution via `findBedrockConfigFile()`.
- **Managed outputs:** settings.json plus marked shell activation blocks only.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default) vs `--scope=project`.
- **Re-apply preserves existing auth mode, permission mode, and storage backend** — when the corresponding flag is omitted and a block already exists, the value is read back from the block. Storage preservation is load-bearing: without it a bare re-apply would reset `--storage` to `keychain` and `ClearOthers` would wipe a working `profile`/`dpapi` credential.
- **`--model` overrides all three model IDs** (opus, sonnet, haiku) when set; `--opus-model`, `--sonnet-model`, `--haiku-model` override individually.
- **`--storage`:** credential storage backend — `keychain` (default), `dpapi` (Windows-only), or `profile` (plaintext file). Resolved once and threaded through every credential path (store on apply, read on `--preserve-key`, runtime read in `launch`, doctor, uninstall); the chosen value is persisted in `auth.storage` so `launch`/`doctor`/`uninstall` read from the right backend. `profile`/`dpapi` paths and formats are v3-compatible so v3 keys migrate for free.
- **`--no-1m-context`:** disables 1M token context window (on by default). `--1m-context` is a hidden no-op kept for script compatibility.
- **Mantle opt-in:** standard Bedrock preserves inference profile IDs by default; use `--mantle` or `--mantle-url` only when explicitly routing through Mantle.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.
- **`--mode`:** sets `permissions.defaultMode` in settings.json. When `auto`, also writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` in env (required for Bedrock; without it auto mode silently does nothing).
- **`--always-thinking`:** writes `alwaysThinkingEnabled: true` as a native settings.json key.
- **`--service-tier`:** writes `ANTHROPIC_BEDROCK_SERVICE_TIER` env var; values: `default`, `flex`, `priority`.
- **`skipWebFetchPreflight`:** always written as `true` for all Bedrock users (avoids domain safety preflight delays).
- **`effortLevel`:** written as both `CLAUDE_CODE_EFFORT_LEVEL` env var (legacy) and native `effortLevel` settings.json key (preferred). Five valid levels: `low`, `medium`, `high`, `xhigh` (default), `max`.
- **Native keys managed by Juggernaut:** `env`, `model`, `modelOverrides`, `effortLevel`, `alwaysThinkingEnabled`, `skipWebFetchPreflight`, `permissions`. All are removed on uninstall.

## Version Management

Version must stay in sync across **three** locations: `VERSION`, `bedrock-config.json` (`.version`), and `var Version` in `cmd/root.go`. CI enforces this — a mismatch fails the lint job.

## Testing

- Standard Go `testing` package. Run with `go test ./... -v` or `make test`.
- Keychain tests skip gracefully when the backend is unavailable — detected via a `skipIfUnavailable()` probe in `keychain_test.go`.
- **Isolate every credential side-channel** in any test that runs `apply`/`uninstall` with `bedrock-api-key`. `ClearOthers` and `MigrateInto` will otherwise read/delete the developer's real machine credentials. Use the `cmd` helper `isolateCredentialEnv(t, home)`, which sets `JUGGERNAUT_KEYCHAIN_SERVICE` (keychain/CredMan), `JUGGERNAUT_HOME` (DPAPI file), and `JUGGERNAUT_PROFILE_TOKEN_PATH` (profile file) to test-scoped locations. The `profile` backend makes most credential behavior testable without a real keyring.
- Tests needing a clean home use `t.TempDir()` with `t.Setenv("HOME", ...)`.
- There is intentionally no `migrate` subcommand — migration is implicit on `apply` and `TestMigrateCommandIsNotRegistered` guards against re-adding one.
- Cobra global flag state leaks between `ExecuteArgs()` calls — `resetFlags()` in `cmd/root.go` resets all subcommand flags to defaults before each invocation.

## CI / Release

- CI (`ci.yml`): lint + version sync check → `go test ./... -v` on ubuntu/macos/windows. Actions pinned to full commit SHAs.
- Release (`release.yml`): triggered on `v*` tags → GoReleaser builds binaries → npm OIDC publish to `juggernaut-bedrock`.
- GoReleaser requires a clean git tree — never commit build artifacts (`bin/` is gitignored).
- npm package is in `npm/` — optional platform sub-packages ship prebuilt binaries; `npm/index.js` resolves and execs the matching package.

## Codacy

`.codacy/tools-configs/` holds generated configs for revive, eslint, trivy, etc. created by `codacy-cli init`. Run `codacy-cli analyze` in WSL2 to check locally. The `.codacy.yml` at root excludes `.remember/` and `.codacy/` from analysis.

PRs run **two separate Codacy checks**: "Codacy Local Analysis" (the CLI, mirrored by `make codacy`) and "Codacy Static Code Analysis" (the server, runs Opengrep/Semgrep, gates on **0 new issues**). They use different engines, so passing one does not guarantee the other. Suppress confirmed false positives with `// nosemgrep: <rule-id>, <rule-id> -- reason` on the **same physical line** as the finding — the server honors `nosemgrep`, not gosec's `#nosec`. For `unsafe.Pointer` syscall args, annotate each argument line individually. Prefer routing user-path file I/O through `internal/safepath` (it centralizes `os.ReadFile`/`WriteFile`/`MkdirAll` so the call site isn't flagged) over annotating.

`golangci-lint` may refuse to run locally if its build Go version is older than the repo's `go.mod` toolchain (`go vet` and `gofmt -l` still work) — rely on the CI `lint` job. Keep a build-tagged helper in the same file as its only caller, or the `unused` linter flags it on the platform that doesn't reference it.
