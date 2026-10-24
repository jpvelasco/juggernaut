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

`juggernaut apply` writes Bedrock routing for the auth mode you select. It does not call AWS. `--auth=iam` trusts the credential chain already on the machine (environment, shared config, or SSO) and does not run a signed STS or Bedrock request. `--auth=bedrock-api-key` stores the key in the OS keychain and does not probe Bedrock at apply time either. `doctor` is the check: API-key mode sends the stored bearer token; IAM mode only confirms the regional endpoint is reachable (an unsigned request — HTTP 403 with an auth error counts as up). Claude Code signs IAM requests itself when it runs.

### Named AWS profiles (`--profile`)

If your credentials live in a named SSO/shared-config profile (no `default` profile), pin it with `--profile` so the credential chain resolves deterministically:

```bash
juggernaut apply --auth=iam --profile=developer
```

The profile name is validated against your local shared config files (file reads only — no AWS call) and persisted in the `juggernaut.auth` block. A re-apply without the flag keeps the stored profile, while `--no-profile` explicitly clears it and returns to the ambient default chain. If that stored profile is later renamed or deleted from your shared config files, a plain re-apply fails on validation — switch to another profile or run `juggernaut apply --no-profile` to come back to the ambient chain. `juggernaut launch` then exports `AWS_PROFILE=<profile>` to the wrapped CLI unless your shell already exports its own value, and `doctor` warns when an IAM config has no profile stored and `AWS_PROFILE` isn't set. Requires `--auth=iam`; a later `apply --auth=bedrock-api-key` clears it.

### Multi-CLI (`--cli`)

| CLI | Flag | Config path (user scope) |
|-----|------|---------------------------|
| Claude Code (default) | `--cli=claude` | `~/.claude/settings.json` |
| OpenAI Codex | `--cli=codex` | `~/.codex/config.toml` |
| OpenCode | `--cli=opencode` | `~/.config/opencode/opencode.json` |
| Grok | `--cli=grok` | `~/.grok/config.toml` (user scope only) |

```bash
# Every CLI defaults to bedrock-runtime (opt in to mantle below)
juggernaut apply --cli=opencode --auth=iam              # or --auth=bedrock-api-key
juggernaut apply --cli=codex --auth=iam
juggernaut apply --cli=grok --auth=iam
# Re-populate the account/region catalog after upgrading from a Mantle release:
juggernaut models refresh --source native --region us-west-2
```

**Codex:** requires Codex CLI ≥ 0.153.4 (the first release with the built-in `amazon-bedrock-runtime` provider). Juggernaut pins `model_provider = "amazon-bedrock-runtime"` plus a region under `[model_providers.amazon-bedrock-runtime.aws]` — no custom `base_url`. If your `codex` binary is older, `apply` and `doctor` warn; update with `npm i -g @openai/codex@latest`.

Codex auth metadata lives in `~/.codex/.juggernaut.json` (or `./.codex/.juggernaut.json` for project scope). A plain `juggernaut apply --cli=codex` migrates older in-file metadata while retaining the auth mode, AWS profile, and endpoint. This removes Codex's `juggernaut is ignored` warning and allows `--strict-config`. After loading the shell activation, run `codex` normally; `juggernaut launch codex` is also available directly.

Managed Codex launches automatically use `--no-daemon` when the installed Codex advertises that option. This starts a process-local session with the current AWS profile or API key rather than attaching to a shared server that may retain older credentials or settings. It does not restart or stop existing Codex sessions. Explicit `--remote` connections, `agents`, `app-server`, and `remote-control` commands are passed through unchanged; their server owns its credentials. Older Codex versions without the flag keep their existing behavior. Launching the binary directly while bypassing the shell wrapper also bypasses this protection.

**Grok:** the Grok CLI only speaks bearer auth, so with `--auth=iam` Juggernaut points Grok's `[auth]` provider at `juggernaut auth-token --iam`, which mints a short-term Bedrock API key from your AWS credentials (SSO included) — no API key to store, and the key is refreshed automatically. A persisted `--profile` is embedded in the mint command (`--profile=<name>`) and threaded into `auth-token --iam`, so Grok works on profile-only machines too. `--auth=bedrock-api-key` keeps reading the keychain key.

