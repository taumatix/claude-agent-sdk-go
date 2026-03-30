# Contributing

Thank you for considering contributing to the Claude Agent SDK for Go.

## Getting started

1. Fork the repository and clone your fork.
2. Ensure you have Go 1.23+ and the [Claude Code CLI](https://claude.ai/code) installed.
3. Run the tests to verify your setup:

```bash
go test ./...
```

## Making changes

- **Bug fixes and small improvements**: open a pull request directly.
- **New features or significant changes**: open an issue first to discuss the approach before investing time in an implementation.

### Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep interfaces small — prefer the minimum set of methods strictly required.
- Prefer small, well-named, reusable functions over large ones.
- Add tests for new behaviour. Use `github.com/stretchr/testify/assert` for assertions and `require` when a nil check is necessary to avoid a panic later in the test.
- Avoid mocking libraries; use stand-alone interface implementations in `_test.go` files (see `.claude/rules/go_test_rule.md`).

### Commit messages

Write commit messages in the imperative mood and explain *why* the change is needed, not just *what* changed.

### Pull requests

- Keep PRs focused: one logical change per PR.
- Add or update tests as appropriate.
- Update documentation (`README.md`, `docs/`) if the public API changes.
- All CI checks must pass before a PR can be merged.

## Reporting issues

Please include:

- Go version (`go version`)
- Claude Code CLI version (`claude --version`)
- A minimal, self-contained reproduction case.
- The full error output.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating you agree to abide by its terms.
