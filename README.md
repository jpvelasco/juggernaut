<p align="center">
  <img src="docs/logo.png" alt="Juggernaut" width="200">
</p>

<p align="center">
  <a href="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml"><img src="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/jpvelasco/juggernaut/releases/latest"><img src="https://img.shields.io/github/v/release/jpvelasco/juggernaut" alt="Release"></a>
  <a href="https://github.com/jpvelasco/juggernaut/blob/main/LICENSE"><img src="https://img.shields.io/github/license/jpvelasco/juggernaut" alt="License"></a>
  <a href="https://github.com/jpvelasco/juggernaut/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/jpvelasco/juggernaut" alt="Go"></a>
  <a href="https://www.npmjs.com/package/juggernaut-bedrock"><img src="https://img.shields.io/npm/v/juggernaut-bedrock" alt="npm"></a>
  <a href="https://app.codacy.com/gh/jpvelasco/juggernaut/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade"><img src="https://app.codacy.com/project/badge/Grade/2bf1e68b80964537b5c65350663c3073" alt="Codacy Grade"></a>
</p>

<h1 align="center">Juggernaut</h1>

<p align="center"><strong>Safe Bedrock routing for coding agents — Claude Code, Codex, OpenCode, and Grok</strong></p>

**Safe Bedrock on-ramp for coding CLIs — not a proxy, not another agent harness.**

For teams already on AWS who want Claude Code, Codex, OpenCode, or Grok on Bedrock without hand-editing configs or MITMing traffic.

