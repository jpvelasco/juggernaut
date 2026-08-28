# Repository Guidelines

## Project

Juggernaut is a cross-platform Go CLI that configures Claude Code, Codex,
OpenCode, and Grok to use Amazon Bedrock. The binary embeds
`bedrock-config.json`; rebuild after changing that file.

Read `CLAUDE.md` for the detailed architecture, provider inventory, managed
configuration keys, and current CI/release design. Keep that document accurate
when architecture or workflows change.

This is a **public** MIT-licensed repository (`github.com/jpvelasco/juggernaut`,
module `github.com/jpvelasco/juggernaut/v5`), distributed via npm only as
`juggernaut-bedrock`; `scripts/install.sh` and `scripts/install.ps1` are
deprecated stubs that print npm instructions. Assume outside contributors and
forks: keep `README.md`, `QUICKSTART.md`, and `CONTRIBUTING.md` current when
flags or user-visible behavior change, and never cite a gitignored path
(`.research/`, `docs/superpowers/`, `.claude/`) as if a fresh clone has it.

## Common Commands

```bash
make build                 # build bin/juggernaut
make test                  # run all Go tests
make test-race             # run with race detector
make test-cover            # generate coverage.out and print total
make lint                  # run golangci-lint
make fmt vet               # format and vet
make ci                    # tidy, format, vet, lint, test
make codacy                # check Codacy dashboard issues with CODACY_API_TOKEN

go test ./internal/schema/... -v
go test ./cmd/... -run TestApply_WritesSettings_IAM -v

cd npm && npm test         # npm launcher tests (Node >= 20; CI uses Node 24)
UPDATE_GOLDEN=1 go test ./cmd/ -run Golden   # only after an INTENTIONAL output change

# measure cmd coverage (make test-cover uses POSIX `tail`, so on Windows use:)
go test ./cmd/ -coverprofile=$env:TEMP\cov.out; go tool cover -func=$env:TEMP\cov.out | Select-Object -Last 1
```

Install git hooks once per clone: `scripts/setup-hooks.ps1` (Windows) or
`bash scripts/setup-hooks.sh` (Linux/macOS). CI is the real gate either way.

On Windows, PowerShell is the default shell. Keep scripts cross-platform unless
they are explicitly platform-specific.

## Architecture

- `main.go` embeds the Bedrock config and calls `cmd.Execute()`.
- `cmd/` contains Cobra commands, hidden launch/auth commands, and shared helpers.
  `models.go` defines `models check` (maintainer-facing release gate against the
  live catalog) and `model_catalog.go` implements `models refresh`/`models list`,
  caching account/region inventory under `~/.juggernaut/model-catalog.json`.
- `internal/provider/` owns the multi-CLI provider interface and implementations
  for `claude`, `codex`, `opencode`, and `grok`. `base.go`'s `BaseProvider`
  supplies the default implementations (`Name`, `BinaryNames`,
  `ConfigFormatName`, `ActivationMarkers`, `Supports`) that each provider
  embeds, so per-CLI structs only override `ConfigPath`, `OwnsConfig`,
  `NativeManagedKeys`, `DeepMergeKeys`, `OwnedSubKeys`, `BuildConfig`, and
  `LaunchSpec`.
- `internal/config/` handles atomic JSON/TOML merge, removal, locking, and backups.
- `internal/activation/` manages marked shell blocks, the owner-only non-secret
  runtime fallback under `~/.juggernaut/runtime/`, and launches real CLI
  binaries via `launch`/`launch-cli` while avoiding wrapper recursion.
- `internal/schema/` builds and validates Claude-specific managed settings.
- `internal/bedrock/` loads embedded or test Bedrock configuration.
- `internal/keychain/` stores the shared Bedrock bearer token, with a versioned
  owner-only file fallback for tokens exceeding the 2560-byte Windows Credential
  Manager limit. Use the `*WithFallback` methods on real credential paths; the
  bare `Set`/`Get`/`Delete` are keychain-only.
- `internal/discovery/` is the only package importing `aws-sdk-go-v2`.
- `internal/safepath/` provides containment and owner-only filesystem operations.
- `npm/` contains the npm launcher and platform packages.

Provider-specific behavior belongs behind `provider.Provider`; avoid adding
CLI-name conditionals in `cmd/`. Add a provider by implementing the interface
and registering it in `internal/provider/provider.go`.

