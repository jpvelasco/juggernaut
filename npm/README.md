# juggernaut-bedrock

**Route Claude Code, Codex, OpenCode, and Grok through Amazon Bedrock with IAM, SSO, or Bedrock API keys.**

Juggernaut is a cross-platform CLI that wires your coding CLI to [Amazon Bedrock](https://aws.amazon.com/bedrock/) instead of vendor APIs. One `apply`, then keep typing `claude`, `codex`, `opencode`, or `grok`.

Built for developers and teams shipping with GenAI today: one command configures routing, protects existing settings, stores API keys in the OS keychain, provides an explicit account-model discovery command, and includes a `doctor` command when something's off.

<p align="center">
  <a href="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml"><img src="https://github.com/jpvelasco/juggernaut/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/jpvelasco/juggernaut/releases/latest"><img src="https://img.shields.io/github/v/release/jpvelasco/juggernaut" alt="Release"></a>
  <a href="https://www.npmjs.com/package/juggernaut-bedrock"><img src="https://img.shields.io/npm/v/juggernaut-bedrock" alt="npm version"></a>
  <a href="https://www.npmjs.com/package/juggernaut-bedrock"><img src="https://img.shields.io/npm/dw/juggernaut-bedrock" alt="npm weekly downloads"></a>
  <a href="https://nodejs.org/"><img src="https://img.shields.io/node/v/juggernaut-bedrock" alt="Node.js version"></a>
  <a href="https://app.codacy.com/gh/jpvelasco/juggernaut/dashboard?utm_source=gh&utm_medium=referral&utm_content=&utm_campaign=Badge_grade"><img src="https://app.codacy.com/project/badge/Grade/2bf1e68b80964537b5c65350663c3073" alt="Codacy Grade"></a>
</p>

## Install

```bash
npm install -g juggernaut-bedrock
```

Or try it without installing globally:

```bash
npx juggernaut-bedrock version
```

Works on **macOS** and **Linux** (`x64` and `arm64`), plus **Windows x64** and **WSL**.

Requires **Node.js 20+**, a supported coding CLI, and access to the desired Amazon Bedrock models. AWS IAM/SSO mode uses your existing AWS credentials; Bedrock API-key mode stores the key in your OS keychain.

## Choose your setup

Already use AWS IAM or SSO?

```bash
juggernaut apply --auth=iam
```

Have a Bedrock API key?

```bash
juggernaut apply --auth=bedrock-api-key
```

Not sure which mode to use?

```bash
juggernaut apply
```

The interactive setup guides you through the first run. After applying, restart or source your shell and continue using your normal CLI command.

## Why developers use it

| Feature | Benefit |
|---------|---------|
| **IAM and SSO** | Use existing AWS identity, roles, federation, and enterprise access controls |
| **Four coding CLIs** | Configure Claude Code, Codex, OpenCode, and Grok side by side |
| **Keychain-only secrets** | Keep Bedrock API keys out of shell profiles and plaintext config |
| **Collision detection** | Refuse to overwrite foreign configuration instead of silently clobbering it |
| **Account-aware model discovery** | See models your AWS account and region can actually use |
| **Safe activation** | Never overwrite an unknown CLI binary; fall through to the real CLI if Juggernaut is unavailable |
| **Cross-platform launcher** | Use the same workflow on macOS, Linux, Windows, and WSL |
| **`doctor` diagnostics** | Check credentials, configuration, activation, PATH, binaries, and legacy artifacts |

## Discover models your account can actually access

Bedrock model access varies by AWS account and region. Juggernaut queries the native Bedrock catalog, caches the result per account and region, and filters the inventory for each coding CLI.

```bash
# Discover models available to the current AWS account
juggernaut models refresh --region=us-west-2
juggernaut models list --region=us-west-2 --cli=opencode
```

`models refresh` is an explicit discovery step and currently requires working AWS IAM or SSO credentials. It is not part of the one-command `apply` setup and is not available through Bedrock API-key authentication. The catalog is stored locally, and `apply` reads it without making an implicit network call, so configuration stays deterministic and offline-friendly.

## Built for teams and individual developers

- **Teams:** use IAM, SSO, roles, CloudTrail, AWS billing, region controls, and VPC endpoints.
- **Individual developers:** use a Bedrock API key stored in the OS keychain while keeping your normal CLI workflow.
- **Multi-agent workflows:** configure multiple coding CLIs side by side while sharing the Bedrock bearer token safely.

## Upgrading

Older Juggernaut users can install v6 directly. **v6 is breaking:** Mantle routing was removed — every CLI now uses `bedrock-runtime` (native Bedrock). After upgrading, re-run `juggernaut models refresh --source native --region <region>` and re-apply each CLI you use:

```bash
npm install -g juggernaut-bedrock@latest
juggernaut apply --auth=iam
```

Using a Bedrock API key from an old Windows v3 install? See the Windows v3 API-key bridge in the [GitHub README](https://github.com/jpvelasco/juggernaut#windows-v3-api-key-installs). You do not need to install v4 first.

## Quickstart

```bash
# 1. Configure — IAM/SSO (recommended) or interactive prompt
juggernaut apply --auth=iam

# 2. Restart/source your shell, then launch your CLI
claude
```

Bedrock API key auth? Run `juggernaut apply --auth=bedrock-api-key`; credentials land in your OS keychain, not your shell history.

## Multi-CLI Support

| CLI | Flag | Config path (user scope) |
|-----|------|---------------------------|
| Claude Code (default) | `--cli=claude` | `~/.claude/settings.json` |
| OpenAI Codex | `--cli=codex` | `~/.codex/config.toml` |
| OpenCode | `--cli=opencode` | `~/.config/opencode/opencode.json` |
| Grok | `--cli=grok` | `~/.grok/config.toml` (user scope only) |

```bash
# Codex supports IAM/SSO or Bedrock API key; OpenCode and Grok accept either auth mode
juggernaut apply --cli=codex --auth=iam
juggernaut apply --cli=opencode --auth=iam
juggernaut apply --cli=grok --auth=iam
```

Activation blocks for different CLIs coexist in one shell profile. The Bedrock bearer token is **shared** across CLIs — uninstalling one does not remove it.

## What it does

```
juggernaut apply --auth=iam
```

That one command:

1. **Writes** Bedrock config to the target CLI's config file (user or project scope)
2. **Sets** model IDs, region, effort level, permission mode, and routing env vars — only after credentials validate
3. **Installs** a marked shell activation block that delegates to `juggernaut launch`

No overwriting the real CLI binary. No copying API keys into env vars. A backup is made before every write.

## Why Bedrock?

| | Direct Vendor API | Amazon Bedrock |
|---|---------------------|----------------|
| **Billing** | Separate account | Your AWS bill |
| **Auth** | API keys | IAM, SSO, roles |
| **Region** | Vendor infra | Your chosen AWS region |
| **Compliance** | Vendor certs | SOC, HIPAA, FedRAMP via AWS |
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
| `apply` | Configure Bedrock for the target `--cli` and install shell activation |
| `show` | Print your current Juggernaut config (`--cli=claude\|codex\|opencode\|grok`, `--json`) |
| `doctor` | Diagnostics for settings, credentials, activation, CLI binary, and legacy artifacts |
| `uninstall` | Remove managed config and token; `--full` also removes shell activation |
| `models refresh` | Discover account/region model inventory from native Bedrock |
| `models list` | List cached model inventory, optionally filtered by CLI compatibility |
| `models check` | Maintainer tool: verify pinned models against live AWS catalog |
| `version` | Print installed version (`--json` for machines) |

### Account model discovery

With working AWS IAM or SSO credentials, discover what models your AWS account actually exposes in a given region:

```bash
# Discover the native Bedrock model inventory for the region
juggernaut models refresh --region=us-west-2

# See what's compatible with a specific CLI
juggernaut models list --region=us-west-2 --cli=opencode
```

The inventory is cached per account/region at `~/.juggernaut/model-catalog.json`. `apply` reads this cache without network calls — deterministic and offline-friendly. Bedrock API-key users can still use `apply`, but should not run `models refresh` unless they also have AWS credentials available.

## Default models

| Tier | Model | Global inference profile |
|------|-------|--------------------------|
| **Primary** | Claude Sonnet 5 | `global.anthropic.claude-sonnet-5` |
| **Opus** | Claude Opus 5 | `global.anthropic.claude-opus-5` |
| **Fable** | Claude Fable 5 | `global.anthropic.claude-fable-5` |
| **Fast** | Claude Haiku 4.5 | `global.anthropic.claude-haiku-4-5-20251001-v1:0` |

Claude Opus 5 is the default Opus tier on Bedrock, with adaptive thinking, 1M context, and 128K output. Opus 4.7 and 4.8 remain available through `--opus-model`. Juggernaut enables Claude Code's 1M context accounting for Opus and Sonnet by appending `[1m]` to the alias environment variables. Use `--no-1m-context` to opt out.

Override all aliases: `juggernaut apply --auth=iam --model=global.anthropic.claude-sonnet-5`
Override one tier: `juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>`
Set native fallback chain: `juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-5,global.anthropic.claude-sonnet-5`

## Effort levels

Controls adaptive thinking depth. Valid values: `low | medium | high | xhigh | max | auto`. Fixed persisted levels (`low | medium | high | xhigh`) are written to native `effortLevel`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut does not expose it as `--effort`.

```bash
juggernaut apply --auth=iam --effort=max
```

Default: `high`. On Opus 4.8/4.7, effort level controls adaptive thinking depth — manual thinking mode is not supported. Opus 5 supports adaptive thinking on by default; disabling it caps effort at `high`.

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

## Safety features

- **Collision detection** — if a target config already has foreign values on keys Juggernaut would write, apply refuses unless you pass `--force`. A backup is always created.
- **Keychain-only secrets** — Bedrock API keys never go into shell profiles or plaintext config.
- **No binary overwrite** — Juggernaut never installs over an unknown file matching a managed CLI name.
- **Graceful fallthrough** — if `juggernaut` is not on PATH, activation wrappers fall through to the real CLI binary.

## Other options

```bash
--always-thinking           # enable extended thinking by default
--service-tier=flex         # Bedrock service tier: default | flex | priority
--fable-model=<id>          # override Fable alias
--fallback-model=a,b        # write native fallbackModel chain
--available-models=a,b      # curate the /model picker
--enforce-available-models  # restrict picker to listed models
--opusplan                  # route /plan to Opus, execution to Sonnet
--effort=high               # low | medium | high | xhigh | max | auto
--mode=auto                 # auto-approve safe tool calls
--scope=project             # write to project scope instead of user scope
--force                     # overwrite colliding foreign leaves (backup kept)
```

## Troubleshooting

Stuck? Start here:

```bash
juggernaut doctor
```

Common fixes: complete Anthropic model access in the Bedrock console (403), refresh SSO (`aws sso login`), or re-run `juggernaut apply`. User-scope Claude applies also keep a non-secret fallback at `~/.juggernaut/runtime/claude.json`, so `doctor` can identify and temporarily recover from a Claude update that replaced `settings.json`.

## Documentation

Full docs, IAM policy, multi-CLI details, model discovery, and platform notes:

**[github.com/jpvelasco/juggernaut](https://github.com/jpvelasco/juggernaut)**

## License

MIT — see [LICENSE](https://github.com/jpvelasco/juggernaut/blob/main/LICENSE).

Juggernaut is an independent tool, not affiliated with Anthropic, Amazon Web Services, OpenAI, or xAI.
