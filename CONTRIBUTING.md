# Contributing to Juggernaut

Thanks for your interest in contributing. Juggernaut is a Go CLI that configures
coding agents (Claude Code, Codex, OpenCode, Grok) to route through Amazon Bedrock.

## Getting Started

1. Fork the repository and clone your fork
2. Install [Go](https://go.dev/dl/) (see `go.mod` for the required version)
3. Optional but recommended: [golangci-lint](https://golangci-lint.run/) for local lint
4. Create a branch for your change (`feat/…`, `fix/…`, `docs/…`, `chore/…`)

```bash
git clone https://github.com/<you>/juggernaut.git
cd juggernaut
make build
make test
```

On Windows, the same `make` targets work when Make is available; otherwise:

```bash
go build -o bin/juggernaut .
go test ./...
go vet ./...
```

## Development

```bash
make build          # bin/juggernaut
make test           # go test ./...
make test-race      # race detector (Linux/macOS preferred)
make test-cover     # coverage.out + total
make lint           # golangci-lint
make fmt vet        # gofmt + go vet
make ci             # tidy, format, vet, lint, test

# Single package / test
go test ./internal/schema/... -v
go test ./cmd/... -run TestApply_WritesSettings_IAM -v

# Dry-run apply (no writes)
./bin/juggernaut apply --auth=iam --dry-run

# npm launcher tests (Node >= 20)
cd npm && npm test

# Install git hooks (optional)
scripts/setup-hooks.ps1     # Windows
bash scripts/setup-hooks.sh # Linux/macOS
```

Architecture, provider inventory, managed keys, and CI/release design live in
[`CLAUDE.md`](CLAUDE.md) and [`AGENTS.md`](AGENTS.md). Read those before changing
`internal/provider`, apply/uninstall behavior, or the npm package.

## Guidelines

- **Test your changes** — new behavior needs focused tests; run the affected package, then `go test ./...`
- **Update docs** — flags or user-visible behavior → update `README.md`, `QUICKSTART.md`, and `CLAUDE.md` as needed
- **Keep it focused** — Juggernaut configures Bedrock routing for coding CLIs; avoid turning it into a general agent framework
- **Cross-platform** — macOS, Linux, and Windows are first-class; prefer `internal/safepath` and existing helpers over OS-specific shortcuts
- **Provider isolation** — put CLI-specific logic behind `provider.Provider`; do not hardcode CLI names in `cmd/` beyond capability gates
- **No secrets in tests or commits** — use non-key-like fixtures; never commit real AWS credentials or Bedrock API keys
- **Version sync** — if you touch version, keep `VERSION`, `bedrock-config.json` `.version`, and `cmd/root.go` `Version` aligned
- **Conventional commits** — `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, `ci:`, `build:`, `perf:`

## Configuration

Defaults and model pins live in `bedrock-config.json` (embedded into the binary at
build time). Do not hardcode Bedrock model IDs or env defaults in scripts.

For Claude Code governance flags (`--available-models`, `--enforce-available-models`),
remember they write user/project settings only — not OS-level managed settings.

## Pull Requests

1. Use a descriptive conventional-commit title
2. Explain what changed and why
3. Ensure CI-relevant checks pass locally (`go test ./...`, `go vet ./...`; `make ci` when tools are available)
4. Update documentation when flags or behavior change
5. Link related issues (`Closes #…`)

## Reporting Issues

Please include:

- OS and shell
- Juggernaut version (`juggernaut version`)
- Output of `juggernaut doctor` (redact secrets)
- Target CLI (`claude`, `codex`, `opencode`, `grok`) if relevant
- Steps to reproduce

Security issues: see [`SECURITY.md`](SECURITY.md) — do not open a public issue for vulnerabilities.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
