# Contributing to Juggernaut

Thanks for your interest in contributing!

## Getting Started

1. Fork the repository
2. Clone your fork
3. Create a branch for your change

## Development

```bash
# Run bash tests (requires Bash 4.0+)
bash ./tests/v2/test_schema.sh
bash ./tests/v2/test_config_manager.sh
bash ./tests/v2/test_keychain.sh
bash ./tests/v2/test_apply.sh
bash ./tests/v2/test_show.sh
bash ./tests/v2/test_doctor.sh
bash ./tests/v2/test_uninstall.sh
bash ./tests/v2/test_install.sh

# Run PowerShell tests (Pester 5)
pwsh -Command "Invoke-Pester ./tests/v2 -CI"

# Preview an apply without writing
./juggernaut apply --auth=iam --dry-run

# Preview an installer wipe without writing
./install.sh --dry-run

# Lint shell scripts
shellcheck juggernaut install.sh commands/*.sh lib/keychain.sh lib/schema.sh lib/config_manager.sh lib/profile_paths.sh lib/doctor.sh tests/v2/test_*.sh
```

## Guidelines

- **Test your changes** — run the test suites above before submitting
- **Update docs** — if adding flags or features, update `README.md`, `QUICKSTART.md`, and `CLAUDE.md`
- **Keep it simple** — this is a focused utility, not a framework
- **Cross-platform** — all changes need `.sh` and `.ps1` variants; targets are macOS, Linux, Windows PowerShell 5.1, Windows PowerShell 7, Windows Git Bash, and WSL

## Pull Requests

1. Create a descriptive PR title
2. Explain what the change does and why
3. Ensure tests pass
4. Update documentation if needed

## Configuration Changes

All environment variables and defaults live in `bedrock-config.json` — this is the single source of truth. Don't hardcode values in scripts.

## Reporting Issues

Please include:
- OS and shell version
- Output of `juggernaut doctor`
- Steps to reproduce

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
