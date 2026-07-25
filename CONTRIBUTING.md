# Contributing

This is a controlled lab project for defensive network-behavior research. Contributions must stay within the bounded lab scope.

## Scope

The project is intentionally bounded:

- The proxy connects only to a single configured `-TargetUrl`.
- Only `GET` and `POST` methods are allowed through the proxy.
- The client/listener beacon pair generates harmless synthetic signals only.
- The project does not implement command execution, persistence, credential access, file collection, process hiding, or an open proxy.

Contributions that expand beyond this scope will not be accepted.

## Guidelines

1. **Preserve bounds** - New features must not remove or weaken existing safety bounds.
2. **Tests required** - All new code should include tests. Use the existing test patterns.
3. **No secrets** - Never commit credentials, tokens, or keys.
4. **Reviewable** - Code should be reviewable by someone unfamiliar with the codebase.
5. **Cross-platform** - Support Windows (PowerShell) and Linux (Python) runners where applicable.

## Development

```bash
# Build all binaries
go build -o bin ./cmd/...

# Run all tests
go test -count=1 ./...

# Run tests with race detector
go test -race -count=1 ./...

# Lint (requires golangci-lint)
golangci-lint run ./...
```

## Pull Request Process

1. Fork the repository and create a feature branch from `main`.
2. Make your changes with tests.
3. Run `go test -race -count=1 ./...` and verify all tests pass.
4. Run `golangci-lint run ./...` and fix any issues.
5. Open a pull request with a clear description of the change and its lab-scope justification.

## Reporting Issues

Report issues at the GitHub repository. Include the command used, logs, and expected vs. actual behavior.
