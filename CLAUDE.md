# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Juggernaut is a cross-platform Go CLI tool that configures coding CLIs (Claude Code, Codex, OpenCode, Grok) to route through Amazon Bedrock instead of each vendor's direct API. It writes provider-specific config (JSON or TOML) to each CLI's config path — user scope (e.g. `~/.claude/settings.json`) or project scope (e.g. `./.claude/settings.json`) — and installs marked shell activation blocks per CLI. The binary is self-contained — `bedrock-config.json` is embedded at build time via `//go:embed`.

**This is a public open-source repository** (`github.com/jpvelasco/juggernaut`, MIT, module path `github.com/jpvelasco/juggernaut/v5`). Distribution is npm-only (`juggernaut-bedrock`); `scripts/install.sh` and `scripts/install.ps1` are deprecated stubs that just print npm instructions. Consequences to keep in mind when editing:

- **Assume outside contributors and forks.** Community files are real inputs: `CONTRIBUTING.md`, `SECURITY.md` (private-disclosure policy + threat model), `CODE_OF_CONDUCT.md`, `.github/ISSUE_TEMPLATE/`, `.github/PULL_REQUEST_TEMPLATE.md`, `.github/CODEOWNERS` (`* @jpvelasco`), and `.github/GITHUB_SETTINGS.md` (the out-of-repo GitHub settings, re-appliable via `gh api`). Update the user-facing docs (`README.md`, `QUICKSTART.md`, `CONTRIBUTING.md`) when flags or behavior change.
- **CI is fork-aware.** Codecov upload steps are guarded by `github.event.pull_request.head.repo.full_name == github.repository`, so fork PRs run tests but skip coverage upload — a fork PR with no Codecov status is expected, not a failure to debug.
- **Never reference local-only paths as if a clone has them.** `.research/` (except two tracked files) and `docs/superpowers/` are gitignored working notes, as is `.claude/`. Anything a contributor must be able to read belongs in a tracked file.
- **`AGENTS.md` is the tracked, contributor-facing companion to this file.** It restates the commands, architecture, and engineering constraints in condensed form; when you change architecture or workflow here, keep `AGENTS.md` in sync.

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

# npm launcher tests (Node >= 20; CI runs Node 24)
cd npm && npm test

# Regenerate the Claude apply golden snapshot after an INTENTIONAL output change
UPDATE_GOLDEN=1 go test ./cmd/ -run Golden

# Install git hooks (pre-commit, commit-msg, pre-push) — REQUIRED once per clone,
# otherwise unformatted/untested code and non-Conventional-Commit messages can
# slip through locally (CI is the real gate either way)
scripts/setup-hooks.ps1    # Windows
bash scripts/setup-hooks.sh  # Linux/macOS

# Check Codacy dashboard issues (requires @codacy/codacy-cloud-cli + CODACY_API_TOKEN)
make codacy
```

## Git Hooks

Hooks live in `.githooks/` (tracked) and are installed into `.git/hooks/` via `scripts/setup-hooks.sh`/`.ps1` (see Commands above) — install once per clone.

- **pre-commit**: runs `gofmt -l` and `go vet` on staged Go files, then `go mod tidy` (fails if `go.mod`/`go.sum` change)
- **commit-msg**: enforces Conventional Commits (`feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert`)
- **pre-push**: fails if any changed non-test Go file has a function at 0.0% coverage (early catch; bypass with `git push --no-verify` in emergencies)

The hooks are a convenience — **CI is the real gate**: Codecov's `patch` status (target 80%, in `codecov.yml`) runs on every PR and cannot be bypassed locally.

## Architecture

Single Go binary. Entry point: `main.go` → `cmd/` → `internal/`.

**`main.go`** embeds `bedrock-config.json` via `//go:embed` and passes the bytes to `cmd.SetEmbeddedConfig()` before calling `cmd.Execute()`.

**Shared helpers in `cmd/helpers.go`:** `homeDir()`, `loadBedrockConfig()`, `toMap()`, `fileExists()`. All command files in `cmd/` use these — don't duplicate them. Config path resolution uses `provider.Get("<cli>").ConfigPath()` instead of a per-CLI helper.

