# Known CI Suppressions

Every static-analysis suppression (`//nolint`, `#nosec`, `// nosemgrep`) left in
the repository is listed here with a one-line rationale. Policy: prefer a small
code adjustment that makes the rule happy; keep a suppression only when the code
is correct as written and the rule cannot express that. **Never add a new
suppression without adding it here.**

Counts were last verified during the Codacy re-assessment on 2026-09-20 and the
post-merge reanalysis on 2026-09-22: the 29 cloud findings clear as follows —
14 via the 16 test-fixture permission-mode lines tightened to the repo policy
(`0o755` dirs → `0o700`, `0o644` files → `0o600`; the `mkdir` and `fileperm`
rule families accept those modes), 8 via suppressions added on 7 lines
(`codex_version.go` is double-flagged by both exec rules; the
`codex_version_test.go` stub keeps `0o755` for the `isExecutable` gate and is
never run), and 7 via the standard `incorrect-default-permission` nosemgrep on
the `MkdirAll(..., 0o700)` fixture lines (that rule fires on any permission
above `0o600`, and a directory must keep the owner-exec bit — matching the ~50
other `MkdirAll 0o700` fixture sites).
`TestCodexBinaryVersion_RealProbe` (POSIX-only) exercises the real probe so the
`codex_version.go` exec line stays covered (Codecov patch gate). The
`isLegacyClaudeShim` deletion also removes a stale `nolint:unused`. Keep these
current when adding or removing suppressions.

Counts were re-verified during the Codacy checks pass on 2026-10-27: 11 open
dashboard findings and 6 local-only sites — 4 `WriteFile 0o700` executable-stub
lines still carrying the 2026-07 dashboard dismissals plus the same-file dedup
twins Codacy collapses by line text (`apply_profile_test.go:453`,
`doctor_profile_test.go:21`) — clear via 16 new annotations: the standard
directory nosemgrep on 7 `MkdirAll 0o700` sites, the fixture nosemgrep on 3
`ReadFile` sites, `go_sql_rule-concat-sqli` on the two `url.URL.Query()` lines,
the fileperm stub annotation on the 4 `WriteFile 0o700` lines, and the exec pair
on `codex_launch.go` (same resolved-binary + constant-args shape as
`codexVersionProbe`). No dashboard dismissals were needed. `Lizard` was also
removed from `.codacy/codacy.config.json` (cloud-metrics only, breaks local
runs) so local Opengrep analysis works.

## Fixed instead of suppressed

- `internal/provider/base.go` — two `//nolint:staticcheck` on the deprecated
  `strings.Title` replaced with a `titleCaseASCII` helper (equivalent for the
  single-word ASCII CLI names that reach the fallback; multi-word display
  names use the `displayName` field).
- `internal/activation/activation_test.go` — `//nolint:unused` removed with the
  genuinely unused `executableFixture` test helper it annotated.
- `internal/activation/artifact.go` — `isLegacyClaudeShim` and its
  `//nolint:unused` deleted: the v4.2.6 shim content is unreachable now that
  v6 routes via `bedrock-runtime`; its test subtest was removed with it.
- `cmd/uninstall_token_test.go`, `cmd/doctor_test.go` — 16 fixture lines
  tightened to the repo policy (`MkdirAll 0o755` → `0o700`, `WriteFile 0o644` →
  `0o600`). Test homes are fake (`setupApplyTest`/`NewTestHome`); Windows
  ignores these bits. The `file-permissions` (fileperm) and `mkdir` findings
  clear by the mode change alone; the `incorrect-default-permission` findings
  on the `MkdirAll 0o700` lines (the rule fires above `0o600`) carry the
  standard directory nosemgrep below.

## Remaining suppressions

### Production Go code

