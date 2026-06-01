# Juggernaut v4: Go Rewrite Design

## Overview

Rewrite Juggernaut from shell scripts (bash + PowerShell) to a single cross-platform Go binary. All existing features and flags are preserved. The shell complexity that existed due to platform divergence is eliminated — Go handles macOS, Linux, and Windows natively.

**Approach:** Simplified Go (Approach B) with a friendly first-run interactive prompt (hint of C).

**Migration path supported:** v3.2.3 → v4.0.0 only.

---

## Repository Structure

```
juggernaut/
├── main.go                          # 10 lines — delegates to cmd/root
├── cmd/
│   ├── root.go                      # Cobra root, version flag, persistent flags
│   ├── apply.go                     # juggernaut apply
│   ├── show.go                      # juggernaut show
│   ├── doctor.go                    # juggernaut doctor
│   ├── uninstall.go                 # juggernaut uninstall
│   ├── migrate.go                   # juggernaut migrate
│   └── version.go                   # juggernaut version
├── internal/
│   ├── bedrock/                     # bedrock-config.json loader (typed structs)
│   ├── schema/                      # JuggernautBlock builder + validator
│   ├── config/                      # settings.json atomic read/merge/write
│   ├── keychain/                    # go-keyring wrapper
│   ├── launcher/                    # claude wrapper binary writer
│   ├── migrate/                     # v3→v4 migration logic
│   └── doctor/                      # diagnostic helpers
├── scripts/
│   ├── install.sh                   # ~50-line bootstrap (OS/arch detect + curl binary)
│   └── install.ps1                  # ~40-line PowerShell equivalent
├── npm/
│   ├── package.json
│   └── install.js                   # postinstall: detect platform, download binary, verify checksum
├── bedrock-config.json              # unchanged — single source of truth
├── VERSION                          # unchanged — GoReleaser + CI read this
├── .goreleaser.yml
├── go.mod
└── go.sum
```

**Legacy files:** All shell-based source (`commands/`, `lib/`, `install.sh`, `install.ps1`, `juggernaut`, `juggernaut.ps1`, `tests/v2/`) move to the `legacy/v3` branch created from the `v3.2.3` tag. They are removed from `main`.

---

## Internal Package Design

### `internal/bedrock`
Loads `bedrock-config.json` into a typed `BedrockConfig` struct once at startup. All other packages consume this struct — no jq subprocess calls anywhere.

```go
type BedrockConfig struct {
    Version              string
    Models               ModelSet
    Environment          map[string]string
    EnvironmentBedrockAuth map[string]string
    Regions              []string
    Defaults             Defaults
}
```

### `internal/schema`
Builds and validates the `JuggernautBlock` that gets merged into `settings.json`. Takes a `BedrockConfig` + apply options, returns a validated typed block. Also derives the native keys (`.env`, `.model`, `.modelOverrides`) that Claude Code reads directly.

Schema version bumps from 1 → 2 for v4 (new field: `meta.goVersion`). Migration handles upgrading v1 blocks.

### `internal/config`
Atomic read/merge/write of `settings.json`. Uses `encoding/json` + `os.Rename` for atomicity. Backup rotation (5 most recent). File locking via `github.com/gofrs/flock` (cross-platform, replaces flock/mkdir-mutex split).

### `internal/keychain`
Thin wrapper around `github.com/zalando/go-keyring`. Service name: `juggernaut-bedrock`. The entire platform-detection tree from `lib/keychain.sh` collapses to ~5 lines. Windows long-key (>1280 char) DPAPI edge case handled via `golang.org/x/sys/windows` — same file location (`~/.juggernaut/bearer-token.dpapi.bin`), now written in Go.

### `internal/launcher`
Creates the `claude` wrapper using the busybox pattern — no embedded binary required:

- **Unix:** `~/.local/bin/claude` is a symlink to the `juggernaut` binary itself
- **Windows:** `%USERPROFILE%\.local\bin\claude.exe` is a copy of the `juggernaut` binary (Windows does not support symlinks without admin rights)

When invoked as `claude` (detected via `os.Args[0]`), the juggernaut binary executes wrapper behavior:
1. Reads bearer token via go-keyring
2. Sets `AWS_BEARER_TOKEN_BEDROCK` in process env
3. Sets `CLAUDE_CODE_USE_BEDROCK=1`
4. Execs the real `claude` binary (resolved by walking PATH, skipping itself)

One binary, zero embed complexity. Works in non-interactive shells, SSH sessions, scripts — anywhere. No shell profile modification required.

### `internal/migrate`
Detects and executes v3 → v4 migration. Idempotent — safe to run multiple times. Steps:
1. Detect existing v3 block in `settings.json` (`schemaVersion: 1`, `meta.managedBy == "juggernaut"`)
2. Detect bearer token in old keychain locations (keychain service `juggernaut-bedrock`, DPAPI file, profile token file)
3. Transfer token to go-keyring
4. Upgrade block schema v1 → v2
5. Install `claude` wrapper binary
6. Strip legacy shell launcher blocks from `~/.bashrc`, `~/.zshrc`, `~/.profile`, `~/.config/fish/config.fish` (marker: `# BEGIN: Juggernaut Launcher`)
7. Remove `~/.juggernaut/` install directory if empty after token transfer

If installed version is older than v3.2.3, migration aborts with an error directing user to upgrade to v3.2.3 first.

### `internal/doctor`
Diagnostic helpers consumed by `cmd/doctor.go`. Checks: block presence and schema validity, auth mode and credential availability, region validation, mantle status, opusplan wiring, launcher binary presence and PATH priority.

---

## Commands & Flags

