<p align="center">
  <img src="docs/logo.png" alt="Juggernaut" width="200">
</p>

<p align="center">
  <a href="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml"><img src="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/jpvelasco/juggernaut/releases/latest"><img src="https://img.shields.io/github/v/release/jpvelasco/juggernaut" alt="Release"></a>
  <a href="https://github.com/jpvelasco/juggernaut/blob/main/LICENSE"><img src="https://img.shields.io/github/license/jpvelasco/juggernaut" alt="License"></a>
  <a href="https://github.com/jpvelasco/juggernaut/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/jpvelasco/juggernaut" alt="Go"></a>
  <a href="https://goreportcard.com/report/github.com/jpvelasco/juggernaut/v5"><img src="https://goreportcard.com/badge/github.com/jpvelasco/juggernaut/v5" alt="Go Report Card"></a>
  <a href="https://www.npmjs.com/package/juggernaut-bedrock"><img src="https://img.shields.io/npm/v/juggernaut-bedrock" alt="npm"></a>
  <a href="https://app.codacy.com/gh/jpvelasco/juggernaut/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade"><img src="https://app.codacy.com/project/badge/Grade/2bf1e68b80964537b5c65350663c3073" alt="Codacy Grade"></a>
</p>

<h1 align="center">Juggernaut</h1>

<p align="center"><strong>Claude Code → Amazon Bedrock in one command</strong></p>

Single cross-platform binary that configures Claude Code to route through Amazon Bedrock instead of Anthropic's direct API — IAM, SSO, or Bedrock API key auth.

**Install from npm:** [`juggernaut-bedrock`](https://www.npmjs.com/package/juggernaut-bedrock) — `npm install -g juggernaut-bedrock`

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

Install Claude Code from Anthropic, then install Juggernaut from npm.

```bash
curl -fsSL https://claude.ai/install.sh | bash
npm install -g juggernaut-bedrock
```

The old `scripts/install.sh` and `scripts/install.ps1` installers are deprecated stubs in v5. Juggernaut is published through [`juggernaut-bedrock`](https://www.npmjs.com/package/juggernaut-bedrock).

### Upgrade from older Juggernaut

Go straight to v5 with npm. You do not need to install an intermediate v3 or v4 release first.

```bash
npm install -g juggernaut-bedrock@latest
juggernaut version
```

Then re-run `apply` with your preferred auth mode:

```bash
juggernaut apply --auth=iam
juggernaut apply --auth=bedrock-api-key
```

#### Windows v3 API-key installs

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

## Configure

```bash
# IAM / SSO (recommended)
juggernaut apply --auth=iam

# Bedrock API key (stored securely in OS keychain)
juggernaut apply --auth=bedrock-api-key

# Interactive first-run — omit flags for a guided prompt
juggernaut apply
```

`juggernaut apply` will not write `CLAUDE_CODE_USE_BEDROCK=1` unless a valid credential source is confirmed.

**Common options:**

```bash
juggernaut apply --auth=iam --region=us-east-1
juggernaut apply --auth=iam --opusplan              # Opus in /plan, Sonnet in execute
juggernaut apply --auth=iam --effort=high           # low | medium | high | xhigh | max | auto
juggernaut apply --auth=iam --mode=auto             # enable agentic safety-classifier mode
juggernaut apply --auth=iam --always-thinking       # extended thinking on by default
juggernaut apply --auth=iam --service-tier=flex     # Bedrock service tier: default | flex | priority
juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>
juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-4-8
juggernaut apply --auth=iam --mantle                # enable Mantle routing
juggernaut apply --auth=iam --dry-run               # preview without writing
juggernaut apply --auth=iam --scope=project         # write to ./.claude/settings.json
```

## Launch

```bash
claude
```

`juggernaut apply` installs a shell function named `claude` in your shell profile. Restart your shell, or source the updated profile, then run Claude normally. Juggernaut never installs over the real `claude` binary.

## Commands

| Command | Description |
|---------|-------------|
| `apply` | Write Juggernaut config to `settings.json` and install shell activation. |
| `show` | Print the current Juggernaut block from user and project scopes. |
| `doctor` | Read-only diagnostics for settings, credentials, activation, Claude Code, and legacy v4.2.6 artifacts. |
| `uninstall` | Remove the Juggernaut block and bearer token. Use `--full` to remove shell activation. |
| `version` | Print the installed version. |

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|---------------------|
| **Primary** | Claude Sonnet 4.6 | `global.anthropic.claude-sonnet-4-6` |
| **Opus** | Claude Opus 4.8 | `global.anthropic.claude-opus-4-8` |
| **Fable alias** | Claude Fable 5 | Configure with `--fable-model=<bedrock-fable-model-id>` |
| **Fast / subagent** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

Juggernaut appends Claude Code's `[1m]` suffix to the Opus and Sonnet alias environment variables by default, and to the configured Fable alias when it matches Claude Code's Fable ID. Claude Code accounts against 1M context for supported aliases locally, then strips that suffix before calling Bedrock. Use `--no-1m-context` to opt out.

Fable is exposed as an opt-in Claude Code alias. Pass `--fable-model` with a model ID that is available in your Bedrock account and region; Juggernaut does not pin a default Fable ID until one is configured.

Override any tier:

```bash
juggernaut apply --auth=iam --opus-model=us.anthropic.claude-opus-4-8
juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>
juggernaut apply --auth=iam --model=global.anthropic.claude-sonnet-4-6  # override all model aliases
juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-4-8,global.anthropic.claude-sonnet-4-6
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-4-8 --enforce-available-models
```

## Effort Levels

Controls adaptive thinking depth. Valid values are `low`, `medium`, `high`, `xhigh`, `max`, and `auto`; Claude Code falls back to the highest supported level for the active model. Juggernaut writes fixed persisted levels (`low`, `medium`, `high`, `xhigh`) to both native `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut does not expose it as `--effort`. Juggernaut defaults to `high`, which matches Sonnet 4.6.

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

> On Opus 4.8 and 4.7, only adaptive thinking is supported. Manual thinking mode is rejected by the API.

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

Juggernaut writes Bedrock settings to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). It also writes a marked shell activation block to your shell profiles:

```bash
# BEGIN: Juggernaut Claude Activation
claude() {
  juggernaut launch -- "$@"
}
# END: Juggernaut Claude Activation
```

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
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "global.anthropic.claude-opus-4-8[1m]",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "global.anthropic.claude-sonnet-4-6[1m]",
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
    "bedrock:ListInferenceProfiles"
  ],
  "Resource": "*"
}
```

See [`iam-policy.json`](iam-policy.json) for the complete policy. For tighter security, restrict the resource to specific regions.

## Uninstall

```bash
# Remove settings and keychain token
juggernaut uninstall

# Preview first
juggernaut uninstall --dry-run

# Also remove Juggernaut shell activation and recover known v4.2.6 launcher artifacts
juggernaut uninstall --full
```

## Troubleshooting

**403 Access Denied** — Complete the Anthropic model access request in the AWS Bedrock console.

**Model not found** — Use global inference profile IDs (Juggernaut does this by default).

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. Bedrock API key auth requires a Secret Service daemon on Linux.

**SSO session expired** — `aws sso login --profile=<your-profile>` and re-run `claude`.

**`juggernaut doctor`** — always the first diagnostic step.

## Notes

- `/login` and `/logout` are disabled when using Bedrock
- `AWS_REGION` is set explicitly by Juggernaut (Claude Code does not read it from `~/.aws/config`)
- Juggernaut is an independent tool, not affiliated with Anthropic or Amazon Web Services