Activation blocks for different CLIs coexist in one shell profile. The Bedrock bearer token is **shared** across CLIs — uninstalling one non-Claude CLI does not remove it (only the last dependent CLI clears it).

### Safety model

Juggernaut treats existing configuration and real CLI binaries as untouchable. Four mechanisms make routing predictable:

- **The Juggernaut law: refuse, don't clobber** — if a target config already has foreign values on keys Juggernaut would write, `apply` refuses unless you pass `--force` (a backup is still created). The check is per-leaf: your own sibling entries in the same config never trigger it, only the exact keys Juggernaut owns.
- **Runtime fallback** — a user-scope apply writes non-secret launch state to `~/.juggernaut/runtime/` (never the bearer token). If Claude Code later replaces `settings.json`, the fallback keeps Bedrock routing active instead of silently falling back to vendor APIs; `doctor` reports when to re-run `apply` to restore the full config. Note the fallback is a **wrapper-only** safety net: only `juggernaut launch` / the shell activation block reads it. A directly-launched `claude` is routed through Bedrock only while the managed `env` block is present in `settings.json`, and `doctor` warns about that gap when the block is missing but the fallback exists.
- **Keychain-only secrets** — Bedrock API keys never go into shell profiles or plaintext config.
- **No binary overwrite** — Juggernaut never installs over an unknown file matching a managed CLI name.

**Common options (Claude Code):**

```bash
juggernaut apply --auth=iam --region=us-east-1
juggernaut apply --auth=iam --profile=developer       # pin a named SSO/shared-config profile
juggernaut apply --auth=iam --no-profile              # clear a stored profile, use the ambient chain
juggernaut apply --auth=iam --opusplan              # Opus in /plan, Sonnet in execute
juggernaut apply --auth=iam --effort=high           # low | medium | high | xhigh | max | auto
juggernaut apply --auth=iam --mode=auto             # enable agentic safety-classifier mode
juggernaut apply --auth=iam --always-thinking       # extended thinking on by default
juggernaut apply --auth=iam --service-tier=flex     # Bedrock service tier: default | flex | priority
juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>
juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-5-5
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-5-5 --enforce-available-models
juggernaut apply --auth=iam --dry-run               # preview config and shell-profile paths, write nothing
juggernaut apply --auth=iam --scope=project         # write to ./.claude/settings.json
juggernaut apply --auth=iam --force                 # overwrite colliding foreign leaves (backup kept)
```

### Mantle endpoint (opt-in)

Claude Code can use the parallel `bedrock-mantle` endpoint while keeping runtime routing available in the same config:

```bash
# Pin a mantle-format model. Mantle IDs are bare anthropic.* IDs — no CRIS
# prefix (global./us./eu./...) and no version suffix (-v1:0).
juggernaut apply --cli=claude --auth=iam --endpoint=mantle \
  --model=anthropic.claude-sonnet-5

# Or pin each tier (IDs are examples; mantle availability is account-allowlisted).
juggernaut apply --cli=claude --auth=iam --endpoint=mantle \
  --opus-model=anthropic.claude-opus-4-7 \
  --sonnet-model=anthropic.claude-sonnet-5 \
  --haiku-model=anthropic.claude-haiku-4-5
```

Juggernaut writes `CLAUDE_CODE_USE_MANTLE=1` next to the existing `CLAUDE_CODE_USE_BEDROCK=1`; Claude Code routes each request by model-ID format. `juggernaut.auth.endpoint=mantle` records the choice, so a plain re-apply keeps the mantle endpoint and its model pins. Pass `--endpoint=runtime` to switch back.

Grok can opt in to the same endpoint. Mantle serves no cross-region inference profiles, so Grok is pinned to a bare foundation ID (`xai.grok-4.6` by default, or the mantle-only `xai.grok-4.3`) and the Responses wire API. Both auth modes work on mantle: `--auth=bedrock-api-key` reads the keychain key, and `--auth=iam` mints a short-term Bedrock API key from your AWS credentials:

