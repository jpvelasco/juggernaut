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

# Run tests with race detector
make test-race

# Run tests with coverage
make test-cover

# Run a single package
go test ./internal/schema/... -v

# Run a single test
go test ./cmd/... -run TestApply_WritesSettings_IAM -v

# Full pre-push check (tidy, fmt, vet, lint, test)
make ci

# Lint
make lint

# Format + vet
make fmt vet

# Dry-run apply (preview settings.json write without committing)
./bin/juggernaut apply --auth=iam --dry-run

# Install git hooks (pre-commit, conventional commits)
scripts/setup-hooks.ps1    # Windows
bash scripts/setup-hooks.sh  # Linux/macOS

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
- `doctor.go` — read-only diagnostics (settings, credentials, activation, Claude Code binary, v4.2.6 artifacts)
- `uninstall.go` — removes the Juggernaut block from settings.json and deletes bearer token; `--full` removes activation blocks
- `version.go` — prints version

**Internal packages in `internal/`:**
- `bedrock/` — loads `bedrock-config.json` into typed structs. `LoadBytes()` is used by cmd (embedded); `Load(path)` is the filesystem fallback for tests.
- `schema/` — builds and validates the `Block`; derives native settings.json keys via `NativeKeys()`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`. `CLAUDE_CODE_ENABLE_AUTO_MODE=1` is auto-set when `PermissionMode=="auto"`.
- `config/` — atomic read/merge/write of settings.json; backup rotation (5 most recent); file locking via `gofrs/flock`. `MergeJuggernautBlock(block, nativeEnv, nativeKeys)` owns all Juggernaut-managed top-level keys.
- `keychain/` — cross-platform credential storage via `go-keyring`. Service name: `juggernaut-bedrock`.
- `activation/` — manages shell profile marker blocks, implements `juggernaut launch`, resolves the real Anthropic `claude` binary while avoiding recursion, and recovers only positively identified v4.2.6 launcher artifacts.
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`
- `authmode/` — constants/helpers for auth mode strings (`IAM`, `BedrockAPIKey`). Use `authmode.BedrockAPIKey` (split var) instead of bare string literals to avoid static secret scanner hits.
- `safepath/` — path containment checks (`JoinUnder`, `withinBase`) and owner-only filesystem helpers (`MkdirAll`, `ReadFile`, `WriteFile` at `0o700`/`0o600`). Use these whenever writing files under a user-controlled base path.

**Activation mode:** shell profiles contain `# BEGIN: Juggernaut Claude Activation` / `# END: Juggernaut Claude Activation` blocks defining `claude` as a function that delegates to `juggernaut launch -- "$@"` (or shell equivalent). Juggernaut must never install, overwrite, move, symlink over, or delete an unknown file named `claude`.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary. Changing it requires rebuilding. Tests fall back to filesystem resolution via `findBedrockConfigFile()`.
- **Managed outputs:** settings.json plus marked shell activation blocks only.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default) vs `--scope=project`.
- **Re-apply preserves existing auth mode** — if `--auth` is omitted and a block already exists, the existing auth mode is read from the block.
- **`--model` overrides all four model IDs** (opus, sonnet, haiku, fable) when set; `--opus-model`, `--sonnet-model`, `--haiku-model`, `--fable-model` override individually. The embedded default config keeps `models.fable` empty until a Bedrock-accessible Fable ID is configured.
- **Credential storage:** always the OS keychain via `go-keyring`. Service name: `juggernaut-bedrock`.
- **`--no-1m-context`:** disables 1M token context window (on by default). `--1m-context` is a hidden no-op kept for script compatibility.
- **Mantle opt-in:** standard Bedrock preserves inference profile IDs by default; use `--mantle` or `--mantle-url` only when explicitly routing through Mantle.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.
- **`--mode`:** sets `permissions.defaultMode` in settings.json. When `auto`, also writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` in env (required for Bedrock). Auto mode on Bedrock additionally requires the **active session model to be Opus 4.7/4.8** (Claude Code v2.1.158+); Sonnet/Haiku are unsupported and Claude Code hides auto from the Shift+Tab cycle. Because Juggernaut's default model is Sonnet, `apply --mode=auto` prints a warning (via `schema.Block.AutoModeUsable()` / `schema.IsAutoModeCapableModel()`) unless the resolved model is Opus. Users unlock it by running Claude Code on Opus (`claude --model opus` or `/model opus`).
- **`--always-thinking`:** writes `alwaysThinkingEnabled: true` as a native settings.json key.
- **`--fallback-model=a,b`:** writes Claude Code native `fallbackModel` as an ordered array. Empty entries are rejected; an empty resolved chain removes the managed key.
- **`--service-tier`:** writes `ANTHROPIC_BEDROCK_SERVICE_TIER` env var; values: `default`, `flex`, `priority`.
- **`skipWebFetchPreflight`:** always written as `true` for all Bedrock users (avoids domain safety preflight delays).
- **`effortLevel`:** fixed persisted levels (`low`, `medium`, `high`, `xhigh`) are written as both `CLAUDE_CODE_EFFORT_LEVEL` and native `effortLevel`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` / `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut intentionally rejects it as `--effort`. Juggernaut defaults to `high`.
- **Native keys managed by Juggernaut:** `env`, `model`, `modelOverrides`, `fallbackModel`, `effortLevel`, `alwaysThinkingEnabled`, `skipWebFetchPreflight`, `permissions`. All are removed on uninstall.

## Version Management

Version must stay in sync across **three** locations: `VERSION`, `bedrock-config.json` (`.version`), and `var Version` in `cmd/root.go`. CI enforces this — a mismatch fails the lint job.

## Testing

- Standard Go `testing` package. Run with `go test ./... -v` or `make test`.
- Keychain tests skip gracefully when the backend is unavailable — detected via a `skipIfUnavailable()` probe in `keychain_test.go`.
- To isolate keychain: set `JUGGERNAUT_KEYCHAIN_SERVICE` env var.
- Tests needing a clean home use `t.TempDir()` with `t.Setenv("HOME", ...)`.
- Cobra global flag state leaks between `ExecuteArgs()` calls — `resetFlags()` in `cmd/root.go` resets all subcommand flags to defaults before each invocation.

## CI / Release

- CI (`ci.yml`): lint + version sync check → `go test ./... -v` on ubuntu/macos/windows → race detector → coverage (52% threshold) → npm tests → shellcheck → gosec. Actions pinned to full commit SHAs. Go module cache is shared across jobs.
- Release (`release.yml`): triggered on `v*` tags → GoReleaser builds and **publishes** the GitHub release (`draft: false` in `.goreleaser.yml`; CI fails on `draft: true`) → verifies the release is non-draft with assets before npm → npm OIDC publish to `juggernaut-bedrock`.
- GoReleaser requires a clean git tree — never commit build artifacts (`bin/` is gitignored).
- npm package is in `npm/` — optional platform sub-packages ship prebuilt binaries; `npm/index.js` resolves and execs the matching package.

## Codacy

`.codacy/tools-configs/` holds generated configs for revive, eslint, trivy, etc. created by `codacy-cli init`. Run `codacy-cli analyze` in WSL2 to check locally. The `.codacy.yml` at root excludes `.remember/` and `.codacy/` from analysis.