Provider config details belong in providers: Codex writes TOML under
`~/.codex/config.toml` using the built-in `amazon-bedrock` provider, OpenCode
writes JSON under `~/.config/opencode/opencode.json` using an OpenAI-compatible
provider block, and Grok writes user-scope-only TOML under `~/.grok/config.toml`
with `auth_provider_command` pointing at `juggernaut auth-token`.

## Engineering Constraints

- Reuse helpers in `cmd/helpers.go`; do not duplicate their behavior.
- Use `internal/safepath` for files beneath user-controlled base directories.
- Preserve unknown user configuration. For nested provider tables, use the
  provider's `DeepMergeKeys()` and `OwnedSubKeys()` contract.
- For Claude, Juggernaut manages native keys `env`, `model`, `modelOverrides`,
  `fallbackModel`, `effortLevel`, `alwaysThinkingEnabled`,
  `skipWebFetchPreflight`, and `permissions`; uninstall should remove only
  managed keys.
- Never install, overwrite, move, symlink over, or delete an unknown file whose
  name matches a managed CLI binary.
- Activation blocks for different CLIs must coexist in the same shell profile.
- The Bedrock bearer token is shared across providers. Uninstalling one
  non-Claude provider must not remove it, and neither may a Claude
  `--scope=project` removal while user-scope or non-Claude configs remain.
- Preserve compatibility for `launch`, `launch-cli`, hidden auth-token behavior,
  deprecated flags, and launched-CLI exit-code passthrough (`Execute()` exits
  with the wrapped CLI's own status; launch commands silence cobra error/usage
  output) unless a task explicitly includes a breaking change.
- `apply --auth` accepts exactly `iam` or `bedrock-api-key` and is validated
  before any state is written (`validateAuthFlag`), so typos can never produce
  a "successful" broken config.
- Claude user-scope apply persists only generated non-secret runtime state;
  project apply must not create global fallback state, bearer tokens must never
  be written there, and user-scope uninstall must remove it.
- Keep `VERSION`, `bedrock-config.json`'s `version`, and `cmd/root.go`'s
  `Version` synchronized.
- Go toolchain is pinned to `go 1.26.6` in `go.mod` (bumped for stdlib
  vulnerability fixes). Use that exact version; `go mod download` may
  reject newer patch versions.
- Do not commit generated binaries, coverage files, or other build artifacts.
- Mantle is opt-in for standard Bedrock Claude routing; non-Claude providers
  route through Mantle because native Bedrock is Claude-only.
- `apply --mode=auto` must keep the Bedrock auto-mode guardrails: it writes
  `CLAUDE_CODE_ENABLE_AUTO_MODE=1`, but should warn unless the resolved active
  model is Opus 4.7-or-later capable, including Opus 5.
- `--fallback-model`, `--service-tier`, `--always-thinking`, `--effort`, and
  `skipWebFetchPreflight` are managed settings; `max` and `auto` effort levels
  are env-only, while fixed levels also persist as native `effortLevel`.

## Testing

Add focused tests with each behavior change, then run the affected package
before the full suite. Use the existing helpers instead of rewriting boilerplate:
`internal/testhome.NewTestHome(t)` sets both `HOME` and `USERPROFILE` (Windows
paths need both), and `internal/testutil` provides `CaptureStdout`, `WithStdin`,
`NestedMapChain`, `ParseJSON`, `OwnedJuggernautBlock`, and `SkipIfNoKeychain`.
Isolate keychain tests with `JUGGERNAUT_KEYCHAIN_SERVICE`; tests should skip when
no backend is available.

For `cmd` command-level tests, reuse `cmd/apply_test_helpers_test.go` and the
`cmd/coverage_batch{1,2}_test.go` files instead of inventing new fixtures:
`setupApplyTest(t)` (fake home + mock PS runner on Windows), `setupApplyTestWithReset`,
`captureStdout`/`captureStderr`, `mockPSRunner`/`mockPSOutputJSON`,
`noHomeEnv`, `blockCredentialWrite`, `chdirTo`, `withBrokenEmbeddedConfig`,
`swapActiveModelsForWrite`, `writeTempBedrockConfig`, and the `stubProvider`
family. Quirks worth knowing:
- `homeDir()` failure branches are tested by clearing the env with `noHomeEnv`,
  and the error text is platform-specific (`$HOME` vs `%userprofile%`) — use
  `assertHomeDirError` rather than matching a substring like `"HOME"`.
- `models --write` tests must `chdirTo` a temp dir holding a `bedrock-config.json`
  copy so the repo-root `../bedrock-config.json` fallback is never mutated.
