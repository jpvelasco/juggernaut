# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, report it **privately** — do not open a public issue.

**Contact:** Open a [GitHub Security Advisory](../../security/advisories/new) or contact the maintainers directly.

**Include:**

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We'll acknowledge receipt within 48 hours and aim to provide a fix timeline within 7 days.

## Supported Versions

Security updates are provided for the **latest release** only.

## Threat Model (summary)

Juggernaut is a local CLI that writes coding-agent configuration and optional shell activation so agents call **Amazon Bedrock** instead of vendor APIs. It runs with the privileges of the invoking user.

### Assets

| Asset | Where it lives | Sensitivity |
|-------|----------------|-------------|
| Bedrock API key (bearer) | OS keychain (`go-keyring`, service `juggernaut-bedrock`) | High — full Bedrock invoke capability |
| AWS IAM/SSO credentials | Existing AWS credential chain (env, shared config, SSO) | High — Juggernaut does not store these |
| Agent config (settings.json, config.toml, …) | User/project paths under home or cwd | Medium — can redirect model traffic |
| Shell activation blocks | User shell profiles (`~/.bashrc`, `~/.zshrc`, PowerShell profile, …) | Medium — wrap CLI launch |
| Config backups | `*.backup.*` next to managed config files | Medium — may contain prior env/auth metadata |

### Trust boundaries

1. **User machine** — Juggernaut and the target coding CLI run as the user; compromise of the user account implies compromise of keys and config.
2. **AWS / Bedrock** — Inference traffic goes to AWS endpoints (or Mantle when configured). Juggernaut does not proxy prompts.
3. **npm distribution** — Prebuilt binaries ship via `juggernaut-bedrock` optional platform packages; the Node shim resolves an allowlisted package only.
4. **Upstream coding CLIs** — Claude Code, Codex, OpenCode, Grok are external binaries; Juggernaut must not overwrite unknown files matching their names.

### Threats and mitigations

| Threat | Mitigation in Juggernaut |
|--------|---------------------------|
| Silent partial npm install runs a **stale binary** (Windows file lock) | Windows `preinstall` aborts if `juggernaut.exe` is running; launcher refuses version skew between root and platform package |
| Overwrite of **foreign** agent config | Collision detection refuses apply when owned leaves already hold non-Juggernaut values; `--force` requires intent and still creates a backup |
| Credential leakage via UserData / plaintext profiles | Bedrock API keys go to the **OS keychain only** — never shell profiles or agent config as plaintext secrets |
| Path traversal under user-controlled bases | `internal/safepath` containment + owner-only file modes for writes under controlled roots |
| Recursive/wrapper confusion | Activation resolves the real target binary; never installs over unknown `claude`/`codex`/… binaries |
| Auth-gated Bedrock without credentials | `CLAUDE_CODE_USE_BEDROCK=1` (and equivalents) only written when auth is validated |
| Tampering with governance lists in user settings | Documented limitation: `availableModels` in user/project scope is not OS-managed enforcement; org policy requires Claude Code managed settings paths Juggernaut does not write |

### Explicit non-goals

- Juggernaut is **not** an enterprise policy engine or MDM substitute.
- It does **not** implement Bedrock Guardrails attachment (tracked separately if needed).
- It does **not** guarantee that a user cannot re-add models via other Claude Code scopes.
- Live AWS calls in `juggernaut models check` require the operator's AWS credentials; that command is maintainer-facing and is not part of default apply/doctor paths.

## Credential handling

- **Prefer IAM/SSO** (`--auth=iam`) so no long-lived Bedrock API key is stored.
- **Bedrock API key** (`--auth=bedrock-api-key`) is stored in the OS keychain only.
- Uninstalling one non-Claude CLI does **not** remove the shared bearer token; only uninstall paths that intentionally clear credentials do.
- Do not commit `.backup.*` files, `.env`, or real keys. Redact `doctor` output before sharing.

## Recommendations for operators

- Use IAM/SSO when possible
- Restrict shell profile permissions (`chmod 600` on Unix)
- Prefer VPN/VPC endpoints for Bedrock in sensitive environments (AWS-side)
- Review collision refusals carefully before using `--force`
- Keep Juggernaut and the target coding CLI updated

## Security tooling in this repository

- CI: gosec, CodeQL, Socket, Codacy, multi-OS Go tests
- npm shim path allowlisting and launch binary staging on Windows
- No live AWS in default unit tests (discovery uses fakes)