**Subcommands in `cmd/`:**
- `apply.go` — builds and writes the Juggernaut block to the target CLI's config; installs shell activation; safely recovers known broken v4.2.6 launcher artifacts (Claude only)
- `launch.go` — hidden command used by shell activation; injects Bedrock runtime env and runs the real underlying CLI binary
- `auth_token.go` — hidden command (`juggernaut auth-token`) some CLIs invoke as an external auth provider to fetch the Bedrock bearer token from the keychain at session start. Two output shapes via `--format`: `json` (`{"access_token":...,"expires_in":N}`, consumed by Grok's `auth_provider_command`) and `token` (bare token on one line, consumed by Codex)
- `show.go` — prints current config from settings.json
- `doctor.go` — read-only diagnostics (`--cli`, `--scope`, `--json`): embedded config load, per-scope settings, keychain, API-key expiry (`checkKeyExpiry`), activation blocks + PowerShell profile discovery, underlying CLI binary, v4.2.6 artifacts, auto-mode readiness, Fable data retention, and live Bedrock connectivity (`checkConnectivity`)
- `uninstall.go` — removes the Juggernaut-managed keys/block from config and deletes the bearer token; `--full` removes activation blocks
- `models.go` — defines the `models` parent command and implements `models check` (maintainer-facing release gate against AWS's live catalog)
- `model_catalog.go` — implements `models refresh`/`models list`; explicitly discovers and caches the current account/region inventory without adding network I/O to `apply`
- `version.go` — prints version

**Multi-CLI provider abstraction:** Juggernaut configures multiple coding CLIs for Bedrock, selected with `--cli` (default `claude`) on `apply`/`uninstall`. Activation blocks use the hidden `launch-cli <cli> -- …` command for non-Claude CLIs; the distinct command name is deliberate so a pre-multi-CLI binary fails instead of silently launching Claude. The historical `launch [cli] -- …` form remains accepted for compatibility. Each CLI is a `provider.Provider` (`internal/provider/`) owning its identity, binary names, config path + format, native managed keys, activation markers, capabilities, and the two-phase output: `BuildConfig(cfg, opts) → ConfigPlan` (what to persist at apply time) and `LaunchSpec() → {TokenEnvVar, StaticEnv, NeedsToken, PersistRuntimeState}` (what the wrapper injects at runtime and whether it opts into non-secret drift fallback). Supported today, registered in `internal/provider/provider.go`'s `init()`:
  - `claude` — JSON `~/.claude/settings.json`
  - `codex` — TOML `~/.codex/config.toml`, routed via built-in `amazon-bedrock` provider with `[model_providers.amazon-bedrock.aws]` for region
  - `opencode` — JSON `~/.config/opencode/opencode.json`, routed via a custom OpenAI-compatible provider block (`@ai-sdk/openai-compatible`) since OpenCode is model-agnostic
  - `grok` — TOML `~/.grok/config.toml` (always user-scoped; Grok has no project config), routed via a `[model.bedrock-grok]` block plus an `[auth]` block whose `auth_provider_command` runs `juggernaut auth-token` — a plain `env_key` does not suppress Grok's interactive login, only `auth_provider_command` does

  Adding a CLI = implement the `Provider` interface + call `register()` in that `init()`; nothing in `cmd/` hardcodes per-CLI logic (Claude-only flags gate on `provider.Supports(Capability)`). Capabilities live in `internal/provider/plan.go`: `CapAutoMode`, `CapOpusplan`, `CapThinking`, `CapServiceTiers`, `CapEffortLevels`, `CapNativeAuth`. Blocks for different CLIs coexist in one shell profile. The Bedrock bearer token is **shared** across CLIs — `uninstall --cli=codex` never removes it.

  The interface's own doc comments in `internal/provider/provider.go` are the authoritative contract (they spell out why `OwnsConfig` must be stricter than "any managed key present", and how `OwnedSubKeys` drives both uninstall and collision detection). Per-model Mantle facts are recorded inline in the provider files — `codexModels` in `codex.go` carries the verified model IDs and API-shape notes. The original design/research write-ups under `.research/` are gitignored local notes, so don't cite them as if a fresh clone has them.

  Some providers own nested config tables where Juggernaut must merge rather than replace (e.g. Grok's `[model.<name>]`, Codex's `[model_providers.<id>]`, OpenCode's `provider.<id>`) so a user's sibling entries survive. `Provider.DeepMergeKeys()` lists which managed keys need this, and `Provider.OwnedSubKeys()` maps each to the exact sub-keys Juggernaut owns — uninstall removes only those, not the whole table.

**Internal packages in `internal/`:**
- `provider/` — the `Provider` interface + registry (`Get(name)`), and per-CLI impls (`claude.go`, `codex.go`, `opencode.go`, `grok.go`). `base.go`'s `BaseProvider` supplies default implementations (`Name`, `BinaryNames`, `ConfigFormatName`, `ActivationMarkers`, `Supports`) that each per-CLI struct embeds, so only the genuinely CLI-specific methods (`ConfigPath`, `OwnsConfig`, `NativeManagedKeys`, `DeepMergeKeys`, `OwnedSubKeys`, `BuildConfig`, `LaunchSpec`) need overriding. `claude.BuildConfig` wraps `schema.Build` (byte-identical, guarded by the `cmd` golden test). The optional `CatalogProvider` interface owns endpoint/protocol compatibility: Claude accepts native Anthropic entries, Codex accepts Mantle GPT-5 entries, OpenCode accepts general OpenAI-compatible Mantle entries, and Grok accepts Mantle xAI entries. `region.go` retains the known-alias Mantle region policy used before a live catalog has been refreshed.
- `discovery/` — the only package that imports `aws-sdk-go-v2`. It inventories native foundation models, per-model account availability, all pages of inference profiles, and the Mantle OpenAI-shaped `/v1/models` endpoint (bearer token or default-profile SigV4). Region snapshots are atomically cached at owner-only `~/.juggernaut/model-catalog.json`; partial source refreshes preserve the other source and region snapshots.
- `bedrock/` — loads `bedrock-config.json` into typed structs. `LoadBytes()` is used by cmd (embedded); `Load(path)` is the filesystem fallback for tests.
- `schema/` — builds and validates the Claude `Block`; derives native settings.json keys via `NativeKeys()`. `CLAUDE_CODE_USE_BEDROCK=1` is gated behind `AuthValidated=true`. `CLAUDE_CODE_ENABLE_AUTO_MODE=1` is auto-set when `PermissionMode=="auto"`.
- `config/` — atomic read/merge/write of config files (JSON or TOML via the `ConfigFormat` interface + `FormatByName`); backup rotation (5 most recent); file locking via `gofrs/flock`. `MergeConfigPlan`/`MergeConfigPlanDeep` and `RemoveManagedKeys`/`RemoveManagedKeysDeep` are the generic provider-driven merge/remove paths (the sole entry points; deep variants respect `DeepMergeKeys`/`OwnedSubKeys`). Underlying `applyManagedKey`/`RemoveManagedKeys` helpers handle individual key writes/removals. `DetectCollisions` (`collision.go`) is the foreign-config guard: given an existing config and a provider's plan, it walks only the exact leaves the provider is about to write (using the same `DeepMergeKeys`/`OwnedSubKeys` metadata, plus a `permissions.defaultMode` special case) and reports any that already hold a foreign value — sibling entries in the same table never trigger it.
- `keychain/` — cross-platform credential storage via `go-keyring`. Service name: `juggernaut-bedrock`, account `bedrock-credential`. The stored bearer token is shared across all configured CLIs. **There is a file fallback, and it matters:** Windows Credential Manager caps a blob at `MaxWindowsKeychainSize` (2560 bytes), which short-term Bedrock keys exceed, so `SetWithFallback`/`GetWithFallback`/`DeleteWithFallback` fall back to an owner-only file at `~/.claude/juggernaut-credential`. That file is versioned — `v2` is DPAPI-encrypted + base64 on Windows, `v1` is plaintext and transparently migrated to `v2` on next write, and an unprefixed file is treated as a possibly-stale legacy (v5.2.2) artifact. Use the `*WithFallback` methods on any real credential path; the bare `Set`/`Get`/`Delete` are keychain-only and will reintroduce the "key not found" class of bug on Windows.
- `activation/` — manages shell profile marker blocks (per-CLI via `CLISpec`/`blockFor`, coexisting in one profile), implements the hidden `launch`/`launch-cli` commands, stores provider-opted non-secret runtime fallback state under `~/.juggernaut/runtime/`, resolves the real target binary (`resolveBinary`) while avoiding recursion, and recovers only positively identified v4.2.6 launcher artifacts (Claude-only).
- `doctor/` — `Report` type with `Check()`, `HasFailures()`, `String()`, `JSON()`. `cmd/doctor.go` reports runtime fallback health/drift and its `checkAutoModeReadiness` inspects the persisted `juggernaut` block per scope, reporting auto-mode readiness (OK/WARN) by calling `schema.Block.AutoModeUsable()`/`AutoModeAvailable()` directly — silent unless `permissionMode=="auto"` is configured.
- `authmode/` — constants/helpers for auth mode strings (`IAM`, `BedrockAPIKey`). Use `authmode.BedrockAPIKey` (split var) instead of bare string literals to avoid static secret scanner hits.
- `safepath/` — path containment checks (`JoinUnder`, `IsUnderBase`, `withinBase`), owner-only filesystem helpers (`MkdirAll`, `ReadFile`, `WriteFile` at `0o700`/`0o600`), and home resolution (`HomeDir`, `HomeDirOrEmpty`). Use these whenever writing files under a user-controlled base path.

**Activation mode:** shell profiles contain per-CLI marker blocks (e.g. `# BEGIN: Juggernaut Claude Activation` / `# END: Juggernaut Claude Activation`) defining a shell function that delegates to `juggernaut launch -- "$@"` (Claude) or `juggernaut launch-cli <cli> -- "$@"` (others), or shell equivalent. Wrappers fall through to the real CLI binary when `juggernaut` is not on PATH. Apply installs PowerShell AllHosts profiles only and strips stale host-specific blocks for that CLI; multi-CLI uninstall also scans historical OneDrive/local Documents paths. Juggernaut must never install, overwrite, move, symlink over, or delete an unknown file matching a managed CLI's binary name.

## Key Design Patterns

- **Embedded config:** `bedrock-config.json` is compiled into the binary. Changing it requires rebuilding. Tests fall back to filesystem resolution via `findBedrockConfigFile()`.
- **Managed outputs:** provider config file, marked shell activation blocks, and,
  for providers that opt in, owner-only non-secret user-scope runtime state
  under `~/.juggernaut/runtime/`.
- **Runtime drift fallback:** Claude user-scope apply stores auth mode plus the
  generated non-secret environment at `~/.juggernaut/runtime/claude.json`.
  Launch uses it only when no managed Claude config remains, emits a repair
  warning, and still fetches API keys dynamically. Project apply never writes
  global fallback state; user uninstall removes it.
- **Auth-gated Bedrock flag:** `CLAUDE_CODE_USE_BEDROCK=1` only lands in settings.json when `AuthValidated=true`.
- **Scope:** `--scope=user` (default) vs `--scope=project`. Grok is always user-scoped — it has no project config concept.
- **Re-apply preserves existing auth mode** — if `--auth` is omitted and a block already exists, the existing auth mode is read from the block.
- **`--model` overrides all four model IDs** (opus, sonnet, haiku, fable) when set; `--opus-model`, `--sonnet-model`, `--haiku-model`, `--fable-model` override individually. The embedded config's `models` map has six entries — the four tiers plus `default` and `fast` — currently pinned to `global.anthropic.claude-opus-5` (opus), `global.anthropic.claude-sonnet-5` (sonnet/default), `global.anthropic.claude-haiku-4-5-20251001-v1:0` (haiku/fast), and `global.anthropic.claude-fable-5` (fable), each verified live against AWS Bedrock's `ListFoundationModels`/`ListInferenceProfiles` APIs.
- **Fable data-retention warning:** Anthropic requires opting in to `provider_data_share` before Fable calls succeed on Bedrock (verified against AWS's abuse-detection docs); there is no AWS API to read an account's actual opt-in status, so Juggernaut cannot check it. `schema.FableDataRetentionWarning` (emitted by `schema.Build` into `Block.Warnings` whenever `schema.IsFable5Model` matches the resolved Fable ID) is the single source of truth, surfaced two ways: once at apply time via `ConfigPlan.Warnings` (same mechanism as the Mantle/auto-mode warnings) and on every `doctor` run via `checkFableDataRetention` (re-runnable since the apply-time note scrolls away). Since Fable is pinned by default, this warns on every apply/doctor run unless a maintainer ships a config without it — it never claims Juggernaut knows what is or isn't collected, only what AWS documents.
- **`juggernaut models check`:** a maintainer-facing, release-preparation command (NOT run implicitly by `apply`/`doctor`) that queries AWS Bedrock's live foundation-model and inference-profile catalogs (`internal/discovery/`, the only package that imports `aws-sdk-go-v2`) and reports whether each pinned tier (`opus`/`sonnet`/`haiku`/`fable`) in `bedrock-config.json` is still `ACTIVE`. Exits non-zero if any tier is `LEGACY` (CI/script-friendly pre-release gate). `--write --set-<tier>=<id>` re-verifies the chosen ID is `ACTIVE` before pinning it — never auto-picks among multiple `ACTIVE` candidates, and `--write` alone (no `--set-*`) is a hard error, not a no-op success. No deprecation countdown: AWS's `endOfLifeTime`/`legacyTime` fields are not populated in practice, so this only ever reports binary `ACTIVE`/`LEGACY`.
- **Account model discovery:** `models refresh --region=<region> [--source=all|native|mantle]` explicitly resolves the STS caller account, queries the account/region catalog, and writes an owner-only cache partitioned by account and region. A non-secret fingerprint of the selected AWS profile/environment credentials binds offline commands to the account resolved during refresh, preventing a profile/account switch from consuming another account's inventory. Native foundation entries are enriched with `GetFoundationModelAvailability`; inference-profile and Mantle list responses are already account/region scoped. `models list [--cli=<provider>] [--show-unsupported]` explains the provider compatibility decision. `apply` consumes the matching cached account/region but never refreshes implicitly, preserving deterministic/offline behavior. OpenCode therefore receives newly available compatible models (including Kimi, GLM, Qwen, GPT OSS, DeepSeek, MiniMax, and future families) without a curated roster; friendly aliases remain only as convenience defaults. A provider must implement optional `CatalogProvider` to participate, keeping CLI-name conditionals out of `cmd/`.
- **Credential storage:** always the OS keychain via `go-keyring`. Service name: `juggernaut-bedrock`.
- **`--no-1m-context`:** disables 1M token context window (on by default). Hidden/deprecated no-ops kept only for script compatibility: `--1m-context` and `--skip-preflight` (never gated any check). `--no-mantle` is likewise accepted-but-redundant since Mantle is off by default. Don't "fix" these by deleting them — removing an accepted flag breaks existing user scripts.
- **Mantle opt-in:** standard Bedrock preserves inference profile IDs by default; use `--mantle` or `--mantle-url` only when explicitly routing through Mantle. Non-Claude providers (Codex, OpenCode, Grok) route through Mantle unconditionally since Bedrock's native Claude-only API doesn't serve other model families.
- **Opusplan-gated ANTHROPIC_MODEL:** only written when `--opusplan` is active.
- **`--mode`:** sets `permissions.defaultMode` in settings.json. When `auto`, also writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` in env (required for Bedrock). Auto mode on Bedrock additionally requires the **active session model to be Sonnet 5 or Opus 4.7 or later (4.7, 4.8, 5)** (Claude Code v2.1.158+); Haiku and older Sonnet/Opus are unsupported and Claude Code hides auto from the Shift+Tab cycle. `apply --mode=auto` prints a warning (via `schema.Block.AutoModeUsable()` / `schema.IsAutoModeCapableModel()`) unless the resolved active model is capable. Users unlock it by running Claude Code on Opus (`claude --model opus` or `/model opus`) when the active model isn't already capable.
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
- **Use the existing test helpers instead of rewriting the boilerplate.** `internal/testhome.NewTestHome(t)` returns a temp dir with `HOME` *and* `USERPROFILE` set (both are needed for the Windows paths); it deliberately imports nothing internal so any package can use it without an import cycle. `internal/testutil` adds `CaptureStdout`, `WithStdin`, `NestedMapChain`, `ParseJSON`, `OwnedJuggernautBlock`, and `SkipIfNoKeychain`.
- Keychain tests skip gracefully when the backend is unavailable (`SkipIfNoKeychain` / the `skipIfUnavailable()` probe in `keychain_test.go`). To isolate keychain state: set `JUGGERNAUT_KEYCHAIN_SERVICE`.
- **AWS is never called in tests.** `cmd/models.go` and `cmd/model_catalog.go` expose their discovery calls as package-level function variables (`listAnthropicModels`, `listInferenceProfiles`, `listFoundationCatalog`, `listMantleCatalog`, `catalogCallerAccount`, `catalogCredentialScope`, `catalogNow`) precisely so tests can swap them. Keep that seam when adding discovery-backed commands.
- `cmd/golden_test.go` diffs Claude's real apply output (settings.json + dry-run) byte-for-byte against `cmd/testdata/golden/`, normalizing `appliedAt` and the temp home path. Any unintended output drift fails here — including drift in `provider.claude.BuildConfig` vs `schema.Build`. Regenerate deliberately with `UPDATE_GOLDEN=1`.
- Cobra global flag state leaks between `ExecuteArgs()` calls — `resetFlags()` in `cmd/root.go` resets all subcommand flags to defaults before each invocation.
- **`cmd` coverage (95.6% on the Windows profile; 93.9% repo-wide) sits at its practical floor.** Batch 1 (`cmd/coverage_batch1_test.go`) and Batch 2 (`cmd/coverage_batch2_test.go`) closed every platform-independent and Windows-leg branch; the remaining `0`-count blocks are the irreducible floor:
  - *Covered on the Linux/macOS CI legs, not the local Windows profile:* the POSIX activation branch in `runDoctor`, `launch.go`'s `resolveSelfPaths` non-Windows return, `reportLegacyRecovery`'s POSIX-only error rename, and the `huh` TUI prompt paths (construction + Run-error branches) in `resolveApplyInputs`/`resolveCredential`.
  - *Impossible to reach by construction:* `json.Marshal*` of plain maps/structs can never error (`auth_token.go`, `models.go`, `show.go`, `doctor.go`); `provider.Get("claude")` always resolves (doctor.go:428/476) and claude's `ConfigPath` is a plain join that never errors (doctor.go:254, show.go:40); `newProviderManager`'s error in `commitApply` is shadowed by `detectForeignCollisions` earlier in the same call (helpers.go:426).
  - *Must not be covered in-process:* `Execute`'s error path calls `os.Exit` (root.go:22); `bedrock.CheckAPIKeyConnectivity` is a live AWS call (doctor.go:311, tests never call AWS); Windows-console TUI helpers like `flushConsoleInput` need a real console (apply_windows.go).
  - When touching `cmd/`, re-run `go test ./cmd/ -coverprofile` and check you have not regressed any of these; a genuinely new closeable block belongs in a `coverage_batch*_test.go` file that restores state in cleanup and reuses `setupApplyTest`/`mockPSRunner`/`testutil` helpers.
- **Irreducible Coverage Floor — test quality rule.** Every new test must close a real gap: an error path, a platform-specific branch, previously untested public behavior, or a documented edge case. Reject pure line-coverage fillers — tests whose only value is a statement counter ticking (e.g. `_ = fn()` "should not crash" smoke calls with no assertion). Prefer one strong test that drives the real failure over several shallow ones. When adding or expanding a `coverage_gaps_*_test.go` file, the PR description must name the exact branch or error it closes. Pruning such tests is allowed only for zero-value ones: near-duplicates whose lines stay covered elsewhere, and never at the cost of measured coverage — verify with `go test <pkg> -coverprofile` before and after.

## CI / Release

- CI (`ci.yml`) jobs: `lint` (golangci-lint + version sync check + GoReleaser auto-publish check), `lint-windows` (golangci-lint on Windows), `vuln` (govulncheck), `test` (race-covered on ubuntu/macos/windows with per-OS Codecov upload via OIDC — merged for true cross-platform coverage, since much of the PowerShell/launcher/keychain code is Windows-only), `test-race`, `test-coverage` (Linux-only **65%** floor via `gotestsum`, JUnit uploaded to Codecov Test Analytics), `npm-test`, `shellcheck`, `gosec`, `trivy`, `goreleaser` (snapshot build + linux binary smoke test), and `codacy-analysis` (push-only). Actions pinned to full commit SHAs.
- **Two different coverage numbers, don't conflate them:** the in-CI 65% floor is Linux-only and deliberately sits *below* the authoritative gate, because Windows-only code isn't exercised on that leg. The merged Codecov `project` gate is 80% with `threshold: 0%`, and `patch` is 80% (`codecov.yml`). `codecov.yml` also defines `cmd`/`internal` components and ignores `main.go`, `npm/**`, `scripts/**`, and the build-tagged `internal/keychain/crypter_{windows,other}.go` DPAPI shims (unmeasurable in a merged multi-OS report).
- The Ubuntu coverage leg saves `coverage.out` as a run-scoped artifact. The separate `codacy-coverage.yml` `workflow_run` workflow runs with trusted repository context, downloads that artifact when available, and uploads it to Codacy with `CODACY_REPOSITORY_API_TOKEN` plus the triggering run's `head_sha`; it never checks out or executes the PR revision.
- CodeQL (`codeql.yml`): separate scheduled + push/PR workflow analyzing `actions`, `go`, and `javascript-typescript` via `.github/codeql/codeql-config.yml`.
- Octopus Review (`octopus.yml`): posts automated review comments on `pull_request_target` opened/synchronize. Bot review comments are advisory — verify a claim against the code before acting on it.
- Socket (`socket.yml`): supply-chain security scan on PRs to `main`; `firewall-free` mode (informational PR comments only, never blocks); pinned to `SocketDev/action@v1.3.2`. Despite being informational, **both Socket contexts are required checks** in the `protect-main` ruleset, so a PR can't merge until they report.
- Dependabot (`.github/dependabot.yml`): weekly GitHub Actions updates.
- Release (`release.yml`): triggered on `v*` tags → GoReleaser builds and **publishes** the GitHub release (`draft: false` in `.goreleaser.yml`; CI fails on `draft: true`) → verifies the release is non-draft with assets before npm → npm OIDC publish of the platform sub-packages, then the main `juggernaut-bedrock` package.
- **Branch protection is a ruleset, not classic branch protection.** `gh api repos/.../branches/main/protection` returns 404 — that's expected; query `gh api repos/jpvelasco/juggernaut/rulesets` instead. The active `protect-main` branch ruleset blocks deletion and non-fast-forward, requires a PR with thread resolution (0 approvals required), enforces strict (up-to-date) status checks, and has **no bypass actors** — so the required checks apply to the owner too. Required contexts: `lint`, `test (ubuntu-latest)`, `test (macos-latest)`, `test (windows-latest)`, `Socket Security: Project Report`, `Socket Security: Pull Request Alerts`. Note what is *not* required: race, coverage, gosec, trivy, goreleaser, and Codacy are informational. A separate `protect-version-tags` tag ruleset guards `v*`.
- GoReleaser requires a clean git tree — never commit build artifacts (`bin/` is gitignored).
- npm package is in `npm/` — optional platform sub-packages ship prebuilt binaries; `npm/index.js` resolves and execs the matching package.

## Codacy

Codacy is **informational**, not a merge or main CI gate — it is not in the `protect-main` required-check list, and CI does not fail on outstanding dashboard issues. Cloud analysis runs via the Codacy GitHub integration (grade badge, PR comments, dashboard), and `ci.yml`'s `codacy-analysis` job additionally runs the Codacy Analysis CLI on **push only** (`max-allowed-issues` is set to max int, so it uploads without failing) because cloud analysis doesn't reliably fire after a squash-merge.

Coverage is handed off from `ci.yml` as a run-scoped artifact and uploaded by the trusted `workflow_run` workflow only when that artifact exists; the uploader passes `workflow_run.head_sha` so coverage is associated with the tested revision. That path is the only Codacy credential in GitHub Actions: repository secret `CODACY_REPOSITORY_API_TOKEN` (a Codacy **repository** API token, used as `project-token` for coverage upload). Do **not** put an account API token in CI.

Local dashboard queries (`make codacy`, `@codacy/codacy-cloud-cli`) use a personal **account** API token via `CODACY_API_TOKEN` on the developer's machine only. `.codacy/codacy.config.json` is the tracked synced tool config (PMD, ESLint8, Semgrep, Trivy, shellcheck, markdownlint, psscriptanalyzer, Lizard, Checkov, jackson, Agentlinter); everything else under `.codacy/` (logs, SARIF, `server-issues.json`) is local-only and gitignored. `.codacy.yml` excludes `.remember/`, `.codacy/`, and `mcps/` from analysis (`npm/` is additionally excluded from the eslint engine).
