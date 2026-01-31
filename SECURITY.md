# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it privately rather than opening a public issue.

**Contact:** Open a [GitHub Security Advisory](../../security/advisories/new) or contact the maintainers directly.

**Include:**
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We'll acknowledge receipt within 48 hours and aim to provide a fix timeline within 7 days.

## Security Considerations

This tool handles:
- **AWS credentials** - Uses existing AWS CLI/SDK credential chain
- **API keys** - Can be stored in shell profile (plaintext) or OS keychain (encrypted)
- **Shell profiles** - Modifies `~/.bashrc`, `~/.zshrc`, etc.

**Recommendations:**
- Use IAM/SSO authentication when possible (no secrets stored)
- Use `--storage=keychain` for API key storage when available
- Ensure shell profile permissions are restricted (`chmod 600`)
- Don't commit `.backup.*` files if they contain sensitive data

## Supported Versions

We provide security updates for the latest release only.