- Tests never call `t.Parallel()` — `os.Chdir` is a process-global side effect,
  so chdir-based tests are safe only in sequence.
- `cmd` has a `TestMain` subprocess harness (`cmd/launch_exitcode_test.go`):
  with `JUGGERNAUT_TEST_WRAPPER_CHILD=1` the test binary re-execs itself as
  the wrapped CLI so `Execute()`'s exit-status translation can be asserted
  from a parent. Never set that env var in other tests; new top-level env-var
  branches belong in that `TestMain` switch.
- The shared `captureStdout` drains only after `fn()` returns — captured
  output larger than the OS pipe buffer deadlocks. For large-output captures
  use a concurrent-drain helper like `captureStreaming`
  (`cmd/show_order_test.go`).

Tests never call AWS. `cmd/models.go` and `cmd/model_catalog.go` expose their
discovery calls as package-level function variables so tests can swap them;
preserve that seam when adding discovery-backed commands.

`cmd/golden_test.go` diffs Claude's apply output byte-for-byte against
`cmd/testdata/golden/`, so unintended output drift fails there.

Cobra global flag state persists between `ExecuteArgs()` calls. Use the existing
`resetFlags()` mechanism when adding or changing flags.

### Irreducible coverage floor

- New tests must target a real error path, platform-specific branch, previously
  untested public behavior, or a documented edge case. Pure line-coverage
  fillers with no new assertion or failure-mode coverage are rejected (no
  `_ = fn()` "should not crash" smokes).
- Prefer one strong test that exercises the real failure over multiple shallow
  ones.
- When adding or expanding a `coverage_gaps_*_test.go`, the PR description must
  state the exact branch/error it closes.
- Pruning zero-value coverage_gaps tests is fine, but never at the cost of
  measured coverage — verify with `go test <pkg> -coverprofile` before and
  after.

Keep cross-platform behavior in mind. CI runs race-covered Go tests on Linux,
macOS, and Windows, plus Windows lint, govulncheck, a separate race job, a
Linux-only coverage-floor job, npm tests, shellcheck, gosec, Trivy, CodeQL, a
GoReleaser snapshot build, and Socket. Merge-blocking checks are only `lint`, the
three `test (<os>)` legs, and the two Socket contexts — the rest, including
Codacy, are informational. The authoritative coverage gate is Codecov (80%
project and patch, in `codecov.yml`), not the lower in-CI Linux floor. Fork PRs
skip Codecov upload by design, so a missing Codecov status there is expected.

Before handing off a substantive change, run at least:

```bash
go test ./...
go vet ./...
git diff --check
```

Run `make ci` when the required local tools are available and the change merits
the broader check.

Static-analysis suppressions (`//nolint`, `#nosec`, `// nosemgrep`) are
documented with rationale in `docs/ci-suppressions.md`. Prefer a small code
adjustment over a suppression; never add one without documenting it there.

## Git and Scope

The worktree may contain user changes. Inspect `git status` and relevant diffs
before editing, preserve unrelated modifications, and do not rewrite history or
discard changes without explicit authorization.

Use conventional commit subjects when asked to commit. Release tags are `v*`
and trigger GitHub release publishing plus npm OIDC publish; do not create or
push a release tag unless explicitly requested.

Installed git hooks (`scripts/setup-hooks.{sh,ps1}`, sourced from `.githooks/`)
enforce more than conventional defaults and will fail your commit/push:

- `commit-msg` — first line must be a **single-type** Conventional Commit:
  `docs+test: ...` is rejected, `type(scope?): description` is required.
- `pre-commit` — fails if `go fmt` / `go mod tidy` would change staged Go files.
- `pre-push` — fails if a changed Go file gains a function at 0.0% coverage. An
  early warning only — CI's Codecov patch gate is the authority. Documented
  emergency bypass: `git push --no-verify`.

`main` is protected by a GitHub **ruleset**, not classic branch protection, so
`gh api repos/.../branches/main/protection` returns 404 — query
`gh api repos/jpvelasco/juggernaut/rulesets` instead. The active `protect-main`
ruleset blocks deletion and non-fast-forward, requires a PR with review-thread
resolution (0 approvals), enforces strict up-to-date status checks, and has **no
bypass actors** — the required checks apply to the owner too. A separate
`protect-version-tags` ruleset guards `v*`. Out-of-repo GitHub settings are
documented and re-appliable via `.github/GITHUB_SETTINGS.md`.

**Protected branches:** `legacy/v3` must never be deleted — it is required for
older releases. When cleaning up branches, skip any branch named `legacy/*`.
