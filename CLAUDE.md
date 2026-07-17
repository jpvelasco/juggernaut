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
  - `codex` — TOML `~/.codex/config.toml`, routed via built-in `amazon-bedrock` provider with `[model_providers.amazon-bedrock.aws]` for region
  - `opencode` — JSON `~/.config/opencode/opencode.json`, routed via a custom OpenAI-compatible provider block (`@ai-sdk/openai-compatible`) since OpenCode is model-agnostic
  - `grok` — TOML `~/.grok/config.toml` (always user-scoped; Grok has no project config), routed via a `[model.bedrock-grok]` block plus an `[auth]` block whose `auth_provider_command` runs `juggernaut auth-token` — a plain `env_key` does not suppress Grok's interactive login, only `auth_provider_command` does

  Adding a CLI = implement the `Provider` interface + call `register()` in that `init()`; nothing in `cmd/` hardcodes per-CLI logic (Claude-only flags gate on `provider.Supports(Capability)`). Blocks for different CLIs coexist in one shell profile. The Bedrock bearer token is **shared** across CLIs — `uninstall --cli=codex` never removes it. Design contract: `.research/provider-interface-design.md`; verified per-model Mantle facts: `.research/multi-harness-bedrock-research.md`.

  Some providers own nested config tables where Juggernaut must merge rather than replace (e.g. Grok's `[model.<name>]`, Codex's `[model_providers.<id>]`, OpenCode's `provider.<id>`) so a user's sibling entries survive. `Provider.DeepMergeKeys()` lists which managed keys need this, and `Provider.OwnedSubKeys()` maps each to the exact sub-keys Juggernaut owns — uninstall removes only those, not the whole table.

**Internal packages in `internal/`:**
- `provider/` — the `Provider` interface + registry (`Get(name)`), and per-CLI impls (`claude.go`, `codex.go`, `opencode.go`, `grok.go`). `claude.BuildConfig` wraps `schema.Build` (byte-identical, guarded by the `cmd` golden test). `region.go` holds the shared Mantle region-enforcement policy (nicknamed the "Iron Fist" in comments): if a model isn't verified available in the requested region, Juggernaut overrides to a known-good region rather than writing a config that can't authenticate.
- `bedrock/` — loads `bedrock-config.json` into typed structs. `LoadBytes()` is used by cmd (embedded); `Load(path)` is the filesystem fallback for tests.
- `schema/` — builds and validates the Claude `Block`; derives native settings.json keys via `NativeKeys()`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`. `CLAUDE_CODE_ENABLE_AUTO_MODE=1` is auto-set when `PermissionMode=="auto"`.
- `config/` — atomic read/merge/write of config files (JSON or TOML via the `ConfigFormat` interface + `FormatByName`); backup rotation (5 most recent); file locking via `gofrs/flock`. `MergeConfigPlan`/`MergeConfigPlanDeep` and `RemoveManagedKeys`/`RemoveManagedKeysDeep` are the generic provider-driven merge/remove paths (deep variants respect `DeepMergeKeys`/`OwnedSubKeys`); `MergeJuggernautBlock`/`RemoveJuggernautBlock` remain as Claude-shaped wrappers over the same underlying `applyManagedKey`/`RemoveManagedKeys` helpers. `DetectCollisions` (`collision.go`) is the foreign-config guard: given an existing config and a provider's plan, it walks only the exact leaves the provider is about to write (using the same `DeepMergeKeys`/`OwnedSubKeys` metadata, plus a `permissions.defaultMode` special case) and reports any that already hold a foreign value — sibling entries in the same table never trigger it.
- `keychain/` — cross-platform credential storage via `go-keyring`. Service name: `juggernaut-bedrock`. The stored bearer token is shared across all configured CLIs.
- `activation/` — manages shell profile marker blocks (per-CLI via `CLISpec`/`blockFor`, coexisting in one profile), implements the hidden `launch`/`launch-cli` commands, resolves the real target binary (`resolveBinary`) while avoiding recursion, and recovers only positively identified v4.2.6 launcher artifacts (Claude-only).
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`. `cmd/doctor.go`'s `checkAutoModeReadiness` inspects the persisted `juggernaut` block per scope and reports auto-mode readiness (OK/WARN) by calling `schema.Block.AutoModeUsable()`/`AutoModeAvailable()` directly — silent unless `permissionMode=="auto"` is configured.
- `authmode/` — constants/helpers for auth mode strings (`IAM`, `BedrockAPIKey`). Use `authmode.BedrockAPIKey` (split var) instead of bare string literals to avoid static secret scanner hits.
- `safepath/` — path containment checks (`JoinUnder`, `withinBase`) and owner-only filesystem helpers (`MkdirAll`, `ReadFile`, `WriteFile` at `0o700`/`0o600`). Use these whenever writing files under a user-controlled base path.

**Activation mode:** shell profiles contain per-CLI marker blocks (e.g. `# BEGIN: Juggernaut Claude Activation` / `# END: Juggernaut Claude Activation`) defining a shell function that delegates to `juggernaut launch -- "$@"` (Claude) or `juggernaut launch-cli <cli> -- "$@"` (others), or shell equivalent. Wrappers fall through to the real CLI binary when `juggernaut` is not on PATH. Apply installs PowerShell AllHosts profiles only and strips stale host-specific blocks for that CLI; multi-CLI uninstall also scans historical OneDrive/local Documents paths. Juggernaut must never install, overwrite, move, symlink over, or delete an unknown file matching a managed CLI's binary name.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary. Changing it requires rebuilding. Tests fall back to filesystem resolution via `findBedrockConfigFile()`.
- **Managed outputs:** provider config file plus marked shell activation blocks only.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default) vs `--scope=project`. Grok is always user-scoped — it has no project config concept.
- **Re-apply preserves existing auth mode** — if `--auth` is omitted and a block already exists, the existing auth mode is read from the block.
- **`--model` overrides all four model IDs** (opus, sonnet, haiku, fable) when set; `--opus-model`, `--sonnet-model`, `--haiku-model`, `--fable-model` override individually. `models.fable` defaults to `global.anthropic.claude-fable-5` in the embedded config, verified live against AWS Bedrock's `ListFoundationModels`/`ListInferenceProfiles` APIs (closes #206).
- **Fable data-retention warning:** Anthropic requires opting in to `provider_data_share` before Fable calls succeed on Bedrock (verified against AWS's abuse-detection docs); there is no AWS API to read an account's actual opt-in status, so Juggernaut cannot check it. `schema.FableDataRetentionWarning` (emitted by `schema.Build` into `Block.Warnings` whenever `schema.IsFable5Model` matches the resolved Fable ID) is the single source of truth, surfaced two ways: once at apply time via `ConfigPlan.Warnings` (same mechanism as the Mantle/auto-mode warnings) and on every `doctor` run via `checkFableDataRetention` (re-runnable since the apply-time note scrolls away). Since Fable is pinned by default, this warns on every apply/doctor run unless a maintainer ships a config without it — it never claims Juggernaut knows what is or isn't collected, only what AWS documents.
- **`juggernaut models check`:** a maintainer-facing, release-preparation command (NOT run implicitly by `apply`/`doctor`) that queries AWS Bedrock's live foundation-model and inference-profile catalogs (`internal/discovery/`, the only package that imports `aws-sdk-go-v2`) and reports whether each pinned tier (`opus`/`sonnet`/`haiku`/`fable`) in `bedrock-config.json` is still `ACTIVE`. Exits non-zero if any tier is `LEGACY` (CI/script-friendly pre-release gate). `--write --set-<tier>=<id>` re-verifies the chosen ID is `ACTIVE` before pinning it — never auto-picks among multiple `ACTIVE` candidates, and `--write` alone (no `--set-*`) is a hard error, not a no-op success. No deprecation countdown: AWS's `endOfLifeTime`/`legacyTime` fields are not populated in practice, so this only ever reports binary `ACTIVE`/`LEGACY`.
- **Credential storage:** always the OS keychain via `go-keyring`. Service name: `juggernaut-bedrock`.
- **`--no-1m-context`:** disables 1M token context window (on by default). `--1m-context` is a hidden no-op kept for script compatibility.
- **Mantle opt-in:** standard Bedrock preserves inference profile IDs by default; use `--mantle` or `--mantle-url` only when explicitly routing through Mantle. Non-Claude providers (Codex, OpenCode, Grok) route through Mantle unconditionally since Bedrock's native Claude-only API doesn't serve other model families.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.
- **`--mode`:** sets `permissions.defaultMode` in settings.json. When `auto`, also writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` in env (required for Bedrock). Auto mode on Bedrock additionally requires the **active session model to be Opus 4.7/4.8** (Claude Code v2.1.158+); Sonnet/Haiku are unsupported and Claude Code hides auto from the Shift+Tab cycle. Because Juggernaut's default model is Sonnet, `apply --mode=auto` prints a warning (via `schema.Block.AutoModeUsable()` / `schema.IsAutoModeCapableModel()`) unless the resolved model is Opus. Users unlock it by running Claude Code on Opus (`claude --model opus` or `/model opus`).
- **`--always-thinking`:** writes `alwaysThinkingEnabled: true` as a native settings.json key.
- **`--fallback-model=a,b`:** writes Claude Code native `fallbackModel` as an ordered array. Empty entries are rejected; an empty resolved chain removes the managed key.
- **`--available-models=a,b` / `--enforce-available-models`:** writes Claude Code native `availableModels` (ordered array, entries are model families/version prefixes/full IDs — Juggernaut validates only shape, never which values Claude Code accepts) and `enforceAvailableModels` (bool). Empty entries in `--available-models` are rejected; `--enforce-available-models` requires a non-empty resolved list or apply errors before writing anything, since Claude Code documents the key as a no-op otherwise. Both keys follow `--fallback-model`'s re-apply convention — omitting either flag on a re-apply removes it, no preservation. **Not tamper-resistant governance:** Juggernaut writes these into user/project scope (`~/.claude/settings.json`), which Claude Code's own docs say gets concatenated with other non-managed sources — a user can still add their own `availableModels` entry in a different scope and reintroduce excluded models. This is personal picker curation, not an enforcement boundary; real organizational enforcement requires deploying `availableModels`/`enforceAvailableModels` in Claude Code's OS-level managed settings (e.g. `/etc/claude-code/managed-settings.json`), which Juggernaut does not write to.
- **`--service-tier`:** writes `ANTHROPIC_BEDROCK_SERVICE_TIER` env var; values: `default`, `flex`, `priority`.
- **`skipWebFetchPreflight`:** always written as `true` for all Bedrock users (avoids domain safety preflight delays).
- **`effortLevel`:** fixed persisted levels (`low`, `medium`, `high`, `xhigh`) are written as both `CLAUDE_CODE_EFFORT_LEVEL` and native `effortLevel`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` / `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut intentionally rejects it as `--effort`. Juggernaut defaults to `high`.
- **Native keys managed by Juggernaut (Claude):** `env`, `model`, `modelOverrides`, `fallbackModel`, `effortLevel`, `alwaysThinkingEnabled`, `skipWebFetchPreflight`, `permissions`. All are removed on uninstall. Other providers manage their own analogous key set via `NativeManagedKeys()`/`DeepMergeKeys()`/`OwnedSubKeys()`.
- **Foreign-config collision detection ("Juggernaut law"):** `apply` refuses to touch a config file it doesn't already own (`Provider.OwnsConfig()` false) if any leaf it's about to write already holds a value — even one leaf collides, the whole apply is refused, not just that key. Detection is per-leaf, not per-file: a user's own sibling entries in the same table (their own model profile, their own env var, their own permission rule) never trigger it — only the exact key/sub-key Juggernaut owns. `--force` bypasses the refusal (a backup is still made either way). A config Juggernaut already owns (re-apply) is exempt — zero new friction. This applies uniformly across all providers via `internal/config.DetectCollisions`, driven entirely by each provider's existing `DeepMergeKeys()`/`OwnedSubKeys()` — a new CLI provider gets this for free with no bespoke collision code, as long as it declares its owned keys honestly (required anyway for correct merge/uninstall).

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
