# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

See `AGENTS.md` for commands, architecture, merge gates, and engineering constraints — it is the single tracked reference for working in this repo.

## Bedrock Config

`bedrock-config.json` is **Claude-only** — it pins Claude Code's `global.anthropic.*` inference profiles and base `environment` / `environment_bedrock_auth` defaults that `internal/schema` builds into `settings.json`. Non-Claude providers (Codex, OpenCode, Grok) do not read model IDs from it; they render their own provider-native config (`~/.codex/config.toml`, `~/.config/opencode/opencode.json`, `~/.grok/config.toml`) from live `foundation`/`profile` discovery (`internal/discovery`) plus the shared bearer token / SigV4 chain.

## Key Design Patterns

- **Provider extensibility:** routing is wired behind `provider.Provider`; a new CLI is a `BaseProvider`-backed struct (one file) + one `register(...)` call in `internal/provider/provider.go`; `cmd/` selects by name and gates flags via `Supports(Capability)`. New CLI-specific knobs become a `Capability`, not a CLI-name branch.
- **Provider isolation:** CLI-specific config lives behind `Provider.BuildConfig`; `cmd/` never branches on CLI name except the three pre-existing Claude-specific paths in `doctor.go`/`helpers.go`.
- **Native routing:** every CLI routes via `bedrock-runtime` (Mantle removed in v6). Re-run `juggernaut models refresh --source native --region <region>` after upgrading to repopulate `~/.juggernaut/model-catalog.json`.
