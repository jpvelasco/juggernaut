# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform Go CLI tool that configures coding CLIs (Claude Code, Codex, OpenCode, Grok) to route through Amazon Bedrock instead of each vendor's direct API. It writes provider-specific config (JSON or TOML) to each CLI's config path — user scope (e.g. `~/.claude/settings.json`) or project scope (e.g. `./.claude/settings.json`) — and installs marked shell activation blocks per CLI. The binary is self-contained — `bedrock-config.json` is embedded at build time via `//go:embed`.

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

# Dry-run apply (preview config write without committing)
./bin/juggernaut apply --auth=iam --dry-run

# Install git hooks (pre-commit, conventional commits)
scripts/setup-hooks.ps1    # Windows
bash scripts/setup-hooks.sh  # Linux/macOS

# Check Codacy dashboard issues (requires @codacy/codacy-cloud-cli + CODACY_API_TOKEN)
make codacy
```

## Architecture

Single Go binary. Entry point: `main.go` → `cmd/` → `internal/`.

**`main.go`** embeds `bedrock-config.json` via `//go:embed` and passes the bytes to `cmd.SetEmbeddedConfig()` before calling `cmd.Execute()`.

**Shared helpers in `cmd/helpers.go`:** `homeDir()`, `settingsPath()`, `loadBedrockConfig()`, `toMap()`, `fileExists()`. All command files in `cmd/` use these — don't duplicate them.

