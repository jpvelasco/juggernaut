# Repository Guidelines

## Project

Juggernaut is a cross-platform Go CLI that configures Claude Code, Codex,
OpenCode, and Grok to use Amazon Bedrock. The binary embeds
`bedrock-config.json`; rebuild after changing that file.

Read `CLAUDE.md` for the detailed architecture, provider inventory, managed
configuration keys, and current CI/release design. Keep that document accurate
when architecture or workflows change.

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
```

On Windows, PowerShell is the default shell. Keep scripts cross-platform unless
they are explicitly platform-specific.

## Architecture

- `main.go` embeds the Bedrock config and calls `cmd.Execute()`.
- `cmd/` contains Cobra commands, hidden launch/auth commands, and shared helpers.
- `internal/provider/` owns the multi-CLI provider interface and implementations
  for `claude`, `codex`, `opencode`, and `grok`.
- `internal/config/` handles atomic JSON/TOML merge, removal, locking, and backups.
- `internal/activation/` manages marked shell blocks and launches real CLI binaries
  via `launch`/`launch-cli` while avoiding wrapper recursion.
- `internal/schema/` builds and validates Claude-specific managed settings.
- `internal/bedrock/` loads embedded or test Bedrock configuration.
- `internal/keychain/` stores the shared Bedrock bearer token.
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
  non-Claude provider must not remove it.
- Preserve compatibility for `launch`, `launch-cli`, hidden auth-token behavior,
  and deprecated flags unless a task explicitly includes a breaking change.
- Keep `VERSION`, `bedrock-config.json`'s `version`, and `cmd/root.go`'s
  `Version` synchronized.
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
before the full suite. Tests that modify home-directory state should use
`t.TempDir()` and `t.Setenv("HOME", ...)`. Isolate keychain tests with
`JUGGERNAUT_KEYCHAIN_SERVICE`; tests should skip when no backend is available.

Cobra global flag state persists between `ExecuteArgs()` calls. Use the existing
`resetFlags()` mechanism when adding or changing flags.

Keep cross-platform behavior in mind. CI runs race-covered Go tests on Linux,
macOS, and Windows, plus a separate race job, coverage-threshold job, npm tests,
shellcheck, gosec, CodeQL, and GoReleaser draft checks.

Before handing off a substantive change, run at least:

```bash
go test ./...
go vet ./...
git diff --check
```

Run `make ci` when the required local tools are available and the change merits
the broader check.

## Git and Scope

The worktree may contain user changes. Inspect `git status` and relevant diffs
before editing, preserve unrelated modifications, and do not rewrite history or
discard changes without explicit authorization.

Use conventional commit subjects when asked to commit. Release tags are `v*`
and trigger GitHub release publishing plus npm OIDC publish; do not create or
push a release tag unless explicitly requested.

**Protected branches:** `legacy/v3` must never be deleted — it is required for
older releases. When cleaning up branches, skip any branch named `legacy/*`.