**Install from npm:** [`juggernaut-bedrock`](https://www.npmjs.com/package/juggernaut-bedrock) — `npm install -g juggernaut-bedrock`

## Why Juggernaut (not a proxy)

- **Configures native CLI settings; does not sit in the request path** — Bedrock traffic flows directly from your CLI to AWS, no intermediate service.
- **Refuses foreign config leaves (Juggernaut law)** — `apply` never clobbers keys you configured elsewhere unless you pass `--force` (a backup is still created).
- **Keychain-only secrets; never overwrites the real CLI binary** — credentials stay in the OS keychain, and Juggernaut never installs over an unknown file matching a managed CLI name.
- **Survives Claude Code updates** — a non-secret runtime fallback keeps Bedrock routing active if an update replaces `settings.json`; `doctor` guides the repair.

## Non-goals

- Not a multi-model jailbreak for Claude Code
- Not an agent orchestrator
- Not affiliated with Anthropic or Amazon Web Services

## Why Bedrock?

| Feature | Direct Anthropic API | Amazon Bedrock |
|---------|---------------------|----------------|
| **Billing** | Separate Anthropic account | Consolidated AWS billing |
| **Authentication** | API keys only | IAM, SSO, roles, federation |
| **Data Residency** | Anthropic infrastructure | Your chosen AWS region |
| **Compliance** | Anthropic's certifications | AWS compliance (SOC, HIPAA, FedRAMP) |
| **Network** | Public internet | VPC endpoints, PrivateLink |
| **Governance** | Limited | IAM policies, CloudTrail, quotas |

## Install

**Prerequisite:** install Claude Code from Anthropic, then install Juggernaut from npm.

```bash
curl -fsSL https://claude.ai/install.sh | bash
npm install -g juggernaut-bedrock
```

The old `scripts/install.sh` and `scripts/install.ps1` installers are deprecated stubs in v5. Juggernaut is published through [`juggernaut-bedrock`](https://www.npmjs.com/package/juggernaut-bedrock).

### 60-second path

```bash
npm install -g juggernaut-bedrock
juggernaut apply --auth=iam   # or --auth=bedrock-api-key
# restart shell, then:
claude
```

## Configure

```bash
# IAM / SSO (recommended) — Claude Code is the default --cli
juggernaut apply --auth=iam

# Bedrock API key (stored securely in OS keychain)
juggernaut apply --auth=bedrock-api-key

# Interactive first-run — omit flags for a guided prompt
juggernaut apply
```

`juggernaut apply` will not enable Bedrock routing unless a valid credential source is confirmed (for Claude Code: no `CLAUDE_CODE_USE_BEDROCK=1` without validated auth).

### Multi-CLI (`--cli`)

| CLI | Flag | Config path (user scope) |
|-----|------|---------------------------|
| Claude Code (default) | `--cli=claude` | `~/.claude/settings.json` |
| OpenAI Codex | `--cli=codex` | `~/.codex/config.toml` |
| OpenCode | `--cli=opencode` | `~/.config/opencode/opencode.json` |
| Grok | `--cli=grok` | `~/.grok/config.toml` (user scope only) |

```bash
# Every CLI routes natively via bedrock-runtime (IAM or Bedrock API key)
juggernaut apply --cli=opencode --auth=iam              # or --auth=bedrock-api-key
juggernaut apply --cli=codex --auth=iam
juggernaut apply --cli=grok --auth=iam
# Re-populate the account/region catalog after upgrading from a Mantle release:
juggernaut models refresh --source native --region us-west-2
```

Activation blocks for different CLIs coexist in one shell profile. The Bedrock bearer token is **shared** across CLIs — uninstalling one non-Claude CLI does not remove it (only the last dependent CLI clears it).

### Safety model

Juggernaut treats existing configuration and real CLI binaries as untouchable. Four mechanisms make routing predictable:

- **The Juggernaut law: refuse, don't clobber** — if a target config already has foreign values on keys Juggernaut would write, `apply` refuses unless you pass `--force` (a backup is still created). The check is per-leaf: your own sibling entries in the same config never trigger it, only the exact keys Juggernaut owns.
- **Runtime fallback** — a user-scope apply writes non-secret launch state to `~/.juggernaut/runtime/` (never the bearer token). If Claude Code later replaces `settings.json`, the fallback keeps Bedrock routing active instead of silently falling back to vendor APIs; `doctor` reports when to re-run `apply` to restore the full config.
- **Keychain-only secrets** — Bedrock API keys never go into shell profiles or plaintext config.
- **No binary overwrite** — Juggernaut never installs over an unknown file matching a managed CLI name.

**Common options (Claude Code):**

```bash
juggernaut apply --auth=iam --region=us-east-1
juggernaut apply --auth=iam --opusplan              # Opus in /plan, Sonnet in execute
juggernaut apply --auth=iam --effort=high           # low | medium | high | xhigh | max | auto
juggernaut apply --auth=iam --mode=auto             # enable agentic safety-classifier mode
juggernaut apply --auth=iam --always-thinking       # extended thinking on by default
juggernaut apply --auth=iam --service-tier=flex     # Bedrock service tier: default | flex | priority
juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>
juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-5
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-5 --enforce-available-models
juggernaut apply --auth=iam --dry-run               # preview without writing
juggernaut apply --auth=iam --scope=project         # write to ./.claude/settings.json
juggernaut apply --auth=iam --force                 # overwrite colliding foreign leaves (backup kept)
```

## Launch

```bash
claude          # after apply --cli=claude (default)
# or: codex / opencode / grok after the matching --cli apply
```

`juggernaut apply` installs a marked shell function for the target CLI. Restart your shell, or source the updated profile, then run the CLI normally.

## Commands

| Command | Description |
|---------|-------------|
| `apply` | Write Juggernaut config for the target `--cli` and install shell activation. |
| `show` | Print the current Juggernaut-managed config from user and project scopes. |
| `doctor` | Read-only diagnostics for settings, credentials, activation, CLI binary, and legacy artifacts. Supports `--cli=claude\|codex\|opencode\|grok`. |
| `uninstall` | Remove managed config keys and optionally the bearer token. Use `--full` to remove shell activation. |
| `models refresh` | Discover the models available to the current AWS account and region from native Bedrock (`foundation` + `profile` sources), then cache locally. |
| `models list` | List the cached inventory, optionally filtered to models compatible with a specific CLI (`--cli=codex\|opencode\|grok`). |
| `models check` | Maintainer tool: check `bedrock-config.json`'s pinned models against AWS Bedrock's live catalog; `--write --set-<tier>=<id>` to update a stale pin. |
| `version` | Print the installed version. |

### Account model discovery

Juggernaut can configure the model inventory the account actually exposes instead of relying on a release-time roster:

```bash
# Query native Bedrock (foundation models + inference profiles) with the default AWS profile
juggernaut models refresh --region=us-west-2
# Alias for the same native sources:
juggernaut models refresh --region=us-west-2 --source native

# See the OpenCode-compatible subset (e.g. Kimi, GLM, Qwen, GPT-OSS, DeepSeek, Grok, GPT-5.6)
juggernaut models list --region=us-west-2 --cli=opencode

# Inspect one catalog family only
juggernaut models list --region=us-east-1 --source=foundation
juggernaut models list --region=us-east-1 --source=profile
```

The inventory is partitioned by AWS account and region at `~/.juggernaut/model-catalog.json` with owner-only permissions. The current profile or environment credential selection is bound to the caller account during refresh, so switching accounts cannot reuse another account's inventory. `apply` reads the matching cached account/region and never makes an implicit network call, so configuration remains deterministic and works offline. Run `models refresh` again when AWS adds models or the account's access changes; `models list --refresh` combines both steps.

Each provider filters the live inventory by what its client protocol can use: Claude Code accepts `foundation`/`profile` Anthropic models, Codex accepts `foundation`/`profile` OpenAI GPT-5.6 (`sol`/`terra`/`luna`), OpenCode accepts `foundation`/`profile` OpenAI-compatible models (injected into its built-in `provider.amazon-bedrock` block with `whitelist`), and Grok accepts `foundation`/`profile` xAI Grok models over the native `bedrock-runtime` OpenAI-compatible endpoint (`https://bedrock-runtime.{region}.amazonaws.com/openai/v1`). This compatibility policy prevents advertising a model to a client that cannot speak its API while avoiding a maintained roster. Explicit `--model` selections remain supported and warn when the cached catalog says they are unavailable in the chosen region.

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|---------------------|
| **Primary** | Claude Sonnet 5 | `global.anthropic.claude-sonnet-5` |
| **Opus** | Claude Opus 5 | `global.anthropic.claude-opus-5` |
| **Fable alias** | Claude Fable 5 | `global.anthropic.claude-fable-5` |
| **Fast / subagent** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

Juggernaut appends Claude Code's `[1m]` suffix to the Opus and Sonnet alias environment variables by default, and to the configured Fable alias when it matches Claude Code's Fable ID. Claude Code accounts against 1M context for supported aliases locally, then strips that suffix before calling Bedrock. Use `--no-1m-context` to opt out.

Claude Opus 5 is the default Opus tier and supports adaptive thinking, max effort, 1M context, and 128K output on Bedrock. Opus 4.7 and 4.8 remain available by passing `--opus-model`. Fable defaults to `global.anthropic.claude-fable-5`, verified live against AWS Bedrock's `ListFoundationModels`/`ListInferenceProfiles` APIs (`ACTIVE` status). Pass `--fable-model` to override with a different Bedrock-accessible model ID.

> **Fable data retention:** Anthropic requires opting in to `provider_data_share` before Fable calls succeed on Bedrock — AWS retains inputs/outputs for up to 30 days and shares them with Anthropic for abuse detection/human review ([AWS docs](https://docs.aws.amazon.com/bedrock/latest/userguide/abuse-detection.html)). Juggernaut has no way to check your account's actual opt-in status (no AWS API exposes it), so `apply` and `doctor` both print this as a standing warning whenever Fable is configured — it is not a promise about what is or isn't collected, only what AWS documents.

Override any tier:

```bash
juggernaut apply --auth=iam --opus-model=global.anthropic.claude-opus-5
juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>
juggernaut apply --auth=iam --model=global.anthropic.claude-sonnet-5  # override all model aliases
juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-5,global.anthropic.claude-sonnet-5
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-5 --enforce-available-models
```

`--available-models`/`--enforce-available-models` write to your user/project `settings.json`, which Claude Code merges with any other non-managed settings the user controls — this curates the `/model` picker, it is not a tamper-resistant restriction. For organization-wide enforcement a user can't bypass, deploy `availableModels`/`enforceAvailableModels` in Claude Code's OS-level managed settings (e.g. `/etc/claude-code/managed-settings.json`) instead; Juggernaut does not write there.

## Effort Levels

Controls adaptive thinking depth. Valid values are `low`, `medium`, `high`, `xhigh`, `max`, and `auto`; Claude Code falls back to the highest supported level for the active model. Juggernaut writes fixed persisted levels (`low`, `medium`, `high`, `xhigh`) to both native `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut does not expose it as `--effort`. Juggernaut defaults to `high`, which matches the supported effort levels of Sonnet 5 and Opus 5.

| Level | Behavior |
|-------|----------|
| `low` | Minimal thinking — fastest, lowest cost |
| `medium` | Moderate thinking |
| `high` | Almost always thinks (default) |
| `xhigh` | Always thinks deeply |
| `max` | Maximum thinking — deepest reasoning, highest cost |
| `auto` | Claude Code selects the effort level |

```bash
juggernaut apply --auth=iam --effort=max
```

> On Opus 4.8 and 4.7, only adaptive thinking is supported — manual thinking mode is rejected by the API. Opus 5's Bedrock model card documents adaptive thinking on by default; it can be disabled, but doing so caps effort at `high`.

## Permission Modes

Controls how Claude Code handles tool-use approvals. Set with `--mode`.

| Mode | Behavior |
|------|----------|
| `default` | Prompts for permission on each action (Claude Code default) |
| `acceptEdits` | Auto-approves file edits and common filesystem commands |
| `plan` | Propose changes only — no execution without explicit approval |
| `auto` | Agentic safety classifier — auto-approves safe actions, blocks destructive ones |
| `dontAsk` | Auto-deny unless pre-approved via rules |
| `bypassPermissions` | Skip all prompts — containers/VMs only |

```bash
juggernaut apply --auth=iam --mode=auto
```

> **Bedrock note:** `auto` mode requires `CLAUDE_CODE_ENABLE_AUTO_MODE=1`. Juggernaut sets this automatically — no manual env var needed.

## What Gets Written

Juggernaut writes Bedrock settings to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). A user-scope apply also writes a non-secret launch fallback to `~/.juggernaut/runtime/claude.json`; it contains the generated environment and auth mode, never the bearer token. This keeps Bedrock routing active if a Claude Code update replaces `settings.json`, while `doctor` warns that the full config should be restored. Juggernaut also writes a marked shell activation block to your shell profiles:

```bash
# BEGIN: Juggernaut Claude Activation
claude() {
  if command -v juggernaut >/dev/null 2>&1; then
    juggernaut launch -- "$@"
  else
    command claude "$@"
  fi
}
# END: Juggernaut Claude Activation
```

If `juggernaut` is not on PATH, the wrapper falls through to the real CLI binary so an incomplete uninstall or PATH skew does not break `claude` / `codex` / `grok`. **Re-running `apply` rewrites the whole marked block** to match the current generator (any edits inside the BEGIN/END markers are replaced; content outside is preserved).

The hidden `juggernaut launch` command reads the Bedrock API key from the OS keychain when your settings use `bedrock-api-key`, sets `AWS_BEARER_TOKEN_BEDROCK` and `CLAUDE_CODE_USE_BEDROCK=1`, resolves the real Anthropic `claude` binary without recursing into Juggernaut, and launches Claude Code with your original arguments.

```json
{
  "juggernaut": { "auth": { "mode": "iam", "region": "us-west-2" }, "meta": { ... } },
  "effortLevel": "high",
  "skipWebFetchPreflight": true,
  "permissions": { "defaultMode": "default" },
  "env": {
    "CLAUDE_CODE_USE_BEDROCK": "1",
    "AWS_REGION": "us-west-2",
    "CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32768",
    "MAX_THINKING_TOKENS": "65536",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "global.anthropic.claude-opus-5[1m]",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "global.anthropic.claude-sonnet-5[1m]",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "global.anthropic.claude-haiku-4-5-20251001-v1:0",
    "CLAUDE_CODE_EFFORT_LEVEL": "high",
    "ENABLE_PROMPT_CACHING_1H": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

## IAM Permissions

```json
{
  "Effect": "Allow",
  "Action": [
    "bedrock:InvokeModel",
    "bedrock:InvokeModelWithResponseStream",
    "bedrock:ListFoundationModels",
    "bedrock:GetFoundationModelAvailability",
    "bedrock:ListInferenceProfiles",
    "sts:GetCallerIdentity"
  ],
  "Resource": "*"
}
```

See [`iam-policy.json`](iam-policy.json) for the complete policy. Model catalog list and availability actions do not support resource-level permissions; restrict model invocation resources to specific regions and model ARNs where practical.

## Uninstall

```bash
# Remove settings and keychain token
juggernaut uninstall

# Preview first
juggernaut uninstall --dry-run

# Also remove Juggernaut shell activation and recover known v4.2.6 launcher artifacts
juggernaut uninstall --full
```

## Migrating from v3/v4 and upgrading to v6

Go straight to v5/v6 with npm. You do not need to install an intermediate v3 or v4 release first. **v6 is breaking:** Mantle routing is removed — every CLI now uses `bedrock-runtime`. After upgrading, re-run `juggernaut models refresh --source native --region <region>` and `juggernaut apply --cli=<cli> --region <region>` per CLI; `--mantle`/`--no-mantle`/`--mantle-url` and `mantle` catalog source are gone, Codex `gpt-5.5`/`5.4` → `gpt-5.6` `sol`/`terra`/`luna`, Grok `xai.grok-4.3` → `xai.grok-4.6`, OpenCode switches from `provider.bedrock-mantle` to the built-in `provider.amazon-bedrock` (verify `qwen3-coder`/`gpt-oss` IDs now end in `-1:0` / `-v1:0`).

```bash
npm install -g juggernaut-bedrock@latest
juggernaut version
```

Then re-run `apply` with your preferred auth mode:

```bash
juggernaut apply --auth=iam
juggernaut apply --auth=bedrock-api-key
```

### Windows v3 API-key installs

If you are upgrading an old Windows installer-based v3 setup and want to keep a DPAPI-stored Bedrock API key, call the npm v5 binary explicitly and bridge the old key once:

```powershell
npm install -g juggernaut-bedrock@latest

$NpmPrefix = (npm prefix -g).Trim()
$Candidates = @(
  (Join-Path $NpmPrefix "juggernaut.cmd"),
  (Join-Path $NpmPrefix "bin\juggernaut.cmd"),
  (Join-Path $env:APPDATA "npm\juggernaut.cmd"),
  (Join-Path $HOME ".npm-global\juggernaut.cmd")
)

$Juggernaut = $Candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $Juggernaut) { throw "npm Juggernaut binary not found" }

& $Juggernaut version

. "$HOME\.juggernaut\lib\keychain.ps1"
$probe = Read-BearerToken
if (-not $probe -or -not $probe.Value) { throw "Old v3 API key not found" }

& $Juggernaut apply `
  --auth=bedrock-api-key `
  --bedrock-key $probe.Value `
  --region=us-west-2 `
  --mode=auto `
  --effort=high

& $Juggernaut doctor
```

Close and reopen PowerShell after `doctor` is green. This avoids the old `C:\Users\<you>\.juggernaut` launcher taking precedence during the upgrade.

## Troubleshooting

**403 Access Denied** — Complete the Anthropic model access request in the AWS Bedrock console.

**Model not found** — Use global inference profile IDs (Juggernaut does this by default).

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. Bedrock API key auth requires a Secret Service daemon on Linux.

**SSO session expired** — `aws sso login --profile=<your-profile>` and re-run `claude`.

**Claude stopped using Bedrock after an update** — Run `juggernaut doctor`. If it reports that the managed config is missing and the runtime fallback is available, Claude can continue launching through Bedrock, but re-run `juggernaut apply --cli=claude` to restore the full `settings.json`.

**`juggernaut doctor`** — always the first diagnostic step.

## Notes

- `/login` and `/logout` are disabled when using Bedrock
- `AWS_REGION` is set explicitly by Juggernaut (Claude Code does not read it from `~/.aws/config`)
