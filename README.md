<p align="center">
  <img src="docs/logo.png" alt="Juggernaut" width="200">
</p>

<h1 align="center">Juggernaut</h1>

<p align="center"><strong>Claude Code Bedrock Setup</strong></p>

**One-command setup for Claude Code with Amazon Bedrock using Global CRIS inference profiles.**

## What This Does

Configures Claude Code to use Amazon Bedrock instead of Anthropic's direct API, with optimized settings for enterprise use:

- **Global CRIS**: Primary model uses cross-region inference for better availability
- **Optimized Tokens**: Bedrock-specific token limits (32768 output, 65536 thinking)
- **Cost Control**: Route through your AWS account for billing/governance
- **Enterprise Ready**: Works with AWS SSO, IAM roles, and corporate identity providers

## Why Bedrock?

| Feature | Direct Anthropic API | Amazon Bedrock |
|---------|---------------------|----------------|
| **Billing** | Separate Anthropic account | Consolidated AWS billing |
| **Authentication** | API keys only | IAM, SSO, roles, federation |
| **Data Residency** | Anthropic infrastructure | Your chosen AWS region |
| **Compliance** | Anthropic's certifications | AWS compliance (SOC, HIPAA, FedRAMP, etc.) |
| **Network** | Public internet | VPC endpoints, PrivateLink |
| **Governance** | Limited controls | IAM policies, CloudTrail, quotas |
| **Cost Tracking** | Anthropic console | AWS Cost Explorer, tags, budgets |

**Choose Bedrock if you:**
- Need consolidated billing through AWS
- Require enterprise SSO/identity federation
- Have data residency or compliance requirements
- Want to use existing AWS governance (IAM, CloudTrail)
- Need private network connectivity (VPC)

**Stick with direct API if you:**
- Want the simplest setup (just an API key)
- Don't have AWS infrastructure
- Are exploring/prototyping individually

## Prerequisites

1. AWS account with Bedrock access enabled
2. Access to Claude models (Opus 4.7, Sonnet 4.6, Haiku 4.5) in Bedrock
3. Claude Code installed (`npm install -g @anthropic-ai/claude-code`)
4. Valid AWS credentials (`aws configure` or SSO)
5. Bash 4.0+ (macOS users: `brew install bash`) or PowerShell 5.1+
6. `jq` (for JSON parsing — the installer checks this automatically)

## Install

Juggernaut v3 ships as a **destructive wipe-and-reinstall**: the installer strips any legacy Juggernaut/Claude-Code-Bedrock blocks from your shell profiles, removes the `juggernaut` key from `~/.claude/settings.json`, and deletes the bearer token storage (OS keychain on macOS/Windows for short keys, per-user DPAPI file at `~/.juggernaut/bearer-token.dpapi.bin` on Windows for long keys >1280 chars, profile file on Linux) **before** placing fresh files. Re-running the installer is the only supported upgrade path — there are no migration scripts, no deprecation windows, no `--yes` flag.

The installer does **not** auto-apply. You run `juggernaut apply` explicitly after install with a chosen auth mode.

**Pin a release tag for reproducible installs.**

> **Windows Defender / AMSI note:** earlier versions advertised `& ([scriptblock]::Create((irm ...)))`. Defender's fileless-malware heuristics flag that pattern regardless of script content. v3.0.4 switches to a download-then-run one-liner — `irm -OutFile`, `Unblock-File`, then execute the file on disk. Same one-line UX; matches Microsoft's own pattern for `dotnet-install.ps1`.

```bash
# Unix / macOS / Linux / Git Bash / WSL
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.sh | bash -s -- --version v3.1.0
```

```powershell
# Windows PowerShell (5.1 or 7) — Defender-friendly download-then-run
$u='https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.ps1'; $p="$env:TEMP\juggernaut-install.ps1"; irm $u -OutFile $p; Unblock-File $p; & $p -Version v3.1.0; Remove-Item $p
```

**Preview what the wipe will remove (no writes):**

```bash
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.sh | bash -s -- --version v3.1.0 --dry-run
```

```powershell
$u='https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.1.0/install.ps1'; $p="$env:TEMP\juggernaut-install.ps1"; irm $u -OutFile $p; Unblock-File $p; & $p -Version v3.1.0 -DryRun; Remove-Item $p
```

**Manual clone:**

```bash
git clone --branch v3.1.0 --depth 1 https://github.com/jpvelasco/juggernaut.git && cd juggernaut
./juggernaut apply --auth=iam
```