| Location | Suppression | Rule | Rationale |
| --- | --- | --- | --- |
| `internal/activation/artifact.go:59` | `#nosec G703,G501` + `nosemgrep go_filesystem_rule-fileread` | file-read of a variable path | Reads the v4.2.6 shim candidate resolved from `binDir` + fixed name lists; read-only, errors handled. |
| `internal/activation/launch.go:209,221` | `#nosec G703` | os.Stat on variable path | Candidates come from PATH/known config paths; both Stat errors are handled. |
| `internal/activation/launch.go:324` | `nosemgrep dangerous-exec-command, go_subproc_rule-subproc` | exec with a variable command | Executes the real CLI binary resolved by `resolveBinaryFrom`, which skips the Juggernaut binary itself and known v4.2.6 artifacts; a fixed name would break the launch contract. |
| `internal/activation/powershell_discovery.go:52` | `nosemgrep dangerous-exec-command` | exec with a variable command | `exe` comes from a fixed candidate list (`pwsh.exe`/`powershell.exe`) with fixed script args; the selection loop requires a variable. |
| `internal/keychain/crypter_windows.go:53,67,77` | `#nosec G115,G103` + `nosemgrep use-of-unsafe-block` | integer conversion / unsafe | DPAPI `DATA_BLOB` marshalling requires `unsafe`; sizes are bounded by the keychain limit well under 4GB. |
| `internal/provider/sidecar.go` (`readSidecarBlock`) | `#nosec G304` | file-read of a variable path | Paths are provider-derived sidecar locations (`.juggernaut.json` next to the provider config), never user input; read-only and every error path is the documented "absent" outcome. |
| `cmd/codex_version.go` (`codexVersionProbe`) | `#nosec G204` + `nosemgrep dangerous-exec-command, go_subproc_rule-subproc` | exec with a variable command | `path` is the codex binary resolved from a fixed binary-name list on PATH; args are the constant `"--version"` with no shell. |
| `internal/provider/codex_launch.go` (`prepareCodexLaunch`) | `nosemgrep dangerous-exec-command, go_subproc_rule-subproc` | exec with a variable command | `path` is the codex binary the activation launcher resolved (`LaunchSpec.PrepareArgs` receives `binPath`, which skips the Juggernaut binary and legacy artifacts); args are the constant `"--help"` with no shell — mirrors `codexVersionProbe`. |

### npm launcher (`npm/index.js`, `npm/index.test.js`)

`nosemgrep path-traversal` / `detect-non-literal-fs-filename` on path joins and
fs calls. All are false positives by construction: package names pass the
`VALID_PACKAGES` allowlist before any join, `safeResolveBin` realpaths and
asserts containment under the owning platform package dir, staging uses
`fs.mkdtempSync` with a constant prefix plus `COPYFILE_EXCL`, and test
fixtures build their trees under `fs.mkdtempSync(os.tmpdir())` roots.

### Test-only Go suppressions (~95 sites)

- `// nosemgrep go.lang.correctness.permissions.file_permission.incorrect-default-permission`
  on `os.MkdirAll(..., 0o700)` — the permission is correct for directories and
  the paths live under `t.TempDir()`/test homes; the rule assumes 0o755 defaults.
- `// nosemgrep go_filesystem_rule-fileread` on `os.ReadFile` — tests read
  fixture paths they just wrote under temp dirs.
- `#nosec G101` (env var names / test-only tokens) in `launch_cli_test.go` and
  `helpers_test_phases_test.go` — `"AWS_BEARER_TOKEN_BEDROCK"` is an env var
  *name*, not a credential.
- `//nolint:gosec` in `helpers_test_phases_test.go:1466,2018` (test-only
  `os.WriteFile` fixtures) and `internal/config/write_test.go:66` (intentional
  `0o555` dir to exercise the write-failure path).
- `// nosemgrep go_filesystem_rule-fileread` on `os.ReadFile` of test-created
  fixtures in `cmd/launch_test.go` (`parseJSONForTest`),
  `cmd/apply_collision_test.go` (pre-write backup glob match), and
  `internal/config/backup_rotation_test.go` (backup glob matches).
- `cmd/launch_exitcode_test.go` - `#nosec G204` + `nosemgrep go_subproc_rule-subproc,dangerous-exec-command` on the wrapper-child harness spawning `os.Executable()` (the test binary itself) so exit-code propagation through `Execute()` can be asserted.
- `// nosemgrep go_sql_rule-concat-sqli` on `url.URL.Query()` in
  `internal/discovery/token_test.go` — net/url query-string parsing of a minted
  Bedrock token, not a database call; the cloud Semgrep rule matches the
  method name.
- `cmd/helpers_test_phases_test.go` — `// nosemgrep go_file-permissions_rule-fileperm`
  on the four `os.WriteFile(..., 0o700)` POSIX shell-stub lines (the fake
  `juggernaut` binary and the `claude` backup it restores): the stubs model
  executable binaries, so they keep the owner-exec bit. These were dismissed on
  the dashboard in 2026-07 and now carry in-code annotations so local Opengrep
  runs stay green.
- Executable test stubs written `0o755` (`cmd/launch_exitcode_test.go`, `cmd/codex_version_test.go` — both the faked-probe stub and the real-probe stub in `TestCodexBinaryVersion_RealProbe`, `internal/activation/auth_modes_degrade_test.go`) - `#nosec G306` + `nosemgrep fileperm/incorrect-default-permission`; POSIX shell stubs must be executable for the resolution/launch pipeline's `isExecutable` gate to accept them. The `stubCodexProbe` stub is never executed (the probe is swapped for a fake); `TestCodexBinaryVersion_RealProbe`'s stub is executed — that is the point (it covers the real probe).
- `internal/config/write_test.go` and `cmd/helpers_test_phases_test.go` - `nosemgrep mkdir/fileperm/incorrect-default-permission` alongside the existing `//nolint:gosec` on the intentional read-only dir and its cleanup chmod restore.