```bash
juggernaut apply --cli=grok --auth=iam --endpoint=mantle
# A stored Bedrock API key works too:
juggernaut apply --cli=grok --auth=bedrock-api-key --endpoint=mantle
# Pin Grok 4.3 (mantle-only; not available on bedrock-runtime):
juggernaut apply --cli=grok --auth=iam --endpoint=mantle --model=xai.grok-4.3
```

Grok 4.6 is served in `us-west-2` and `us-gov-east-1`; Grok 4.3 in `us-west-2`, `us-east-1`, `us-east-2`, and `us-gov-west-1`. `apply` routes an unserved region to one the selected model is served in and prints a warning. Grok 4.7 is runtime-only.

Codex and OpenCode can opt in with a bare OpenAI model ID. Both use the Responses API, including client-side tool calls, and default to `openai.gpt-6-astra` on mantle:

```bash
juggernaut apply --cli=codex --auth=iam --endpoint=mantle --model=openai.gpt-5.4
juggernaut apply --cli=opencode --auth=iam --endpoint=mantle --model=openai.gpt-5.4
# The shared API key works too:
juggernaut apply --cli=opencode --auth=bedrock-api-key --endpoint=mantle
# Return to runtime:
juggernaut apply --cli=opencode --endpoint=runtime
```

Codex uses its built-in `amazon-bedrock` provider with an `aws.region` table (requires Codex ≥ 0.153.4). OpenCode mantle requires **OpenCode V2** (verified with 2.1.4): it uses `providers.amazon-bedrock-mantle`, the native `@opencode/ai/providers/amazon-bedrock/mantle/responses` package, and the explicit `/openai/v1` base URL. IAM signing is native in both clients; the Juggernaut launch wrapper supplies the shared API key for API-key mode and the saved AWS profile for IAM. OpenCode keeps endpoint/auth metadata in its existing owner-only sidecar. Run through the installed shell activation or `juggernaut launch opencode -- --standalone` so the server inherits the selected credentials; an already-running shared OpenCode server retains its own environment.

The mantle Responses roster accepts `sol`, `luna`, `astra`, `terra`, or the bare IDs `openai.gpt-6.1-sol`, `openai.gpt-6-sol`, `openai.gpt-6-luna`, `openai.gpt-6-astra`, `openai.gpt-5.6-terra`, `openai.gpt-5.6-luna`, `openai.gpt-5.4`, and `openai.gpt-5.5`. CRIS IDs are rejected. GPT-OSS is supported by OpenCode's runtime configuration, but is excluded from this mantle Responses route. OpenCode's mantle picker includes the selected model plus active, available foundation models from the cached inventory that match this roster and region. `models list --cli` continues to describe runtime compatibility.

Runtime stays the default for all four CLIs. Re-applying retains the selected endpoint. OpenCode's root `model` is the CLI's own native default, so a plain apply preserves whatever default is already configured — yours or a previously pinned one — and only passes `--model` changes it; a fresh apply without a default seeds `gpt-oss-120b` (Codex still resets to its default model unless `--model` is re-passed). Uninstall keeps a preserved user default and removes only models that point at Juggernaut's OpenCode provider entries. Switching OpenCode endpoints removes the inactive Juggernaut provider entry, preserves other providers, and replaces a default model that pointed at the removed entry.

Mantle serves a subset of regions; `apply` overrides an unsupported region to a mantle-served one and prints a warning. Codex/OpenCode use model-specific region availability: for example, GPT-6.1 Sol is served in `us-east-1`, GPT-6 Astra in `us-east-1` and `us-west-2`, and GPT-5.5 in `us-east-1` and `us-east-2`. For Claude Code, Codex, and OpenCode, `doctor` probes mantle reachability; Codex/OpenCode also validate the model, region, and native routing. This unsigned probe does not verify inference authorization. Mantle IAM inference needs `bedrock-mantle:CreateInference`; Claude token counting also needs `bedrock-mantle:CountTokens`. Both actions are already in [`iam-policy.json`](iam-policy.json), alongside `bedrock-mantle:ListModels` for discovery. To surface mantle IDs in Claude Code's `/model` picker, add them to `--available-models` (that flag also restricts the picker to the listed entries).

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
| `show` | Print the current Juggernaut-managed config from user and project scopes. Supports `--cli=claude\|codex\|opencode\|grok` (default claude) and `--json`. |
| `doctor` | Read-only diagnostics for settings, activation, CLI binary, and legacy artifacts. IAM connectivity OK means the Bedrock endpoint is reachable, not that credentials were signed and accepted. API-key mode probes with the stored bearer token. For the mantle endpoint, doctor checks the mantle region and probes its reachability. Supports `--cli=claude\|codex\|opencode\|grok`. |
| `logs export` | Write a local diagnostic zip for support. Redacted by default; `--raw` / `--include-secrets` includes secrets for private/self use only. |
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

