# Juggernaut ↔ Claude Code (Bedrock) Parity Plan

Research date: 2026-06-26. Goal (per JP): make Juggernaut behave **as close to
stock Claude Code as possible, but on Bedrock** — seamless, original-flavor.

Sources (all verified, not from memory):
- https://code.claude.com/docs/en/settings
- https://code.claude.com/docs/en/env-vars
- https://code.claude.com/docs/en/model-config
- https://code.claude.com/docs/en/amazon-bedrock
- https://code.claude.com/docs/en/permission-modes

---

## DONE this session (branch `fix/auto-mode-bedrock-parity`, commit d152b19)

1. **Auto mode fixed.** Root cause was NOT a Juggernaut write bug — settings.json
   was correct. On Bedrock, Claude Code only offers auto mode when the **active
   session model is Opus 4.7/4.8** (v2.1.158+). Juggernaut defaults to Sonnet 4.6,
   so auto was hidden from the Shift+Tab cycle. Per JP's choice ("warn, don't
   force"), `apply --mode=auto` now prints an actionable warning unless the
   resolved model is Opus. New: `schema.IsAutoModeCapableModel`,
   `schema.Block.AutoModeUsable`, `cmd.warnAutoModeModel` + tests.

2. **No-op telemetry vars fixed.** `DISABLE_AUTOUPDATE` (real var is
   `DISABLE_AUTOUPDATER`) and stale `DISABLE_BUG_COMMAND` replaced by the single
   documented aggregate `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
   (= autoupdater + feedback command + error reporting + telemetry).

---

## PROPOSED — pick what you want; ordered by value for "stock CC on Bedrock"

### Tier A — closes real functional gaps

| # | Feature | Flag(s) | Writes | Why it matters on Bedrock |
|---|---------|---------|--------|---------------------------|
| A1 | **Fallback model chain** | `--fallback-model a,b` | `fallbackModel` native key (array) | Stock CC resilience: when a model is overloaded/unavailable CC switches instead of failing. No equivalent today. |
| A2 | **Fable 5 pinning** | `--fable-model <id>` (+ auto `_NAME`/`_DESC`/`_CAPABILITIES`) | `ANTHROPIC_DEFAULT_FABLE_MODEL` + companions | Juggernaut already references `claude-fable-5` in 1M logic but never pins it. **More importantly:** automatic Fable→Opus content fallback only works on Bedrock when BOTH `ANTHROPIC_DEFAULT_FABLE_MODEL` and `ANTHROPIC_DEFAULT_OPUS_MODEL` are set (opus already is). Without it, Fable users get hard refusals on flagged content. |
| A3 | **effort `auto`** | `--effort=auto` | `CLAUDE_CODE_EFFORT_LEVEL=auto` / `effortLevel=auto` | Docs now list `auto` as a valid value; Juggernaut's validator rejects it. One-line validator change. |

### Tier B — enterprise / governance (Bedrock is the ONLY delivery path)

Server-managed settings are NOT delivered to Bedrock sessions, so these MUST come
from a settings file — exactly what Juggernaut writes.

| # | Feature | Flag(s) | Writes | Notes |
|---|---------|---------|--------|-------|
| B1 | **Model allowlist** | `--available-models a,b` | `availableModels` (array) | Restricts `/model` picker + alias resolution. |
| B2 | **Enforce default** | `--enforce-available-models` | `enforceAvailableModels: true` | Extends allowlist to the Default option (v2.1.175+). Pairs with B1. |
| B3 | **Bedrock Guardrails** | `--guardrail-id <id>` `--guardrail-version <v>` | `ANTHROPIC_CUSTOM_HEADERS` with `X-Amzn-Bedrock-GuardrailIdentifier`/`...GuardrailVersion` | Native Bedrock content filtering. Note: must enable cross-region inference on the guardrail when using cross-region profiles. |

### Tier C — endpoint / routing / passthrough knobs

| # | Feature | Flag(s) | Writes | Notes |
|---|---------|---------|--------|-------|
| C1 | **Custom Bedrock endpoint** | `--bedrock-base-url <url>` | `ANTHROPIC_BEDROCK_BASE_URL` | Private endpoints / gateways. |
| C2 | **Haiku region override** | `--haiku-region <r>` | `ANTHROPIC_SMALL_FAST_MODEL_AWS_REGION` | Run the small/fast model in a different region. |
| C3 | **Mantle gateway auth skip** | `--skip-mantle-auth` | `CLAUDE_CODE_SKIP_MANTLE_AUTH=1` | For LLM gateways injecting AWS creds server-side. Extends existing Mantle support. |
| C4 | **Prompt-caching opt-out** | `--no-prompt-caching` / per-model | `DISABLE_PROMPT_CACHING[_HAIKU/_SONNET/_OPUS/_FABLE]` | Today 1h caching is forced on with no escape hatch; some Bedrock regions don't support caching. |
| C5 | **Token/limit passthrough** | `--max-output-tokens`, `--max-thinking-tokens` | `CLAUDE_CODE_MAX_OUTPUT_TOKENS`, `MAX_THINKING_TOKENS` | These are already hardcoded in bedrock-config.json (32768 / 65536). Expose as overridable flags. |
| C6 | **Adaptive thinking opt-out** | `--no-adaptive-thinking` | `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING=1` | Only affects Opus 4.6 / Sonnet 4.6 (reverts to fixed MAX_THINKING_TOKENS). Niche. |

### Tier D — diagnostics (small, high-UX-value)

| # | Feature | Where | Notes |
|---|---------|-------|-------|
| D1 | **doctor: auto-mode readiness check** | `internal/doctor` + `cmd/doctor.go` | Surface the SAME auto-mode diagnostic in `doctor`: "auto mode requested but model is Sonnet → won't appear." This is where users look when something "doesn't work." Natural complement to the apply warning shipped this session. |

---

## Recommended sequencing

1. **This PR (done):** auto-mode warning + telemetry var fix. Tight, high-confidence.
2. **PR 2 — "Fable + fallback + effort=auto" (Tier A):** the functional gaps that
   most affect day-to-day stock-CC behavior. A2 fixes a real Fable-on-Bedrock
   breakage. Medium size.
3. **PR 3 — governance (Tier B):** availableModels/enforce/guardrails. Enterprise
   rollout story. Independent, can be parallelized.
4. **PR 4 — endpoint/passthrough (Tier C) + doctor check (D1):** grab-bag of knobs;
   cherry-pick which JP actually wants.

## Open questions for JP
- Tier C5: the hardcoded 32768/65536 token limits — keep as defaults but make
  overridable, or leave alone? (They're sensible defaults.)
- Do you want a single mega-flag set or incremental PRs as above? (Recommend
  incremental.)
- Any of Tier C/D you'd rather skip entirely to keep the surface small?
