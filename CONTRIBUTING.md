# Contributing to Juggernaut

Thanks for your interest in contributing!

## Getting Started

1. Fork the repository
2. Clone your fork
3. Create a branch for your change

## Development

```bash
# Run tests (requires Bash 4.0+)
bash ./test.sh

# Test changes in dry-run mode
./setup --dry-run

# Lint shell scripts
shellcheck setup-claude-bedrock.sh uninstall.sh validate-setup.sh apply-config.sh
```

## Guidelines

- **Test your changes** - Run `bash ./test.sh` before submitting
- **Update docs** - If adding flags or features, update README.md and CLAUDE.md
- **Keep it simple** - This is a focused utility, not a framework
- **Cross-platform** - Changes should work on macOS, Linux, and Windows (PowerShell)

## Pull Requests

1. Create a descriptive PR title
2. Explain what the change does and why
3. Ensure tests pass
4. Update documentation if needed

## Configuration Changes

All environment variables and defaults live in `bedrock-config.json` - this is the single source of truth. Don't hardcode values in scripts.

## Reporting Issues

Please include:
- OS and shell version
- Output of `./validate-setup.sh` (if applicable)
- Steps to reproduce

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