Each provider filters the live inventory by what its client protocol can use: Claude Code accepts `foundation`/`profile` Anthropic models, Codex accepts `foundation`/`profile` OpenAI GPT-6.x and GPT-5.6 (`sol`/`luna`/`astra`/`terra`, where `terra` remains the last GPT-5.6 generation), OpenCode accepts `foundation`/`profile` OpenAI-compatible models (injected into its built-in `provider.amazon-bedrock` block with a `whitelist` of discovered models, omitted when none are found), and Grok accepts `foundation`/`profile` xAI Grok models over the native `bedrock-runtime` OpenAI-compatible endpoint (`https://bedrock-runtime.{region}.amazonaws.com/openai/v1`). This compatibility policy prevents advertising a model to a client that cannot speak its API while avoiding a maintained roster. Explicit `--model` selections remain supported and warn when the cached catalog says they are unavailable in the chosen region.

**Codex and OpenCode auth metadata:** Both clients validate their native config keys, so Juggernaut stores auth mode, region, profile, and endpoint in an owner-only `.juggernaut.json` beside the config. For Codex this is inside `.codex`; for OpenCode it is `./.juggernaut.json` (project) or `~/.config/opencode/.juggernaut.json` (user). The bearer token stays in the keychain. Launch reads the project sidecar then the legacy project config before checking user-scope metadata; migrating user scope never overrides a legacy project selection. Apply and scope-filtered doctor use the selected scope. A plain apply migrates older managed in-file blocks; if the sidecar cannot be saved, the config write is aborted and the old metadata is retained. Uninstall removes the matching sidecar.

## Default Models

| Tier | Model | Global CRIS Profile |
|------|-------|---------------------|
| **Primary** | Claude Sonnet 5.5 | `global.anthropic.claude-sonnet-5-5` |
| **Opus** | Claude Opus 5.5 | `global.anthropic.claude-opus-5-5` |
| **Fable alias** | Claude Fable 5.1 | `global.anthropic.claude-fable-5-1` |
| **Fast / subagent** | Claude Haiku 5.5 | `global.anthropic.claude-haiku-5-5` |

Juggernaut appends Claude Code's `[1m]` suffix to the Opus and Sonnet alias environment variables by default, and to the configured Fable alias when it matches Claude Code's Fable ID. Claude Code accounts against 1M context for supported aliases locally, then strips that suffix before calling Bedrock. Use `--no-1m-context` to opt out.

Claude Opus 5.5 is the default Opus tier and supports adaptive thinking (always on, cannot be disabled), max effort, 1M context, and 128K output on Bedrock. Opus 5, 4.8, and 4.7 remain available by passing `--opus-model`. Fable defaults to `global.anthropic.claude-fable-5-1`, verified live against AWS Bedrock's `ListFoundationModels`/`ListInferenceProfiles` APIs (`ACTIVE` status). Pass `--fable-model` to override with a different Bedrock-accessible model ID.

