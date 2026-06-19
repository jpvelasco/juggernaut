# juggernaut-bedrock

**Claude Code → Amazon Bedrock in one command.**

Juggernaut is a cross-platform CLI that wires [Claude Code](https://docs.anthropic.com/en/docs/claude-code) to [Amazon Bedrock](https://aws.amazon.com/bedrock/) instead of Anthropic's direct API. Install Claude Code with Anthropic's installer, run one `apply`, then keep typing `claude`.

Built for developers shipping with GenAI today: IAM and SSO for teams, API keys for solo runs, and a `doctor` command when something's off.

<p align="center">
  <a href="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml"><img src="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/jpvelasco/juggernaut/releases/latest"><img src="https://img.shields.io/github/v/release/jpvelasco/juggernaut" alt="Release"></a>
  <a href="https://www.npmjs.com/package/juggernaut-bedrock"><img src="https://img.shields.io/npm/v/juggernaut-bedrock" alt="npm"></a>
  <a href="https://app.codacy.com/gh/jpvelasco/juggernaut/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade"><img src="https://app.codacy.com/project/badge/Grade/2bf1e68b80964537b5c65350663c3073" alt="Codacy Grade"></a>
</p>

## Install

```bash
curl -fsSL https://claude.ai/install.sh | bash
npm install -g juggernaut-bedrock
```

Or try it without installing globally:

```bash
npx juggernaut-bedrock version
```

Works on **macOS**, **Linux**, **Windows**, and **WSL** — `x64` and `arm64`.

## Quickstart

```bash
# 1. Install Claude Code and Juggernaut (above)

# 2. Configure - IAM/SSO (recommended) or interactive prompt
juggernaut apply --auth=iam

# 3. Restart/source your shell, then launch Claude Code
claude
```

Bedrock API key auth? Run `juggernaut apply --auth=bedrock-api-key`; credentials land in your OS keychain, not your shell history.

## What it does

```
juggernaut apply --auth=iam
```

That one command:

1. **Writes** Bedrock config to `~/.claude/settings.json` (or project scope)
2. **Sets** model IDs, region, effort level, permission mode, and `CLAUDE_CODE_USE_BEDROCK=1` — only after credentials check out
3. **Installs** a marked shell activation block with a `claude` function that delegates to `juggernaut launch`

No overwriting the real Claude Code binary. No copying API keys into env vars.

## Why Bedrock?

| | Direct Anthropic API | Amazon Bedrock |
|---|---------------------|----------------|
| **Billing** | Separate account | Your AWS bill |
| **Auth** | API keys | IAM, SSO, roles |
| **Region** | Anthropic infra | Your chosen AWS region |
| **Compliance** | Anthropic certs | SOC, HIPAA, FedRAMP via AWS |
| **Network** | Public internet | VPC endpoints, PrivateLink |

## Auth modes

| Mode | Command | Best for |
|------|---------|----------|
| **IAM / SSO** | `juggernaut apply --auth=iam` | Teams, enterprise, existing AWS identity |
| **Bedrock API key** | `juggernaut apply --auth=bedrock-api-key` | Solo devs, quick setup |
| **Interactive** | `juggernaut apply` (no flags) | First run — guided prompts |
| **Preview** | `juggernaut apply --dry-run` | See what would change, change nothing |

## Commands

| Command | What it does |
|---------|--------------|
| `apply` | Configure Bedrock + install shell activation |
| `show` | Print your current Juggernaut config |
| `doctor` | Diagnostics for settings, credentials, activation, Claude Code, and legacy v4.2.6 artifacts |
| `uninstall` | Remove config and token; `--full` also removes shell activation |
| `version` | Print installed version (`--json` for machines) |

## Default models

| Tier | Model | Global inference profile |
|------|-------|--------------------------|
| **Primary** | Claude Sonnet 4.6 | `global.anthropic.claude-sonnet-4-6` |
| **Opus** | Claude Opus 4.8 | `global.anthropic.claude-opus-4-8` |
| **Fast** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

Override any tier: `juggernaut apply --auth=iam --model=global.anthropic.claude-sonnet-4-6`

## Effort levels

Controls adaptive thinking depth. Valid values: `low | medium | high | xhigh | max`

```bash
juggernaut apply --auth=iam --effort=max
```

Default: `xhigh`. On Opus 4.8/4.7, effort level controls adaptive thinking depth — manual thinking mode is not supported.

## Permission modes

Controls how Claude Code handles tool-use approvals:

| Mode | Behavior |
|------|----------|
| `default` | Prompts for each action |
| `acceptEdits` | Auto-approves file edits |
| `plan` | Propose only, no execution |
| `auto` | Agentic safety classifier |
| `bypassPermissions` | Skip all prompts (containers/VMs only) |

```bash
juggernaut apply --auth=iam --mode=auto
```

Auto mode on Bedrock requires `CLAUDE_CODE_ENABLE_AUTO_MODE=1` — Juggernaut sets this automatically.

## Other options

```bash
--always-thinking       # enable extended thinking by default (alwaysThinkingEnabled)
--service-tier=flex     # Bedrock service tier: default | flex | priority
--opusplan              # route /plan to Opus 4.8, execution to Sonnet 4.6
--mode=auto             # auto-approve safe tool calls with background checks
--mantle                # enable Mantle routing
--scope=project         # write to ./.claude/settings.json instead of ~/.claude/
```

## Troubleshooting

Stuck? Start here:

```bash
juggernaut doctor
```

Common fixes: complete Anthropic model access in the Bedrock console (403), refresh SSO (`aws sso login`), or re-run `juggernaut apply`.

## Documentation

Full docs, IAM policy, migration guide, and platform notes:

**[github.com/jpvelasco/juggernaut](https://github.com/jpvelasco/juggernaut)**

## License

MIT — see [LICENSE](https://github.com/jpvelasco/juggernaut/blob/main/LICENSE).

Juggernaut is an independent tool, not affiliated with Anthropic or Amazon Web Services.
