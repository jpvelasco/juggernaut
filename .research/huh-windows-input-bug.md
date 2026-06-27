# Research: huh Windows PowerShell Input Bug

**Opened:** 2026-06-25
**Status:** Active investigation
**Goal:** Find the smoking gun for why `huh` password input returns empty string on Windows PowerShell when running `juggernaut apply --auth=bedrock-api-key`

---

## Symptom

When running `juggernaut apply --auth=bedrock-api-key` on Windows PowerShell:
- The `huh` form for the Bedrock API key prompt appears
- User types the key
- The form returns an empty string
- `keychain.Default().Set(token)` is never called (token is empty)
- All subsequent Claude Code invocations fail with API errors

**Workaround confirmed:** Passing the key via `--bedrock-key` flag works perfectly.

---

## Current Dependency Versions

```
github.com/charmbracelet/huh         v1.0.0
github.com/charmbracelet/bubbletea   v1.3.6
github.com/charmbracelet/bubbles     v0.21.1-0.20250623103423-23b8fd6302d7
```

---

## Known Issues (Confirmed Related)

### Bubble Tea (Underlying TUI Framework)

| Issue | Title | Status | Fix Version |
|-------|-------|--------|-------------|
| [#1167](https://github.com/charmbracelet/bubbletea/issues/1167) | First character input lost on successive programs on Windows | Fixed in v2 only — **NOT backported to v1.x** |
| [#1192](https://github.com/charmbracelet/bubbletea/pull/1192) | v2 fix: drain console input buffer before reading | Fixed | v2 (via ultraviolet) |
| [#1368](https://github.com/charmbracelet/bubbletea/pull/1368) | Fix: first character input lost on successive programs | Merged July 7 2025 but **NOT in v1.3.6 tag** — tag was cut before merge |
| [#923](https://github.com/charmbracelet/bubbletea/issues/923) | First character lost when spawning external cmd.exe | Open | — |

### huh (Form Library)

| Issue | Title | Status | Fix |
|-------|-------|--------|-----|
| [#286](https://github.com/charmbracelet/huh/issues/286) | Tab/Enter inserts spaces into fields on Windows | **Still open** | — |
| [#281](https://github.com/charmbracelet/huh/issues/281) | Keypresses ignored on 2nd form on Windows | "Fixed" by PR #520 (Feb 2025, v0.7.0+) — **BUT the fix was incomplete** (see below) |
| [#372](https://github.com/charmbracelet/huh/issues/372) | First keystroke ignored after spinner on Windows | "Fixed" by PR #520 (Feb 2025, v0.7.0+) — **BUT the fix was incomplete** (see below) |

---

## The Root Fix: FlushConsoleInputBuffer

The definitive fix for Windows console input buffer issues is `FlushConsoleInputBuffer(conin)` — drains the input buffer before starting the event loop.

**Where the fix lives:**
- `charmbracelet/ultraviolet/cancelreader_windows.go` line 48: `xwindows.FlushConsoleInputBuffer(conin)`
- This is used by **Bubble Tea v2** (via `charm.land/bubbletea/v2`)
- **NOT used by Bubble Tea v1.x** (including v1.3.6 through v1.3.10)

---

## Critical Finding: PR #520 Was Not a Real Fix

huh PR [#520](https://github.com/charmbracelet/huh/pull/520) ("fix: ignore next input bug on Windows") claimed to fix issues #281 and #372. It was just a version bump from bubbletea v1.3.3 to v1.3.4 — **neither of which contains `FlushConsoleInputBuffer`**. The PR was based on the mistaken assumption that the fix was already in bubbletea v1.3.4.

**Verification:** I inspected `inputreader_windows.go` from bubbletea v1.3.3, v1.3.4, v1.3.6, v1.3.7, v1.3.8, v1.3.9, and v1.3.10 — **NONE** contain `FlushConsoleInputBuffer`.

---

## Dependency Chain Analysis

### Our current chain (BROKEN):
```
juggernaut → huh v1.0.0 → bubbletea v1.3.6 → inputreader_windows.go (NO FlushConsoleInputBuffer) ❌
```

### The fixed chain (huh/v2):
```
juggernaut → huh/v2 → bubbletea v2 → ultraviolet → FlushConsoleInputBuffer ✅
```

### The fixed chain (alternative — upgrade bubbletea only):
```
juggernaut → huh v1.0.0 → bubbletea v2 → ultraviolet → FlushConsoleInputBuffer ✅
```
⚠️ This won't work because huh v1.0.0 imports `github.com/charmbracelet/bubbletea` (v1 import path), not `charm.land/bubbletea/v2`.

---

## Key Question

We are on `huh` v1.0.0 + `bubbletea` v1.3.6 which DOES NOT include the fix.
**The fix was never backported to any Bubble Tea v1.x release.**

---

## Files of Interest in Juggernaut

- `cmd/apply.go` — `resolveCredential()` (line 312-343): the credential prompt using `huh` with `EchoModePassword`
- `cmd/apply.go` — `resolveApplyInputs()` (line 226-309): the apply form
- `internal/activation/activation.go` — `Launch()`: reads token from keychain

---

## Files of Interest in Dependencies

- `bubbletea@v1.3.6/inputreader_windows.go` — **NO** `FlushConsoleInputBuffer`
- `ultraviolet@v0.0.0-20251205161215-1948445e3318/cancelreader_windows.go` — **HAS** `FlushConsoleInputBuffer` at line 48
- `charm.land/bubbletea/v2@v2.0.2/input.go` — uses `ultraviolet` package

---

## Findings Log

### 2026-06-25 — Smoking Gun Found
- Inspected `bubbletea@v1.3.6/inputreader_windows.go` — **NO** `FlushConsoleInputBuffer`
- Inspected `bubbletea@v1.3.7` through `v1.3.10` — **NONE** contain the fix
- Confirmed `ultraviolet` (used by bubbletea v2) **HAS** the fix at line 48
- Confirmed `huh/v2` depends on `charm.land/bubbletea/v2` which uses ultraviolet
- Confirmed PR #520 was NOT a real fix — just a version bump from v1.3.3 to v1.3.4, neither of which has the fix
- Confirmed PR #1368 was merged July 7 2025, same day as v1.3.6 tag — tag was cut BEFORE the PR merged

### 2026-06-25 — Two-Form Scenario Confirmed
- Juggernaut runs TWO separate `huh` forms: first `resolveApplyInputs()` then `resolveCredential()`
- This is the exact "successive programs" scenario described in Bubble Tea #1167
- The first form works fine; the second form (credential prompt) returns empty string
- This is because the first form's `huh.Run()` creates a Bubble Tea program that doesn't flush the input buffer on exit, leaving stale events in the Windows console input buffer that are consumed by the second form
