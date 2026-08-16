# Contributing to AIB Protocol

Thank you for your interest in contributing to AIB Protocol! We welcome
contributions of all kinds: bug reports, code, documentation, tests, and
design discussions.

## Code of Conduct

By participating in this project you agree to abide by our
[Code of Conduct](CODE_OF_CONDUCT.md).

## How Can I Contribute?

- **Reporting bugs** — open an issue using the bug report template
- **Suggesting enhancements** — open an issue using the feature request template
- **Pull requests** — fixes and features (see workflow below)
- **Documentation** — typo fixes and clarifications are always welcome

## Development Workflow

1. Fork the repository and create your branch from `main`:
   ```bash
   git checkout -b feature/my-feature
   ```
2. Make your changes with **focused, well-described commits**.
3. Ensure the code builds and tests pass locally:
   ```bash
   go build ./...
   go vet ./...
   go test ./...
   ```
4. Push your branch and open a Pull Request against `main`.

### Pull Request Guidelines

- Keep PRs small and focused; one logical change per PR.
- Include tests for any new functionality.
- Update documentation when behaviour changes.
- Reference the issue being fixed (`Fixes #123`) in the PR description.
- A maintainer must review and approve every PR before merge.

## Code Style

- Standard Go formatting (`gofmt` / `goimports`).
- Follow effective Go practices: https://go.dev/doc/effective_go
- Write table-driven tests where practical.
- Keep public APIs documented (`godoc` comments).

## Commit Messages

Use concise commit messages with a conventional prefix where possible:

```
feat: add inference channel dispute endpoint
fix: correct vesting schedule rounding
docs: expand PoAIW consensus explanation
test: make migration tests deterministic
chore: update CI Go version
```

## Reporting Security Issues

**Do not open public issues for security vulnerabilities.** See
[SECURITY.md](SECURITY.md) for disclosure instructions.

## License

By contributing, you agree that your contributions will be licensed under the
MIT License that covers this repository.