All v3 flags preserved. Breaking changes: none.

### `juggernaut apply`
```
--auth=iam|bedrock-api-key    required on first run (interactive if omitted)
--bedrock-key=KEY             interactive if auth=bedrock-api-key and no key found
--preserve-key                reuse existing key from keychain/env
--region=REGION               default: us-west-2 (interactive on first run if omitted)
--model=ID                    override all models
--opus-model=ID
--sonnet-model=ID
--haiku-model=ID
--effort=low|medium|high|xhigh|max
--opusplan / --no-opusplan    interactive on first run
--1m-context / --no-1m-context
--no-mantle
--mantle-url=URL
--scope=user|project          default: user
--dry-run
--skip-preflight
```

Interactive prompt (via `github.com/charmbracelet/huh`) activates only when required flags are missing AND no existing config is found. Scripted/CI usage bypasses it entirely.

### `juggernaut show`
```
--scope=user|project          default: both
--json                        NEW: machine-readable output
```

### `juggernaut doctor`
```
--scope=user|project
--json                        NEW: machine-readable output
```

### `juggernaut uninstall`
```
--scope=user|project
--full                        also remove launcher binary and ~/.juggernaut
--force / -f
--dry-run
```

### `juggernaut migrate`
```
--dry-run                     preview what would change
```
Explicit migration command. Also runs implicitly (idempotently) on first run of any command when a v3 installation is detected.

### `juggernaut version`
```
--json                        NEW: {"version":"4.0.0","build":"..."}
```

---

## Migration Flow

### Implicit (existing users)
On first run of any command after installing v4, migration is detected and runs automatically:

```
Existing Juggernaut configuration detected (v3.2.3, bedrock-api-key auth).
Migrating to Juggernaut v4...

  ✓ Bearer token found in keychain — transferring to go-keyring
  ✓ Settings block found — upgrading schema v1 → v2
  ✓ Installing claude wrapper binary → ~/.local/bin/claude
  ✓ Removing legacy shell launcher blocks from ~/.bashrc, ~/.zshrc

Migration complete. No credentials were re-entered.
```

### Explicit (cautious users)
```bash
juggernaut migrate --dry-run   # preview
juggernaut migrate             # execute
```

### Version gate
If `meta.schemaVersion` or installed version indicates older than v3.2.3:
```
Legacy version detected (pre-v3.2.3). Please upgrade to v3.2.3 first:
  curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash
Then re-run: juggernaut migrate
```

### IAM users
No credential transfer needed. Migration is: schema upgrade + launcher swap only.

---

## Distribution

### GoReleaser targets
```
linux   amd64, arm64    → .tar.gz
darwin  amd64, arm64    → .tar.gz
windows amd64           → .zip
checksums.txt           → SHA256 for all artifacts
```

Version injected at build time: `-ldflags "-X github.com/jpvelasco/juggernaut/cmd.Version={{ .Version }}"`.

### Install path 1 — npm (primary)
```bash
npm install -g juggernaut
```
`npm/install.js` postinstall: detects `process.platform` + `process.arch` → downloads binary from GitHub Releases → verifies SHA256 against `checksums.txt` → places in package bin directory. npm handles PATH.

### Install path 2 — curl one-liner
```bash
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/latest/scripts/install.sh | bash
```
~50-line script: detect OS/arch → curl binary from GitHub Releases → verify checksum → drop in `~/.local/bin`. Never needs updating between releases (always pulls `latest` tag).

### Install path 3 — PowerShell one-liner
```powershell
irm https://raw.githubusercontent.com/jpvelasco/juggernaut/latest/scripts/install.ps1 | iex
```
~40-line equivalent of install.sh.

### Install path 4 — direct binary download
GitHub Releases page. GoReleaser publishes automatically on tag push.

### Release workflow
```
1. Bump VERSION file
2. Update bedrock-config.json (.version) + cmd/root.go Version var
3. CI verifies version sync across all 3 locations (fails on mismatch)
4. git tag v4.0.0 && git push --tags
5. GoReleaser: builds all targets, publishes GitHub Release with assets + checksums.txt
6. CI: npm publish (juggernaut package points to new GitHub Release)
```

---

## Legacy Handling

- `v3.2.3` tag: permanent, all existing install URLs continue working forever
- `legacy/v3` branch: created from `v3.2.3` tag, README gets deprecation banner
- GitHub Release `v3.2.3`: deprecation notice added pointing to v4+
- `main` branch: clean Go from v4.0.0 onward

---

## Key Dependencies

```
github.com/spf13/cobra              CLI framework (same as ludus)
github.com/charmbracelet/huh        Interactive prompts (first-run only)
github.com/zalando/go-keyring       Cross-platform keychain
github.com/gofrs/flock              Cross-platform file locking
golang.org/x/sys/windows            DPAPI for Windows long-key edge case
```

---

## Version Sync Locations (v4)

| Location | Field |
|---|---|
| `VERSION` | plaintext semver |
| `bedrock-config.json` | `.version` |
| `cmd/root.go` | `var Version = "4.0.0"` (dev fallback; overridden by ldflags at build) |

CI fails if any mismatch. Three locations instead of four (PowerShell `schema.ps1` is gone).

---

## Testing

- Unit tests for each `internal/` package (Go standard `testing`)
- Integration tests for `apply`, `doctor`, `show`, `uninstall`, `migrate` using temp HOME directories
- Keychain tests: `JUGGERNAUT_KEYCHAIN_SERVICE` env var override preserved for test isolation
- CI: lint (`golangci-lint`) → test (ubuntu, macos, windows) — same parallel structure as today
- `make test`, `make lint` preserved as Makefile targets