**Subcommands in `cmd/`:**
- `apply.go` — builds and writes the Juggernaut block to the target CLI's config; installs shell activation; safely recovers known broken v4.2.6 launcher artifacts (Claude only)
- `launch.go` — hidden command used by shell activation; injects Bedrock runtime env and runs the real underlying CLI binary
- `auth_token.go` — hidden command (`juggernaut auth-token`) some CLIs invoke as an external auth provider to fetch the Bedrock bearer token from the keychain at session start. Two output shapes via `--format`: `json` (`{"access_token":...,"expires_in":N}`, consumed by Grok's `auth_provider_command`) and `token` (bare token on one line, consumed by Codex)
- `show.go` — prints current config from settings.json
- `doctor.go` — read-only diagnostics (settings, credentials, activation, underlying CLI binary, v4.2.6 artifacts)
- `uninstall.go` — removes the Juggernaut-managed keys/block from config and deletes the bearer token; `--full` removes activation blocks
- `version.go` — prints version

**Multi-CLI provider abstraction:** Juggernaut configures multiple coding CLIs for Bedrock, selected with `--cli` (default `claude`) on `apply`/`uninstall`. Activation blocks use the hidden `launch-cli <cli> -- …` command for non-Claude CLIs; the distinct command name is deliberate so a pre-multi-CLI binary fails instead of silently launching Claude. The historical `launch [cli] -- …` form remains accepted for compatibility. Each CLI is a `provider.Provider` (`internal/provider/`) owning its identity, binary names, config path + format, native managed keys, activation markers, capabilities, and the two-phase output: `BuildConfig(cfg, opts) → ConfigPlan` (what to persist at apply time) and `LaunchSpec() → {TokenEnvVar, StaticEnv, NeedsToken}` (what the wrapper injects at runtime). Supported today, registered in `internal/provider/provider.go`'s `init()`:
  - `claude` — JSON `~/.claude/settings.json`
  - `codex` — TOML `~/.codex/config.toml`, routed to Bedrock Mantle via a `[model_providers.bedrock-mantle]` block with per-model `base_url`/`wire_api`
  - `opencode` — JSON `~/.config/opencode/opencode.json`, routed via a custom OpenAI-compatible provider block (`@ai-sdk/openai-compatible`) since OpenCode is model-agnostic
  - `grok` — TOML `~/.grok/config.toml` (always user-scoped; Grok has no project config), routed via a `[model.bedrock-grok]` block plus an `[auth]` block whose `auth_provider_command` runs `juggernaut auth-token` — a plain `env_key` does not suppress Grok's interactive login, only `auth_provider_command` does

  Adding a CLI = implement the `Provider` interface + call `register()` in that `init()`; nothing in `cmd/` hardcodes per-CLI logic (Claude-only flags gate on `provider.Supports(Capability)`). Blocks for different CLIs coexist in one shell profile. The Bedrock bearer token is **shared** across CLIs — `uninstall --cli=codex` never removes it. Design contract: `.research/provider-interface-design.md`; verified per-model Mantle facts: `.research/multi-harness-bedrock-research.md`.

  Some providers own nested config tables where Juggernaut must merge rather than replace (e.g. Grok's `[model.<name>]`, Codex's `[model_providers.<id>]`, OpenCode's `provider.<id>`) so a user's sibling entries survive. `Provider.DeepMergeKeys()` lists which managed keys need this, and `Provider.OwnedSubKeys()` maps each to the exact sub-keys Juggernaut owns — uninstall removes only those, not the whole table.

**Internal packages in `internal/`:**
- `provider/` — the `Provider` interface + registry (`Get(name)`), and per-CLI impls (`claude.go`, `codex.go`, `opencode.go`, `grok.go`). `claude.BuildConfig` wraps `schema.Build` (byte-identical, guarded by the `cmd` golden test). `region.go` holds the shared Mantle region-enforcement policy (nicknamed the "Iron Fist" in comments): if a model isn't verified available in the requested region, Juggernaut overrides to a known-good region rather than writing a config that can't authenticate.
- `bedrock/` — loads `bedrock-config.json` into typed structs. `LoadBytes()` is used by cmd (embedded); `Load(path)` is the filesystem fallback for tests.
- `schema/` — builds and validates the Claude `Block`; derives native settings.json keys via `NativeKeys()`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`. `CLAUDE_CODE_ENABLE_AUTO_MODE=1` is auto-set when `PermissionMode=="auto"`.
- `config/` — atomic read/merge/write of config files (JSON or TOML via the `ConfigFormat` interface + `FormatByName`); backup rotation (5 most recent); file locking via `gofrs/flock`. `MergeConfigPlan`/`MergeConfigPlanDeep` and `RemoveManagedKeys`/`RemoveManagedKeysDeep` are the generic provider-driven merge/remove paths (deep variants respect `DeepMergeKeys`/`OwnedSubKeys`); `MergeJuggernautBlock`/`RemoveJuggernautBlock` remain as Claude-shaped wrappers over the same underlying `applyManagedKey`/`RemoveManagedKeys` helpers.
- `keychain/` — cross-platform credential storage via `go-keyring`. Service name: `juggernaut-bedrock`. The stored bearer token is shared across all configured CLIs.
- `activation/` — manages shell profile marker blocks (per-CLI via `CLISpec`/`blockFor`, coexisting in one profile), implements the hidden `launch`/`launch-cli` commands, resolves the real target binary (`resolveBinary`) while avoiding recursion, and recovers only positively identified v4.2.6 launcher artifacts (Claude-only).
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`
- `authmode/` — constants/helpers for auth mode strings (`IAM`, `BedrockAPIKey`). Use `authmode.BedrockAPIKey` (split var) instead of bare string literals to avoid static secret scanner hits.
- `safepath/` — path containment checks (`JoinUnder`, `withinBase`) and owner-only filesystem helpers (`MkdirAll`, `ReadFile`, `WriteFile` at `0o700`/`0o600`). Use these whenever writing files under a user-controlled base path.

**Activation mode:** shell profiles contain per-CLI marker blocks (e.g. `# BEGIN: Juggernaut Claude Activation` / `# END: Juggernaut Claude Activation`) defining a shell function that delegates to `juggernaut launch -- "$@"` (Claude) or `juggernaut launch-cli <cli> -- "$@"` (others), or shell equivalent. Juggernaut must never install, overwrite, move, symlink over, or delete an unknown file matching a managed CLI's binary name.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary. Changing it requires rebuilding. Tests fall back to filesystem resolution via `findBedrockConfigFile()`.
- **Managed outputs:** provider config file plus marked shell activation blocks only.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default) vs `--scope=project`. Grok is always user-scoped — it has no project config concept.
- **Re-apply preserves existing auth mode** — if `--auth` is omitted and a block already exists, the existing auth mode is read from the block.
- **`--model` overrides all four model IDs** (opus, sonnet, haiku, fable) when set; `--opus-model`, `--sonnet-model`, `--haiku-model`, `--fable-model` override individually. The embedded default config keeps `models.fable` empty until a Bedrock-accessible Fable ID is configured.
- **Credential storage:** always the OS keychain via `go-keyring`. Service name: `juggernaut-bedrock`.
- **`--no-1m-context`:** disables 1M token context window (on by default). `--1m-context` is a hidden no-op kept for script compatibility.
- **Mantle opt-in:** standard Bedrock preserves inference profile IDs by default; use `--mantle` or `--mantle-url` only when explicitly routing through Mantle. Non-Claude providers (Codex, OpenCode, Grok) route through Mantle unconditionally since Bedrock's native Claude-only API doesn't serve other model families.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.
- **`--mode`:** sets `permissions.defaultMode` in settings.json. When `auto`, also writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` in env (required for Bedrock). Auto mode on Bedrock additionally requires the **active session model to be Opus 4.7/4.8** (Claude Code v2.1.158+); Sonnet/Haiku are unsupported and Claude Code hides auto from the Shift+Tab cycle. Because Juggernaut's default model is Sonnet, `apply --mode=auto` prints a warning (via `schema.Block.AutoModeUsable()` / `schema.IsAutoModeCapableModel()`) unless the resolved model is Opus. Users unlock it by running Claude Code on Opus (`claude --model opus` or `/model opus`).
- **`--always-thinking`:** writes `alwaysThinkingEnabled: true` as a native settings.json key.
- **`--fallback-model=a,b`:** writes Claude Code native `fallbackModel` as an ordered array. Empty entries are rejected; an empty resolved chain removes the managed key.
- **`--service-tier`:** writes `ANTHROPIC_BEDROCK_SERVICE_TIER` env var; values: `default`, `flex`, `priority`.
- **`skipWebFetchPreflight`:** always written as `true` for all Bedrock users (avoids domain safety preflight delays).
- **`effortLevel`:** fixed persisted levels (`low`, `medium`, `high`, `xhigh`) are written as both `CLAUDE_CODE_EFFORT_LEVEL` and native `effortLevel`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` / `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut intentionally rejects it as `--effort`. Juggernaut defaults to `high`.
- **Native keys managed by Juggernaut (Claude):** `env`, `model`, `modelOverrides`, `fallbackModel`, `effortLevel`, `alwaysThinkingEnabled`, `skipWebFetchPreflight`, `permissions`. All are removed on uninstall. Other providers manage their own analogous key set via `NativeManagedKeys()`/`DeepMergeKeys()`/`OwnedSubKeys()`.

## Version Management

Version must stay in sync across **three** locations: `VERSION`, `bedrock-config.json` (`.version`), and `var Version` in `cmd/root.go`. CI enforces this — a mismatch fails the lint job.

## Git Branches

**Protected branches:** `legacy/v3` must never be deleted — it is required for
older releases. When cleaning up branches after merges, skip any branch named
`legacy/*`.

## Testing

- Standard Go `testing` package. Run with `go test ./... -v` or `make test`.
- Keychain tests skip gracefully when the backend is unavailable — detected via a `skipIfUnavailable()` probe in `keychain_test.go`.
- To isolate keychain: set `JUGGERNAUT_KEYCHAIN_SERVICE` env var.
- Tests needing a clean home use `t.TempDir()` with `t.Setenv("HOME", ...)`.
- Cobra global flag state leaks between `ExecuteArgs()` calls — `resetFlags()` in `cmd/root.go` resets all subcommand flags to defaults before each invocation.

## CI / Release

- CI (`ci.yml`): lint job (golangci-lint + version sync check + GoReleaser draft check) → `go test ./... -v -race`-covered on ubuntu/macos/windows with per-OS Codecov upload via OIDC (merged for true cross-platform coverage, since much of the PowerShell/launcher/keychain code is Windows-only) → separate race-detector job → separate Linux coverage-threshold job (60% floor, JUnit uploaded to Codecov Test Analytics) → npm tests → shellcheck → gosec. Actions pinned to full commit SHAs.
- CodeQL (`codeql.yml`): separate scheduled + push/PR workflow analyzing `actions`, `go`, and `javascript-typescript` via `.github/codeql/codeql-config.yml`.
- Octopus Review (`octopus.yml`): posts automated review comments on `pull_request_target` opened/synchronize.
- Release (`release.yml`): triggered on `v*` tags → GoReleaser builds and **publishes** the GitHub release (`draft: false` in `.goreleaser.yml`; CI fails on `draft: true`) → verifies the release is non-draft with assets before npm → npm OIDC publish to `juggernaut-bedrock`.
- GoReleaser requires a clean git tree — never commit build artifacts (`bin/` is gitignored).
- npm package is in `npm/` — optional platform sub-packages ship prebuilt binaries; `npm/index.js` resolves and execs the matching package.

## Codacy

Codacy cloud analysis runs on every push/PR via the Codacy GitHub integration — `Codacy Static Code Analysis` is a required status check on `main`. Use `make codacy` to check dashboard issues locally (requires `@codacy/codacy-cloud-cli` + `CODACY_API_TOKEN`). The `.codacy/` directory holds synced tool configs (eslint, opengrep, pmd, trivy) and `.codacy.yml` excludes `.remember/`, `.codacy/`, and `mcps/` from analysis (`npm/` is additionally excluded from the eslint engine).