The installer installs to `$HOME/.juggernaut` by default (override with `JUGGERNAUT_DIR`) and places a launcher at `$HOME/.local/bin/juggernaut`. Add that directory to your `PATH` to invoke `juggernaut` from anywhere.

Both scripts normalize the version automatically — `3.0.2` and `v3.0.2` both work.

### Launcher wrappers

The installer also places a **claude launcher** so fresh shells pick up your bearer token automatically. On both platforms the launcher is a **shell function** that intercepts the `claude` command — it resolves before binaries on PATH, so it survives Anthropic's `claude update` self-rewrites that would clobber a symlink:

- **Unix / macOS / Linux / Git Bash**: a bracketed `claude() { ... }` function block is appended to your `~/.bashrc`, `~/.zshrc`, and/or `~/.profile` (only files that already exist; if none do, the matching rc for `$SHELL` is seeded). The function reads `AWS_BEARER_TOKEN_BEDROCK` from Juggernaut bearer token storage (macOS keychain, Linux profile token file, or Windows DPAPI/keychain when running under Git Bash) via `bearer_token_get` in `lib/keychain.sh`, exports it, and runs the real binary via `command claude "$@"`.
- **Windows PowerShell**: a `function claude { ... }` block is written to `$PROFILE.CurrentUserCurrentHost` (and the sibling host's profile when present). PowerShell resolves functions before applications on the command name `claude`, so the function intercepts the call, reads Windows Credential Manager or the DPAPI file, and invokes the real `claude.exe`.

This closes a v3.0.0 gap: `settings.json` flips `CLAUDE_CODE_USE_BEDROCK=1`, but Claude Code reads the bearer token only from its own process env. Without the launcher, a fresh shell with the token only in Juggernaut storage would hang. With the launcher, the hand-off happens automatically — and because it's a shell function, not a file on disk, Anthropic's own installer (which writes `~/.local/bin/claude` directly) is never fighting us.

**To skip the launcher** (set the token yourself): export `AWS_BEARER_TOKEN_BEDROCK` before running `claude`. The launcher preserves any pre-set value and only injects from Juggernaut bearer token storage when the env var is unset.

**Editor / IDE note:** tools that invoke `claude.exe` directly (bypassing both interactive shells and the PowerShell profile function) won't get the injection and must set the env var themselves.

### Windows Notes

Juggernaut v3 supports Windows PowerShell 5.1 and PowerShell 7 side-by-side:

- The installer scans both PowerShell 5.1 (`Documents\WindowsPowerShell\profile.ps1`) and PowerShell 7 (`Documents\PowerShell\profile.ps1`) paths, plus `$PROFILE.CurrentUserAllHosts` and `$PROFILE.AllUsersAllHosts` from both editions.
- If your `Documents\` folder is redirected by OneDrive, `$PROFILE.*` resolves correctly under `%OneDrive%\Documents\...` — the installer follows that redirection automatically.
- Non-admin users: the `AllUsers` profile strip and any other All-Users writes may require elevation. If they do, the installer **warns and skips** those paths (not fails). `CurrentUser` paths, the settings.json removal, and the keychain delete all work without elevation.
- Windows stores the bearer token via Credential Manager for short keys (~≤1280 chars) or, for long-form keys (>1280 chars), a per-user DPAPI-encrypted file at `~/.juggernaut/bearer-token.dpapi.bin`. Both are per-user; neither is synced across machines.
- First-run script policy friction: `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser`.

## Configure

After install, pick an auth mode and run `juggernaut apply` explicitly. v3 refuses to write `CLAUDE_CODE_USE_BEDROCK=1` to `~/.claude/settings.json` unless a valid auth source is present (`aws sts get-caller-identity` succeeds, `$AWS_BEARER_TOKEN_BEDROCK` is set, or Juggernaut bearer token storage exists). This prevents the "installer silently routed me through Bedrock with no credentials" class of hang.

**IAM / SSO (recommended):**

```bash
aws sso login --profile=<your-profile>          # or: aws configure
export AWS_PROFILE=<your-profile>

juggernaut apply --auth=iam
```

```powershell
# Windows PowerShell
.\juggernaut.ps1 apply -Auth iam
```

**Bedrock API key — three input methods:**

```bash
# Recommended: pipe the key in (paste-safe, nothing in terminal scrollback)
echo "$BEDROCK_KEY" | juggernaut apply --auth=bedrock-api-key

# Interactive: visible prompt at the terminal (paste-safe on all platforms)
juggernaut apply --auth=bedrock-api-key

# Inline (CI/CD only): key visible in process list / shell history
juggernaut apply --auth=bedrock-api-key --bedrock-key=br-xxxxxxxxxxxx
```

The key is stored in the OS keychain (`juggernaut-bedrock` target) on macOS/Windows for short keys (~≤1280 chars), per-user DPAPI file on Windows for long keys (>1280 chars), or in a profile file on Linux.

> **Paste truncation note:** On Linux/macOS under tmux, screen, or SSH, pasting into `read -s` prompts can silently truncate the key. The piped form avoids this entirely. Juggernaut rejects keys under 40 characters with an error that steers you to the pipe form.

| Type | Duration | Use Case |
|------|----------|----------|
| Short-term | Up to 12 hours | Production (recommended) |
| Long-term | Up to 30 days | Exploration/testing only |

**Custom region (default: us-west-2):**

```bash
juggernaut apply --auth=iam --region=us-east-1
```

**Custom models:**

```bash
juggernaut apply --auth=iam --opus-model=us.anthropic.claude-opus-4-7
juggernaut apply --auth=iam --sonnet-model=eu.anthropic.claude-sonnet-4-6
juggernaut apply --auth=iam --haiku-model=ap.anthropic.claude-haiku-4-5-20251001-v1:0
juggernaut apply --auth=iam --model-prefix=us
```

**OpusPlan and effort level:**

```bash
juggernaut apply --auth=iam --opusplan                # Opus in /plan, Sonnet in execute
juggernaut apply --auth=iam --effort=xhigh            # low | medium | high | xhigh (default) | max
```

**1M context windows** (Opus uses its native 1M; enable on Sonnet with):

```bash
juggernaut apply --auth=iam --1m-context
juggernaut apply --auth=iam --no-1m-context           # revert
```

**Mantle routing** is on by default in v3. Disable with:

```bash
juggernaut apply --auth=iam --no-mantle
juggernaut apply --auth=iam --mantle-url=https://mantle.example.internal
```

**Scope and preview:**

```bash
juggernaut apply --auth=iam --scope=user              # ~/.claude/settings.json (default)
juggernaut apply --auth=iam --scope=project           # ./.claude/settings.json
juggernaut apply --auth=iam --dry-run                 # preview without writing
```

**Verify:**

```bash
juggernaut doctor
claude                                                 # launch
```

A healthy bearer-token setup looks like:

```text
Credentials
  Auth: Bedrock API key
  Source: AWS_BEARER_TOKEN_BEDROCK
  Status: OK

Mantle
  Status: enabled
  Reason: Bedrock API key detected

Opusplan
  Status: enabled
  Status: OK
```

## Commands

| Command | Description |
|---------|-------------|
| `apply` | Write Juggernaut config to `settings.json`. Requires `--auth=iam` or `--auth=bedrock-api-key` on first run. |
| `show` | Print the current Juggernaut block from both user and project scopes. |
| `doctor` | Read-only diagnostics — checks credentials, region, models, Mantle, and opusplan drift. |
| `uninstall` | Remove the Juggernaut block from settings.json (all scopes by default) and delete the bearer token storage entry (OS keychain on macOS/Windows for short keys, per-user DPAPI file on Windows for long keys >1280 chars, profile file on Linux). |

## Configuration Details

Juggernaut writes **only** to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). v3 does not write to shell profiles. The `juggernaut` key holds Juggernaut's own state; the `env` block holds the Claude Code environment variables:

- `CLAUDE_CODE_USE_BEDROCK=1` — gated behind explicit auth validation
- `AWS_REGION=us-west-2` — default region
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS=32768`
- `MAX_THINKING_TOKENS=65536`
- `ANTHROPIC_MODEL=global.anthropic.claude-sonnet-4-6` — primary model (Global CRIS)
- `DISABLE_ERROR_REPORTING=1`, `DISABLE_TELEMETRY=1`, `DISABLE_AUTOUPDATE=1`, `DISABLE_BUG_COMMAND=1`
- `ANTHROPIC_DEFAULT_OPUS_MODEL` / `SONNET` / `HAIKU` — `/model` picker entries
- `ENABLE_PROMPT_CACHING_1H=1` — Bedrock 1-hour prompt caching

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|-------------------|
| **Primary** | Claude Sonnet 4.6 | `global.anthropic.claude-sonnet-4-6` |
| **Opus** | Claude Opus 4.7 | `global.anthropic.claude-opus-4-7` |
| **Fast** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

### Model Capabilities

| Feature | Opus 4.7 | Sonnet 4.6 | Haiku 4.5 |
|---------|----------|------------|-----------|
| Effort levels | Yes | Yes | No |
| Max effort | Yes | No | No |
| Extended thinking | Yes | Yes | No |
| Adaptive thinking | Yes | Yes | No |
| Interleaved thinking | Yes | Yes | No |

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                      juggernaut apply                               │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                 ┌─────────────────┴─────────────────┐
                 ▼                                   ▼
┌───────────────────────────────┐   ┌───────────────────────────────┐
│   commands/apply.sh           │   │   commands/apply.ps1          │
│   (Unix/macOS/Linux/WSL)      │   │   (Windows PowerShell)        │
└───────────────────────────────┘   └───────────────────────────────┘
                 │                                   │
                 └─────────────────┬─────────────────┘
                                   ▼
                 ┌─────────────────────────────────────┐
                 │       bedrock-config.json           │
                 │   (single source of truth for       │
                 │    env vars, regions, defaults)     │
                 └─────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   ~/.claude/settings.json                           │
│                                                                     │
│  {                                                                  │
│    "juggernaut": { … state … },                                     │
│    "env":        { … Claude Code env vars … }                       │
│  }                                                                  │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Claude Code → Amazon Bedrock                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Key Design Decisions:**
- **Single output target**: `~/.claude/settings.json`. No shell-profile fallback. No triple-write drift.
- **Single config source**: `bedrock-config.json` — consumed by both Bash and PowerShell.
- **Auth-gated writes**: `CLAUDE_CODE_USE_BEDROCK=1` only lands when auth is verified.
- **Atomic writes**: `config_manager` handles backup rotation (5 backups retained), file locking, and mode preservation.
- **Destructive installer**: wipe-and-reinstall on every run; there is no in-place upgrade.

## Files

- `juggernaut` / `juggernaut.ps1` — CLI entry point
- `commands/` — subcommand implementations (`apply`, `show`, `doctor`, `uninstall`)
- `lib/` — shared libraries (`schema`, `config_manager`, `keychain`, `doctor`, `profile_paths`)
- `install.sh` / `install.ps1` — wipe-and-reinstall installers
- `bedrock-config.json` — single source of truth for env vars, regions, and defaults
- `tests/v2/` — bash and Pester test suites

## Troubleshooting

### Run diagnostics first

```bash
juggernaut doctor
```

### Check environment variables

```bash
echo $CLAUDE_CODE_USE_BEDROCK
echo $AWS_REGION
```

### Verify AWS credentials

```bash
aws sts get-caller-identity
```

### List available Bedrock models

```bash
aws bedrock list-foundation-models --region us-west-2 --by-provider anthropic
```

### Authentication Precedence

Claude Code honors the following precedence (this is Claude Code behavior, not Juggernaut's):

| Priority | Credential Source | Notes |
|----------|-------------------|-------|
| 1 (highest) | `AWS_BEARER_TOKEN_BEDROCK` | API key always takes precedence if set |
| 2 | Environment variables | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` |
| 3 | AWS credentials file | `~/.aws/credentials` |
| 4 | AWS config/SSO | `~/.aws/config` with profiles |

If you manually set `AWS_BEARER_TOKEN_BEDROCK` in your own shell profile and it expires, Claude Code will hang. Juggernaut itself does not set that variable; unset it and re-run if you hit this.

### Common Issues

1. **"API Error: exceeded token maximum"** — restart terminal or `source ~/.zshrc`
2. **`juggernaut apply` exits with "auth validation required"** — pass `--auth=iam` or `--auth=bedrock-api-key` explicitly
3. **Authentication errors** — re-authenticate: `aws sso login --profile=<profile>`
4. **Region errors** — verify model availability; try `us-east-1` or `us-west-2`
5. **PowerShell execution policy error** — `Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser`
6. **"jq is required"** — `brew install jq` / `sudo apt install jq` / `winget install jqlang.jq`

## Uninstall

### Remove Juggernaut configuration

```bash
# Preview what will be removed
juggernaut uninstall --dry-run

# Remove the Juggernaut block from settings.json, bearer token storage,
# and the Claude launcher shell/profile block
juggernaut uninstall

# Limit to one scope
juggernaut uninstall --scope=user
juggernaut uninstall --scope=project
```

"juggernaut uninstall" removes the "juggernaut" key from settings.json, deletes the bearer token storage entry (OS keychain on macOS/Windows for short keys, per-user DPAPI file on Windows for long keys >1280 chars, profile file on Linux), removes the Juggernaut "claude" launcher block from shell profiles, and removes the legacy "~/.local/bin/claude" symlink if present. It intentionally leaves the "juggernaut" command launcher and install directory in place so you can reconfigure later.

### Fully remove Juggernaut

> Warning: This will permanently delete your Juggernaut installation, all stored tokens, and configuration. This action cannot be undone.

For most users, re-running the installer is still the easiest full-wipe path: the installer performs a destructive wipe before reinstalling. Use "juggernaut uninstall --full" only when you want Juggernaut removed from the machine instead of reinstalled.

macOS:

```bash
juggernaut uninstall --full --dry-run
juggernaut uninstall --full --yes

# Clear the current shell's command cache. Start a new terminal if "claude"
# or "juggernaut" was already loaded in this shell.
hash -r 2>/dev/null || true
unset -f claude 2>/dev/null || true
unfunction claude 2>/dev/null || true
functions -e claude 2>/dev/null || true

# Optional sanity checks: these should print nothing for Juggernaut.
command -v juggernaut || true
type claude
```

Linux, WSL, and Git Bash:

```bash
juggernaut uninstall --full --dry-run
juggernaut uninstall --full --yes

# Clear the current shell's command cache. Start a new terminal if "claude"
# or "juggernaut" was already loaded in this shell.
hash -r 2>/dev/null || true
unset -f claude 2>/dev/null || true
unfunction claude 2>/dev/null || true
functions -e claude 2>/dev/null || true

# Optional sanity checks: these should print nothing for Juggernaut.
command -v juggernaut || true
type claude
```

Windows PowerShell:

```powershell
juggernaut uninstall --full --dry-run
juggernaut uninstall --full --yes

# Start a new PowerShell session if "claude" or "juggernaut" was already loaded.

# Optional sanity checks: these should not point at Juggernaut.
Get-Command juggernaut -ErrorAction SilentlyContinue
Get-Command claude -All
```

If "Get-Command claude -All" still shows a PowerShell "Function" named "claude",
open a new PowerShell session. If it still appears after that, remove the
"# BEGIN: Juggernaut Launcher" block from your PowerShell profile and restart
PowerShell.

If you installed with "JUGGERNAUT_DIR", full uninstall removes that custom install directory instead of "$HOME/.juggernaut" / "$HOME\.juggernaut".

The installer does not modify your PATH. If you manually added "~/.local/bin" (or equivalent) only for Juggernaut, remove that entry from your shell profile after deletion.

After uninstalling, Claude Code will prompt you to log in with your Anthropic account.

## IAM Permissions Required

Your AWS user/role needs:
- `bedrock:InvokeModel`
- `bedrock:InvokeModelWithResponseStream`
- `bedrock:ListInferenceProfiles`

See `iam-policy.json` for the complete policy.

**Security Note:** The provided IAM policy uses wildcard regions (`arn:aws:bedrock:*:...`) for flexibility. For tighter security, restrict to specific regions (e.g., `arn:aws:bedrock:us-west-2:...`).

## Security Considerations

### Authentication Methods (Most to Least Secure)

| Method | Security | Use Case |
|--------|----------|----------|
| IAM/SSO | Most secure | Production — no secrets in commands |
| API key + keychain/Credential Manager/DPAPI/profile | Varies | Key encrypted at rest in OS keychain/Credential Manager or per-user DPAPI file; Linux profile storage is plaintext |
| API key (pipe) | Secure | `echo $KEY \| juggernaut apply ...` — key never in terminal scrollback |
| API key (interactive) | Secure | Visible prompt, paste-safe on all platforms |
| API key (inline) | Least secure | CI/CD only — visible in process list |

### API Key Authentication

**Pipe mode (`echo $KEY | juggernaut apply --auth=bedrock-api-key`)** is the recommended path:
- Key never appears in terminal scrollback
- Works reliably under tmux, screen, SSH, and all terminal emulators
- Script-friendly

**Interactive mode (`juggernaut apply --auth=bedrock-api-key`)**:
- Prompts with a visible `>` indicator — paste-safe on all platforms
- Key appears in terminal scrollback (same as it would in any editor)

**Inline mode (`--bedrock-key=xxx`)** — use only for CI/CD:
- Command-line arguments are visible to other users via `ps aux`
- Commands are saved to shell history

**For CI/CD**, use secrets management:

```bash
# GitHub Actions
juggernaut apply --auth=bedrock-api-key --bedrock-key=${{ secrets.BEDROCK_KEY }}
```

## Notes

- `/login` and `/logout` commands are disabled when using Bedrock
- Authentication is handled through AWS credentials
- `AWS_REGION` is required (Claude Code doesn't read from `.aws/config`)
- Credentials need periodic refresh if using SSO/temporary credentials