> **Fable data retention:** Anthropic requires opting in to `provider_data_share` before Fable calls succeed on Bedrock — AWS retains inputs/outputs for up to 30 days and shares them with Anthropic for abuse detection/human review ([AWS docs](https://docs.aws.amazon.com/bedrock/latest/userguide/abuse-detection.html)). Juggernaut has no way to check your account's actual opt-in status (no AWS API exposes it), so `apply` and `doctor` both print this as a standing warning whenever Fable is configured — it is not a promise about what is or isn't collected, only what AWS documents.

Override any tier:

```bash
juggernaut apply --auth=iam --opus-model=global.anthropic.claude-opus-5-5
juggernaut apply --auth=iam --fable-model=<bedrock-fable-model-id>
juggernaut apply --auth=iam --model=global.anthropic.claude-sonnet-5-5  # override all model aliases
juggernaut apply --auth=iam --fallback-model=global.anthropic.claude-opus-5-5,global.anthropic.claude-sonnet-5-5
juggernaut apply --auth=iam --available-models=sonnet,claude-opus-5-5 --enforce-available-models
```

`--available-models`/`--enforce-available-models` write to your user/project `settings.json`, which Claude Code merges with any other non-managed settings the user controls — this curates the `/model` picker, it is not a tamper-resistant restriction. For organization-wide enforcement a user can't bypass, deploy `availableModels`/`enforceAvailableModels` in Claude Code's OS-level managed settings (e.g. `/etc/claude-code/managed-settings.json`) instead; Juggernaut does not write there.

## Effort Levels

Controls adaptive thinking depth. Valid values are `low`, `medium`, `high`, `xhigh`, `max`, and `auto`; Claude Code falls back to the highest supported level for the active model. Juggernaut writes fixed persisted levels (`low`, `medium`, `high`, `xhigh`) to both native `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`; `max` and `auto` are env-only because Claude Code settings do not accept them as persisted `effortLevel` values. Ultracode is separate from `effortLevel` and `CLAUDE_CODE_EFFORT_LEVEL`, so Juggernaut does not expose it as `--effort`. Juggernaut defaults to `high`, which matches the supported effort levels of Sonnet 5.5, Opus 5.5, and Haiku 5.5 (the first Haiku with effort controls).

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

> On Opus 4.8 and 4.7, only adaptive thinking is supported — manual thinking mode is rejected by the API. Opus 5's Bedrock model card documents adaptive thinking on by default; it can be disabled, but doing so caps effort at `high`. Opus 5.5's card documents adaptive thinking as always on — it cannot be disabled. Haiku 5.5 is the first Haiku with effort controls; disabling its adaptive thinking caps effort at `high`.

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
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "global.anthropic.claude-opus-5-5[1m]",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "global.anthropic.claude-sonnet-5-5[1m]",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "global.anthropic.claude-haiku-5-5",
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

If you opt in to the mantle endpoint (`--endpoint=mantle`), also allow the mantle inference actions from [`iam-policy.json`](iam-policy.json). They are project-scoped:

```json
{
  "Effect": "Allow",
  "Action": [
    "bedrock-mantle:CreateInference",
    "bedrock-mantle:CountTokens"
  ],
  "Resource": "arn:aws:bedrock-mantle:*:*:project/*"
}
```

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

Go straight to v6 with npm. You do not need to install an intermediate v3 or v4 release first. **v6 is breaking:** every CLI defaults to `bedrock-runtime`; all four CLIs can explicitly opt back in to `bedrock-mantle` with `--endpoint=mantle`. After upgrading, run `juggernaut models refresh --source native --region <region>` and `juggernaut apply --cli=<cli> --region <region>` per CLI. The old `--mantle`/`--no-mantle`/`--mantle-url` flags and `mantle` catalog source are gone (use `--endpoint`). Runtime uses CRIS where required; mantle uses bare model IDs. OpenCode runtime uses `provider.amazon-bedrock`; its new mantle route requires OpenCode V2 and uses `providers.amazon-bedrock-mantle`.

Re-applying over a v5 Mantle-era config migrates it automatically (a backup is saved first): the old Codex `model_provider = "amazon-bedrock"` is rewritten to the built-in `amazon-bedrock-runtime` and the stale custom table is removed. With an explicit `--endpoint=mantle`, Codex keeps that provider ID but removes the v5 overrides in favor of the native mantle provider. A current config's `juggernaut.auth.endpoint=mantle` record distinguishes the opt-in from v5. Requires Codex CLI ≥ 0.153.4 — `apply` and `doctor` warn on older binaries.

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

**Shareable diagnostics** — `juggernaut logs export` writes a redacted zip (tokens, account IDs, and home paths stripped). Use `--raw` only on your own machine.

## Notes

- `/login` and `/logout` are disabled when using Bedrock
- `AWS_REGION` is set explicitly by Juggernaut (Claude Code does not read it from `~/.aws/config`)
