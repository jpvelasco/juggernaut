<p align="center">
  <img src="docs/logo.png" alt="Juggernaut" width="200">
</p>

<p align="center">
  <a href="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml"><img src="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/jpvelasco/juggernaut/releases/latest"><img src="https://img.shields.io/github/v/release/jpvelasco/juggernaut" alt="Release"></a>
  <a href="https://github.com/jpvelasco/juggernaut/blob/main/LICENSE"><img src="https://img.shields.io/github/license/jpvelasco/juggernaut" alt="License"></a>
  <a href="https://github.com/jpvelasco/juggernaut/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/jpvelasco/juggernaut" alt="Go"></a>
  <a href="https://goreportcard.com/report/github.com/jpvelasco/juggernaut/v4"><img src="https://goreportcard.com/badge/github.com/jpvelasco/juggernaut/v4" alt="Go Report Card"></a>
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

### Via npm (recommended)

Published as [`juggernaut-bedrock`](https://www.npmjs.com/package/juggernaut-bedrock) on npm. Works everywhere Claude Code is installed.

```bash
npm install -g juggernaut-bedrock --allow-scripts=juggernaut-bedrock
```

> The `--allow-scripts` flag is required because newer npm versions block postinstall scripts by default. The postinstall script downloads the platform binary from GitHub Releases and verifies its SHA-256 checksum.

**curl (Unix / macOS / Linux / Git Bash / WSL):**

```bash
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/latest/scripts/install.sh | bash
```

**PowerShell (Windows):**

```powershell
irm https://raw.githubusercontent.com/jpvelasco/juggernaut/latest/scripts/install.ps1 | iex
```

## Configure

```bash
# IAM / SSO (recommended)
juggernaut apply --auth=iam

# Bedrock API key (stored securely in OS keychain)
juggernaut apply

# Interactive first-run — omit flags for a guided prompt
juggernaut apply
```

`juggernaut apply` will not write `CLAUDE_CODE_USE_BEDROCK=1` unless a valid credential source is confirmed.

**Common options:**

```bash
juggernaut apply --auth=iam --region=us-east-1
juggernaut apply --auth=iam --opusplan              # Opus in /plan, Sonnet in execute
juggernaut apply --auth=iam --effort=xhigh          # low | medium | high | xhigh | max
juggernaut apply --auth=iam --mode=auto             # enable agentic safety-classifier mode
juggernaut apply --auth=iam --always-thinking       # extended thinking on by default
juggernaut apply --auth=iam --service-tier=flex     # Bedrock service tier: default | flex | priority
juggernaut apply --auth=iam --no-mantle             # disable Mantle routing
juggernaut apply --auth=iam --dry-run               # preview without writing
juggernaut apply --auth=iam --scope=project         # write to ./.claude/settings.json
```

## Launch

```bash
claude
```

Juggernaut installs a `claude` shim (`~/.local/bin/claude` on Unix, `claude.cmd` on Windows) that reads your bearer token from the OS keychain and injects it before launching Claude Code. No manual environment setup required.

## Commands

| Command | Description |
|---------|-------------|
| `apply` | Write Juggernaut config to `settings.json` and install the launcher shim. |
| `show` | Print the current Juggernaut block from user and project scopes. |
| `doctor` | Read-only diagnostics — checks block, credentials, launcher shim. |
| `uninstall` | Remove the Juggernaut block, bearer token, and launcher shim. |
| `migrate` | Migrate from v3 (shell-based) to v4 (Go). Runs automatically on first apply. |
| `version` | Print the installed version. |

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|---------------------|
| **Primary** | Claude Sonnet 4.6 | `global.anthropic.claude-sonnet-4-6` |
| **Opus** | Claude Opus 4.8 | `global.anthropic.claude-opus-4-8` |
| **Fast / subagent** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

Override any tier:

```bash
juggernaut apply --auth=iam --opus-model=us.anthropic.claude-opus-4-8
juggernaut apply --auth=iam --model=global.anthropic.claude-sonnet-4-6  # override all
```

## Effort Levels

Controls adaptive thinking depth. Valid for all Claude 4 models on Bedrock.

| Level | Behavior |
|-------|----------|
| `low` | Minimal thinking — fastest, lowest cost |
| `medium` | Moderate thinking |
| `high` | Almost always thinks |
| `xhigh` | Always thinks deeply (default) |
| `max` | Maximum thinking — deepest reasoning, highest cost |

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

Juggernaut writes only to `~/.claude/settings.json` (user scope) or `./.claude/settings.json` (project scope). No shell profile modification.

```json
{
  "juggernaut": { "auth": { "mode": "iam", "region": "us-west-2" }, "meta": { ... } },
  "effortLevel": "xhigh",
  "skipWebFetchPreflight": true,
  "permissions": { "defaultMode": "default" },
  "env": {
    "CLAUDE_CODE_USE_BEDROCK": "1",
    "AWS_REGION": "us-west-2",
    "CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32768",
    "MAX_THINKING_TOKENS": "65536",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "global.anthropic.claude-opus-4-8",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "global.anthropic.claude-sonnet-4-6",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "global.anthropic.claude-haiku-4-5-20251001-v1:0",
    "CLAUDE_CODE_EFFORT_LEVEL": "xhigh",
    "ENABLE_PROMPT_CACHING_1H": "1",
    "DISABLE_TELEMETRY": "1"
  }
}
```

## Migrating from v3

If you have a v3 (shell-based) installation, just install v4 and run:

```bash
juggernaut migrate
```

Or simply run `juggernaut apply` — migration runs automatically on first use when a v3 config is detected. Credentials are transferred, shell launcher blocks are removed from your profiles, and no re-entry of credentials is required.

Minimum supported migration source: v3.2.3. If you're on an older version, upgrade the shell version first:

```bash
curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash
```

> The v3 shell-based release is preserved on the [`legacy/v3`](https://github.com/jpvelasco/juggernaut/tree/legacy/v3) branch and tagged [`v3.2.3`](https://github.com/jpvelasco/juggernaut/releases/tag/v3.2.3). All v3 install URLs continue to work.

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
# Remove config and launcher shim
juggernaut uninstall

# Preview first
juggernaut uninstall --dry-run

# Remove everything including the shim binary
juggernaut uninstall --full
```

## Troubleshooting

**403 Access Denied** — Complete the Anthropic model access request in the AWS Bedrock console.

**Model not found** — Use global inference profile IDs (Juggernaut does this by default).

**Keychain unavailable on headless Linux** — IAM auth works without a keychain. For Bedrock API key auth, use `--storage=profile` to store in a local file instead.

**SSO session expired** — `aws sso login --profile=<your-profile>` and re-run `claude`.

**`juggernaut doctor`** — always the first diagnostic step.

## Notes

- `/login` and `/logout` are disabled when using Bedrock
- `AWS_REGION` is set explicitly by Juggernaut (Claude Code does not read it from `~/.aws/config`)
- Juggernaut is an independent tool, not affiliated with Anthropic or Amazon Web Services
