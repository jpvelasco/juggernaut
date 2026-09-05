# Known CI Suppressions

Every static-analysis suppression (`//nolint`, `#nosec`, `// nosemgrep`) left in
the repository is listed here with a one-line rationale. Policy: prefer a small
code adjustment that makes the rule happy; keep a suppression only when the code
is correct as written and the rule cannot express that. **Never add a new
suppression without adding it here.**

Counts were last verified during the cleanup in PR #371; keep them current when
adding or removing suppressions.

## Fixed instead of suppressed (PR #371)

- `internal/provider/base.go` — two `//nolint:staticcheck` on the deprecated
  `strings.Title` replaced with a `titleCaseASCII` helper (equivalent for the
  single-word ASCII CLI names that reach the fallback; multi-word display
  names use the `displayName` field).
- `internal/activation/activation_test.go` — `//nolint:unused` removed with the
  genuinely unused `executableFixture` test helper it annotated.

## Remaining suppressions

### Production Go code

| Location | Suppression | Rule | Rationale |
| --- | --- | --- | --- |
| `internal/activation/artifact.go:59,97` | `#nosec G703,G501` + `nosemgrep go_filesystem_rule-fileread` | file-read of a variable path | Reads candidate v4.2.6 shim paths resolved from `binDir` + fixed name lists; read-only, errors handled. |
| `internal/activation/launch.go:212,224` | `#nosec G703` | os.Stat on variable path | Candidates come from PATH/known config paths; both Stat errors are handled. |
| `internal/activation/launch.go:297` | `nosemgrep dangerous-exec-command` | exec with a variable command | Executes the real CLI binary resolved by `resolveBinaryFrom`, which skips the Juggernaut binary itself and known v4.2.6 artifacts; a fixed name would break the launch contract. |
| `internal/activation/powershell_discovery.go:52` | `nosemgrep dangerous-exec-command` | exec with a variable command | `exe` comes from a fixed candidate list (`pwsh.exe`/`powershell.exe`) with fixed script args; the selection loop requires a variable. |
| `internal/keychain/crypter_windows.go:53,67,77` | `#nosec G115,G103` + `nosemgrep use-of-unsafe-block` | integer conversion / unsafe | DPAPI `DATA_BLOB` marshalling requires `unsafe`; sizes are bounded by the keychain limit well under 4GB. |
| `internal/provider/sidecar.go` (`readSidecarBlock`) | `#nosec G304` | file-read of a variable path | Paths are provider-derived sidecar locations (`.juggernaut.json` next to the provider config), never user input; read-only and every error path is the documented "absent" outcome. |

### npm launcher (`npm/index.js`, `npm/index.test.js`)

`nosemgrep path-traversal` / `detect-non-literal-fs-filename` on path joins and
fs calls. All are false positives by construction: package names pass the
`VALID_PACKAGES` allowlist before any join, `safeResolveBin` realpaths and
asserts containment under the owning platform package dir, staging uses
`fs.mkdtempSync` with a constant prefix plus `COPYFILE_EXCL`, and test
fixtures build their trees under `fs.mkdtempSync(os.tmpdir())` roots.

### Test-only Go suppressions (~45 sites)

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
- `internal/activation/artifact.go:92` `//nolint:unused` — `isLegacyClaudeShim`
  is retained for future v4.2.6 artifact recovery; covered by tests, not yet
  called from production code.
- `cmd/launch_exitcode_test.go` - `#nosec G204` + `nosemgrep go_subproc_rule-subproc,dangerous-exec-command` on the wrapper-child harness spawning `os.Executable()` (the test binary itself) so exit-code propagation through `Execute()` can be asserted.
- Executable test stubs written `0o755` (`cmd/launch_exitcode_test.go`, `internal/activation/auth_modes_degrade_test.go`) - `#nosec G306` + `nosemgrep fileperm/incorrect-default-permission`; POSIX shell stubs must be executable for the launch pipeline to resolve and run them.
- `internal/config/write_test.go` and `cmd/helpers_test_phases_test.go` - `nosemgrep mkdir/fileperm/incorrect-default-permission` alongside the existing `//nolint:gosec` on the intentional read-only dir and its cleanup chmod restore.
