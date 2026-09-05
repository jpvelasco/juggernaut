# Repository Guidelines

## Project

Juggernaut is a cross-platform Go CLI that configures Claude Code, Codex, OpenCode, and Grok to use Amazon Bedrock. Binary embeds `bedrock-config.json` — rebuild after editing. Module `github.com/jpvelasco/juggernaut/v5`, public MIT, npm-only as `juggernaut-bedrock` (`scripts/install.sh|ps1` are deprecated stubs). Keep `README.md`, `QUICKSTART.md`, `CONTRIBUTING.md` current when flags/behavior change; never cite gitignored `.research/`, `docs/superpowers/`, `.claude/` as if present in a fresh clone.

`CLAUDE.md` is a pointer to this file — this is the single tracked reference. Keep it accurate when architecture/workflows change.

## Commands

```bash
make build                 # bin/juggernaut (ldflags inject VERSION)
make test                  # go test ./... -v
make test-race             # -race
make test-cover            # coverage.out + total (POSIX tail — see Windows note)
make lint                  # golangci-lint
make fmt vet               # go fmt + go vet
make ci                    # tidy, fmt, vet, lint, test
make codacy                # needs CODACY_API_TOKEN

go test ./internal/schema/... -v
go test ./cmd/... -run TestApply_WritesSettings_IAM -v
cd npm && npm test         # Node >=20, CI uses 24
UPDATE_GOLDEN=1 go test ./cmd/ -run Golden   # only after intentional output change

# Windows: make test-cover uses tail, run instead:
go test ./cmd/ -coverprofile=$env:TEMP\cov.out; go tool cover -func=$env:TEMP\cov.out | Select-Object -Last 1
```

Hooks: `scripts/setup-hooks.ps1` (Windows) or `bash scripts/setup-hooks.sh` (Linux/macOS). PowerShell 7 is default shell on Windows; keep scripts cross-platform.

## Architecture

- `main.go` embeds Bedrock config → `cmd.Execute()` (Cobra).
- `cmd/` — commands, hidden `launch`/`launch-cli`/`auth-token`, shared helpers (`helpers.go`). `models.go` (`models check` release gate) and `model_catalog.go` (`models refresh`/`list`, cache `~/.juggernaut/model-catalog.json` per account+region). `logs.go` (`logs export` writes a redacted diagnostic zip; `--raw` is opt-in).
- `internal/provider/` — `Provider` interface + 4 implementations (`claude`, `codex`, `opencode`, `grok`). `base.go:BaseProvider` supplies `Name`, `BinaryNames`, `ConfigFormatName`, `ActivationMarkers`, `Supports`; providers override `ConfigPath`, `OwnsConfig`, `NativeManagedKeys`, `DeepMergeKeys`, `OwnedSubKeys`, `BuildConfig`, `LaunchSpec`. New CLI = one file + `register()` in `provider.go`; new knobs = `Capability`, not `if cli=="..."` in `cmd/`.
- `internal/config/` — atomic JSON/TOML merge, removal, locking, backups. Respects `DeepMergeKeys`/`OwnedSubKeys` for nested tables.
- `internal/activation/` — marked shell blocks (one per CLI, coexist in same profile) + owner-only runtime fallback `~/.juggernaut/runtime/` + launch wrapper that resolves real CLI binary and avoids recursion.
- `internal/schema/` — builds/validates Claude managed settings (`settings.json`).
- `internal/bedrock/` — loads embedded/test Bedrock config. `bedrock-config.json` is Claude-only (non-Claude providers render from live discovery + shared token).
- `internal/keychain/` — shared bearer token; `*WithFallback` methods are the real path (handles 2560-byte Windows Credential Manager limit via versioned owner-only file). Bare `Set`/`Get`/`Delete` are keychain-only.
- `internal/discovery/` — only package importing `aws-sdk-go-v2`.
- `internal/safepath/` — containment + owner-only FS ops. Use for anything under user-controlled bases.
- `internal/redact/` — diagnostic-bundle privacy: tokens, account IDs, home paths, emails, hostnames, LAN IPs → stable placeholders.
- `npm/` — launcher + platform packages.

Config paths: Claude `~/.claude/settings.json` (user) / `./.claude/settings.json` (project) — the only CLI with both scopes; Codex `~/.codex/config.toml` / `./.codex/config.toml` via the built-in `amazon-bedrock-runtime` provider (`[model_providers.amazon-bedrock-runtime.aws]` region table; requires Codex ≥ 0.153.4 — apply/doctor warn on older binaries); OpenCode `~/.config/opencode/opencode.json` / `./opencode.json` via `provider.amazon-bedrock` (region + discovered `models` + `whitelist`); Grok `~/.grok/config.toml` user-only (`base_url=https://bedrock-runtime.{region}.amazonaws.com/openai/v1`, model `global.xai.grok-4.6`; `auth_provider_command="juggernaut auth-token"` only for `--auth=bedrock-api-key`). Launch reads `juggernaut.auth.mode` from project then user for non-Claude CLIs (token-gated only for API-key mode).

## Constraints

- Reuse `cmd/helpers.go`; don't duplicate. Use `internal/safepath` for user-controlled paths.
- Preserve unknown user config. Nested provider tables must use `DeepMergeKeys`/`OwnedSubKeys` — never replace whole table.
- Claude managed keys: `env`, `model`, `modelOverrides`, `fallbackModel`, `effortLevel`, `alwaysThinkingEnabled`, `skipWebFetchPreflight`, `permissions`. Uninstall removes only managed keys.
- Never install/overwrite/move/symlink/delete an unknown file matching a managed CLI binary. Activation blocks for different CLIs must coexist.
- Bearer token is shared across providers. Uninstalling one non-Claude provider must not delete it; Claude `--scope=project` uninstall must not either while user-scope or non-Claude configs remain.
- Keep `VERSION`, `bedrock-config.json:version`, `cmd/root.go:Version` in sync (CI verifies).
- Go toolchain pinned to `go 1.26.6` in `go.mod`; use exact version.
- `apply --auth` is validated before any write — only `iam` or `bedrock-api-key` accepted (`validateAuthFlag`).
- Claude user-scope apply writes only non-secret runtime state to `~/.juggernaut/runtime/`; project apply must not create global fallback, and tokens must never be written there; user-scope uninstall removes it.
- `apply --mode=auto` writes `CLAUDE_CODE_ENABLE_AUTO_MODE=1` but must warn unless active model is Opus 4.7+ (incl. Opus 5).
- `--fallback-model`, `--service-tier`, `--always-thinking`, `--effort`, `skipWebFetchPreflight` are managed; `max`/`auto` effort are env-only, fixed levels also persist as native `effortLevel`.
- Preserve compat for `launch`/`launch-cli`, hidden `auth-token`, deprecated flags, and exit-code passthrough (`Execute()` exits with wrapped CLI's status; launch commands silence cobra error/usage).
- Mantle removed in v6 — all CLIs route via `bedrock-runtime`; after upgrade `juggernaut models refresh --source native --region <region>`.
- v5 Codex configs still on the custom `amazon-bedrock` provider id are legacy for v6 (that table points at the dead Mantle host): a plain `apply` migrates them to the built-in `amazon-bedrock-runtime` and strips `model_providers.amazon-bedrock` (Codex-only — OpenCode's `provider.amazon-bedrock` is a different, current table and never gets touched). The version gate requires Codex ≥ 0.153.4 (warns on apply/doctor).
- Suppressions (`//nolint`, `#nosec`, `// nosemgrep`) require rationale in `docs/ci-suppressions.md`; prefer code fix over suppression.

## Testing

Add focused tests per behavior change; run affected package before full suite.

- `internal/testhome.NewTestHome(t)` sets both `HOME` and `USERPROFILE` (Windows needs both).
- `internal/testutil` — `CaptureStdout`, `WithStdin`, `NestedMapChain`, `ParseJSON`, `OwnedJuggernautBlock`, `SkipIfNoKeychain`. Isolate keychain tests with `JUGGERNAUT_KEYCHAIN_SERVICE`; skip when no backend.
- `cmd` helpers: reuse `cmd/apply_test_helpers_test.go` + `coverage_batch{1,2}_test.go` — `setupApplyTest(t)` (fake home + mock PS runner), `setupApplyTestWithReset`, `captureStdout`/`captureStderr`, `mockPSRunner`/`mockPSOutputJSON`, `noHomeEnv`, `blockCredentialWrite`, `chdirTo`, `withBrokenEmbeddedConfig`, `swapActiveModelsForWrite`, `writeTempBedrockConfig`, `stubProvider` family.
- `homeDir()` failure: clear env with `noHomeEnv`, assert via `assertHomeDirError` (message is `$HOME` vs `%userprofile%` per OS).
- `models --write` tests must `chdirTo` a temp dir with a `bedrock-config.json` copy — never mutate repo-root fallback.
- No `t.Parallel()` in `cmd` — `os.Chdir` is process-global.
- `cmd/launch_exitcode_test.go:TestMain` re-execs with `JUGGERNAUT_TEST_WRAPPER_CHILD=1` to assert `Execute()` exit-code translation. Never set that var elsewhere.
- `captureStdout` drains only after `fn()` returns — large output deadlocks at pipe-buffer size; use concurrent-drain `captureStreaming` (`cmd/show_order_test.go`).
- Tests never call AWS. `cmd/models.go` + `cmd/model_catalog.go` expose discovery as package vars for swapping.
- `cmd/golden_test.go` diffs Claude apply output byte-for-byte against `cmd/testdata/golden/`.
- Cobra global flag state persists across `ExecuteArgs()` calls — use `resetFlags()` when adding/changing flags.

Irreducible coverage floor: new tests must cover a real error/platform branch or documented edge case — no `_ = fn()` smokes. Prefer one strong failure test over many shallow ones. `coverage_gaps_*_test.go` changes must state the exact branch in the PR description. Verify `go test <pkg> -coverprofile` before/after pruning.

## CI & Git

Before handoff: `go test ./... && go vet ./... && git diff --check` (or `make ci` when tools available). Never commit `bin/`, `coverage.out`, or build artifacts.

CI (`.github/workflows/ci.yml`): `lint` + `lint-windows` (golangci-lint v2.12.2), `test` matrix (ubuntu/macos/windows with OIDC Codecov merge — Windows coverage strips `\r`; fork PRs skip upload), `test-race`, `test-coverage` (Linux floor 65%), `npm-test` (Node 24), `shellcheck`, `gosec`, `trivy`, `goreleaser` snapshot, `codacy-analysis` on push. **Merge-blocking**: `lint`, three `test (<os>)` legs, two Socket contexts. Rest is informational. Authoritative gate is Codecov 80% project+patch (`codecov.yml`), not the 65% Linux floor. Codecov merge requires all 3 OS legs because Windows-only keychain/launcher code is uncovered on Linux.

Hooks (`.githooks/`): `commit-msg` — single-type Conventional Commit (`type(scope?): description`, `docs+test:` rejected); `pre-commit` — fails if `go fmt`/`go mod tidy` would change staged files; `pre-push` — warns if changed Go file gains a 0.0%-coverage function (bypass `git push --no-verify`, but Codecov patch gate is authority).

`main` protected by ruleset `protect-main` (not classic branch protection — `branches/main/protection` 404s; query `gh api repos/jpvelasco/juggernaut/rulesets`). Blocks deletion/non-fast-forward, requires PR with resolved threads (0 approvals), strict status checks, no bypass actors. `protect-version-tags` guards `v*`. Settings in `.github/GITHUB_SETTINGS.md`. Never delete `legacy/v3`.

Conventional commits: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`. Tags `v*` trigger release + npm OIDC publish — don't create/push unless requested. Feature branches off `main`; squash-merge unless history matters; inspect `git status`/`diff` before editing and preserve unrelated changes.
